package workspacecmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func RunDoctor(
	ctx context.Context,
	cfg WorkspaceCLIConfig,
	tail int,
	out io.Writer,
) error {
	fmt.Fprintln(out, "workspace doctor")
	fmt.Fprintln(out, "scope: compares local runtime metadata, manager diagnostics when available, and process identity")
	_ = RunStatus(ctx, cfg, out)
	if branch := gitOutput(
		ctx,
		cfg.CheckoutPath,
		"rev-parse",
		"--abbrev-ref",
		"HEAD",
	); branch != "" {
		fmt.Fprintf(out, "git_branch: %s\n", branch)
	}
	if commit := gitOutput(ctx, cfg.CheckoutPath, "rev-parse", "HEAD"); commit != "" {
		fmt.Fprintf(out, "git_commit: %s\n", commit)
	}

	findings := doctorFindings{}
	checkLocalRuntime(cfg, out, &findings)
	checkManagerDiagnostics(ctx, cfg, out, &findings)

	for _, target := range []WorkspaceLogTarget{WorkspaceLogWeb, WorkspaceLogTemporal, WorkspaceLogTSWorker} {
		fmt.Fprintf(out, "\n--- %s log ---\n", target)
		_ = RunLogs(ctx, cfg, target, tail, out)
	}
	printDoctorSummary(out, findings)
	return nil
}

type doctorFindings struct {
	staleMetadata  bool
	deadProcess    bool
	managerProblem bool
	publicProblem  bool
	notes          []string
}

func (f *doctorFindings) add(format string, args ...any) {
	f.notes = append(f.notes, fmt.Sprintf(format, args...))
}

func checkLocalRuntime(cfg WorkspaceCLIConfig, out io.Writer, findings *doctorFindings) {
	fmt.Fprintln(out, "\nLocal checks:")
	if cfg.Metadata.Slug == "" {
		findings.staleMetadata = true
		findings.add("workspace.env missing VAMOS_WORKSPACE_SLUG")
	}
	if cfg.Metadata.CheckoutPath == "" || !samePath(cfg.Metadata.CheckoutPath, cfg.CheckoutPath) {
		findings.staleMetadata = true
		findings.add("workspace.env checkout %q does not match current checkout %q", cfg.Metadata.CheckoutPath, cfg.CheckoutPath)
	}
	if cfg.Metadata.ProjectID == "" {
		findings.add("workspace.env missing VAMOS_WORKSPACE_PROJECT_ID; manager lifecycle lookup may be degraded")
	}
	for component, pid := range cfg.Status.PIDs {
		alive := processAlive(pid)
		fmt.Fprintf(out, "pid_alive.%s: %t\n", component, alive)
		if pid != 0 && !alive {
			findings.deadProcess = true
			findings.add("status.json has dead %s pid %d", component, pid)
		}
	}
	for component, path := range cfg.Status.Logs {
		if path == "" {
			continue
		}
		if !pathWithin(path, filepath.Join(cfg.CheckoutPath, ".vamos")) {
			findings.staleMetadata = true
			findings.add("status.json %s log path points outside current checkout: %s", component, path)
		}
	}
}

type managerDiagnostics struct {
	Workspace struct {
		Slug         string `json:"Slug"`
		Status       string `json:"Status"`
		Error        string `json:"Error"`
		CheckoutPath string `json:"CheckoutPath"`
		URL          string `json:"URL"`
	} `json:"workspace"`
	Metadata struct {
		Slug         string `json:"Slug"`
		CheckoutPath string `json:"CheckoutPath"`
		ManagerURL   string `json:"ManagerURL"`
	} `json:"metadata"`
	RuntimeStatus struct {
		Status string            `json:"status"`
		Logs   map[string]string `json:"logs"`
		Ports  map[string]int    `json:"ports"`
		PIDs   map[string]int    `json:"pids"`
	} `json:"runtime_status"`
	RuntimeEnvSnapshot struct {
		WorkspaceSlug string `json:"workspace_slug"`
		CheckoutPath  string `json:"checkout_path"`
		Web           struct {
			PID           int    `json:"pid"`
			PublicBaseURL string `json:"public_base_url"`
			DatabasePath  string `json:"database_path"`
			DefaultCWD    string `json:"default_cwd"`
		} `json:"web"`
	} `json:"runtime_env_snapshot"`
	PIDAlive    bool   `json:"pid_alive"`
	PortOpen    bool   `json:"port_open"`
	PublicURL   string `json:"public_url"`
	LatestError string `json:"latest_error"`
}

func checkManagerDiagnostics(ctx context.Context, cfg WorkspaceCLIConfig, out io.Writer, findings *doctorFindings) {
	fmt.Fprintln(out, "\nManager diagnostics:")
	if strings.TrimSpace(cfg.ManagerURL) == "" || strings.TrimSpace(cfg.RestartToken) == "" || strings.TrimSpace(cfg.Metadata.Slug) == "" {
		fmt.Fprintln(out, "manager_diagnostics: skipped (missing manager URL, restart token, or slug)")
		findings.managerProblem = true
		return
	}
	diag, status, err := fetchManagerDiagnostics(ctx, cfg)
	if err != nil {
		fmt.Fprintf(out, "manager_diagnostics: unavailable: %v\n", err)
		findings.managerProblem = true
		findings.add("manager diagnostics unavailable: %v", err)
		return
	}
	fmt.Fprintf(out, "manager_diagnostics_status: %s\n", status)
	fmt.Fprintf(out, "manager_workspace_status: %s\n", diag.Workspace.Status)
	fmt.Fprintf(out, "manager_workspace_checkout: %s\n", diag.Workspace.CheckoutPath)
	fmt.Fprintf(out, "manager_workspace_url: %s\n", firstNonEmpty(diag.Workspace.URL, diag.PublicURL))
	if diag.Workspace.Error != "" || diag.LatestError != "" {
		fmt.Fprintf(out, "manager_latest_error: %s\n", firstNonEmpty(diag.LatestError, diag.Workspace.Error))
	}
	fmt.Fprintf(out, "manager_pid_alive: %t\n", diag.PIDAlive)
	fmt.Fprintf(out, "manager_port_open: %t\n", diag.PortOpen)
	if diag.RuntimeEnvSnapshot.WorkspaceSlug != "" {
		fmt.Fprintf(out, "runtime_env.workspace_slug: %s\n", diag.RuntimeEnvSnapshot.WorkspaceSlug)
		fmt.Fprintf(out, "runtime_env.checkout_path: %s\n", diag.RuntimeEnvSnapshot.CheckoutPath)
		fmt.Fprintf(out, "runtime_env.web.public_base_url: %s\n", diag.RuntimeEnvSnapshot.Web.PublicBaseURL)
		fmt.Fprintf(out, "runtime_env.web.default_cwd: %s\n", diag.RuntimeEnvSnapshot.Web.DefaultCWD)
	}
	if !samePath(diag.Workspace.CheckoutPath, cfg.CheckoutPath) {
		findings.managerProblem = true
		findings.add("manager checkout %q does not match current checkout %q", diag.Workspace.CheckoutPath, cfg.CheckoutPath)
	}
	if diag.RuntimeEnvSnapshot.WorkspaceSlug != "" && diag.RuntimeEnvSnapshot.WorkspaceSlug != cfg.Metadata.Slug {
		findings.staleMetadata = true
		findings.add("runtime env snapshot slug %q does not match workspace slug %q", diag.RuntimeEnvSnapshot.WorkspaceSlug, cfg.Metadata.Slug)
	}
	if diag.RuntimeEnvSnapshot.CheckoutPath != "" && !samePath(diag.RuntimeEnvSnapshot.CheckoutPath, cfg.CheckoutPath) {
		findings.staleMetadata = true
		findings.add("runtime env snapshot checkout %q does not match current checkout %q", diag.RuntimeEnvSnapshot.CheckoutPath, cfg.CheckoutPath)
	}
	if !diag.PIDAlive || !diag.PortOpen || strings.EqualFold(diag.Workspace.Status, "crashed") {
		findings.deadProcess = true
	}
}

func fetchManagerDiagnostics(ctx context.Context, cfg WorkspaceCLIConfig) (managerDiagnostics, string, error) {
	var diag managerDiagnostics
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	endpoint, err := url.JoinPath(strings.TrimRight(cfg.ManagerURL, "/"), "internal", "workspaces", cfg.Metadata.Slug, "diagnostics")
	if err != nil {
		return diag, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return diag, "", err
	}
	req.Header.Set("X-Vamos-Workspace-Restart-Token", cfg.RestartToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return diag, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return diag, resp.Status, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return diag, resp.Status, json.NewDecoder(resp.Body).Decode(&diag)
}

func printDoctorSummary(out io.Writer, findings doctorFindings) {
	fmt.Fprintln(out, "\nDiagnosis:")
	if len(findings.notes) == 0 && !findings.deadProcess && !findings.managerProblem && !findings.staleMetadata {
		fmt.Fprintln(out, "  no obvious local/manager mismatch detected")
	} else {
		for _, note := range findings.notes {
			fmt.Fprintf(out, "  - %s\n", note)
		}
	}
	fmt.Fprintln(out, "\nSuggested recovery:")
	if findings.staleMetadata || findings.managerProblem {
		fmt.Fprintln(out, "  go run ./cmd/vamos-runtime ctl workspace register-current --project github.com/CoreyCole/vamos")
	}
	if findings.deadProcess || findings.staleMetadata || findings.managerProblem {
		fmt.Fprintln(out, "  just build")
		fmt.Fprintln(out, "  curl -k -sS -D - --max-time 15 \"https://$(grep '^VAMOS_WORKSPACE_SLUG=' .vamos/run/workspace.env | cut -d= -f2- | tr -d '\"'\\'').workspaces.creative-mode.ai/\" -o /tmp/vamos-feature-home.html")
		fmt.Fprintln(out, "  # Healthy auth-protected result is usually HTTP 307; 503 Workspace recovery is not ready.")
		return
	}
	fmt.Fprintln(out, "  none; workspace identity and process checks look healthy")
}

func gitOutput(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		proc, err := os.FindProcess(pid)
		return err == nil && proc != nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return strings.Contains(err.Error(), "operation not permitted") ||
		strings.Contains(err.Error(), "permission denied") ||
		strings.Contains(err.Error(), strconv.Itoa(pid)) && os.IsPermission(err)
}

func pathWithin(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
