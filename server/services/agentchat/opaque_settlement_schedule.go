package agentchat

import (
	"context"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

const opaqueSettlementDeliveryInterval = time.Minute

func EnsureOpaqueSettlementDeliverySchedule(
	ctx context.Context,
	temporalClient client.Client,
	thoughtsRoot string,
) error {
	if temporalClient == nil {
		return nil
	}
	id := OpaqueSettlementDeliveryScheduleID(thoughtsRoot)
	handle := temporalClient.ScheduleClient().GetHandle(ctx, id)
	if _, err := handle.Describe(ctx); err == nil {
		return handle.Trigger(ctx, client.ScheduleTriggerOptions{})
	}
	_, err := temporalClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID: id,
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: opaqueSettlementDeliveryInterval},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        id + ":run",
			Workflow:  OpaqueSettlementDiscoveryWorkflow,
			Args:      []any{OpaqueSettlementDeliveryInput{ThoughtsRoot: thoughtsRoot}},
			TaskQueue: temporalmgr.GoTaskQueue,
		},
		Overlap:            enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		TriggerImmediately: true,
	})
	return err
}

func OpaqueSettlementDeliveryScheduleID(thoughtsRoot string) string {
	return "opaque-settlement-discovery:" + normalizeTemporalIDPart(thoughtsRoot)
}
