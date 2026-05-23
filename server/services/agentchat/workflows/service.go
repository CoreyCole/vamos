package workflows

import (
	"context"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type Service struct {
	Definitions *wruntime.Registry
	Store       Store
	Runner      Runner
}

type Store interface {
	LoadWorkspaceState(ctx context.Context, workspaceID string) (wruntime.State, error)
	SaveWorkspaceState(
		ctx context.Context,
		workspaceID string,
		state wruntime.State,
	) error
	LoadRun(ctx context.Context, runID string) (db.AgentRun, error)
	SaveRunResult(ctx context.Context, runID string, result wruntime.WorkflowResult) error
	AppendWorkflowEvents(
		ctx context.Context,
		workspaceID string,
		run db.AgentRun,
		events []wruntime.Event,
	) error
	ArtifactExists(ctx context.Context, workspaceID, relPath string) (bool, error)
	FinalAssistantText(ctx context.Context, threadID, headEntryID string) (string, error)
	WorkspacePlanningCwd(ctx context.Context, workspaceID string) (string, error)
}

type Runner interface {
	StartNodeRun(ctx context.Context, input StartNodeRunInput) (string, error)
}

type StartNodeRunInput struct {
	WorkspaceID string
	ThreadID    string
	NodeID      wruntime.NodeID
	Prompt      string
	Attempt     int
	Cwd         string
}

type NoopRunner struct{}

func (NoopRunner) StartNodeRun(context.Context, StartNodeRunInput) (string, error) {
	return "", nil
}
