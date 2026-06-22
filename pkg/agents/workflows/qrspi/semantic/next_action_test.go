package semantic

import (
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestProjectNextActionFixtureMatrix(t *testing.T) {
	tests := []struct {
		name     string
		state    wruntime.State
		result   wruntime.WorkflowResult
		want     NextActionKind
		wantNext wruntime.NodeID
	}{
		{
			name:     "wait human",
			state:    wruntime.State{Status: wruntime.WorkspaceStatusWaitingHuman, HumanGate: &wruntime.HumanGateState{To: qrspi.NodeHumanReviewOutline, Reason: "review outline"}},
			result:   wruntime.WorkflowResult{SourceNodeID: qrspi.NodeOutline, Status: wruntime.StatusComplete, Outcome: wruntime.OutcomeComplete, PrimaryArtifact: "thoughts/example/outline.md"},
			want:     NextActionWaitHuman,
			wantNext: qrspi.NodeHumanReviewOutline,
		},
		{
			name:     "continue pending",
			state:    wruntime.State{Status: wruntime.WorkspaceStatusIdle, PendingNextNodeID: qrspi.NodeResearch},
			result:   wruntime.WorkflowResult{SourceNodeID: qrspi.NodeQuestion, Status: wruntime.StatusComplete, Outcome: wruntime.OutcomeComplete},
			want:     NextActionContinuePending,
			wantNext: qrspi.NodeResearch,
		},
		{
			name:     "start next running",
			state:    wruntime.State{Status: wruntime.WorkspaceStatusRunning, PendingNextNodeID: qrspi.NodeResearch},
			result:   wruntime.WorkflowResult{SourceNodeID: qrspi.NodeQuestion, Status: wruntime.StatusComplete, Outcome: wruntime.OutcomeComplete},
			want:     NextActionStartNext,
			wantNext: qrspi.NodeResearch,
		},
		{
			name:   "blocked",
			state:  wruntime.State{Status: wruntime.WorkspaceStatusBlocked},
			result: wruntime.WorkflowResult{SourceNodeID: qrspi.NodeVerify, Status: wruntime.StatusBlocked, PrimaryArtifact: "thoughts/example/verify.md"},
			want:   NextActionBlocked,
		},
		{
			name:   "error",
			state:  wruntime.State{Status: wruntime.WorkspaceStatusError},
			result: wruntime.WorkflowResult{SourceNodeID: qrspi.NodeVerify, Status: wruntime.StatusError, PrimaryArtifact: "thoughts/example/verify.md"},
			want:   NextActionError,
		},
		{
			name:   "done",
			state:  wruntime.State{Status: wruntime.WorkspaceStatusDone},
			result: wruntime.WorkflowResult{SourceNodeID: qrspi.NodeDone, Status: wruntime.StatusComplete, Outcome: wruntime.OutcomeComplete},
			want:   NextActionDone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectNextActionFromState(tt.result, tt.state)
			if got.Kind != tt.want || got.NextNodeID != tt.wantNext {
				t.Fatalf("action/next = %q/%q, want %q/%q", got.Kind, got.NextNodeID, tt.want, tt.wantNext)
			}
		})
	}
}

func TestProjectInvalidResultAction(t *testing.T) {
	retry := ProjectInvalidResultAction(qrspi.NodePlan, "missing qrspi_result", false)
	if retry.Kind != NextActionInvalidRetry || retry.CurrentNodeID != qrspi.NodePlan || retry.Evidence[0] != "missing qrspi_result" {
		t.Fatalf("retry action = %#v", retry)
	}
	exhausted := ProjectInvalidResultAction(qrspi.NodePlan, "missing qrspi_result", true)
	if exhausted.Kind != NextActionInvalidExhausted {
		t.Fatalf("exhausted action = %#v", exhausted)
	}
}
