package qrspicmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestChildCompleteWritesValidatedStatus(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := writePiSession(t, sessionDir, "session.jsonl", "session-1", filepath.Join(dir, "repo"), assistantLine(testResultYAML("review-outline", "complete", "complete", "thoughts/example/reviews/outline/review.md", "")))
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-outline",
			Cwd:                  filepath.Join(dir, "repo"),
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	var out strings.Builder
	tmux := &recordingTmux{}
	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1", Output: "json"}, deps{Tmux: tmux}, &out)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-plan" || status.Wake.Mode != "deliver" {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Normalizations) != 1 || status.Normalizations[0].Canonical != "ready-for-plan" {
		t.Fatalf("normalizations = %+v", status.Normalizations)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if !disk.Validated || disk.DeliveryID == "" || disk.Result.Outcome != "ready-for-plan" {
		t.Fatalf("disk status = %+v", disk)
	}
	if !strings.Contains(out.String(), `"validated": true`) {
		t.Fatalf("json output = %q", out.String())
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewOutline {
		t.Fatalf("child-complete advanced workflow to %q; want still review-outline", loaded.Workflow.CurrentNodeID)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.LifecycleStatus != "completed" || loaded.ActiveChild.LastDeliveryID == "" {
		t.Fatalf("loaded active child = %+v", loaded.ActiveChild)
	}

	status, err = RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete duplicate error = %v", err)
	}
	if status.DeliveryID != loaded.ActiveChild.LastDeliveryID || status.Wake.Mode != "suppress" || status.Wake.Reason != "duplicate_delivery" {
		t.Fatalf("duplicate status = %+v", status)
	}
}

func TestLenientPositiveOutcomeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(t, sessionDir, "session.jsonl", "session-1", repo, assistantLine(testResultYAML("review-plan", "complete", "complete", "thoughts/example/reviews/plan/review.md", "")))
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
			Cwd:                  repo,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: &recordingTmux{}}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-workspace" || len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
	if status.Normalizations[0].Original != "complete" || status.Normalizations[0].Canonical != "ready-for-workspace" {
		t.Fatalf("normalization = %+v", status.Normalizations[0])
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewPlan {
		t.Fatalf("child-complete advanced workflow to %q; want still review-plan", loaded.Workflow.CurrentNodeID)
	}
}

func TestChildCompleteManagerAwareReviewPlanNormalization(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(t, sessionDir, "session.jsonl", "session-1", filepath.Join(dir, "repo"), assistantLine(testResultYAML("review-plan", "complete", "complete", "thoughts/example/reviews/plan/review.md", "")))
	state := ManagerState{
		CanonicalPlanDir:  filepath.Join(dir, "thoughts", "plan", "reviews", "impl-review"),
		ImplementationCwd: filepath.Join(dir, "repo"),
		ManagerPaneID:     "%parent",
		Workflow:          testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
			Cwd:                  filepath.Join(dir, "repo"),
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: &recordingTmux{}}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Result.Outcome != "ready-for-implement" || len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestChildCompleteInvalidResultSuppressesThenExhausts(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	donePath := filepath.Join(dir, "done")
	sessionPath := writePiSession(t, filepath.Join(dir, "sessions"), "session.jsonl", "session-1", filepath.Join(dir, "repo"), assistantLine("not yaml"))
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeDesign, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "design",
			Cwd:                  filepath.Join(dir, "repo"),
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           filepath.Join(dir, "sessions"),
			SessionPath:          sessionPath,
			DonePath:             donePath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	writeFile(t, donePath, "")
	tmux := &recordingTmux{}
	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete retry error = %v", err)
	}
	if status.Validated || status.ManagerNeeded || status.Wake.Mode != "suppress" || status.Reason != "retryable_invalid_result" {
		t.Fatalf("retry status = %+v", status)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %#v, want reprompt", tmux.pastes)
	}

	status, err = RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete exhausted error = %v", err)
	}
	if !status.ManagerNeeded || !status.RetryExhausted || status.Result.Status != "invalid_result" || status.Wake.Mode != "deliver" {
		t.Fatalf("exhausted status = %+v", status)
	}
}
