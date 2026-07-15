package qrspi

import (
	"strings"
	"testing"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestQRSPIProjectPlanningTransitions(t *testing.T) {
	def, err := ProjectPlanningDefinition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}

	state = assertStartsNext(t, def, state, NodeMilestoneQuestion, NodeMilestoneResearch)
	state = assertStartsNext(t, def, state, NodeMilestoneResearch, NodeMilestoneDesign)
	state = assertStartsNext(
		t,
		def,
		state,
		NodeMilestoneDesign,
		NodeMilestoneCreateTickets,
	)
	state = assertStartsNext(t, def, state, NodeMilestoneCreateTickets, NodeDone)
	state = assertTerminal(t, def, state, NodeDone)
}

func TestQRSPIProjectPlanningWorkflowRenderers(t *testing.T) {
	def, err := ProjectPlanningDefinition()
	if err != nil {
		t.Fatal(err)
	}

	mermaid := wruntime.RenderMermaid(def)
	for _, want := range []string{
		"milestone-question -- outcome=complete --> milestone-research",
		"milestone-research -- outcome=complete --> milestone-design",
		"milestone-design -- outcome=complete --> milestone-create-tickets",
		"milestone-create-tickets -- outcome=complete --> done",
	} {
		if !strings.Contains(mermaid, want) {
			t.Fatalf("RenderMermaid() = %q, want %q", mermaid, want)
		}
	}
	for _, forbidden := range []string{"milestone-outline", "milestone-plan"} {
		if strings.Contains(mermaid, forbidden) {
			t.Fatalf("RenderMermaid() = %q, should not contain %q", mermaid, forbidden)
		}
	}

	table := wruntime.RenderTransitionTable(def)
	for _, want := range []string{
		"| milestone-question | outcome=complete | milestone-research |",
		"| milestone-research | outcome=complete | milestone-design |",
		"| milestone-design | outcome=complete | milestone-create-tickets |",
	} {
		if !strings.Contains(table, want) {
			t.Fatalf("RenderTransitionTable() = %q, want %q", table, want)
		}
	}
	for _, forbidden := range []string{"milestone-outline", "milestone-plan"} {
		if strings.Contains(table, forbidden) {
			t.Fatalf(
				"RenderTransitionTable() = %q, should not contain %q",
				table,
				forbidden,
			)
		}
	}
}
