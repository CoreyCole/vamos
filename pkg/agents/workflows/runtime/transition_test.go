package runtime

import (
	"strings"
	"testing"
)

type transitionConfig struct {
	StartNonHuman bool `json:"startNonHuman"`
	Auto          bool `json:"auto"`
}

func (c transitionConfig) IsAutoMode() bool               { return c.Auto }
func (c transitionConfig) ShouldStartNonHumanEdges() bool { return c.StartNonHuman }

func transitionDefinition(t *testing.T) Definition {
	t.Helper()
	def, err := New[struct{}]("test").
		Start("start").
		Agent("start", PromptSpec{Static: "start"}).
		Agent("review", PromptSpec{Static: "review"}).
		HumanReview("human", "approve review").
		Done("done").
		Edge("start", "review").
		HumanGate("review", "human", "approve review").
		Edge("human", "done").
		ResultParser(parserStub{}).
		ResultConverter(converterStub{}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func completeResult(node NodeID) WorkflowResult {
	return WorkflowResult{
		WorkflowType:    "test",
		SourceNodeID:    node,
		Status:          "complete",
		Summary:         "done",
		PrimaryArtifact: "thoughts/example.md",
		DisplayNext:     "/next",
		Evidence:        EvidenceRef{RunID: "run-1"},
	}
}

func TestDecideTransitionDiscussModeLeavesPendingNextIdle(t *testing.T) {
	def, err := New[transitionConfig]("test").
		Config(transitionConfig{StartNonHuman: false}, nil).
		Start("start").
		Agent("start", PromptSpec{Static: "start"}).
		Agent("review", PromptSpec{Static: "review"}).
		Done("done").
		Edge("start", "review").
		Edge("review", "done").
		ResultParser(parserStub{}).
		ResultConverter(converterStub{}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}

	decision, err := DecideTransition(def, state, completeResult("start"))
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if decision.StartNext || decision.NextNodeID != "review" || decision.State.CurrentNodeID != "review" || decision.State.PendingNextNodeID != "review" || decision.State.Status != WorkspaceStatusIdle {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDecideTransitionCompleteStartsNextNode(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}

	decision, err := DecideTransition(def, state, completeResult("start"))
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if !decision.StartNext || decision.NextNodeID != "review" ||
		decision.State.CurrentNodeID != "review" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.State.Nodes["start"].Status != NodeStatusComplete {
		t.Fatalf("start node state = %+v", decision.State.Nodes["start"])
	}
	if decision.State.LastResult == nil ||
		decision.State.LastResult.PrimaryArtifact != "thoughts/example.md" {
		t.Fatalf("last result = %+v", decision.State.LastResult)
	}
}

func TestDecideTransitionHumanGatedEdgeWaits(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = "review"

	decision, err := DecideTransition(def, state, completeResult("review"))
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if !decision.WaitingHuman || decision.StartNext || decision.NextNodeID != "human" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.State.Status != WorkspaceStatusWaitingHuman ||
		decision.State.HumanGate == nil {
		t.Fatalf("state = %+v", decision.State)
	}
	if decision.State.HumanGate.ReviewContextNodeID != "review" ||
		decision.State.HumanGate.ReviewArtifact != "thoughts/example.md" {
		t.Fatalf("human gate = %+v", decision.State.HumanGate)
	}
}

func TestDecideTransitionStopStatuses(t *testing.T) {
	def := transitionDefinition(t)
	for _, tt := range []struct {
		status          string
		workspaceStatus WorkspaceStatus
		nodeStatus      NodeStatus
		stopReason      string
	}{
		{status: "needs_human", workspaceStatus: WorkspaceStatusWaitingHuman, nodeStatus: NodeStatusPending, stopReason: "human"},
		{status: "blocked", workspaceStatus: WorkspaceStatusBlocked, nodeStatus: NodeStatusBlocked, stopReason: "blocked"},
		{status: "error", workspaceStatus: WorkspaceStatusError, nodeStatus: NodeStatusError, stopReason: "error"},
	} {
		t.Run(tt.status, func(t *testing.T) {
			state, err := InitialState(def, nil)
			if err != nil {
				t.Fatal(err)
			}
			result := completeResult("start")
			result.Status = ResultStatus(tt.status)
			decision, err := DecideTransition(def, state, result)
			if err != nil {
				t.Fatalf("DecideTransition() error = %v", err)
			}
			if decision.StartNext || decision.State.Status != tt.workspaceStatus ||
				!strings.Contains(decision.StopReason, tt.stopReason) {
				t.Fatalf("decision = %+v", decision)
			}
			if got := decision.State.Nodes["start"].Status; got != tt.nodeStatus {
				t.Fatalf("node status = %q, want %q", got, tt.nodeStatus)
			}
		})
	}
}

func TestDecideTransitionTerminalDone(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = "done"

	decision, err := DecideTransition(def, state, completeResult("done"))
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if decision.State.Status != WorkspaceStatusDone || decision.StartNext ||
		decision.StopReason != "terminal" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDecideTransitionResultSourceMismatch(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecideTransition(def, state, completeResult("review"))
	if err == nil || !strings.Contains(err.Error(), "does not match current node") {
		t.Fatalf("DecideTransition() error = %v, want mismatch", err)
	}
}

func TestDecideTransitionUnsupportedStatus(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := completeResult("start")
	result.Status = ResultStatus("weird")
	_, err = DecideTransition(def, state, result)
	if err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("DecideTransition() error = %v, want invalid status", err)
	}
}

func TestApplyBypassNodes(t *testing.T) {
	def := transitionDefinition(t)
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = "review"

	got, events := ApplyBypassNodes(def, state, "review", "human", "skip")
	if got.CurrentNodeID != "human" || got.PendingNextNodeID != "human" ||
		got.Status != WorkspaceStatusIdle {
		t.Fatalf("state = %+v", got)
	}
	if got.Nodes["review"].Status != NodeStatusBypassed ||
		got.Nodes["review"].BypassReason != "skip" {
		t.Fatalf("review node = %+v", got.Nodes["review"])
	}
	if len(events) != 1 || events[0].Type != "workflow_node_bypassed" ||
		events[0].NodeID != "review" {
		t.Fatalf("events = %+v", events)
	}
}
