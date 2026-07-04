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
	pastes       []recordedPaste
	keys         []recordedKeys
	kills        []TmuxPane
	layouts      []recordedLayout
	missingPanes map[string]bool
}

type recordedPaste struct {
	pane TmuxPane
	text string
}

type recordedKeys struct {
	pane TmuxPane
	keys []string
}

type recordedLayout struct {
	pane   TmuxPane
	layout string
}

func (r *recordingTmux) SplitPane(
	ctx context.Context,
	req TmuxSplitRequest,
) (TmuxPane, error) {
	return TmuxPane{ID: "%new"}, nil
}

func (r *recordingTmux) SendKeys(
	ctx context.Context,
	pane TmuxPane,
	keys []string,
) error {
	r.keys = append(
		r.keys,
		recordedKeys{pane: pane, keys: append([]string(nil), keys...)},
	)
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

func (r *recordingTmux) SelectLayout(
	ctx context.Context,
	pane TmuxPane,
	layout string,
) error {
	r.layouts = append(r.layouts, recordedLayout{pane: pane, layout: layout})
	return nil
}

func (r *recordingTmux) PaneExists(ctx context.Context, pane TmuxPane) (bool, error) {
	if r.missingPanes != nil && r.missingPanes[pane.ID] {
		return false, nil
	}
	return strings.TrimSpace(pane.ID) != "", nil
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
	err := RunRepromptChild(
		t.Context(),
		RepromptChildOptions{
			StateFile: stateFile,
			PlanDir:   "thoughts/example",
			Stage:     "design",
			Attempt:   1,
			ErrorFile: errFile,
		},
		deps{Tmux: tmux},
		&out,
	)
	if err != nil {
		t.Fatalf("RunRepromptChild error = %v", err)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %d, want 1", len(tmux.pastes))
	}
	if tmux.pastes[0].pane.ID != "%9" {
		t.Fatalf("paste pane = %q, want %%9", tmux.pastes[0].pane.ID)
	}
	for _, want := range []string{"Validation error:", "missing qrspi_result", "```yaml", "workspace_metadata:", "next:"} {
		if !strings.Contains(tmux.pastes[0].text, want) {
			t.Fatalf("correction prompt missing %q: %q", want, tmux.pastes[0].text)
		}
	}
	if len(tmux.keys) != 1 || tmux.keys[0].pane.ID != "%9" ||
		strings.Join(tmux.keys[0].keys, ",") != "Enter" {
		t.Fatalf("keys = %#v, want Enter to %%9", tmux.keys)
	}
	if !strings.Contains(out.String(), `"type":"child_reprompted"`) ||
		!strings.Contains(out.String(), `"childId":"child-1"`) {
		t.Fatalf("output = %q", out.String())
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ID != initial.ActiveChild.ID ||
		loaded.ActiveChild.SessionID != initial.ActiveChild.SessionID ||
		loaded.ActiveChild.TmuxPaneID != initial.ActiveChild.TmuxPaneID {
		t.Fatalf("active child changed: %#v", loaded.ActiveChild)
	}
}

func TestRunRepromptChildUsesProviderErrorRecoveryPrompt(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	initial := ManagerState{
		Workflow: testWorkflowState(t, qrspi.NodeImplement, nil),
		ActiveChild: &ChildRunRef{
			ID:         "child-1",
			Stage:      "implement",
			Cwd:        filepath.Join(dir, "repo"),
			TmuxPaneID: "%9",
			SessionID:  "session-1",
			SessionDir: filepath.Join(dir, "sessions"),
		},
	}
	saveManagerState(t, stateFile, initial)

	tmux := &recordingTmux{}
	err := RunRepromptChild(t.Context(), RepromptChildOptions{
		StateFile: stateFile,
		PlanDir:   "thoughts/example",
		Stage:     "implement",
		Attempt:   1,
		ErrorText: "session /tmp/session.jsonl ended with provider error before qrspi_result",
	}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunRepromptChild error = %v", err)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %d, want 1", len(tmux.pastes))
	}
	prompt := tmux.pastes[0].text
	for _, want := range []string{"provider/runtime error", "Continue the same QRSPI stage", "Do not emit a synthetic qrspi_result"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("provider recovery prompt missing %q: %q", want, prompt)
		}
	}
	if strings.Contains(prompt, "Re-emit exactly one corrected") ||
		strings.Contains(prompt, "Validation error:") {
		t.Fatalf(
			"provider recovery prompt should not be YAML correction prompt: %q",
			prompt,
		)
	}
}

func TestRunRepromptChildRejectsStageMismatch(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(
		t,
		stateFile,
		ManagerState{
			ActiveChild: &ChildRunRef{ID: "child-1", Stage: "research", TmuxPaneID: "%9"},
		},
	)
	tmux := &recordingTmux{}
	err := RunRepromptChild(
		t.Context(),
		RepromptChildOptions{
			StateFile: stateFile,
			PlanDir:   "thoughts/example",
			Stage:     "design",
		},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			`active child stage "research" does not match requested stage "design"`,
		) {
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
	err := RunRepromptChild(
		t.Context(),
		RepromptChildOptions{
			StateFile: stateFile,
			PlanDir:   "thoughts/example",
			Stage:     "design",
		},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err == nil || !strings.Contains(err.Error(), "no active child to reprompt") {
		t.Fatalf("expected missing active child error, got %v", err)
	}
}

func TestContinueInvalidResultRepromptsSameActiveChild(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionPath := filepath.Join(dir, "sessions", "session.jsonl")
	donePath := filepath.Join(dir, "done")
	initial := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		SourceCwd:        filepath.Join(dir, "repo"),
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:          "child-1",
			Stage:       "design",
			Cwd:         filepath.Join(dir, "repo"),
			TmuxPaneID:  "%9",
			SessionID:   "session-1",
			SessionDir:  filepath.Join(dir, "sessions"),
			SessionPath: sessionPath,
			DonePath:    donePath,
			StatusPath:  filepath.Join(dir, "status.json"),
		},
	}
	saveManagerState(t, stateFile, initial)
	writeSessionTestFile(
		t,
		sessionPath,
		sessionHeader(
			"session-1",
			initial.ActiveChild.Cwd,
		)+"\n"+assistantLine(
			"I finished this without the required YAML.",
		)+"\n",
	)
	writeFile(t, donePath, "")

	tmux := &recordingTmux{}
	runner := &fakeChildRunner{startErr: os.ErrInvalid}
	var out strings.Builder
	err := RunContinue(
		t.Context(),
		ContinueOptions{StateFile: stateFile},
		deps{Tmux: tmux, Runner: runner},
		&out,
	)
	if err != nil {
		t.Fatalf("RunContinue error = %v", err)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%9" {
		t.Fatalf("pastes = %#v, want one paste to %%9", tmux.pastes)
	}
	if !strings.Contains(tmux.pastes[0].text, "required QRSPI") &&
		!strings.Contains(tmux.pastes[0].text, "qrspi_result") {
		t.Fatalf("correction prompt missing validation context: %q", tmux.pastes[0].text)
	}
	if len(tmux.keys) != 1 || tmux.keys[0].pane.ID != "%9" ||
		strings.Join(tmux.keys[0].keys, ",") != "Enter" {
		t.Fatalf("keys = %#v, want Enter to %%9", tmux.keys)
	}
	if len(runner.started) != 0 {
		t.Fatalf("runner started = %#v, want none", runner.started)
	}
	if !strings.Contains(out.String(), "retry: reprompted active child") {
		t.Fatalf("continue output = %q", out.String())
	}
	if _, err := os.Stat(donePath); !os.IsNotExist(err) {
		t.Fatalf("done marker stat err = %v, want removed", err)
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ID != initial.ActiveChild.ID ||
		loaded.ActiveChild.SessionID != initial.ActiveChild.SessionID ||
		loaded.ActiveChild.TmuxPaneID != initial.ActiveChild.TmuxPaneID {
		t.Fatalf("active child changed: %#v", loaded.ActiveChild)
	}
	if loaded.ActiveChild.ValidationRetryCount != 1 ||
		loaded.ActiveChild.LastRepromptAttempt != 1 {
		t.Fatalf(
			"retry state = count %d attempt %d, want 1/1",
			loaded.ActiveChild.ValidationRetryCount,
			loaded.ActiveChild.LastRepromptAttempt,
		)
	}

	var exhaustedOut strings.Builder
	err = RunContinue(
		t.Context(),
		ContinueOptions{StateFile: stateFile},
		deps{Tmux: tmux, Runner: runner},
		&exhaustedOut,
	)
	if err != nil {
		t.Fatalf("RunContinue retry exhaustion error = %v", err)
	}
	if !strings.Contains(exhaustedOut.String(), "retry: exhausted") ||
		!strings.Contains(
			exhaustedOut.String(),
			"guidance: Inspect child output/artifacts",
		) {
		t.Fatalf("retry exhaustion output = %q", exhaustedOut.String())
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("duplicate reprompt pasted: %#v", tmux.pastes)
	}
}
