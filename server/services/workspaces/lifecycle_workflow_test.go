package workspaces

import (
	"testing"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestStartWorkspaceWorkflowRunsStartActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	env.RegisterActivityWithOptions(
		func(input WorkspaceLifecycleWorkflowInput) error {
			calls++
			if input.Slug != "foo" || input.TransitionID != "transition-1" {
				t.Fatalf("input=%#v", input)
			}
			return nil
		},
		activity.RegisterOptions{Name: "StartWorkspace"},
	)

	env.ExecuteWorkflow(StartWorkspaceWorkflow, WorkspaceLifecycleWorkflowInput{
		Slug:         "foo",
		TransitionID: "transition-1",
		Kind:         WorkspaceTransitionStart,
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error=%v", err)
	}
	if calls != 1 {
		t.Fatalf("activity calls=%d want 1", calls)
	}
}
