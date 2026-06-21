package qrspicmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

type recordingTmux struct {
	pastes []recordedPaste
	keys   []recordedKeys
	kills  []TmuxPane
}

type recordedPaste struct {
	pane TmuxPane
	text string
}

type recordedKeys struct {
	pane TmuxPane
	keys []string
}

func (r *recordingTmux) SplitPane(ctx context.Context, req TmuxSplitRequest) (TmuxPane, error) {
	return TmuxPane{ID: "%new"}, nil
}

func (r *recordingTmux) SendKeys(ctx context.Context, pane TmuxPane, keys []string) error {
	r.keys = append(r.keys, recordedKeys{pane: pane, keys: append([]string(nil), keys...)})
	return nil
}

func (r *recordingTmux) PasteText(ctx context.Context, pane TmuxPane, text string) error {
	r.pastes = append(r.pastes, recordedPaste{pane: pane, text: text})
	return nil
}

func (r *recordingTmux) KillPane(ctx context.Context, pane TmuxPane) error {
	r.kills = append(r.kills, pane)
	return nil
}

func TestRunRepromptChildPastesCorrectionToActiveChild(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	initial := ManagerState{
		Workflow: testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:         "child-1",
			Stage:      "design",
			Cwd:        filepath.Join(dir, "repo"),
			TmuxPaneID: "%9",
			SessionID:  "session-1",
			SessionDir: filepath.Join(dir, "sessions"),
		},
	}
	saveManagerState(t, stateFile, initial)
	errFile := filepath.Join(dir, "validation.err")
	if err := os.WriteFile(errFile, []byte("missing qrspi_result"), 0o644); err != nil {
		t.Fatal(err)
	}

	tmux := &recordingTmux{}
	var out strings.Builder
	err := RunRepromptChild(t.Context(), RepromptChildOptions{StateFile: stateFile, PlanDir: "thoughts/example", Stage: "design", Attempt: 1, ErrorFile: errFile}, deps{Tmux: tmux}, &out)
	if err != nil {
		t.Fatalf("RunRepromptChild error = %v", err)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %d, want 1", len(tmux.pastes))
	}
	if tmux.pastes[0].pane.ID != "%9" {
		t.Fatalf("paste pane = %q, want %%9", tmux.pastes[0].pane.ID)
	}
	if !strings.Contains(tmux.pastes[0].text, "missing qrspi_result") {
		t.Fatalf("correction prompt missing validation error: %q", tmux.pastes[0].text)
	}
	if len(tmux.keys) != 1 || tmux.keys[0].pane.ID != "%9" || strings.Join(tmux.keys[0].keys, ",") != "Enter" {
		t.Fatalf("keys = %#v, want Enter to %%9", tmux.keys)
	}
	if !strings.Contains(out.String(), `"type":"child_reprompted"`) || !strings.Contains(out.String(), `"childId":"child-1"`) {
		t.Fatalf("output = %q", out.String())
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ID != initial.ActiveChild.ID || loaded.ActiveChild.SessionID != initial.ActiveChild.SessionID || loaded.ActiveChild.TmuxPaneID != initial.ActiveChild.TmuxPaneID {
		t.Fatalf("active child changed: %#v", loaded.ActiveChild)
	}
}

func TestRunRepromptChildRejectsStageMismatch(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Stage: "research", TmuxPaneID: "%9"}})
	tmux := &recordingTmux{}
	err := RunRepromptChild(t.Context(), RepromptChildOptions{StateFile: stateFile, PlanDir: "thoughts/example", Stage: "design"}, deps{Tmux: tmux}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), `active child stage "research" does not match requested stage "design"`) {
		t.Fatalf("expected stage mismatch, got %v", err)
	}
	if len(tmux.pastes) != 0 {
		t.Fatalf("unexpected paste on stage mismatch: %#v", tmux.pastes)
	}
}

func TestRunRepromptChildRejectsMissingActiveChild(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{})
	err := RunRepromptChild(t.Context(), RepromptChildOptions{StateFile: stateFile, PlanDir: "thoughts/example", Stage: "design"}, deps{Tmux: &recordingTmux{}}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "no active child to reprompt") {
		t.Fatalf("expected missing active child error, got %v", err)
	}
}
