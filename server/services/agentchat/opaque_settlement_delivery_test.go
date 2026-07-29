package agentchat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

type recordingOpaqueReceiver struct {
	mu       sync.Mutex
	requests []OpaqueSettlementDeliveryRequest
	fail     int
}

type recordingOpaqueWorker struct{ workflows, activities []any }

type opaquePlanSourceFunc func(context.Context) ([]DiscoveredPlanWorkspace, error)

func (f opaquePlanSourceFunc) Scan(
	ctx context.Context,
) ([]DiscoveredPlanWorkspace, error) {
	return f(ctx)
}

func (w *recordingOpaqueWorker) RegisterWorkflow(
	v any,
) {
	w.workflows = append(w.workflows, v)
}

func (w *recordingOpaqueWorker) RegisterActivity(
	v any,
) {
	w.activities = append(w.activities, v)
}

func TestRegisterOpaqueSettlementDelivery(t *testing.T) {
	worker := &recordingOpaqueWorker{}
	activities := &OpaqueSettlementDeliveryActivities{}
	RegisterOpaqueSettlementDelivery(worker, activities)
	if len(worker.workflows) != 1 || len(worker.activities) != 1 ||
		worker.activities[0] != activities {
		t.Fatal("opaque settlement worker registration is incomplete")
	}
	if got := reflect.ValueOf(worker.workflows[0]).
		Pointer(); got != reflect.ValueOf(OpaqueSettlementDiscoveryWorkflow).
		Pointer() {
		t.Fatal("registered workflow is not OpaqueSettlementDiscoveryWorkflow")
	}
}

func (r *recordingOpaqueReceiver) DeliverOpaqueSettlement(
	_ context.Context,
	request OpaqueSettlementDeliveryRequest,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	if r.fail > 0 {
		r.fail--
		return errors.New("lost response")
	}
	return nil
}

func writeOpaqueFixture(
	t *testing.T,
	root, thread, session, entry string,
	piRuns int,
) (string, []byte) {
	t.Helper()
	plan := filepath.Join(root, "project", "plans", "plan")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(plan, "AGENTS.md"),
		[]byte("---\n---\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < piRuns; i++ {
		if err := AppendHermesTranscript(
			plan,
			HermesTranscriptEvent{
				ID:          string(rune('a' + i)),
				Type:        "pi_run",
				ThreadID:    thread,
				PiSessionID: session,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(
		opaqueSettlementEnvelope{
			Version:          1,
			Kind:             "pi_assistant_settlement",
			Session:          session,
			Plan:             "project/plans/plan",
			ManagerThread:    thread,
			AssistantEntryID: entry,
			SettledAt:        time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
			RawResponse:      "evidence",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		session,
		"settlements",
		entry+".json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return plan, body
}

func TestOpaqueSettlementDeliveryRetriesExactBytesAndProjectsDedup(t *testing.T) {
	root := t.TempDir()
	_, raw := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	receiver := &recordingOpaqueReceiver{fail: 1}
	a := &OpaqueSettlementDeliveryActivities{ThoughtsRoot: root, Receiver: receiver}
	if err := a.DeliverOpaqueSettlements(
		context.Background(),
		OpaqueSettlementDeliveryInput{},
	); err == nil {
		t.Fatal("first lost response unexpectedly succeeded")
	}
	if err := a.DeliverOpaqueSettlements(
		context.Background(),
		OpaqueSettlementDeliveryInput{},
	); err != nil {
		t.Fatal(err)
	}
	if err := a.DeliverOpaqueSettlements(
		context.Background(),
		OpaqueSettlementDeliveryInput{},
	); err != nil {
		t.Fatal(err)
	}
	if len(receiver.requests) != 2 {
		t.Fatalf("deliveries = %d, want 2", len(receiver.requests))
	}
	if receiver.requests[0] != receiver.requests[1] {
		t.Fatal("retry changed immutable gateway request")
	}
	bytesBase64 := receiver.requests[0].SettlementBytesBase64
	if bytesBase64 == "" {
		t.Fatal("missing exact bytes")
	}
	if stringMustDecode(t, bytesBase64) != string(raw) {
		t.Fatal("gateway request did not retain exact settlement bytes")
	}
}

func TestOpaqueSettlementDeliveryProjectionSerializesConcurrentDiscovery(t *testing.T) {
	root := t.TempDir()
	writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	receiver := &recordingOpaqueReceiver{}
	a := &OpaqueSettlementDeliveryActivities{ThoughtsRoot: root, Receiver: receiver}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.DeliverOpaqueSettlements(
				context.Background(),
				OpaqueSettlementDeliveryInput{},
			); err != nil {
				t.Errorf("deliver: %v", err)
			}
		}()
	}
	wg.Wait()
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	if len(receiver.requests) != 1 {
		t.Fatalf("concurrent receiver calls = %d, want 1", len(receiver.requests))
	}
	if receiver.requests[0].DeliveryID != "opaque-settlement:project/plans/plan:session:entry" {
		t.Fatalf("delivery id = %q", receiver.requests[0].DeliveryID)
	}
}

func TestOpaqueSettlementDeliveryRejectsForgedAndDuplicateBindings(t *testing.T) {
	for _, runs := range []int{0, 2} {
		t.Run("bindings", func(t *testing.T) {
			root := t.TempDir()
			writeOpaqueFixture(t, root, "thread", "session", "entry", runs)
			receiver := &recordingOpaqueReceiver{}
			err := (&OpaqueSettlementDeliveryActivities{ThoughtsRoot: root, Receiver: receiver}).DeliverOpaqueSettlements(
				context.Background(),
				OpaqueSettlementDeliveryInput{},
			)
			if err == nil {
				t.Fatal("forged binding accepted")
			}
			if len(receiver.requests) != 0 {
				t.Fatal("forged binding reached receiver")
			}
		})
	}
}

func TestOpaqueSettlementDeliveryRejectsForgedPathIdentity(t *testing.T) {
	root := t.TempDir()
	plan, raw := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	var envelope opaqueSettlementEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Session = "forged"
	forged, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		"session",
		"settlements",
		"entry.json",
	)
	if err := os.WriteFile(path, forged, 0o600); err != nil {
		t.Fatal(err)
	}
	receiver := &recordingOpaqueReceiver{}
	if err := (&OpaqueSettlementDeliveryActivities{ThoughtsRoot: root, Receiver: receiver}).DeliverOpaqueSettlements(
		context.Background(),
		OpaqueSettlementDeliveryInput{},
	); err == nil {
		t.Fatal("forged path identity accepted")
	}
	if len(receiver.requests) != 0 {
		t.Fatal("forged identity reached receiver")
	}
}

func TestOpaqueSettlementDiscoveryUsesOnlyBoundedProjectedPlans(t *testing.T) {
	root := t.TempDir()
	planA := filepath.Join(root, "z")
	planB := filepath.Join(root, "a")
	for _, dir := range []string{planA, planB} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	plans := make([]DiscoveredPlanWorkspace, opaqueSettlementScanLimit+1)
	for i := range plans {
		dir := filepath.Join(root, "capped", string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		plans[i].PlanDir = dir
	}
	a := &OpaqueSettlementDeliveryActivities{
		PlanSource: opaquePlanSourceFunc(
			func(context.Context) ([]DiscoveredPlanWorkspace, error) {
				return plans, nil
			},
		),
	}
	if _, err := a.planDirectories(context.Background(), root); err == nil {
		t.Fatal("plan source exceeding cap accepted")
	}
	a.PlanSource = opaquePlanSourceFunc(
		func(context.Context) ([]DiscoveredPlanWorkspace, error) {
			return []DiscoveredPlanWorkspace{
				{PlanDir: planA},
				{PlanDir: planB},
				{PlanDir: planA},
			}, nil
		},
	)
	got, err := a.planDirectories(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "a" || filepath.Base(got[1]) != "z" {
		t.Fatalf("projected plans = %v, want sorted, deduplicated contained plans", got)
	}
}

func TestOpaqueSettlementDiscoveryRejectsSymlinkEscapes(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "plan")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	a := &OpaqueSettlementDeliveryActivities{
		PlanSource: opaquePlanSourceFunc(
			func(context.Context) ([]DiscoveredPlanWorkspace, error) {
				return []DiscoveredPlanWorkspace{{PlanDir: link}}, nil
			},
		),
	}
	if _, err := a.planDirectories(context.Background(), root); err == nil {
		t.Fatal("symlinked projected plan outside root accepted")
	}
}

func TestOpaqueSettlementPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	plan, _ := writeOpaqueFixture(t, root, "thread", "session", "entry", 1)
	outside := filepath.Join(t.TempDir(), "entry.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		"session",
		"settlements",
		"entry.json",
	)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := validateOpaqueSettlementPath(plan, path); err == nil {
		t.Fatal("settlement symlink escape accepted")
	}
}

func TestOpaqueSettlementScheduleIdentityIsStable(t *testing.T) {
	if got, want := OpaqueSettlementDeliveryScheduleID(
		"configured/root",
	), "opaque-settlement-discovery:configured-root"; got != want {
		t.Fatalf("schedule ID = %q, want %q", got, want)
	}
	if OpaqueSettlementDeliveryScheduleID(
		"/a/b",
	) == OpaqueSettlementDeliveryScheduleID(
		"/a/c",
	) {
		t.Fatal("schedule identity is not configured-root-scoped")
	}
}

func stringMustDecode(t *testing.T, value string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
