package agentchat

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestRuntimeNextNodeLabelUsesPendingGateNotDisplayNext(t *testing.T) {
	def := wruntime.Definition{Nodes: map[wruntime.NodeID]wruntime.Node{
		qrspi.NodeHumanReviewOutline: {
			ID:          qrspi.NodeHumanReviewOutline,
			DisplayName: "Human Review Outline",
		},
		qrspi.NodePlan: {ID: qrspi.NodePlan, DisplayName: "Plan"},
	}}

	got := RuntimeNextNodeLabel(def, wruntime.State{
		PendingNextNodeID: qrspi.NodeHumanReviewOutline,
		LastResult: &wruntime.WorkflowResultSnapshot{
			DisplayNext: "/q-plan thoughts/example/outline.md",
		},
	})
	if got != "Human Review Outline" {
		t.Fatalf("RuntimeNextNodeLabel() = %q, want human gate label", got)
	}
}

func TestProjectQRSPIWorkflowCardUsesRuntimeProjection(t *testing.T) {
	policy := WorkspaceWorkflowPolicyProjection{
		AutoMode:          true,
		ModeLabel:         "Assisted: auto-continue safe gates",
		ReviewLabel:       "Planning reviews on",
		RetryLabel:        "Retries 1",
		EnablePlanReviews: true,
	}
	cwd := WorkspaceCwdProjection{
		Path:  "/tmp/implementation-copy",
		Label: "implementation-copy",
		Scope: "implementation_workspace",
	}
	card, err := ProjectQRSPIWorkflowCard(
		wruntime.State{Status: wruntime.WorkspaceStatusWaitingHuman},
		wruntime.WorkflowResult{
			SourceNodeID:    qrspi.NodeOutline,
			Status:          wruntime.StatusComplete,
			Outcome:         wruntime.OutcomeComplete,
			Summary:         "Outline complete.",
			PrimaryArtifact: "thoughts/example/outline.md",
			DisplayNext:     "/q-review thoughts/example/outline.md",
		},
		policy,
		cwd,
		"Human Review Outline",
		"workspace-1",
		"thread-1",
	)
	if err != nil {
		t.Fatalf("ProjectQRSPIWorkflowCard() error = %v", err)
	}
	if card == nil || card.RuntimeNextStep != "Human Review Outline" ||
		card.ThreadID != "thread-1" ||
		card.PrimaryArtifact != "thoughts/example/outline.md" ||
		!card.WaitingHuman || card.Policy.ModeLabel != policy.ModeLabel ||
		card.Cwd.Path != cwd.Path {
		t.Fatalf("card = %#v, want runtime-derived projection", card)
	}
}

func TestQRSPIWorkflowCardViewDoesNotRenderDisplayNextAsCommand(t *testing.T) {
	card := QRSPIWorkflowCard{
		ThreadID:        "thread-1",
		WorkspaceID:     "workspace-1",
		Stage:           string(qrspi.NodeOutline),
		Status:          string(wruntime.StatusComplete),
		Outcome:         string(wruntime.OutcomeComplete),
		Summary:         "Outline complete.",
		PrimaryArtifact: "thoughts/example/outline.md",
		RuntimeNextStep: "Human Review Outline",
		NextSteps:       []string{"Read qrspi-planning.", "Start q-review immediately."},
		AgentProgress: AgentProgress{
			State:         "waiting_human",
			CurrentNodeID: string(qrspi.NodeHumanReviewOutline),
		},
		JumpCurrentHref: "/thoughts/example?context=chat&thread=thread-1#msg-current",
		JumpNextEndHref: "/thoughts/example?context=chat&thread=thread-1#msg-next",
		RawResult:       "```yaml\nqrspi_result:\n  stage: outline\n```",
		Cwd: WorkspaceCwdProjection{
			Label: "planning",
			Scope: "planning_checkout",
		},
		Policy: WorkspaceWorkflowPolicyProjection{
			ModeLabel:   "Manual: stop at human gates",
			ReviewLabel: "Planning reviews on",
			RetryLabel:  "Retries 1",
		},
		WaitingHuman: true,
	}
	var body strings.Builder
	if err := QRSPIWorkflowCardView(
		card,
	).Render(context.Background(), &body); err != nil {
		t.Fatalf("render QRSPIWorkflowCardView() error = %v", err)
	}
	rendered := body.String()
	if !strings.Contains(rendered, "Human Review Outline") {
		t.Fatalf("rendered card missing runtime next label: %s", rendered)
	}
	if strings.Contains(rendered, "/q-review") {
		t.Fatalf("rendered card leaked display slash command: %s", rendered)
	}
	if !strings.Contains(rendered, "/agent-chat/thread/thread-1/workflow/advance") {
		t.Fatalf("rendered card missing thread proceed form: %s", rendered)
	}
	if strings.Contains(rendered, "/agent-chat/workspace-1/workflow/advance") {
		t.Fatalf("rendered card leaked workspace action URL: %s", rendered)
	}
	for _, want := range []string{
		"Next steps",
		"Start q-review immediately.",
		"Progress",
		"waiting_human · human-review-outline",
		"Jump to current agent position",
		"Jump to end of next step",
		"overflow-x-auto",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered card missing %q: %s", want, rendered)
		}
	}
}

func TestAttachQRSPIWorkflowCardToLatestAssistantMessage(t *testing.T) {
	card := &QRSPIWorkflowCard{Stage: string(qrspi.NodeOutline)}
	messages := attachQRSPIWorkflowCardToLatestAssistantMessage([]TranscriptMessage{
		{Role: "assistant", DOMID: "old"},
		{Role: "user", DOMID: "user"},
		{Role: "assistant", DOMID: "latest"},
	}, card)
	if messages[2].WorkflowCard != card {
		t.Fatalf("latest assistant message did not receive workflow card: %#v", messages)
	}
	if messages[0].WorkflowCard != nil {
		t.Fatalf("older assistant message unexpectedly received workflow card")
	}
}
