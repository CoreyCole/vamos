package qrspicmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestSteerChildPastesFeedbackToActiveChild(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	feedbackFile := filepath.Join(dir, "feedback.md")
	donePath := filepath.Join(dir, "done")
	validationStatusPath := filepath.Join(dir, "validation-status.json")
	writeFile(t, feedbackFile, "please revise the outline before completing")
	writeFile(t, donePath, "")
	writeFile(t, validationStatusPath, `{"terminalBoundary":true}`)
	saveManagerState(t, stateFile, ManagerState{
		CanonicalPlanDir: "thoughts/example",
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
		Delivery: ManagerDeliveryState{QueuedWake: &QueuedWake{
			DeliveryID: "old", ChildID: "child-1", ChildGeneration: 1,
		}},
		ActiveChild: &ChildRunRef{
			ID:                      "child-1",
			Stage:                   "outline",
			TmuxPaneID:              "%9",
			DonePath:                donePath,
			ValidationStatusPath:    validationStatusPath,
			Generation:              1,
			ValidationRetryCount:    1,
			LastEvidenceFingerprint: "old",
		},
	})

	tmux := &recordingTmux{}
	var out bytes.Buffer
	result, err := RunSteerChild(t.Context(), SteerChildOptions{StateFile: stateFile, FeedbackFile: feedbackFile, Stage: "outline"}, deps{Tmux: tmux}, &out)
	if err != nil {
		t.Fatalf("RunSteerChild error = %v", err)
	}
	if result.Stage != "outline" || result.PaneID != "%9" || result.FeedbackPath != feedbackFile {
		t.Fatalf("result = %#v", result)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%9" {
		t.Fatalf("pastes = %#v", tmux.pastes)
	}
	paste := tmux.pastes[0].text
	for _, want := range []string{"q-manager steering feedback", "source: human_feedback", "stage: outline", "feedback_file: " + feedbackFile, "please revise the outline"} {
		if !strings.Contains(paste, want) {
			t.Fatalf("paste missing %q: %q", want, paste)
		}
	}
	for _, forbidden := range []string{"CorrectionPrompt", "qrspi_result schema"} {
		if strings.Contains(paste, forbidden) {
			t.Fatalf("steering prompt includes correction wording %q: %q", forbidden, paste)
		}
	}
	if len(tmux.keys) != 1 || tmux.keys[0].pane.ID != "%9" || strings.Join(tmux.keys[0].keys, ",") != "Enter" {
		t.Fatalf("keys = %#v, want Enter to %%9", tmux.keys)
	}
	if !strings.Contains(out.String(), "steered child: outline (%9)") || !strings.Contains(out.String(), "next: vamos qrspi continue --state-file "+stateFile) {
		t.Fatalf("output = %q", out.String())
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild.Generation != 2 || loaded.ActiveChild.LifecycleStatus != "steered" ||
		loaded.ActiveChild.ValidationRetryCount != 0 || loaded.ActiveChild.LastEvidenceFingerprint != "" ||
		loaded.Delivery.QueuedWake != nil {
		t.Fatalf("steered state = %+v", loaded)
	}
	for _, path := range []string{donePath, validationStatusPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale completion marker %s still exists: %v", path, err)
		}
	}
}

func TestSteerChildRejectsFeedbackSourceProblems(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	feedbackFile := filepath.Join(dir, "feedback.md")
	writeFile(t, feedbackFile, "feedback")

	cases := []struct {
		name string
		opts SteerChildOptions
		want string
	}{
		{"missing", SteerChildOptions{StateFile: stateFile}, "feedback-file or feedback is required"},
		{"both", SteerChildOptions{StateFile: stateFile, FeedbackFile: feedbackFile, Feedback: "inline"}, "use only one of --feedback-file or --feedback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunSteerChild(t.Context(), tc.opts, deps{Tmux: &recordingTmux{}}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSteerChildRejectsWrongStage(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Stage: "research", TmuxPaneID: "%9"}})
	tmux := &recordingTmux{}
	_, err := RunSteerChild(t.Context(), SteerChildOptions{StateFile: stateFile, Feedback: "feedback", Stage: "design"}, deps{Tmux: tmux}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `active child stage "research" does not match requested stage "design"`) {
		t.Fatalf("expected stage mismatch, got %v", err)
	}
	if len(tmux.pastes) != 0 {
		t.Fatalf("unexpected paste on stage mismatch: %#v", tmux.pastes)
	}
}

func TestSteerChildRejectsNoActiveChildWithNextCommand(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)})
	_, err := RunSteerChild(t.Context(), SteerChildOptions{StateFile: stateFile, Feedback: "feedback"}, deps{Tmux: &recordingTmux{}}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no active child to steer") || !strings.Contains(err.Error(), "vamos qrspi start-next --state-file "+stateFile) {
		t.Fatalf("error = %v", err)
	}
}

func TestSteerChildPersistsNewEpochBeforePasteFailure(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{ActiveChild: &ChildRunRef{
		ID: "child-1", Stage: "plan", TmuxPaneID: "%9", Generation: 1,
	}})
	tmux := &recordingTmux{pasteErr: errors.New("paste failed")}

	_, err := RunSteerChild(
		t.Context(),
		SteerChildOptions{StateFile: stateFile, Feedback: "feedback"},
		deps{Tmux: tmux},
		&bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "paste failed") {
		t.Fatalf("RunSteerChild error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild.Generation != 2 ||
		loaded.ActiveChild.LifecycleStatus != "steer_delivery_failed" ||
		loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionManualChildSteer {
		t.Fatalf("state after paste failure = %+v", loaded)
	}
}

func TestSteerChildNDJSON(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Stage: "plan", TmuxPaneID: "%9"}})
	var out bytes.Buffer
	_, err := RunSteerChild(t.Context(), SteerChildOptions{StateFile: stateFile, Feedback: "feedback", Output: "ndjson"}, deps{Tmux: &recordingTmux{}}, &out)
	if err != nil {
		t.Fatalf("RunSteerChild error = %v", err)
	}
	for _, want := range []string{`"type":"child_steered"`, `"stage":"plan"`, `"paneId":"%9"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %q", want, out.String())
		}
	}
}
