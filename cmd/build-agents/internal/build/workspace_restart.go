package build

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceMetadata struct {
	Slug         string
	CheckoutPath string
	ManagerURL   string
	RestartToken string
}

type WorkspaceRestartClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type WorkspaceRestartOptions struct {
	CheckoutPath string
	Components   []string
	Force        bool
	Client       WorkspaceRestartClient
	Stdout       io.Writer
}

type WorkspaceRestartAttemptResult struct {
	Handled bool
	Err     error
	Force   bool
}

type WorkspaceRestartResult struct {
	Handled        bool
	GracefulError  error
	ForceError     error
	ForceAttempted bool
	ForceSucceeded bool
}

const (
	workspaceRestartResponseBodyLimit = 64 * 1024
	workspaceRestartLogTailLines      = 80
)

func TryWorkspaceRestart(
	ctx context.Context,
	checkoutPath string,
	components []string,
) (bool, error) {
	result, err := TryWorkspaceRestartWithRecovery(ctx, WorkspaceRestartOptions{
		CheckoutPath: checkoutPath,
		Components:   components,
		Client:       http.DefaultClient,
		Stdout:       os.Stdout,
	})
	if !result.Handled {
		return false, err
	}
	return true, err
}

func TryWorkspaceRestartWithRecovery(
	ctx context.Context,
	opts WorkspaceRestartOptions,
) (WorkspaceRestartResult, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	first := tryWorkspaceRestartAttempt(ctx, opts)
	result := WorkspaceRestartResult{Handled: first.Handled}
	if !first.Handled || first.Err == nil {
		return result, first.Err
	}

	result.GracefulError = first.Err
	printGracefulRestartFailure(opts.Stdout, first.Err)
	fmt.Fprintln(opts.Stdout, "--- attempting forceful workspace restart ---")

	forceOpts := opts
	forceOpts.Force = true
	force := tryWorkspaceRestartAttempt(ctx, forceOpts)
	result.ForceAttempted = force.Handled
	result.ForceError = force.Err
	if !force.Handled {
		return result, first.Err
	}
	if force.Err != nil {
		return result, fmt.Errorf(
			"workspace graceful restart failed:\n%w\n\nworkspace force restart failed:\n%v\n\n%s",
			first.Err,
			force.Err,
			workspaceRestartRecoveryCommands(),
		)
	}
	result.ForceSucceeded = true
	fmt.Fprintln(opts.Stdout, "restart: vamos workspace restarted after force retry")
	return result, nil
}

func tryWorkspaceRestartAttempt(
	ctx context.Context,
	opts WorkspaceRestartOptions,
) WorkspaceRestartAttemptResult {
	handled, err := tryWorkspaceRestart(ctx, opts)
	return WorkspaceRestartAttemptResult{Handled: handled, Err: err, Force: opts.Force}
}

func printGracefulRestartFailure(out io.Writer, err error) {
	if out == nil {
		return
	}
	fmt.Fprintln(out, "restart: graceful workspace restart failed")
	fmt.Fprintln(out, "--- graceful restart failure ---")
	fmt.Fprintln(out, err)
}

func workspaceRestartRecoveryCommands() string {
	return strings.Join([]string{
		"Build completed but workspace restart failed.",
		"Inspect:",
		"  agentsctl workspace doctor",
		"  agentsctl workspace logs web --tail 120",
		"Recover:",
		"  agentsctl workspace restart --force",
	}, "\n")
}

func tryWorkspaceRestart(
	ctx context.Context,
	opts WorkspaceRestartOptions,
) (bool, error) {
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	meta, err := readWorkspaceMetadata(workspaceEnvPath(opts.CheckoutPath))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(meta.ManagerURL) == "" ||
		strings.TrimSpace(meta.RestartToken) == "" {
		return false, nil
	}
	checkoutPath := meta.CheckoutPath
	if strings.TrimSpace(checkoutPath) == "" {
		checkoutPath = opts.CheckoutPath
	}
	body, err := json.Marshal(struct {
		Slug         string   `json:"slug"`
		CheckoutPath string   `json:"checkout_path"`
		Components   []string `json:"components,omitempty"`
		Force        bool     `json:"force,omitempty"`
	}{
		Slug:         meta.Slug,
		CheckoutPath: checkoutPath,
		Components:   append([]string(nil), opts.Components...),
		Force:        opts.Force,
	})
	if err != nil {
		return true, err
	}
	endpoint := strings.TrimRight(meta.ManagerURL, "/") + "/internal/workspaces/restart"
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vamos-Workspace-Restart-Token", meta.RestartToken)
	resp, err := opts.Client.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(
		io.LimitReader(resp.Body, workspaceRestartResponseBodyLimit),
	)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return true, workspaceRestartError(opts.CheckoutPath, resp.Status, responseBody)
	}
	printWorkspaceRestartTarget(opts.Stdout, responseBody, meta)
	return true, nil
}

func printWorkspaceRestartTarget(
	out io.Writer,
	responseBody []byte,
	meta WorkspaceMetadata,
) {
	if out == nil {
		return
	}
	summary := workspaceRestartResponseSummary(responseBody)
	if summary.Slug == "" {
		summary.Slug = strings.TrimSpace(meta.Slug)
	}
	if summary.URL != "" {
		fmt.Fprintf(out, "workspace URL: %s\n", summary.URL)
		return
	}
	if summary.Slug != "" && strings.TrimSpace(meta.ManagerURL) != "" {
		fmt.Fprintf(
			out,
			"workspace slug: %s (open from %s/workspaces)\n",
			summary.Slug,
			strings.TrimRight(strings.TrimSpace(meta.ManagerURL), "/"),
		)
	}
}

type workspaceRestartSummary struct {
	Slug string
	URL  string
}

func workspaceRestartResponseSummary(responseBody []byte) workspaceRestartSummary {
	var raw map[string]any
	if err := json.Unmarshal(responseBody, &raw); err != nil {
		return workspaceRestartSummary{}
	}
	return workspaceRestartSummary{
		Slug: stringValueForAnyCase(raw, "slug"),
		URL:  stringValueForAnyCase(raw, "url"),
	}
}

func stringValueForAnyCase(values map[string]any, key string) string {
	for k, v := range values {
		if !strings.EqualFold(k, key) {
			continue
		}
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func workspaceRestartError(checkoutPath, status string, responseBody []byte) error {
	parts := []string{"workspace restart API returned " + status}
	body := strings.TrimSpace(string(responseBody))
	if body != "" {
		parts = append(parts, "response body:\n"+body)
	}
	if diagnostics := workspaceRestartDiagnostics(checkoutPath); diagnostics != "" {
		parts = append(parts, diagnostics)
	}
	return errors.New(strings.Join(parts, "\n\n"))
}

func workspaceRestartDiagnostics(checkoutPath string) string {
	paths := workspaceRuntimePaths(checkoutPath)
	data, err := os.ReadFile(paths.statusJSON)
	if err != nil {
		return ""
	}
	var status workspaceRuntimeStatusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return ""
	}
	lines := []string{"workspace status diagnostics:"}
	if strings.TrimSpace(status.Status) != "" {
		lines = append(lines, "status: "+status.Status)
	}
	if strings.TrimSpace(status.Phase) != "" {
		lines = append(lines, "phase: "+status.Phase)
	}
	if strings.TrimSpace(status.Error) != "" {
		lines = append(lines, "error: "+status.Error)
	}
	if path := strings.TrimSpace(status.Logs["web"]); path != "" {
		lines = append(lines, "web log: "+path)
		if tail := tailTextFile(
			path,
			workspaceRestartLogTailLines,
			workspaceRestartResponseBodyLimit,
		); tail != "" {
			lines = append(lines, "web log tail:\n"+tail)
		}
	}
	return strings.Join(lines, "\n")
}

func tailTextFile(path string, maxLines int, maxBytes int64) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func workspaceEnvPath(checkoutPath string) string {
	return filepath.Join(checkoutPath, ".vamos", "run", "workspace.env")
}

func readWorkspaceMetadata(path string) (WorkspaceMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return WorkspaceMetadata{}, err
	}
	defer f.Close()
	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(key)] = unshellWorkspaceValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return WorkspaceMetadata{}, err
	}
	return WorkspaceMetadata{
		Slug:         vals["VAMOS_WORKSPACE_SLUG"],
		CheckoutPath: vals["VAMOS_WORKSPACE_CHECKOUT"],
		ManagerURL:   vals["VAMOS_WORKSPACE_MANAGER_URL"],
		RestartToken: vals["VAMOS_WORKSPACE_RESTART_TOKEN"],
	}, nil
}

func unshellWorkspaceValue(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		inner := strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		return strings.ReplaceAll(inner, "'\\''", "'")
	}
	return value
}

func findCheckoutRoot(cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			if filepath.Base(current) == "agents" &&
				filepath.Base(filepath.Dir(current)) == "pkg" {
				return filepath.Dir(filepath.Dir(current))
			}
			return current
		}
		if _, err := os.Stat(
			filepath.Join(current, "pkg", "agents", "go.mod"),
		); err == nil {
			return current
		}
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd)
		}
		current = parent
	}
}
