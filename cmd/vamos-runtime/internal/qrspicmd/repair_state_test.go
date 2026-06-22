package qrspicmd

import (
	"bytes"
	"os"
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

func TestRepairStateClearFailedChildRequiresTerminalFailure(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	active := childHealthRef(dir)
	state := ManagerState{CanonicalPlanDir: "thoughts/example", Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: active}
	saveManagerState(t, stateFile, state)

	var out bytes.Buffer
	err := RunRepairState(t.Context(), RepairStateOptions{StateFile: stateFile, ClearFailedChild: true}, deps{Tmux: &recordingTmux{}}, &out)
	if err == nil || !strings.Contains(err.Error(), "not terminal failed") {
		t.Fatalf("RunRepairState err = %v, want terminal failure refusal; out=%q", err, out.String())
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.PendingCleanupChild != nil {
		t.Fatalf("loaded = %+v, want active child preserved", loaded)
	}
}

func TestRepairStateClearFailedChild(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	active := childHealthRef(dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\n")
	state := ManagerState{CanonicalPlanDir: "thoughts/example", Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: active}
	saveManagerState(t, stateFile, state)

	var out bytes.Buffer
	if err := RunRepairState(t.Context(), RepairStateOptions{StateFile: stateFile, ClearFailedChild: true}, deps{Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}}, &out); err != nil {
		t.Fatalf("RunRepairState error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild != nil || loaded.PendingCleanupChild == nil || loaded.PendingCleanupChild.ID != active.ID {
		t.Fatalf("loaded = %+v, want cleared active and pending cleanup", loaded)
	}
	if loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionChildLaunchFailed {
		t.Fatalf("last action card = %+v", loaded.LastActionCard)
	}
	for _, want := range []string{"repaired: cleared failed child child-1", "action: child_launch_failed", "safe command: vamos qrspi repair-state --state-file " + stateFile + " --clear-failed-child --relaunch"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %q", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "validation-recoveries.jsonl")); err != nil {
		t.Fatalf("recovery log missing: %v", err)
	}
}

func TestRepairStateClearFailedChildRelaunchesSameNode(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "failed-relaunch.json")
	active := childHealthRef(fixture.dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\n")
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: active}
	saveManagerState(t, stateFile, state)
	runner := &fakeChildRunner{panes: []string{"%new"}}
	var out bytes.Buffer
	if err := RunRepairState(t.Context(), RepairStateOptions{StateFile: stateFile, ClearFailedChild: true, Relaunch: true}, deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}, CommandRunner: qrspiOKCommandRunner()}, &out); err != nil {
		t.Fatalf("RunRepairState error = %v\nout=%s", err, out.String())
	}
	if len(runner.started) != 1 || runner.started[0].Stage != string(qrspi.NodeDesign) {
		t.Fatalf("started = %+v, want one design relaunch", runner.started)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.TmuxPaneID != "%new" || loaded.PendingCleanupChild != nil {
		t.Fatalf("loaded = %+v, want replacement active child and cleaned pending child", loaded)
	}
	if !strings.Contains(out.String(), "started child: design (%new)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestStartNextForceReplacesDeadChild(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "force-replace.json")
	active := childHealthRef(fixture.dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\n")
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: active}
	saveManagerState(t, stateFile, state)
	runner := &fakeChildRunner{panes: []string{"%new"}}
	var out bytes.Buffer
	result, err := RunStartNext(t.Context(), StartNextOptions{StateFile: stateFile, Force: true}, deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}, CommandRunner: qrspiOKCommandRunner()}, &out)
	if err != nil {
		t.Fatalf("RunStartNext error = %v\nout=%s", err, out.String())
	}
	if len(runner.started) != 1 || result.ActiveChild == nil || result.ActiveChild.TmuxPaneID != "%new" {
		t.Fatalf("result=%+v started=%+v, want replacement", result, runner.started)
	}
}

func TestStartNextForceProtectsRunningChild(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "force-protect.json")
	active := childHealthRef(fixture.dir)
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: active}
	saveManagerState(t, stateFile, state)
	runner := &fakeChildRunner{panes: []string{"%new"}}
	var out bytes.Buffer
	result, err := RunStartNext(t.Context(), StartNextOptions{StateFile: stateFile, Force: true}, deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{}, CommandRunner: qrspiOKCommandRunner()}, &out)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if len(runner.started) != 0 || result.ActiveChild == nil || result.ActiveChild.ID != active.ID {
		t.Fatalf("result=%+v started=%+v, want running child protected", result, runner.started)
	}
}

func qrspiOKCommandRunner() fakeCommandRunner {
	return fakeCommandRunner{results: map[string]CommandResult{
		"pi --help":    {Stdout: "--session-id\n--session-dir\n--name\n--extension\n"},
		"pi --version": {Stdout: "pi test\n"},
	}, errs: map[string]error{}}
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
