package qrspicmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestRepairStateAlignsActiveChildAndLogsActionCard(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild:      &ChildRunRef{ID: "child-1", Stage: string(qrspi.NodeImplement), SessionPath: filepath.Join(dir, "session.jsonl")},
	}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	if err := RunRepairState(t.Context(), RepairStateOptions{StateFile: stateFile, AlignActiveChild: true}, deps{}, &out); err != nil {
		t.Fatalf("RunRepairState error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeImplement || loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionStateDesync {
		t.Fatalf("loaded = %+v", loaded)
	}
	if !strings.Contains(out.String(), "repair-state --state-file") || !strings.Contains(out.String(), "continue --state-file") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := filepath.Glob(filepath.Join(dir, "validation-recoveries.jsonl")); err != nil {
		t.Fatalf("recovery log glob error = %v", err)
	}
}

func TestMarkChildActiveSupersedesQueuedWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		Delivery:    ManagerDeliveryState{QueuedWake: &QueuedWake{DeliveryID: "old", ChildID: "child-1", ChildGeneration: 1, Payload: "wake"}},
		Workflow:    testWorkflowState(t, qrspi.NodeImplement, nil),
		ActiveChild: &ChildRunRef{ID: "child-1", Stage: string(qrspi.NodeImplement), Generation: 1, LifecycleStatus: "completed"},
	}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	if err := RunMarkChildActive(t.Context(), MarkChildActiveOptions{StateFile: stateFile, ChildID: "child-1", Reason: "manual reprompt"}, deps{}, &out); err != nil {
		t.Fatalf("RunMarkChildActive error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild.Generation != 2 || loaded.ActiveChild.LifecycleStatus != "manual_reprompt" || loaded.Delivery.QueuedWake != nil {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionSupersededQueuedWake {
		t.Fatalf("action card = %+v", loaded.LastActionCard)
	}
	if !strings.Contains(out.String(), "child active: child-1 generation 2") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestContinueHumanGateEmitsActionCard(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "human-gate-state.json")
	sessionPath := filepath.Join(fixture.dir, "human-gate-session.jsonl")
	active := &ChildRunRef{ID: "outline-child", Stage: "outline", Cwd: fixture.projectRoot, TmuxPaneID: "%child", SessionID: "outline-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, ManagerRunID: "human-run", SourceCwd: fixture.projectRoot, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeOutline, nil)}
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(active.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("outline", "needs_human", "", "thoughts/example/design.md", ""))+"\n")

	text, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: &fakeChildRunner{}}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"stop: waiting human", "action: human_gate", "review: test goal", "steer-child --state-file"} {
		if !strings.Contains(text, want) {
			t.Fatalf("continue output missing %q: %q", want, text)
		}
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionHumanGate || !loaded.LastActionCard.RequiresHuman {
		t.Fatalf("last action card = %+v", loaded.LastActionCard)
	}
}
