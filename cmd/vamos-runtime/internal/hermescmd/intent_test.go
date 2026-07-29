package hermescmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractPiIntent(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		outcome        Outcome
		recommendation string
		summary        string
		artifacts      []string
		metadata       map[string]any
		diagnostic     string
	}{
		{
			name:           "fenced aliases preserve presentation and metadata",
			response:       "Finished the implementation.\n```yaml\nstate: handoff\nnext: implement\nexplanation: Ready for the next slice\nartifacts:\n  - thoughts/me/plan.md\nowner_note: keep this\n```",
			outcome:        OutcomeHandoff,
			recommendation: "implement",
			summary:        "Ready for the next slice",
			artifacts: []string{
				"thoughts/me/plan.md",
			},
			metadata: map[string]any{"owner_note": "keep this"},
		},
		{
			name:     "nested result form",
			response: "result:\n  decision: complete\nsummary: done\n",
			outcome:  OutcomeComplete, summary: "done",
		},
		{
			name:     "scalar result alias",
			response: "result: needs_human\nartifact: thoughts/me/question.md\n",
			outcome:  OutcomeNeedsHuman, artifacts: []string{"thoughts/me/question.md"},
		},
		{
			name:     "unfenced compact yaml after prose",
			response: "The child has finished its analysis.\noutcome: complete\nsummary: concise conclusion\n",
			outcome:  OutcomeComplete,
			summary:  "concise conclusion",
		},
		{
			name:     "prose never infers lifecycle",
			response: "I completed the work and recommend review.",
			outcome:  OutcomeBlocked, diagnostic: "missing lifecycle value",
		},
		{
			name:       "unknown lifecycle blocks",
			response:   "outcome: ship_it\ncustom: retained\n",
			outcome:    OutcomeBlocked,
			metadata:   map[string]any{"custom": "retained"},
			diagnostic: "unknown lifecycle value",
		},
		{
			name:       "duplicate aliases block even when equal",
			response:   "outcome: complete\nstatus: complete\n",
			outcome:    OutcomeBlocked,
			diagnostic: "duplicate or conflicting lifecycle values",
		},
		{
			name:       "conflicting lifecycle blocks",
			response:   "outcome: complete\ndecision: blocked\n",
			outcome:    OutcomeBlocked,
			diagnostic: "duplicate or conflicting lifecycle values",
		},
		{
			name:     "malformed lifecycle blocks and preserves yaml",
			response: "```yaml\noutcome: [complete\nunknown: retained\n```",
			outcome:  OutcomeBlocked, diagnostic: "malformed lifecycle YAML",
		},
		{
			name:     "non scalar lifecycle blocks",
			response: "outcome:\n  value: complete\n",
			outcome:  OutcomeBlocked, diagnostic: "malformed lifecycle value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := ExtractPiIntent(test.response)
			if intent.Outcome != test.outcome {
				t.Fatalf("Outcome = %q, want %q", intent.Outcome, test.outcome)
			}
			if intent.Next != NextNone {
				t.Fatalf("Next = %q, want canonical %q", intent.Next, NextNone)
			}
			if intent.RawResponse != test.response {
				t.Fatalf("RawResponse = %q, want response", intent.RawResponse)
			}
			if test.diagnostic != "" &&
				!strings.Contains(
					strings.Join(intent.Diagnostics, "; "),
					test.diagnostic,
				) {
				t.Fatalf("Diagnostics = %v, want %q", intent.Diagnostics, test.diagnostic)
			}
			if intent.Recommendation != test.recommendation ||
				intent.Summary != test.summary {
				t.Fatalf("intent = %+v", intent)
			}
			if !reflect.DeepEqual(intent.Artifacts, test.artifacts) {
				t.Fatalf("Artifacts = %#v, want %#v", intent.Artifacts, test.artifacts)
			}
			if !reflect.DeepEqual(intent.Metadata, test.metadata) {
				t.Fatalf("Metadata = %#v, want %#v", intent.Metadata, test.metadata)
			}
			if strings.Contains(test.response, ":") && intent.RawYAML == "" &&
				test.name != "prose never infers lifecycle" {
				t.Fatal("RawYAML was not retained")
			}
		})
	}
}

func TestExtractPiIntentRecommendationNeverAdvancesManagedCheckpoint(t *testing.T) {
	for _, next := range []NextAction{
		NextNone, NextQuestion, NextResearch, NextDesign, NextOutline, NextPlan,
		NextWorkspace, NextImplement, NextReview, NextVerify, NextMilestoneQuestion,
		NextMilestoneResearch, NextMilestoneDesign, NextMilestoneCreateTickets,
	} {
		t.Run(string(next), func(t *testing.T) {
			intent := ExtractPiIntent("outcome: handoff\nnext: " + string(next))
			if intent.Recommendation != string(next) {
				t.Fatalf("Recommendation = %q, want %q", intent.Recommendation, next)
			}
			checkpoint := PiCheckpoint{Next: NextReview}
			ApplyPiIntent(&checkpoint, intent)
			if checkpoint.Next != NextNone {
				t.Fatalf("managed checkpoint advanced to %q", checkpoint.Next)
			}
			if checkpoint.Outcome != OutcomeHandoff {
				t.Fatalf("Outcome = %q", checkpoint.Outcome)
			}
		})
	}
}
