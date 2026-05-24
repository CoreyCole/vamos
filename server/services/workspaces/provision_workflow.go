package workspaces

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func WorkspaceProvisionWorkflow(
	ctx workflow.Context,
	input WorkspaceProvisionInput,
) (WorkspaceProvisionResult, error) {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    15 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)
	var result WorkspaceProvisionResult
	err := workflow.ExecuteActivity(ctx, "ProvisionWorkspace", input).Get(ctx, &result)
	return result, err
}
