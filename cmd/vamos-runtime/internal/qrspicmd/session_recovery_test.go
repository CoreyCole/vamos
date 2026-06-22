package qrspicmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestFindLatestRelevantChildSessionClassifiesManualNew(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions")
	cwd := filepath.Join(dir, "repo")
	oldPath := writePiSession(t, sessionDir, "old.jsonl", "session-old", cwd, assistantLine(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", "")))
	newPath := writePiSession(t, sessionDir, "new.jsonl", "session-new", cwd, assistantLine(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", "")))
	oldTime := time.Date(2026, 6, 21, 1, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	chtimes(t, oldPath, oldTime)
	chtimes(t, newPath, newTime)

	candidate, ok, err := FindLatestRelevantChildSession(LatestChildQuery{Stage: qrspi.NodeDesign, WorkspaceCwd: cwd, SessionDir: sessionDir, ActiveChild: &ChildRunRef{ID: "child-1", Stage: "design", Cwd: cwd, SessionID: "session-old", SessionDir: sessionDir, SessionPath: oldPath}})
	if err != nil {
		t.Fatalf("FindLatestRelevantChildSession error = %v", err)
	}
	if !ok {
		t.Fatal("FindLatestRelevantChildSession ok = false")
	}
	if candidate.SessionPath != newPath || candidate.Classification != "manual_new" {
		t.Fatalf("candidate = %#v, want new manual session", candidate)
	}
}

func TestRunRebindChildUpdatesActiveSessionAndSupersedesWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	cwd := filepath.Join(dir, "repo")
	newPath := writePiSession(t, sessionDir, "new.jsonl", "session-new", cwd, assistantLine(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", "")))
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild:      &ChildRunRef{ID: "child-1", Stage: "design", Cwd: cwd, SessionID: "session-old", SessionDir: sessionDir, SessionPath: filepath.Join(sessionDir, "old.jsonl"), Generation: 1},
		Delivery:         ManagerDeliveryState{QueuedWake: &QueuedWake{ChildID: "child-1", ChildGeneration: 1, DeliveryID: "old", Payload: "wake"}},
	}
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := RunRebindChild(t.Context(), RebindChildOptions{StateFile: stateFile, SessionFile: newPath, Stage: "design", Reason: "manual-new", Output: "text"}, deps{}, &out); err != nil {
		t.Fatalf("RunRebindChild error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.SessionPath != newPath || loaded.ActiveChild.SessionID != "session-new" {
		t.Fatalf("active child = %#v, want rebound new session", loaded.ActiveChild)
	}
	if loaded.ActiveChild.Generation != 2 {
		t.Fatalf("generation = %d, want 2", loaded.ActiveChild.Generation)
	}
	if loaded.Delivery.QueuedWake != nil {
		t.Fatalf("queued wake = %#v, want superseded nil", loaded.Delivery.QueuedWake)
	}
	if !strings.Contains(out.String(), "rebound child") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunValidateLatestCanApplyRebindAndContinue(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	cwd := filepath.Join(dir, "repo")
	oldPath := writePiSession(t, sessionDir, "old.jsonl", "session-old", cwd, assistantLine("not final"))
	newPath := writePiSession(t, sessionDir, "new.jsonl", "session-new", cwd, assistantLine(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", "")))
	oldTime := time.Date(2026, 6, 21, 1, 0, 0, 0, time.UTC)
	chtimes(t, oldPath, oldTime)
	chtimes(t, newPath, oldTime.Add(time.Minute))
	state := ManagerState{CanonicalPlanDir: "thoughts/example", SourceCwd: cwd, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil), ActiveChild: &ChildRunRef{ID: "child-1", Stage: "design", Cwd: cwd, SessionID: "session-old", SessionDir: sessionDir, SessionPath: oldPath, Generation: 1}}
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := RunValidateLatest(t.Context(), ValidateLatestOptions{StateFile: stateFile, Stage: "design", ApplyRebind: true, Continue: true, Output: "text"}, deps{Runner: recoveryFakeRunner{paneID: "%new"}}, &out); err != nil {
		t.Fatalf("RunValidateLatest error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeOutline {
		t.Fatalf("current node = %q, want outline", loaded.Workflow.CurrentNodeID)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.Stage != "outline" {
		t.Fatalf("active child = %#v, want launched outline child", loaded.ActiveChild)
	}
	if !strings.Contains(out.String(), "validated: design complete") || !strings.Contains(out.String(), "started child: outline") {
		t.Fatalf("output = %q", out.String())
	}
}

type recoveryFakeRunner struct{ paneID string }

func (r recoveryFakeRunner) Start(ctx context.Context, req ChildRunRequest) (ChildRun, error) {
	return ChildRun{ID: req.ID, Pane: TmuxPane{ID: r.paneID}, OutputPath: req.OutputPath, SessionID: req.SessionID, SessionDir: req.SessionDir, DonePath: req.DonePath, StatusPath: req.StatusPath}, nil
}

func (r recoveryFakeRunner) Wait(ctx context.Context, run ChildRun) (ChildRunResult, error) {
	return ChildRunResult{ID: run.ID, OutputPath: run.OutputPath, SessionID: run.SessionID, SessionDir: run.SessionDir, DonePath: run.DonePath, StatusPath: run.StatusPath}, nil
}

func chtimes(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}
