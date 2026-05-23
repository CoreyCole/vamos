package workspaces

import (
	"context"
	"fmt"
)

type temporalWorkflowStarter interface {
	StartWorkflowIdempotent(
		ctx context.Context,
		workflowID string,
		workflowFunc, input any,
	) (string, error)
}

// TemporalLifecycleStarter starts one short Temporal workflow per lifecycle transition.
type TemporalLifecycleStarter struct {
	temporal temporalWorkflowStarter
}

func NewTemporalLifecycleStarter(
	temporal temporalWorkflowStarter,
) *TemporalLifecycleStarter {
	return &TemporalLifecycleStarter{temporal: temporal}
}

func (s *TemporalLifecycleStarter) StartTransition(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	if s == nil || s.temporal == nil {
		return fmt.Errorf("temporal lifecycle starter is not configured")
	}
	workflowID := fmt.Sprintf("workspace-lifecycle/%s/%s", input.Slug, input.TransitionID)
	workflowFn := any(StartWorkspaceWorkflow)
	switch input.Kind {
	case WorkspaceTransitionStop:
		workflowFn = StopWorkspaceWorkflow
	case WorkspaceTransitionRestart:
		workflowFn = RestartWorkspaceWorkflow
	}
	_, err := s.temporal.StartWorkflowIdempotent(ctx, workflowID, workflowFn, input)
	return err
}
