package conversationworkflow

import (
	"fmt"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
)

func RunTurnWorkflow(
	ctx workflow.Context,
	input conversation.RunInput,
) (conversation.RunResult, error) {
	tsActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:           temporalmgr.TSTaskQueue,
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporalsdk.RetryPolicy{
			MaximumAttempts: 1,
		},
	})
	goActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:           temporalmgr.GoTaskQueue,
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporalsdk.RetryPolicy{
			MaximumAttempts: 3,
		},
	})

	var result conversation.RunResult
	if err := workflow.ExecuteActivity(tsActCtx, "RunConversationTurn", input).
		Get(ctx, &result); err != nil {
		finalizeInput := conversation.NewActivityFailureInput(input, err)
		if finalizeErr := workflow.ExecuteActivity(
			goActCtx,
			"FailConversationRunAfterActivityError",
			finalizeInput,
		).Get(ctx, nil); finalizeErr != nil {
			return conversation.RunResult{}, fmt.Errorf(
				"run conversation turn: %w; failure finalizer: %v",
				err,
				finalizeErr,
			)
		}
		return conversation.RunResult{}, fmt.Errorf("run conversation turn: %w", err)
	}
	return result, nil
}
