package conversationworkflow

import (
	"errors"
	"fmt"
	"strings"
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
	return runPreparedTurn(ctx, input)
}

func ThreadInboxWorkflow(
	ctx workflow.Context,
	input conversation.ThreadWorkflowInput,
) error {
	inbox := make([]conversation.ThreadMail, 0, 4)
	seen := map[string]struct{}{}
	lastSpeaker := ""
	ch := workflow.GetSignalChannel(ctx, conversation.ThreadMailSignal)

	for {
		for {
			var mail conversation.ThreadMail
			ok := ch.ReceiveAsync(&mail)
			if !ok {
				break
			}
			opID := strings.TrimSpace(mail.OpID)
			if opID == "" {
				continue
			}
			if _, dup := seen[opID]; dup {
				continue
			}
			seen[opID] = struct{}{}
			inbox = append(inbox, mail)
		}
		if len(inbox) == 0 {
			return settleIdleThread(ctx, input.ThreadID, lastSpeaker)
		}
		item := inbox[0]
		inbox = inbox[1:]
		_ = drainMailItem(ctx, item)
		lastSpeaker = strings.TrimSpace(item.SpeakerAgentID)
	}
}

func settleIdleThread(ctx workflow.Context, threadID, speakerSlug string) error {
	goActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:           temporalmgr.GoTaskQueue,
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporalsdk.RetryPolicy{
			MaximumAttempts: 3,
		},
	})
	var usage conversation.ThreadUsage
	if err := workflow.ExecuteActivity(
		goActCtx,
		conversation.ActivityInspectThreadUsage,
		conversation.ThreadWorkflowInput{ThreadID: threadID},
	).Get(ctx, &usage); err != nil {
		return fmt.Errorf("inspect thread usage: %w", err)
	}
	if err := workflow.ExecuteActivity(
		goActCtx,
		conversation.ActivitySettleHotRoom,
		conversation.SettleIdleInput{
			ThreadID:    threadID,
			UsageHot:    usage.Hot,
			SpeakerSlug: speakerSlug,
		},
	).Get(ctx, nil); err != nil {
		return fmt.Errorf("settle idle thread: %w", err)
	}
	return nil
}

func drainMailItem(ctx workflow.Context, mail conversation.ThreadMail) error {
	goActCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:           temporalmgr.GoTaskQueue,
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporalsdk.RetryPolicy{
			MaximumAttempts: 3,
		},
	})
	var input conversation.RunInput
	if err := workflow.ExecuteActivity(
		goActCtx,
		conversation.ActivityPrepareThreadTurn,
		mail,
	).Get(ctx, &input); err != nil {
		return fmt.Errorf("prepare thread turn: %w", err)
	}
	_, err := runPreparedTurn(ctx, input)
	return err
}

func runPreparedTurn(
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
			return conversation.RunResult{}, errors.Join(
				fmt.Errorf("run conversation turn: %w", err),
				fmt.Errorf("failure finalizer: %w", finalizeErr),
			)
		}
		return conversation.RunResult{}, fmt.Errorf("run conversation turn: %w", err)
	}
	return result, nil
}
