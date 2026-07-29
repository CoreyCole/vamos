package agentchat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpaqueSettlementSuccessorValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		value OpaqueSettlementSuccessor
		want  bool
	}{
		{"none", OpaqueSettlementSuccessor{Action: "none"}, true},
		{"start child", OpaqueSettlementSuccessor{Action: "start_child", Target: "child_1", Discovery: "manager inventory"}, true},
		{"steer", OpaqueSettlementSuccessor{Action: "steer_existing", Target: "thread_1", Discovery: "thread inventory"}, true},
		{"handoff", OpaqueSettlementSuccessor{Action: "handoff", Target: "handoff_1", Discovery: "artifact inventory"}, true},
		{"unknown", OpaqueSettlementSuccessor{Action: "execute"}, false},
		{"unsafe", OpaqueSettlementSuccessor{Action: "start_child", Target: "../child", Discovery: "inventory"}, false},
		{"missing discovery", OpaqueSettlementSuccessor{Action: "handoff", Target: "handoff"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if (validateOpaqueSettlementSuccessor(test.value) == nil) != test.want {
				t.Fatalf(
					"validation = %v, want %v",
					validateOpaqueSettlementSuccessor(test.value),
					test.want,
				)
			}
		})
	}
}

func TestOpaqueSettlementReceiveAndDecisionOnlyAppendTranscriptEvents(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "plans", "p")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		plan,
		HermesTranscriptEvent{
			ID:          "run",
			Type:        "pi_run",
			ThreadID:    "thread",
			PiSessionID: "session",
		},
	); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(
		opaqueSettlementEnvelope{
			Version:       1,
			Kind:          "opaque_pi_settlement",
			Plan:          "plans/p",
			ManagerThread: "thread",
			Session:       "session",
			FinalEntryID:  "entry",
			Fences:        []string{"a: b\n"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{thoughtsRoot: root}
	if err := s.ReceiveOpaqueSettlement(
		context.Background(),
		OpaqueSettlementDeliveryRequest{
			Version:               1,
			DeliveryID:            "delivery",
			Plan:                  "plans/p",
			ManagerThread:         "thread",
			Session:               "session",
			FinalEntryID:          "entry",
			SettlementBytesBase64: base64.StdEncoding.EncodeToString(raw),
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideOpaqueSettlementSuccessor(
		context.Background(),
		plan,
		"thread",
		"session",
		"entry",
		OpaqueSettlementSuccessor{
			Action:    "handoff",
			Target:    "handoff_1",
			Discovery: "artifact inventory",
		},
	); err != nil {
		t.Fatal(err)
	}
	events, err := readHermesTranscript(plan, "thread")
	if err != nil {
		t.Fatal(err)
	}
	if got := events[len(events)-1]; got.Type != opaqueSettlementDecisionEvent ||
		got.Successor == nil ||
		got.Successor.Action != "handoff" {
		t.Fatalf("decision event = %#v", got)
	}
	if got := events[1].Settlement; got == nil || got.RawResponse != string(raw) ||
		len(got.YAMLBlocks) != 1 {
		t.Fatalf("received evidence = %#v", got)
	}
}

func TestOpaqueSettlementCardIsEvidenceOnly(t *testing.T) {
	card := OpaqueSettlementCard(
		HermesTranscriptEvent{
			At: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			Settlement: &OpaqueSettlementEvidence{
				Plan:        "plans/p",
				Thread:      "thread",
				Session:     "session",
				Entry:       "entry",
				RawResponse: `{"fences":[]}`,
			},
		},
		"/record",
	)
	var out strings.Builder
	if err := card.Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"Raw response", "No fenced YAML blocks were present", "Select next action", "Recording this decision never executes it."} {
		if !strings.Contains(html, want) {
			t.Fatalf("card missing %q: %s", want, html)
		}
	}
}
