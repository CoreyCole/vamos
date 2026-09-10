//go:build !integration || unit
// +build !integration unit

package conversationworkflow

import (
	"errors"
	"strings"
	"testing"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
)

func TestRunTurnWorkflowDoesNotRetryWholePiTurnActivity(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	finalizeCalls := 0
	env.RegisterActivityWithOptions(
		func(conversation.RunInput) (conversation.RunResult, error) {
			calls++
			return conversation.RunResult{}, errors.New("callback delivery failed")
		},
		activity.RegisterOptions{Name: "RunConversationTurn"},
	)
	env.RegisterActivityWithOptions(
		func(input conversation.ActivityFailureInput) error {
			finalizeCalls++
			if input.RunID != "run-1" {
				t.Fatalf("finalizer run id = %q, want run-1", input.RunID)
			}
			return nil
		},
		activity.RegisterOptions{Name: "FailConversationRunAfterActivityError"},
	)

	env.ExecuteWorkflow(RunTurnWorkflow, conversation.RunInput{RunID: "run-1"})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() == nil {
		t.Fatal("workflow error = nil, want error")
	}
	if calls != 1 {
		t.Fatalf("RunConversationTurn calls = %d, want 1", calls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
}

func TestThreadInboxWorkflowDrainsTwoMailsThenCompletes(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	prepareCalls := 0
	turnCalls := 0
	speakers := make([]string, 0, 2)
	env.RegisterActivityWithOptions(
		func(mail conversation.ThreadMail) (conversation.RunInput, error) {
			prepareCalls++
			speakers = append(speakers, mail.SpeakerAgentID)
			return conversation.RunInput{
				RunID:    mail.OpID,
				ThreadID: mail.ThreadID,
				Prompt:   mail.Body,
			}, nil
		},
		activity.RegisterOptions{Name: conversation.ActivityPrepareThreadTurn},
	)
	env.RegisterActivityWithOptions(
		func(input conversation.RunInput) (conversation.RunResult, error) {
			turnCalls++
			return conversation.RunResult{
				RunID:    input.RunID,
				ThreadID: input.ThreadID,
			}, nil
		},
		activity.RegisterOptions{Name: "RunConversationTurn"},
	)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(conversation.ThreadMailSignal, conversation.ThreadMail{
			ThreadID: "thread-1", OpID: "op-a", SpeakerAgentID: "agent-a", Body: "one",
		})
		env.SignalWorkflow(conversation.ThreadMailSignal, conversation.ThreadMail{
			ThreadID: "thread-1", OpID: "op-b", SpeakerAgentID: "agent-b", Body: "two",
		})
		env.SignalWorkflow(conversation.ThreadMailSignal, conversation.ThreadMail{
			ThreadID: "thread-1", OpID: "op-a", SpeakerAgentID: "agent-a", Body: "dup",
		})
	}, 0)

	env.ExecuteWorkflow(
		ThreadInboxWorkflow,
		conversation.ThreadWorkflowInput{ThreadID: "thread-1"},
	)
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() != nil {
		t.Fatalf("workflow error: %v", env.GetWorkflowError())
	}
	if prepareCalls != 2 || turnCalls != 2 {
		t.Fatalf("prepare=%d turn=%d, want 2/2", prepareCalls, turnCalls)
	}
	if strings.Join(speakers, ",") != "agent-a,agent-b" {
		t.Fatalf("speakers=%v, want sequential agent-a then agent-b", speakers)
	}
}

func TestThreadInboxWorkflowContinuesAfterFailedDrain(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	prepareCalls := 0
	turnCalls := 0
	env.RegisterActivityWithOptions(
		func(mail conversation.ThreadMail) (conversation.RunInput, error) {
			prepareCalls++
			return conversation.RunInput{
				RunID:    mail.OpID,
				ThreadID: mail.ThreadID,
				Prompt:   mail.Body,
			}, nil
		},
		activity.RegisterOptions{Name: conversation.ActivityPrepareThreadTurn},
	)
	env.RegisterActivityWithOptions(
		func(input conversation.RunInput) (conversation.RunResult, error) {
			turnCalls++
			if input.RunID == "op-a" {
				return conversation.RunResult{}, errors.New("pi failed")
			}
			return conversation.RunResult{
				RunID:    input.RunID,
				ThreadID: input.ThreadID,
			}, nil
		},
		activity.RegisterOptions{Name: "RunConversationTurn"},
	)
	env.RegisterActivityWithOptions(
		func(conversation.ActivityFailureInput) error { return nil },
		activity.RegisterOptions{Name: "FailConversationRunAfterActivityError"},
	)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(conversation.ThreadMailSignal, conversation.ThreadMail{
			ThreadID: "thread-1", OpID: "op-a", Body: "one",
		})
		env.SignalWorkflow(conversation.ThreadMailSignal, conversation.ThreadMail{
			ThreadID: "thread-1", OpID: "op-b", Body: "two",
		})
	}, 0)

	env.ExecuteWorkflow(
		ThreadInboxWorkflow,
		conversation.ThreadWorkflowInput{ThreadID: "thread-1"},
	)
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() != nil {
		t.Fatalf("workflow error: %v", env.GetWorkflowError())
	}
	if prepareCalls != 2 || turnCalls != 2 {
		t.Fatalf("prepare=%d turn=%d, want 2/2", prepareCalls, turnCalls)
	}
}

func TestRunTurnWorkflowIncludesFinalizerError(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(
		func(conversation.RunInput) (conversation.RunResult, error) {
			return conversation.RunResult{}, errors.New("callback delivery failed")
		},
		activity.RegisterOptions{Name: "RunConversationTurn"},
	)
	env.RegisterActivityWithOptions(
		func(conversation.ActivityFailureInput) error {
			return errors.New("finalizer unavailable")
		},
		activity.RegisterOptions{Name: "FailConversationRunAfterActivityError"},
	)

	env.ExecuteWorkflow(RunTurnWorkflow, conversation.RunInput{RunID: "run-1"})

	err := env.GetWorkflowError()
	if err == nil {
		t.Fatal("workflow error = nil, want error")
	}
	if !strings.Contains(err.Error(), "callback delivery failed") ||
		!strings.Contains(err.Error(), "finalizer unavailable") {
		t.Fatalf("workflow error = %v, want both activity and finalizer errors", err)
	}
}
