package agentchat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpaqueSettlementCardRendersEvidenceNotEnvelope(t *testing.T) {
	card := OpaqueSettlementCard(
		HermesTranscriptEvent{
			At: time.Now(),
			Settlement: &OpaqueSettlementEvidence{
				Plan:        "plans/p",
				Thread:      "thread",
				Session:     "session",
				Entry:       "entry",
				RawResponse: "```yaml\nexact: evidence\n```\n",
				YAMLBlocks: []opaqueSettlementFence{
					{Language: "yaml", Raw: "```yaml\nexact: evidence\n```\n"},
				},
			},
		},
		"/record",
	)
	var out strings.Builder
	if err := card.Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "exact: evidence") ||
		strings.Contains(out.String(), "raw_response") {
		t.Fatalf("card did not render evidence: %s", out.String())
	}
}

func TestOpaqueSettlementDecisionUsesContainedEvidenceWithoutDelivery(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "plans", "p")
	path := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		"session",
		"settlements",
		"entry.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(
		opaqueSettlementEnvelope{
			Version:          1,
			Kind:             "pi_assistant_settlement",
			Session:          "session",
			Plan:             "plans/p",
			ManagerThread:    "thread",
			AssistantEntryID: "entry",
			SettledAt:        time.Now().UTC(),
			RawResponse:      "raw",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Service{thoughtsRoot: root}
	successor := OpaqueSettlementSuccessor{
		Action:     "handoff",
		Discovery:  opaqueSettlementDiscoveryReference("session", "entry"),
		Rationale:  "human review",
		Actor:      "manager@example.com",
		Provenance: "test",
	}
	if err := s.DecideOpaqueSettlementSuccessor(
		context.Background(),
		plan,
		"thread",
		"session",
		"entry",
		successor,
	); err != nil {
		t.Fatal(err)
	}
	events, err := readHermesTranscript(plan, "thread")
	if err != nil {
		t.Fatal(err)
	}
	got := events[len(events)-1]
	if got.Type != opaqueSettlementDecisionEvent || got.Successor == nil ||
		got.Successor.Actor != successor.Actor ||
		got.At.IsZero() {
		t.Fatalf("decision audit record = %#v", got)
	}
	for _, action := range []string{"none", "handoff"} {
		if err := validateOpaqueSettlementSuccessor(
			OpaqueSettlementSuccessor{Action: action, Target: "forbidden"},
		); err == nil {
			t.Fatalf("%s accepted target", action)
		}
	}
}
