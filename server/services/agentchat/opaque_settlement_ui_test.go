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

func TestReceiveOpaqueSettlementDerivesDeliveryIDForDedup(t *testing.T) {
	root := t.TempDir()
	plan, raw := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	admissions := newOpaqueAdmissionStore()
	s := &Service{thoughtsRoot: root, opaqueSettlementAdmissions: admissions}
	request := OpaqueSettlementDeliveryRequest{
		Version: opaqueSettlementDeliveryVersion,
		DeliveryID: opaqueSettlementDeliveryID(
			"project/plans/plan",
			"session",
			"entry",
		),
		Plan:                  "project/plans/plan",
		ManagerThread:         "thread",
		Session:               "session",
		FinalEntryID:          "entry",
		SettlementBytesBase64: base64.StdEncoding.EncodeToString(raw),
	}
	if err := admissions.Admit(context.Background(), request, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.ReceiveOpaqueSettlement(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := s.ReceiveOpaqueSettlement(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	events, err := readHermesTranscript(plan, "thread")
	if err != nil {
		t.Fatal(err)
	}
	received := 0
	for _, event := range events {
		if event.Type == opaqueSettlementReceivedEvent {
			received++
		}
	}
	if received != 1 {
		t.Fatalf("received events = %d, want 1", received)
	}
	request.DeliveryID = "caller-chosen-id"
	if err := s.ReceiveOpaqueSettlement(context.Background(), request); err == nil {
		t.Fatal("accepted caller-chosen alternate delivery ID")
	}
}

func TestReceiveOpaqueSettlementRejectsDifferentValidBytesForIdentity(t *testing.T) {
	root := t.TempDir()
	_, raw := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	admissions := newOpaqueAdmissionStore()
	s := &Service{thoughtsRoot: root, opaqueSettlementAdmissions: admissions}
	request := OpaqueSettlementDeliveryRequest{
		Version: opaqueSettlementDeliveryVersion,
		DeliveryID: opaqueSettlementDeliveryID(
			"project/plans/plan",
			"session",
			"entry",
		),
		Plan:                  "project/plans/plan",
		ManagerThread:         "thread",
		Session:               "session",
		FinalEntryID:          "entry",
		SettlementBytesBase64: base64.StdEncoding.EncodeToString(raw),
	}
	if err := admissions.Admit(context.Background(), request, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.ReceiveOpaqueSettlement(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var envelope opaqueSettlementEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.RawResponse = "different but valid evidence"
	conflict, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	request.SettlementBytesBase64 = base64.StdEncoding.EncodeToString(conflict)
	if err := s.ReceiveOpaqueSettlement(context.Background(), request); err == nil {
		t.Fatal("accepted different valid envelope bytes for immutable identity")
	}
}

func TestOpaqueSettlementDecisionRejectsUnboundSettlement(t *testing.T) {
	for _, runs := range []int{0, 2} {
		t.Run("binding", func(t *testing.T) {
			root := t.TempDir()
			plan, _ := writeOpaqueFixture(t, root, "thread", "session", "entry", runs)
			err := (&Service{thoughtsRoot: root}).DecideOpaqueSettlementSuccessor(
				context.Background(),
				plan,
				"thread",
				"session",
				"entry",
				OpaqueSettlementSuccessor{
					Action:     "handoff",
					Discovery:  opaqueSettlementDiscoveryReference("session", "entry"),
					Rationale:  "human review",
					Actor:      "manager@example.com",
					Provenance: "test",
				},
			)
			if err == nil {
				t.Fatal("accepted settlement without exact pi_run binding")
			}
		})
	}
}

func TestOpaqueSettlementTargetsRequireBoundSchema(t *testing.T) {
	root := t.TempDir()
	plan, _ := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	launchPath := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"hermes",
		"launches",
		"launch.json",
	)
	if err := os.MkdirAll(filepath.Dir(launchPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeLaunch := func(value any) {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(launchPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	valid := opaqueSettlementLaunchArtifact{
		Version:       1,
		Kind:          "pi_child_launch",
		LaunchID:      "launch",
		Plan:          "project/plans/plan",
		ManagerThread: "thread",
	}
	writeLaunch(valid)
	start := OpaqueSettlementSuccessor{Action: "start_child", Target: "launch"}
	if err := validateOpaqueSettlementTarget(
		plan,
		"thread",
		"project/plans/plan",
		start,
	); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{"not json", opaqueSettlementLaunchArtifact{Version: 1, Kind: "pi_child_launch", LaunchID: "other", Plan: "project/plans/plan", ManagerThread: "thread"}, opaqueSettlementLaunchArtifact{Version: 1, Kind: "pi_child_launch", LaunchID: "launch", Plan: "other", ManagerThread: "thread"}, opaqueSettlementLaunchArtifact{Version: 1, Kind: "pi_child_launch", LaunchID: "launch", Plan: "project/plans/plan", ManagerThread: "other-thread"}} {
		if text, ok := value.(string); ok {
			if err := os.WriteFile(launchPath, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		} else {
			writeLaunch(value)
		}
		if err := validateOpaqueSettlementTarget(
			plan,
			"thread",
			"project/plans/plan",
			start,
		); err == nil {
			t.Fatal("accepted malformed or mismatched launch artifact")
		}
	}
	steer := OpaqueSettlementSuccessor{Action: "steer_existing", Target: "session"}
	if err := validateOpaqueSettlementTarget(
		plan,
		"thread",
		"project/plans/plan",
		steer,
	); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		plan,
		HermesTranscriptEvent{
			ID:          "duplicate",
			Type:        "pi_run",
			ThreadID:    "thread",
			PiSessionID: "session",
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := validateOpaqueSettlementTarget(
		plan,
		"thread",
		"project/plans/plan",
		steer,
	); err == nil {
		t.Fatal("accepted ambiguously bound steer target")
	}
}

func TestOpaqueSettlementDecisionRequiresDiscoveryAdmission(t *testing.T) {
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
	if err := AppendHermesTranscript(plan, HermesTranscriptEvent{
		ID: "pi-run", Type: "pi_run", ThreadID: "thread", PiSessionID: "session",
	}); err != nil {
		t.Fatal(err)
	}
	admissions := newOpaqueAdmissionStore()
	s := &Service{thoughtsRoot: root, opaqueSettlementAdmissions: admissions}
	forgedAdmission := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		"session",
		"discovery-projections",
		"entry.json",
	)
	if err := os.MkdirAll(filepath.Dir(forgedAdmission), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		forgedAdmission,
		[]byte(`{"forged":true}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
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
	); err == nil {
		t.Fatal("accepted path-addressable settlement without discovery admission")
	}
	activity := &OpaqueSettlementDeliveryActivities{
		ThoughtsRoot: root,
		Admissions:   admissions,
		PlanSource: opaquePlanSourceFunc(
			func(context.Context) ([]DiscoveredPlanWorkspace, error) {
				return []DiscoveredPlanWorkspace{{PlanDir: plan}}, nil
			},
		),
		Receiver: &recordingOpaqueReceiver{fail: 1},
	}
	if err := activity.DeliverOpaqueSettlements(
		context.Background(), OpaqueSettlementDeliveryInput{},
	); err == nil {
		t.Fatal("lost delivery unexpectedly succeeded")
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
