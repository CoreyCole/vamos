package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultWorkspaceDiagnosticsTimeout = 2 * time.Second

type WorkspaceDiagnosticsOptions struct {
	CheckoutPath string
	Client       *http.Client
	Stdout       io.Writer
	Timeout      time.Duration
}

type workspaceDiagnosticResponse struct {
	Workspace           workspaceDiagnosticWorkspace  `json:"workspace"`
	RuntimeStatus       *workspaceRuntimeStatusFile   `json:"runtime_status,omitempty"`
	DesiredState        *workspaceDesiredStateFile    `json:"desired_state,omitempty"`
	LifecycleDiagnostic *workspaceLifecycleDiagnostic `json:"lifecycle_diagnostic,omitempty"`
}

type workspaceDiagnosticWorkspace struct {
	Slug         string `json:"slug"`
	Status       string `json:"status"`
	CheckoutPath string `json:"checkout_path"`
}

type workspaceDesiredStateFile struct {
	Desired string `json:"desired"`
}

type workspaceLifecycleDiagnostic struct {
	ProjectID       string                          `json:"project_id"`
	WorkspaceSlug   string                          `json:"workspace_slug"`
	CheckoutPath    string                          `json:"checkout_path"`
	Lifecycle       string                          `json:"lifecycle"`
	LifecycleSource string                          `json:"lifecycle_source"`
	RuntimeStatus   string                          `json:"runtime_status,omitempty"`
	RuntimeSource   string                          `json:"runtime_source"`
	Sync            workspaceSyncDiagnostic         `json:"sync"`
	Diagnostics     []workspaceDiagnosticDiagnostic `json:"diagnostics,omitempty"`
	CleanupMessage  string                          `json:"cleanup_message"`
}

type workspaceSyncDiagnostic struct {
	LastStartedAt  time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt time.Time `json:"last_finished_at,omitempty"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	Warnings       []any     `json:"warnings,omitempty"`
}

type workspaceDiagnosticDiagnostic struct {
	Source   string `json:"source"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

func TryPrintWorkspacePreflight(ctx context.Context, opts WorkspaceDiagnosticsOptions) (bool, error) {
	return tryPrintWorkspaceDiagnostics(ctx, opts, "preflight")
}

func TryPrintWorkspaceFinalStatus(ctx context.Context, opts WorkspaceDiagnosticsOptions) (bool, error) {
	return tryPrintWorkspaceDiagnostics(ctx, opts, "final")
}

func tryPrintWorkspaceDiagnostics(
	ctx context.Context,
	opts WorkspaceDiagnosticsOptions,
	phase string,
) (bool, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	meta, err := readWorkspaceMetadata(workspaceEnvPath(opts.CheckoutPath))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if strings.TrimSpace(meta.ProjectID) == "" {
		fmt.Fprint(opts.Stdout, formatLocalOnlyWorkspaceDiagnostic(opts.CheckoutPath, errors.New("workspace.env missing VAMOS_WORKSPACE_PROJECT_ID"), phase))
		return true, nil
	}
	if strings.TrimSpace(meta.ManagerURL) == "" || strings.TrimSpace(meta.RestartToken) == "" || strings.TrimSpace(meta.Slug) == "" {
		fmt.Fprint(opts.Stdout, formatLocalOnlyWorkspaceDiagnostic(opts.CheckoutPath, errors.New("workspace manager metadata incomplete"), phase))
		return true, nil
	}
	diag, err := fetchWorkspaceDiagnostics(ctx, opts, meta)
	if err != nil {
		fmt.Fprint(opts.Stdout, formatLocalOnlyWorkspaceDiagnostic(opts.CheckoutPath, err, phase))
		return true, nil
	}
	fmt.Fprint(opts.Stdout, formatWorkspaceDiagnostic(diag, phase))
	return true, nil
}

func fetchWorkspaceDiagnostics(
	ctx context.Context,
	opts WorkspaceDiagnosticsOptions,
	meta WorkspaceMetadata,
) (workspaceDiagnosticResponse, error) {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultWorkspaceDiagnosticsTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint, err := url.JoinPath(strings.TrimRight(meta.ManagerURL, "/"), "internal", "workspaces", meta.Slug, "diagnostics")
	if err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	query := u.Query()
	query.Set("project_id", meta.ProjectID)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	req.Header.Set("X-Vamos-Workspace-Restart-Token", meta.RestartToken)
	resp, err := client.Do(req)
	if err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, workspaceRestartResponseBodyLimit))
	if err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workspaceDiagnosticResponse{}, fmt.Errorf("manager diagnostics API returned %s", resp.Status)
	}
	var diag workspaceDiagnosticResponse
	if err := json.Unmarshal(body, &diag); err != nil {
		return workspaceDiagnosticResponse{}, err
	}
	return diag, nil
}

func formatWorkspaceDiagnostic(diag workspaceDiagnosticResponse, phase string) string {
	lines := []string{fmt.Sprintf("Workspace diagnostics (%s)", phase)}
	if lifecycle := diag.LifecycleDiagnostic; lifecycle != nil {
		lines = append(lines, fmt.Sprintf(
			"Manager lifecycle: %s (source: manager DB)",
			firstNonEmptyBuild(lifecycle.Lifecycle, "unknown"),
		))
		syncStatus := firstNonEmptyBuild(lifecycle.Sync.Status, "unknown")
		syncLine := "Scheduled sync: " + syncStatus
		if !lifecycle.Sync.LastFinishedAt.IsZero() {
			syncLine += "; last finished " + formatWorkspaceDiagnosticAge(lifecycle.Sync.LastFinishedAt) + " ago"
		}
		warningCount := len(lifecycle.Sync.Warnings)
		for _, diagnostic := range lifecycle.Diagnostics {
			if strings.EqualFold(diagnostic.Severity, "warning") || strings.EqualFold(diagnostic.Severity, "error") {
				warningCount++
			}
		}
		if warningCount > 0 {
			syncLine += fmt.Sprintf("; warnings: %d", warningCount)
		}
		lines = append(lines, syncLine)
		runtimeStatus := firstNonEmptyBuild(lifecycle.RuntimeStatus, runtimeStatusFromResponse(diag), "unknown")
		lines = append(lines, fmt.Sprintf(
			"Local runtime diagnostics: %s (source: .vamos/run/status.json; diagnostic only)",
			runtimeStatus,
		))
		for _, diagnostic := range lifecycle.Diagnostics {
			if diagnostic.Message != "" {
				lines = append(lines, fmt.Sprintf("Warning: %s", diagnostic.Message))
			}
		}
		if strings.TrimSpace(lifecycle.CleanupMessage) != "" {
			lines = append(lines, "Cleanup: "+lifecycle.CleanupMessage)
		}
	} else {
		lines = append(lines, "Manager lifecycle unavailable; showing local runtime diagnostics only.")
		lines = append(lines, "Local runtime diagnostics: "+firstNonEmptyBuild(runtimeStatusFromResponse(diag), "unknown")+" (source: .vamos/run/status.json; diagnostic only)")
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatLocalOnlyWorkspaceDiagnostic(checkoutPath string, err error, phase string) string {
	lines := []string{fmt.Sprintf("Workspace diagnostics (%s)", phase)}
	unavailable := "Manager lifecycle unavailable; showing local runtime diagnostics only."
	if err != nil {
		unavailable = fmt.Sprintf("Manager lifecycle unavailable: %v; showing local runtime diagnostics only.", err)
	}
	lines = append(lines, unavailable)
	lines = append(lines, "Local runtime diagnostics: "+firstNonEmptyBuild(readLocalRuntimeStatus(checkoutPath), "unknown")+" (source: .vamos/run/status.json; diagnostic only)")
	return strings.Join(lines, "\n") + "\n"
}

func runtimeStatusFromResponse(diag workspaceDiagnosticResponse) string {
	if diag.RuntimeStatus != nil {
		return strings.TrimSpace(diag.RuntimeStatus.Status)
	}
	return strings.TrimSpace(diag.Workspace.Status)
}

func readLocalRuntimeStatus(checkoutPath string) string {
	data, err := os.ReadFile(workspaceRuntimePaths(checkoutPath).statusJSON)
	if err != nil {
		return ""
	}
	var status workspaceRuntimeStatusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return ""
	}
	return strings.TrimSpace(status.Status)
}

func formatWorkspaceDiagnosticAge(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func firstNonEmptyBuild(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
