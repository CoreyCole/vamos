package agentchat

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type PlanWorkspaceDiscoveryActivities struct {
	Syncer *PlanWorkspaceSyncer
}

// Deprecated: use SyncWorkspacesWorkflow.
func PlanWorkspaceDiscoveryWorkflow(
	ctx workflow.Context,
	input PlanWorkspaceDiscoveryInput,
) (PlanWorkspaceDiscoveryResult, error) {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var result PlanWorkspaceDiscoveryResult
	err := workflow.ExecuteActivity(activityCtx, "SyncPlanWorkspaces", input).
		Get(ctx, &result)
	return result, err
}

// Deprecated: use WorkspaceSyncActivities.SyncWorkspaces.
func (a *PlanWorkspaceDiscoveryActivities) SyncPlanWorkspaces(
	ctx context.Context,
	input PlanWorkspaceDiscoveryInput,
) (PlanWorkspaceDiscoveryResult, error) {
	if a == nil || a.Syncer == nil {
		return PlanWorkspaceDiscoveryResult{}, errors.New(
			"plan workspace discovery activity requires syncer",
		)
	}
	return a.Syncer.Sync(ctx, input)
}
