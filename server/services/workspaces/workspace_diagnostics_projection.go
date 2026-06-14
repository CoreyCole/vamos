package workspaces

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkspaceDiagnosticSource string

const (
	WorkspaceDiagnosticSourceManagerDB WorkspaceDiagnosticSource = "manager_db_lifecycle"
	WorkspaceDiagnosticSourceSync      WorkspaceDiagnosticSource = "scheduled_sync_diagnostics"
	WorkspaceDiagnosticSourceLocalRun  WorkspaceDiagnosticSource = "local_runtime_diagnostics"
)

type WorkspaceDiagnosticSeverity string

const (
	WorkspaceDiagnosticInfo    WorkspaceDiagnosticSeverity = "info"
	WorkspaceDiagnosticWarning WorkspaceDiagnosticSeverity = "warning"
	WorkspaceDiagnosticError   WorkspaceDiagnosticSeverity = "error"
)

type WorkspaceDiagnostic struct {
	Source        WorkspaceDiagnosticSource   `json:"source"`
	Severity      WorkspaceDiagnosticSeverity `json:"severity"`
	Code          string                      `json:"code"`
	Message       string                      `json:"message"`
	Detail        string                      `json:"detail,omitempty"`
	ProjectID     string                      `json:"project_id,omitempty"`
	WorkspaceSlug string                      `json:"workspace_slug,omitempty"`
	CheckoutPath  string                      `json:"checkout_path,omitempty"`
}

type WorkspaceSyncDiagnostic struct {
	LastStartedAt  time.Time               `json:"last_started_at,omitempty"`
	LastFinishedAt time.Time               `json:"last_finished_at,omitempty"`
	Status         string                  `json:"status"`
	Error          string                  `json:"error,omitempty"`
	Counts         ImplWorkspaceSyncResult `json:"counts"`
	Warnings       []WorkspaceDiagnostic   `json:"warnings,omitempty"`
}

type WorkspaceLifecycleDiagnostic struct {
	ProjectID       string                    `json:"project_id"`
	WorkspaceSlug   string                    `json:"workspace_slug"`
	CheckoutPath    string                    `json:"checkout_path"`
	Lifecycle       ImplWorkspaceStatus       `json:"lifecycle"`
	LifecycleSource WorkspaceDiagnosticSource `json:"lifecycle_source"`
	RuntimeStatus   string                    `json:"runtime_status,omitempty"`
	RuntimeSource   WorkspaceDiagnosticSource `json:"runtime_source"`
	Sync            WorkspaceSyncDiagnostic   `json:"sync"`
	Diagnostics     []WorkspaceDiagnostic     `json:"diagnostics,omitempty"`
	CleanupMessage  string                    `json:"cleanup_message"`
}

func WorkspaceSyncDiagnosticFromRow(row db.WorkspaceSyncDiagnostic) WorkspaceSyncDiagnostic {
	finished := time.Time{}
	if row.FinishedAt.Valid {
		finished = row.FinishedAt.Time
	}
	return WorkspaceSyncDiagnostic{
		LastStartedAt:  row.StartedAt,
		LastFinishedAt: finished,
		Status:         row.Status,
		Error:          row.Error,
		Counts: ImplWorkspaceSyncResult{
			Scanned:     int(row.Scanned),
			Discovered:  int(row.Discovered),
			Upserted:    int(row.Upserted),
			RepairedEnv: int(row.RepairedEnv),
			Merged:      int(row.Merged),
			CleanedUp:   int(row.CleanedUp),
			Changed:     row.Changed,
		},
		Warnings: workspaceSyncWarningsFromJSON(row.WarningsJson),
	}
}

func workspaceSyncWarningsFromJSON(raw string) []WorkspaceDiagnostic {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var warnings []WorkspaceDiagnostic
	if err := json.Unmarshal([]byte(raw), &warnings); err != nil {
		return []WorkspaceDiagnostic{{
			Source:   WorkspaceDiagnosticSourceSync,
			Severity: WorkspaceDiagnosticWarning,
			Code:     "workspace_sync_warnings_unreadable",
			Message:  "Scheduled sync warnings could not be decoded.",
			Detail:   err.Error(),
		}}
	}
	return warnings
}

func BuildWorkspaceLifecycleDiagnostic(
	row db.ImplWorkspace,
	runtime WorkspaceLifecycleSnapshot,
	hasRuntime bool,
	sync WorkspaceSyncDiagnostic,
) WorkspaceLifecycleDiagnostic {
	lifecycle := ImplWorkspaceStatus(strings.TrimSpace(row.Status))
	if lifecycle == "" {
		lifecycle = ImplWorkspaceStatusActive
	}
	runtimeStatus := ""
	if hasRuntime {
		runtimeStatus = string(runtime.Workspace.Status)
	}
	diagnostics := diagnosticsForWorkspace(sync, row.ProjectID, row.WorkspaceSlug, row.CheckoutPath)
	diagnostics = append(diagnostics, DetectLifecycleRuntimeConflicts(row, runtime, hasRuntime)...)
	return WorkspaceLifecycleDiagnostic{
		ProjectID:       row.ProjectID,
		WorkspaceSlug:   row.WorkspaceSlug,
		CheckoutPath:    row.CheckoutPath,
		Lifecycle:       lifecycle,
		LifecycleSource: WorkspaceDiagnosticSourceManagerDB,
		RuntimeStatus:   runtimeStatus,
		RuntimeSource:   WorkspaceDiagnosticSourceLocalRun,
		Sync:            sync,
		Diagnostics:     diagnostics,
		CleanupMessage:  cleanupHumanApprovalMessage(row),
	}
}

func DetectLifecycleRuntimeConflicts(
	row db.ImplWorkspace,
	runtime WorkspaceLifecycleSnapshot,
	hasRuntime bool,
) []WorkspaceDiagnostic {
	if !hasRuntime {
		return nil
	}
	lifecycle := ImplWorkspaceStatus(strings.TrimSpace(row.Status))
	if lifecycle != ImplWorkspaceStatusMerged && lifecycle != ImplWorkspaceStatusCleanedUp {
		return nil
	}
	if !runtimeLooksLiveOrFailed(runtime) {
		return nil
	}
	runtimeStatus := string(runtime.Workspace.Status)
	return []WorkspaceDiagnostic{{
		Source:        WorkspaceDiagnosticSourceLocalRun,
		Severity:      WorkspaceDiagnosticWarning,
		Code:          "local_runtime_conflicts_with_manager_lifecycle",
		Message:       "Local runtime diagnostics may be stale for this non-active workspace.",
		Detail:        fmt.Sprintf("manager DB lifecycle=%s; local runtime=%s; local files are diagnostic only", lifecycle, runtimeStatus),
		ProjectID:     row.ProjectID,
		WorkspaceSlug: row.WorkspaceSlug,
		CheckoutPath:  row.CheckoutPath,
	}}
}

func diagnosticsForWorkspace(
	sync WorkspaceSyncDiagnostic,
	projectID, slug, checkoutPath string,
) []WorkspaceDiagnostic {
	projectID = strings.TrimSpace(projectID)
	slug = strings.TrimSpace(slug)
	path := cleanPathKey(checkoutPath)
	out := make([]WorkspaceDiagnostic, 0, len(sync.Warnings))
	for _, warning := range sync.Warnings {
		warningProjectID := strings.TrimSpace(warning.ProjectID)
		warningSlug := strings.TrimSpace(warning.WorkspaceSlug)
		warningPath := cleanPathKey(warning.CheckoutPath)
		matchesProjectSlug := projectID != "" && slug != "" && warningProjectID == projectID && warningSlug == slug
		matchesPath := path != "" && warningPath == path
		if matchesProjectSlug || matchesPath {
			out = append(out, warning)
		}
	}
	return out
}

func runtimeLooksLiveOrFailed(runtime WorkspaceLifecycleSnapshot) bool {
	switch runtime.Workspace.Status {
	case StatusRunning, StatusStarting, StatusStopping, StatusFailed, StatusCrashed:
		return true
	}
	if runtime.DesiredState == WorkspaceDesiredRunning {
		return true
	}
	return runtime.ObservedState == WorkspaceObservedRunning ||
		runtime.ObservedState == WorkspaceObservedStarting ||
		runtime.ObservedState == WorkspaceObservedStopping ||
		runtime.ObservedState == WorkspaceObservedFailed ||
		runtime.ObservedState == WorkspaceObservedCrashed
}

func cleanupHumanApprovalMessage(row db.ImplWorkspace) string {
	return "Cleanup requires human approval. Do not clean up or delete this checkout unless explicitly approved."
}
