package agentchat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

// Deprecated: use EnsureSyncWorkspacesSchedule.
func EnsurePlanWorkspaceDiscoverySchedule(
	ctx context.Context,
	temporalClient client.Client,
	input PlanWorkspaceDiscoveryInput,
) error {
	if temporalClient == nil {
		return nil
	}

	scheduleID := PlanWorkspaceDiscoveryScheduleID(input.ProjectInstanceKey)
	handle := temporalClient.ScheduleClient().GetHandle(ctx, scheduleID)
	if described, err := handle.Describe(ctx); err == nil && described != nil {
		// The schedule already exists for this workspace. Trigger it instead of
		// updating in place: the Temporal Go SDK can panic while converting a
		// partial ScheduleUpdate for existing schedules in local workspace runs.
		return handle.Trigger(ctx, client.ScheduleTriggerOptions{})
	}

	_, err := temporalClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:                 scheduleID,
		Spec:               planWorkspaceDiscoverySpec(),
		Action:             planWorkspaceDiscoveryAction(input),
		Overlap:            enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		TriggerImmediately: true,
	})
	return err
}

func planWorkspaceDiscoverySchedule(input PlanWorkspaceDiscoveryInput) *client.Schedule {
	return &client.Schedule{
		Spec:   ptr(planWorkspaceDiscoverySpec()),
		Action: planWorkspaceDiscoveryAction(input),
		Policy: &client.SchedulePolicies{
			Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		},
	}
}

func planWorkspaceDiscoverySpec() client.ScheduleSpec {
	return client.ScheduleSpec{
		Intervals: []client.ScheduleIntervalSpec{{
			Every: defaultPlanWorkspaceDiscoveryInterval,
		}},
	}
}

func planWorkspaceDiscoveryAction(
	input PlanWorkspaceDiscoveryInput,
) *client.ScheduleWorkflowAction {
	return &client.ScheduleWorkflowAction{
		ID:        PlanWorkspaceDiscoveryWorkflowID(input.ProjectInstanceKey),
		Workflow:  PlanWorkspaceDiscoveryWorkflow,
		Args:      []any{input},
		TaskQueue: temporalmgr.GoTaskQueue,
	}
}

// Deprecated: use SyncWorkspacesScheduleID.
func PlanWorkspaceDiscoveryScheduleID(projectInstanceKey string) string {
	return "agent-chat-plan-workspace-discovery:" + normalizeTemporalIDPart(
		projectInstanceKey,
	)
}

// Deprecated: use SyncWorkspacesWorkflowID.
func PlanWorkspaceDiscoveryWorkflowID(projectInstanceKey string) string {
	return PlanWorkspaceDiscoveryScheduleID(projectInstanceKey) + ":scan"
}

func projectInstanceKey(projectName, projectRoot string) string {
	name := normalizeTemporalIDPart(projectName)
	root := filepath.Clean(strings.TrimSpace(projectRoot))
	sum := sha256.Sum256([]byte(root))
	return fmt.Sprintf("%s-%s", name, hex.EncodeToString(sum[:])[:12])
}

func normalizeTemporalIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	if value == "" {
		return "project"
	}
	return value
}

func ptr[T any](value T) *T {
	return &value
}
