package qrspicmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestInspectActiveChildHealthRunning(t *testing.T) {
	state := ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Stage: "design", TmuxPaneID: "%9"}}
	health, err := InspectActiveChildHealth(t.Context(), state, "state.json", deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != ActiveChildRunning {
		t.Fatalf("status = %s, want %s", health.Status, ActiveChildRunning)
	}
}

func TestInspectActiveChildHealthFinishedSuccessNeedsValidation(t *testing.T) {
	dir := t.TempDir()
	active := childHealthRef(dir)
	writeFile(t, active.StatusPath, `{"exitCode":0}`)
	writeFile(t, active.DonePath, "")
	state := ManagerState{ActiveChild: active}
	health, err := InspectActiveChildHealth(t.Context(), state, "state.json", deps{Tmux: &recordingTmux{}})
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != ActiveChildFinishedNeedsValidation || health.ExitCode == nil || *health.ExitCode != 0 {
		t.Fatalf("health = %+v, want success-needs-validation", health)
	}
}

func TestInspectActiveChildHealthLaunchFailed(t *testing.T) {
	dir := t.TempDir()
	active := childHealthRef(dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\nFlags:\n  --session string\n")
	state := ManagerState{ActiveChild: active}
	health, err := InspectActiveChildHealth(t.Context(), state, "state.json", deps{Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != ActiveChildLaunchFailed || health.ExitCode == nil || *health.ExitCode != 1 {
		t.Fatalf("health = %+v, want launch failed exit 1", health)
	}
	if !containsLine(health.OutputTail, "Error: unknown option --session-id") || containsLine(health.OutputTail, "Usage:") {
		t.Fatalf("output tail = %#v", health.OutputTail)
	}
}

func TestInspectActiveChildHealthDoesNotMarkFailureWhenSessionHasResult(t *testing.T) {
	dir := t.TempDir()
	active := childHealthRef(dir)
	active.Cwd = dir
	active.SessionPath = filepath.Join(active.SessionDir, "session.jsonl")
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeSessionTestFile(t, active.SessionPath, sessionHeader(active.SessionID, active.Cwd)+"\n"+assistantLine(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", ""))+"\n")
	state := ManagerState{ActiveChild: active}
	health, err := InspectActiveChildHealth(t.Context(), state, "state.json", deps{Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != ActiveChildFinishedNeedsValidation {
		t.Fatalf("status = %s, want %s", health.Status, ActiveChildFinishedNeedsValidation)
	}
}

func TestInspectActiveChildHealthPaneMissing(t *testing.T) {
	state := ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Stage: "design", TmuxPaneID: "%missing"}}
	health, err := InspectActiveChildHealth(t.Context(), state, "state.json", deps{Tmux: &recordingTmux{missingPanes: map[string]bool{"%missing": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != ActiveChildPaneMissing {
		t.Fatalf("status = %s, want %s", health.Status, ActiveChildPaneMissing)
	}
}

func TestContinueStopsWithLaunchFailedBeforeYAMLReprompt(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	active := childHealthRef(dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\n")
	state := ManagerState{CanonicalPlanDir: "thoughts/example", ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)}
	saveManagerState(t, stateFile, state)
	var out bytes.Buffer
	if err := RunContinue(t.Context(), ContinueOptions{StateFile: stateFile, PlanDir: "thoughts/example", Output: "text"}, deps{Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "action: child_launch_failed") || !strings.Contains(text, "safe command: vamos qrspi repair-state") {
		t.Fatalf("output = %s", text)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ValidationRetryCount != 0 {
		t.Fatalf("active child changed/reprompted: %+v", loaded.ActiveChild)
	}
}

func childHealthRef(dir string) *ChildRunRef {
	sessionDir := filepath.Join(dir, "sessions")
	return &ChildRunRef{
		ID:         "child-1",
		Stage:      "design",
		Cwd:        dir,
		TmuxPaneID: "%9",
		SessionID:  "session-1",
		SessionDir: sessionDir,
		OutputPath: filepath.Join(dir, "output.txt"),
		StatusPath: filepath.Join(dir, "status.json"),
		DonePath:   filepath.Join(dir, "done"),
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
