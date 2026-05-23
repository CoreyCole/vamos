package workspaces

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// StartWorkspaceWorkflow starts a workspace for a transition owner.
func StartWorkspaceWorkflow(
	ctx workflow.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return runWorkspaceLifecycleWorkflow(ctx, input, "StartWorkspace")
}

// StopWorkspaceWorkflow stops a workspace for a transition owner.
func StopWorkspaceWorkflow(
	ctx workflow.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return runWorkspaceLifecycleWorkflow(ctx, input, "StopWorkspace")
}

// RestartWorkspaceWorkflow restarts a workspace for a transition owner.
func RestartWorkspaceWorkflow(
	ctx workflow.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	return runWorkspaceLifecycleWorkflow(ctx, input, "RestartWorkspace")
}

func runWorkspaceLifecycleWorkflow(
	ctx workflow.Context,
	input WorkspaceLifecycleWorkflowInput,
	activityName string,
) error {
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
	return workflow.ExecuteActivity(ctx, activityName, input).Get(ctx, nil)
}
