package workspaces

import (
	"context"
	"testing"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestCleanupWorkspaceWorkflowRunsCleanupActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	env.RegisterActivityWithOptions(
		func(input WorkspaceCleanupWorkflowInput) error {
			calls++
			if input.Slug != "foo" || input.TransitionID != "transition-1" || input.Disposition != WorkspaceCleanupDispositionMerged {
				t.Fatalf("input=%#v", input)
			}
			return nil
		},
		activity.RegisterOptions{Name: "CleanupWorkspace"},
	)

	env.ExecuteWorkflow(CleanupWorkspaceWorkflow, WorkspaceCleanupWorkflowInput{
		Slug:         "foo",
		TransitionID: "transition-1",
		Disposition:  WorkspaceCleanupDispositionMerged,
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

func TestCleanupWorkflowIDIsDeterministic(t *testing.T) {
	got := CleanupWorkspaceWorkflowID(" foo ", " transition-1 ")
	want := "workspace-cleanup/foo/transition-1"
	if got != want {
		t.Fatalf("CleanupWorkspaceWorkflowID=%q want %q", got, want)
	}
}

type fakeCleanupTemporalClient struct {
	workflowIDs []string
}

func (f *fakeCleanupTemporalClient) StartWorkflowIdempotent(ctx context.Context, workflowID string, workflowFunc, input any) (string, error) {
	f.workflowIDs = append(f.workflowIDs, workflowID)
	return "run-1", nil
}

func TestTemporalCleanupStarterStartsDeterministicWorkflow(t *testing.T) {
	client := &fakeCleanupTemporalClient{}
	starter := NewTemporalCleanupStarter(client)
	if err := starter.StartCleanup(context.Background(), WorkspaceCleanupWorkflowInput{Slug: "foo", TransitionID: "transition-1"}); err != nil {
		t.Fatalf("StartCleanup: %v", err)
	}
	if len(client.workflowIDs) != 1 || client.workflowIDs[0] != "workspace-cleanup/foo/transition-1" {
		t.Fatalf("workflowIDs=%v", client.workflowIDs)
	}
}
