package workspaces

import (
	"context"
	"fmt"
)

// WorkspaceLifecycleActivities own process side effects for lifecycle workflows.
type WorkspaceLifecycleActivities struct {
	Manager *ManagerService
}

// StartWorkspace starts a workspace for the transition owner and completes it.
func (a *WorkspaceLifecycleActivities) StartWorkspace(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return a.complete(
		ctx,
		input,
		func(ctx context.Context, _ Workspace) (Workspace, error) {
			return a.Manager.startOwnedLifecycleTransition(ctx, input)
		},
	)
}

// StopWorkspace stops a workspace for the transition owner and completes it.
func (a *WorkspaceLifecycleActivities) StopWorkspace(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return a.complete(
		ctx,
		input,
		func(ctx context.Context, ws Workspace) (Workspace, error) {
			if ws.Status == StatusStopped || ws.Status == "" {
				return ws, nil
			}
			return a.Manager.Stop(ctx, input.Slug)
		},
	)
}

// RestartWorkspace restarts a workspace for the transition owner and completes it.
func (a *WorkspaceLifecycleActivities) RestartWorkspace(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return a.complete(
		ctx,
		input,
		func(ctx context.Context, _ Workspace) (Workspace, error) {
			if !a.Manager.transitionOwnsWorkspace(input.Slug, input.TransitionID) {
				return Workspace{}, nil
			}
			stopped, err := a.Manager.Stop(ctx, input.Slug)
			if err != nil {
				return stopped, err
			}
			return a.Manager.startOwnedLifecycleTransition(ctx, input)
		},
	)
}

func (a *WorkspaceLifecycleActivities) complete(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
	fn func(context.Context, Workspace) (Workspace, error),
) error {
	if a.Manager == nil {
		return fmt.Errorf("workspace manager is not configured")
	}
	ws, ok := a.Manager.Lookup(input.Slug)
	if !ok {
		return fmt.Errorf("unknown workspace %q", input.Slug)
	}
	if !a.Manager.transitionOwnsWorkspace(input.Slug, input.TransitionID) {
		return nil
	}
	next, err := fn(ctx, ws)
	if err != nil {
		_ = a.Manager.CompleteTransition(
			ctx,
			input.Slug,
			input.TransitionID,
			WorkspaceTransitionResult{
				ObservedState: WorkspaceObservedFailed,
				Error:         err.Error(),
			},
		)
		return err
	}
	if next.Slug == "" {
		next = ws
	}
	observed := observedFromStatus(next.Status)
	if observed == WorkspaceObservedStarting || observed == WorkspaceObservedStopping {
		observed = desiredTerminalObserved(input.Kind)
	}
	return a.Manager.CompleteTransition(
		ctx,
		input.Slug,
		input.TransitionID,
		WorkspaceTransitionResult{
			ObservedState: observed,
			Workspace:     next,
		},
	)
}

func desiredTerminalObserved(kind WorkspaceTransitionKind) WorkspaceObservedState {
	if kind == WorkspaceTransitionStop {
		return WorkspaceObservedStopped
	}
	return WorkspaceObservedRunning
}
