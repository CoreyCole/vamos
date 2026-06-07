package qrspi

import (
	"encoding/json"
	"strings"
	"testing"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type transitionParserStub struct{}

func (transitionParserStub) Parse(
	string,
	wruntime.ParseContext,
) (any, error) {
	return nil, nil
}

func (transitionParserStub) CorrectionPrompt(
	error,
	int,
) string {
	return ""
}

type transitionConverterStub struct{}

func (transitionConverterStub) ToWorkflowResult(
	any,
	wruntime.ParseContext,
) (wruntime.WorkflowResult, error) {
	return wruntime.WorkflowResult{}, nil
}

func TestQRSPITransitions(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, absent := range []wruntime.NodeID{"human-review-design", "review-design", "research-for-review-design", "address-review-research-design"} {
		if _, ok := def.Nodes[absent]; ok {
			t.Fatalf("%s should not be in canonical QRSPI graph", absent)
		}
	}
	state = assertStartsNext(t, def, state, NodeQuestion, NodeResearch)
	state = assertStartsNext(t, def, state, NodeResearch, NodeDesign)
	state = assertStartsNext(t, def, state, NodeDesign, NodeOutline)
	state = assertStartsNext(t, def, state, NodeOutline, NodeReviewOutline)
	state = assertStartsNext(t, def, state, NodeReviewOutline, NodePlan)
	state = assertStartsNext(t, def, state, NodePlan, NodeReviewPlan)
	state = assertStartsNext(t, def, state, NodeReviewPlan, NodeWorkspace)
	state = assertStartsNext(t, def, state, NodeWorkspace, NodeImplement)
	state = assertStartsNext(t, def, state, NodeImplement, NodeReviewImplementation)
	state = assertWaitsHuman(
		t,
		def,
		state,
		NodeReviewImplementation,
		NodeHumanReviewImplementation,
	)
	state = advanceHumanGate(state)
	state = assertStartsNext(t, def, state, NodeHumanReviewImplementation, NodeDone)
	state = assertTerminal(t, def, state, NodeDone)
}

func TestQRSPIPlanReviewsDisabledUsesConfigEdges(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := json.Marshal(
		Policy{AutoMode: false, EnablePlanReviews: false, InvalidResultRetryLimit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, policy)
	if err != nil {
		t.Fatal(err)
	}

	state = assertStartsNext(t, def, state, NodeQuestion, NodeResearch)
	state = assertStartsNext(t, def, state, NodeResearch, NodeDesign)
	state = assertStartsNext(t, def, state, NodeDesign, NodeOutline)
	state = assertStartsNext(t, def, state, NodeOutline, NodePlan)
	state = assertStartsNext(t, def, state, NodePlan, NodeWorkspace)
	state = assertStartsNext(t, def, state, NodeWorkspace, NodeImplement)
}

func TestQRSPIOutcomeTransitionTable(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = NodeReviewOutline

	result := qrspiResult(NodeReviewOutline, wruntime.StatusComplete)
	result.Outcome = wruntime.OutcomeNeedsReviewResearch
	decision, err := wruntime.DecideTransition(def, state, result)
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if !decision.StartNext || decision.NextNodeID != NodeResearchForReviewOutline {
		t.Fatalf("decision = %+v, want %s", decision, NodeResearchForReviewOutline)
	}
}

func TestQRSPIReviewOutlineReadyForPlanTransition(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = NodeReviewOutline

	result := qrspiResult(NodeReviewOutline, wruntime.StatusComplete)
	result.Outcome = wruntime.OutcomeReadyForPlan
	decision, err := wruntime.DecideTransition(def, state, result)
	if err != nil {
		t.Fatalf("DecideTransition() error = %v", err)
	}
	if !decision.StartNext || decision.WaitingHuman || decision.NextNodeID != NodePlan {
		t.Fatalf("decision = %+v, want direct plan start", decision)
	}
}

func TestQRSPITransitionOutcomesMustBeDeclared(t *testing.T) {
	_, err := wruntime.New[struct{}]("bad").
		Start("start").
		Agent("start", wruntime.PromptSpec{Static: "run"}).
		Statuses(wruntime.StatusComplete).
		Outcomes(wruntime.OutcomeComplete).
		Done("done").
		From("start").On(wruntime.OutcomeReadyForWorkspace).GoTo("done").
		ResultParser(transitionParserStub{}).
		ResultConverter(transitionConverterStub{}).
		Build()
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("Build() error = %v, want undeclared outcome", err)
	}
}

func TestQRSPIWorkflowRenderersExposeReviewBranches(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}

	mermaid := wruntime.RenderMermaid(def)
	for _, want := range []string{
		"design -- outcome=complete --> outline",
		"review-outline -- outcome=ready-for-plan --> plan",
		"review-plan -- outcome=ready-for-workspace --> workspace",
		"review-implementation -- outcome=needs-followup --> question",
	} {
		if !strings.Contains(mermaid, want) {
			t.Fatalf("RenderMermaid() = %q, want %q", mermaid, want)
		}
	}

	table := wruntime.RenderTransitionTable(def)
	for _, want := range []string{
		"| From | Condition | To |",
		"| design | outcome=complete | outline |",
		"| plan | predicate | review-plan |",
		"| workspace | outcome=complete | implement |",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("RenderTransitionTable() = %q, want %q", table, want)
		}
	}
}

func assertStartsNext(
	t *testing.T,
	def wruntime.Definition,
	state wruntime.State,
	from, to wruntime.NodeID,
) wruntime.State {
	t.Helper()
	decision, err := wruntime.DecideTransition(
		def,
		state,
		qrspiResult(from, wruntime.StatusComplete),
	)
	if err != nil {
		t.Fatalf("DecideTransition(%s) error = %v", from, err)
	}
	if !decision.StartNext || decision.WaitingHuman || decision.NextNodeID != to ||
		decision.State.CurrentNodeID != to {
		t.Fatalf("DecideTransition(%s) = %+v, want start %s", from, decision, to)
	}
	return decision.State
}

func assertWaitsHuman(
	t *testing.T,
	def wruntime.Definition,
	state wruntime.State,
	from, to wruntime.NodeID,
) wruntime.State {
	t.Helper()
	decision, err := wruntime.DecideTransition(
		def,
		state,
		qrspiResult(from, wruntime.StatusComplete),
	)
	if err != nil {
		t.Fatalf("DecideTransition(%s) error = %v", from, err)
	}
	if !decision.WaitingHuman || decision.StartNext || decision.NextNodeID != to ||
		decision.State.HumanGate == nil {
		t.Fatalf("DecideTransition(%s) = %+v, want human gate %s", from, decision, to)
	}
	return decision.State
}

func assertTerminal(
	t *testing.T,
	def wruntime.Definition,
	state wruntime.State,
	from wruntime.NodeID,
) wruntime.State {
	t.Helper()
	decision, err := wruntime.DecideTransition(
		def,
		state,
		qrspiResult(from, wruntime.StatusDone),
	)
	if err != nil {
		t.Fatalf("DecideTransition(%s) error = %v", from, err)
	}
	if decision.State.Status != wruntime.WorkspaceStatusDone || decision.StartNext {
		t.Fatalf("DecideTransition(%s) = %+v, want done", from, decision)
	}
	return decision.State
}

func advanceHumanGate(state wruntime.State) wruntime.State {
	state.CurrentNodeID = state.HumanGate.To
	state.PendingNextNodeID = state.HumanGate.To
	state.HumanGate = nil
	state.Status = wruntime.WorkspaceStatusIdle
	return state
}

func qrspiResult(
	node wruntime.NodeID,
	status wruntime.ResultStatus,
) wruntime.WorkflowResult {
	return wruntime.WorkflowResult{
		WorkflowType:    string(AgentChatWorkflowType),
		SourceNodeID:    node,
		Status:          status,
		Outcome:         defaultOutcome(node),
		Summary:         "completed " + string(node),
		PrimaryArtifact: "thoughts/example.md",
		Evidence:        wruntime.EvidenceRef{RunID: "run-" + string(node)},
	}
}

func defaultOutcome(node wruntime.NodeID) wruntime.ResultOutcome {
	switch node {
	case NodeReviewOutline:
		return wruntime.OutcomeReadyForPlan
	case NodeReviewImplementation:
		return wruntime.OutcomeReadyForHumanReview
	case NodeReviewPlan:
		return wruntime.OutcomeReadyForWorkspace
	default:
		return wruntime.OutcomeComplete
	}
}
