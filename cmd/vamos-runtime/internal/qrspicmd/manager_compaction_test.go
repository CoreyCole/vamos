package qrspicmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestManagerUsagePercent(t *testing.T) {
	percent := 81.5
	if got, ok := managerUsagePercent(
		ManagerUsageInput{UsagePercent: &percent},
	); !ok ||
		got != percent {
		t.Fatalf("usage percent = %v/%v", got, ok)
	}
	tokens, window := 82, 100
	if got, ok := managerUsagePercent(
		ManagerUsageInput{Tokens: &tokens, Window: &window},
	); !ok ||
		got != 82 {
		t.Fatalf("token usage percent = %v/%v", got, ok)
	}
	if _, ok := managerUsagePercent(ManagerUsageInput{}); ok {
		t.Fatal("empty usage returned ok")
	}
}

func TestManagerCompactionSkipsWithoutUsage(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	state := ManagerState{
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
	}
	var out bytes.Buffer
	updated, status, err := maybeStartManagerCompaction(
		t.Context(),
		state,
		stateFile,
		ManagerUsageInput{},
		deps{},
		&out,
	)
	if err != nil {
		t.Fatalf("maybeStartManagerCompaction error = %v", err)
	}
	if status.Started || updated.Delivery.Status == "compacting" {
		t.Fatalf("status = %+v state = %+v", status, updated.Delivery)
	}
	if !strings.Contains(out.String(), "skipped; no explicit usage input") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestManagerCompactionThresholdWritesHandoffAndMarksCompacting(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	usage := 90.0
	state := ManagerState{
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerRunID:     "run-1",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:         "child-1",
			Stage:      "outline",
			TmuxPaneID: "%child",
			SessionID:  "session-1",
		},
	}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	updated, status, err := maybeStartManagerCompaction(
		t.Context(),
		state,
		stateFile,
		ManagerUsageInput{UsagePercent: &usage},
		deps{Clock: fixture.clock},
		&out,
	)
	if err != nil {
		t.Fatalf("maybeStartManagerCompaction error = %v", err)
	}
	if !status.Started || updated.Delivery.Status != "compacting" ||
		updated.Delivery.ManagerPaneID != "%parent" {
		t.Fatalf("status = %+v delivery = %+v", status, updated.Delivery)
	}
	text := out.String()
	if !strings.Contains(text, "handoff written") ||
		!strings.Contains(text, "manager-ready --state-file") {
		t.Fatalf("output = %q", text)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ID != "child-1" ||
		loaded.Delivery.Status != "compacting" {
		t.Fatalf("loaded = %+v", loaded)
	}
	matches, err := filepath.Glob(
		filepath.Join(
			fixture.projectRoot,
			fixture.planDir,
			"handoffs",
			"*_q-manager-operational-handoff.md",
		),
	)
	if err != nil || len(matches) != 1 {
		t.Fatalf("handoff matches = %#v err=%v", matches, err)
	}
	handoff := readText(t, matches[0])
	for _, want := range []string{"State file:", "Active child:", "manager-ready --state-file", "delivery marked compacting"} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("handoff missing %q:\n%s", want, handoff)
		}
	}
}

func TestManagerCompactionSkipsBelowNinetyAndPersistsUsageDiagnostic(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	usage := 89.9
	state := ManagerState{
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
	}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	updated, status, err := maybeStartManagerCompaction(
		t.Context(),
		state,
		stateFile,
		ManagerUsageInput{UsagePercent: &usage, Source: "pi-extension-context"},
		deps{Clock: fixture.clock},
		&out,
	)
	if err != nil {
		t.Fatalf("maybeStartManagerCompaction error = %v", err)
	}
	if status.Started || updated.Delivery.Status == "compacting" {
		t.Fatalf("status = %+v delivery = %+v", status, updated.Delivery)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastManagerUsage == nil || loaded.LastManagerUsage.Percent == nil ||
		*loaded.LastManagerUsage.Percent != 89.9 ||
		loaded.LastManagerUsage.Source != "pi-extension-context" {
		t.Fatalf("last usage = %+v", loaded.LastManagerUsage)
	}
	if !strings.Contains(out.String(), "usage 89.9% < 90%") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestManagerCompactionStartedSignalIsStableAndQueueSafe(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	usage := 90.0
	state := ManagerState{
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:         "child-1",
			Stage:      "outline",
			TmuxPaneID: "%child",
			SessionID:  "session-1",
		},
	}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	updated, status, err := maybeStartManagerCompaction(
		t.Context(),
		state,
		stateFile,
		ManagerUsageInput{UsagePercent: &usage, Source: "pi-extension-context"},
		deps{Clock: fixture.clock},
		&out,
	)
	if err != nil {
		t.Fatalf("maybeStartManagerCompaction error = %v", err)
	}
	if !status.Started || status.HandoffPath == "" || status.ReadyCommand == "" {
		t.Fatalf("status = %+v", status)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.Status != "compacting" || loaded.LastManagerUsage == nil {
		t.Fatalf("loaded = %+v", loaded)
	}
	if updated.Delivery.Status != "compacting" {
		t.Fatalf("updated delivery = %+v", updated.Delivery)
	}
	text := out.String()
	for _, want := range []string{"q-manager-parent-compact: started", "handoff:", "ready: vamos qrspi manager-ready --state-file"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestManagerCompactionQueuesAndFlushesWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	usage := 90.0
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		SourceCwd:        dir,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      1,
			LifecycleStatus: "completed",
		},
	}
	saveManagerState(t, stateFile, state)
	updated, compactionStatus, err := maybeStartManagerCompaction(
		t.Context(),
		state,
		stateFile,
		ManagerUsageInput{UsagePercent: &usage},
		deps{},
		&bytes.Buffer{},
	)
	if err != nil || !compactionStatus.Started {
		t.Fatalf("compaction status = %+v err=%v", compactionStatus, err)
	}
	status := ChildCompletionStatus{
		Validated:  true,
		ChildID:    "child-1",
		DeliveryID: "child-1:1:outline:complete:complete:artifact",
		Result: ChildCompletionResult{
			Stage:    "outline",
			Status:   "complete",
			Outcome:  "complete",
			Artifact: "artifact",
		},
	}
	tmux := &recordingTmux{}
	queued, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		updated,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatalf("queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "queue" || queued.Delivery.QueuedWake == nil ||
		len(tmux.pastes) != 0 {
		t.Fatalf("wake=%+v delivery=%+v pastes=%#v", wake, queued.Delivery, tmux.pastes)
	}
	saveManagerState(t, stateFile, queued)
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%new"},
		deps{Tmux: tmux},
		&bytes.Buffer{},
	); err != nil {
		t.Fatalf("RunManagerReady error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil ||
		loaded.Delivery.LastDeliveryID != status.DeliveryID ||
		len(tmux.pastes) != 1 {
		t.Fatalf("loaded=%+v pastes=%#v", loaded.Delivery, tmux.pastes)
	}
}
