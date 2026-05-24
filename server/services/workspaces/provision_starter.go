package workspaces

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

type TemporalProvisionStarter struct {
	temporal temporalWorkflowStarter
}

func NewTemporalProvisionStarter(temporal temporalWorkflowStarter) *TemporalProvisionStarter {
	return &TemporalProvisionStarter{temporal: temporal}
}

func (s *TemporalProvisionStarter) StartProvision(
	ctx context.Context,
	input WorkspaceProvisionInput,
) (WorkspaceProvisionResult, error) {
	if s == nil || s.temporal == nil {
		return WorkspaceProvisionResult{}, fmt.Errorf("temporal provision starter is not configured")
	}
	workflowID := fmt.Sprintf(
		"workspace-provision/%s/%s",
		sanitizeProvisionID(input.WorkspaceSlug),
		shortProvisionHash(input.PlanDir),
	)
	_, err := s.temporal.StartWorkflowIdempotent(ctx, workflowID, WorkspaceProvisionWorkflow, input)
	if err != nil {
		return WorkspaceProvisionResult{}, err
	}
	return WorkspaceProvisionResult{
		WorkspacePath: input.RequestedPath,
		WorkspaceSlug: input.WorkspaceSlug,
		BaseRef:       input.ParentStackRef,
		Status:        WorkspaceProvisionStatusComplete,
		Message:       "workspace provision workflow started",
	}, nil
}

func sanitizeProvisionID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "workspace"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return b.String()
}

func shortProvisionHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:12]
}
