package qrspicmd

import (
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestChildIntentClassifiesCurrentEvidence(t *testing.T) {
	valid := ParsedDecision{}
	tests := []struct {
		name      string
		evidence  ChildEvidence
		want      ChildIntentKind
		retryable bool
		manager   bool
	}{
		{
			name: "graph valid result",
			evidence: ChildEvidence{
				CurrentResult: &ResultEvidence{Parsed: &valid, ExplicitCompletion: true},
			},
			want: ChildIntentGraphValidResult,
		},
		{
			name: "interactive malformed result",
			evidence: ChildEvidence{
				Interaction: ChildInteractionInteractiveChat,
				CurrentResult: &ResultEvidence{
					ExplicitCompletion: true,
					ParseError:         "bad result",
				},
			},
			want: ChildIntentInteractiveChat,
		},
		{
			name: "repairable result",
			evidence: ChildEvidence{
				Interaction: ChildInteractionStageWork,
				CurrentResult: &ResultEvidence{
					ExplicitCompletion: true,
					ParseError:         "bad result",
				},
			},
			want:      ChildIntentRepairableResult,
			retryable: true,
		},
		{
			name: "structured pivot",
			evidence: ChildEvidence{
				Interaction: ChildInteractionStageWork,
				CurrentManagerRequest: &ChildManagerRequest{
					Kind:          "pivot",
					RequestedNode: qrspi.NodeQuestion,
					Reason:        "bug found",
				},
			},
			want:    ChildIntentPivotRequest,
			manager: true,
		},
		{
			name: "natural pivot",
			evidence: ChildEvidence{
				Interaction:    ChildInteractionStageWork,
				CurrentMessage: SessionMessageEvidence{Text: "Need follow-up research for this bug."},
			},
			want:    ChildIntentPivotRequest,
			manager: true,
		},
		{
			name: "manager question",
			evidence: ChildEvidence{
				Interaction:    ChildInteractionStageWork,
				CurrentMessage: SessionMessageEvidence{Text: "Which artifact should I inspect?"},
			},
			want:    ChildIntentManagerQuestion,
			manager: true,
		},
		{
			name: "provider failure",
			evidence: ChildEvidence{
				Interaction:     ChildInteractionStageWork,
				CurrentTerminal: &AssistantTerminalEvidence{StopReason: "error", ErrorMessage: "provider unavailable"},
			},
			want:    ChildIntentProviderFailure,
			manager: true,
		},
		{
			name: "no result",
			evidence: ChildEvidence{
				Interaction: ChildInteractionStageWork,
			},
			want:    ChildIntentNoResultIncomplete,
			manager: true,
		},
		{
			name: "ambiguous prose",
			evidence: ChildEvidence{
				Interaction:    ChildInteractionStageWork,
				CurrentMessage: SessionMessageEvidence{Text: "I changed several things."},
			},
			want:    ChildIntentAmbiguousUnsafe,
			manager: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyChildIntent(tt.evidence)
			if got.Kind != tt.want || got.Retryable != tt.retryable || got.ManagerNeeded != tt.manager {
				t.Fatalf("ClassifyChildIntent() = %+v", got)
			}
		})
	}
}

func TestGatherChildEvidenceUsesCurrentActiveBranchMessage(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	sessionPath := writePiSession(
		t,
		filepath.Join(dir, "sessions"),
		"session.jsonl",
		"session-1",
		repo,
		assistantLineWithIDs("root", "", "starting"),
		assistantLineWithIDs(
			"abandoned",
			"root",
			testResultYAML(
				"review-outline",
				"complete",
				"complete",
				"thoughts/example/review.md",
				"",
			),
		),
		assistantLineWithIDs("active", "root", "Which artifact should I inspect?"),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:          "child-1",
			Stage:       "review-outline",
			Cwd:         repo,
			SessionID:   "session-1",
			SessionPath: sessionPath,
			Generation:  1,
		},
	}
	evidence, err := GatherChildEvidence(state, ChildCompletionOptions{
		Boundary:    ChildBoundaryAgentSettled,
		Interaction: ChildInteractionStageWork,
	})
	if err != nil {
		t.Fatalf("GatherChildEvidence() error = %v", err)
	}
	if evidence.CurrentMessage.MessageID != "active" ||
		evidence.CurrentResult != nil || evidence.LatestGraphValidResult != nil {
		t.Fatalf("evidence = %+v", evidence)
	}
	if got := ClassifyChildIntent(evidence); got.Kind != ChildIntentManagerQuestion {
		t.Fatalf("intent = %+v", got)
	}
}

func TestParseChildManagerRequestStrictEnvelope(t *testing.T) {
	request, err := ParseChildManagerRequest("" +
		"```yaml\n" +
		"q_manager_request:\n" +
		"  kind: pivot\n" +
		"  requested_node: research\n" +
		"  plan_dir: thoughts/example/reviews/followup\n" +
		"  reason: verification found a bug\n" +
		"```\n")
	if err != nil {
		t.Fatalf("ParseChildManagerRequest() error = %v", err)
	}
	if request == nil || request.RequestedNode != qrspi.NodeResearch {
		t.Fatalf("request = %+v", request)
	}

	_, err = ParseChildManagerRequest("```yaml\nq_manager_request:\n  kind: pivot\n  requested_node: research\n  unknown: true\n```")
	if err == nil {
		t.Fatal("expected strict unknown-field error")
	}
}
