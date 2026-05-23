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
