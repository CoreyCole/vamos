package workspaces

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// CleanupWorkspaceWorkflow removes a feature workspace checkout/runtime state.
func CleanupWorkspaceWorkflow(ctx workflow.Context, input WorkspaceCleanupWorkflowInput) error {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    15 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)
	return workflow.ExecuteActivity(ctx, "CleanupWorkspace", input).Get(ctx, nil)
}
