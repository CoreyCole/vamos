package workspaces

import (
	"context"
	"fmt"
	"strings"
)

// TemporalCleanupStarter starts one idempotent workflow per cleanup request.
type TemporalCleanupStarter struct {
	temporal temporalWorkflowStarter
}

func NewTemporalCleanupStarter(temporal temporalWorkflowStarter) *TemporalCleanupStarter {
	return &TemporalCleanupStarter{temporal: temporal}
}

func (s *TemporalCleanupStarter) StartCleanup(ctx context.Context, input WorkspaceCleanupWorkflowInput) error {
	if s == nil || s.temporal == nil {
		return fmt.Errorf("temporal cleanup starter is not configured")
	}
	workflowID := CleanupWorkspaceWorkflowID(input.Slug, input.TransitionID)
	_, err := s.temporal.StartWorkflowIdempotent(ctx, workflowID, CleanupWorkspaceWorkflow, input)
	return err
}

func CleanupWorkspaceWorkflowID(slug, transitionID string) string {
	return fmt.Sprintf("workspace-cleanup/%s/%s", strings.TrimSpace(slug), strings.TrimSpace(transitionID))
}
