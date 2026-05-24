package agentchat

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type WorkspaceSyncActivities struct {
	Syncer     workspaceSyncRunner
	OnComplete func(context.Context, SyncWorkspacesResult, error)
}

type workspaceSyncRunner interface {
	Sync(context.Context, SyncWorkspacesInput) (SyncWorkspacesResult, error)
}

func SyncWorkspacesWorkflow(
	ctx workflow.Context,
	input SyncWorkspacesInput,
) (SyncWorkspacesResult, error) {
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

	var result SyncWorkspacesResult
	err := workflow.ExecuteActivity(activityCtx, "SyncWorkspaces", input).
		Get(ctx, &result)
	return result, err
}

func (a *WorkspaceSyncActivities) SyncWorkspaces(
	ctx context.Context,
	input SyncWorkspacesInput,
) (SyncWorkspacesResult, error) {
	if a == nil || a.Syncer == nil {
		return SyncWorkspacesResult{}, errors.New(
			"workspace sync activity requires syncer",
		)
	}
	result, err := a.Syncer.Sync(ctx, input)
	if input.RunCompletionHook && a.OnComplete != nil {
		a.OnComplete(ctx, result, err)
	}
	return result, err
}
