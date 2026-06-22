package semantic

import (
	"path"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type SourceKind string

const (
	SourceRunCompletion   SourceKind = "run_completion"
	SourceExternalImport  SourceKind = "external_import"
	SourceCLIChildSession SourceKind = "cli_child_session"
	SourceManualRecovery  SourceKind = "manual_recovery"
)

type Context struct {
	WorkflowType      wruntime.WorkflowID
	State             wruntime.State
	ExpectedNodeID    wruntime.NodeID
	Source            SourceKind
	PlanDir           string
	ImplementationCwd string
	PlanningCwd       string
	RunID             string
}

type Normalization struct {
	Field     string `json:"field"`
	Original  string `json:"original"`
	Canonical string `json:"canonical"`
	Reason    string `json:"reason"`
}

func ReviewPlanPositiveOutcome(ctx Context) (wruntime.ResultOutcome, bool) {
	if strings.TrimSpace(ctx.ImplementationCwd) != "" || IsReviewDir(ctx.PlanDir) || len(ctx.State.Followups) > 0 {
		return wruntime.OutcomeReadyForImplement, true
	}
	return wruntime.OutcomeReadyForWorkspace, true
}

func IsReviewDir(planDir string) bool {
	planDir = strings.TrimSpace(path.Clean(strings.ReplaceAll(planDir, "\\", "/")))
	return strings.Contains(planDir, "/reviews/")
}

func ExecutionCwdFromWorkspaceResult(result wruntime.WorkflowResult) string {
	if workspace := strings.TrimSpace(result.Workspace); workspace != "" {
		return workspace
	}
	return strings.TrimSpace(qrspi.WorkflowResultImplementationWorkspace(result))
}

func positiveOutcome(outcome string) bool {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(outcome)), "_", "-") {
	case "complete", "completed", "done", "success", "succeeded", "ok", "ready", "approved", "ready-to-plan", "ready-for-planning", "ready-to-implement", "ready-to-implementation", "ready-to-workspace", "ready-for-workspace", "ready-for-workspaces":
		return true
	default:
		return false
	}
}

func cleanArtifactPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean(value), "./")
}
