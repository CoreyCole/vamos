package agentchat

import (
	"context"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

const defaultWorkspaceSyncInterval = time.Minute

func EnsureSyncWorkspacesSchedule(
	ctx context.Context,
	temporalClient client.Client,
	input SyncWorkspacesInput,
) error {
	if temporalClient == nil {
		return nil
	}

	scheduleID := SyncWorkspacesScheduleID(input.ProjectInstanceKey)
	handle := temporalClient.ScheduleClient().GetHandle(ctx, scheduleID)
	if described, err := handle.Describe(ctx); err == nil && described != nil {
		// The schedule already exists for this workspace. Trigger it instead of
		// updating in place: the Temporal Go SDK can panic while converting a
		// partial ScheduleUpdate for existing schedules in local workspace runs.
		return handle.Trigger(ctx, client.ScheduleTriggerOptions{})
	}

	_, err := temporalClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:                 scheduleID,
		Spec:               syncWorkspacesSpec(),
		Action:             syncWorkspacesAction(input),
		Overlap:            enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		TriggerImmediately: true,
	})
	return err
}

func syncWorkspacesSchedule(input SyncWorkspacesInput) *client.Schedule {
	return &client.Schedule{
		Spec:   ptr(syncWorkspacesSpec()),
		Action: syncWorkspacesAction(input),
		Policy: &client.SchedulePolicies{
			Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		},
	}
}

func syncWorkspacesSpec() client.ScheduleSpec {
	return client.ScheduleSpec{
		Intervals: []client.ScheduleIntervalSpec{{
			Every: defaultWorkspaceSyncInterval,
		}},
	}
}

func syncWorkspacesAction(input SyncWorkspacesInput) *client.ScheduleWorkflowAction {
	return &client.ScheduleWorkflowAction{
		ID:        SyncWorkspacesWorkflowID(input.ProjectInstanceKey),
		Workflow:  SyncWorkspacesWorkflow,
		Args:      []any{input},
		TaskQueue: temporalmgr.GoTaskQueue,
	}
}

func SyncWorkspacesScheduleID(projectInstanceKey string) string {
	return "agent-chat-sync-workspaces:" + normalizeTemporalIDPart(projectInstanceKey)
}

func SyncWorkspacesWorkflowID(projectInstanceKey string) string {
	return SyncWorkspacesScheduleID(projectInstanceKey) + ":sync"
}
