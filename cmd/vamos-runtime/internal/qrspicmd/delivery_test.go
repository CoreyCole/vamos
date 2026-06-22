package qrspicmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestDeliveryReadyManagerReceivesOneWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(t, sessionDir, "session.jsonl", "session-1", filepath.Join(dir, "repo"), assistantLine(testResultYAML("review-outline", "complete", "complete", "thoughts/example/reviews/outline/review.md", "")))
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeReviewOutline, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-outline",
			Cwd:                  filepath.Join(dir, "repo"),
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	tmux := &recordingTmux{}
	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Wake.Mode != "deliver" || len(tmux.pastes) != 1 || len(tmux.keys) != 1 {
		t.Fatalf("status = %+v pastes=%#v keys=%#v", status, tmux.pastes, tmux.keys)
	}
	if tmux.pastes[0].pane.ID != "%parent" || !strings.Contains(tmux.pastes[0].text, "q_manager_child_wake:") {
		t.Fatalf("paste = %#v", tmux.pastes[0])
	}
	status, err = RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"}, deps{Tmux: tmux}, &strings.Builder{})
	if err != nil {
		t.Fatalf("duplicate RunChildComplete error = %v", err)
	}
	if status.Wake.Mode != "suppress" || status.Wake.Reason != "duplicate_delivery" || len(tmux.pastes) != 1 {
		t.Fatalf("duplicate status = %+v pastes=%#v", status, tmux.pastes)
	}
}

func TestDeliveryQueuesWhileCompactingAndManagerReadyFlushes(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		ManagerPaneID: "%parent",
		Delivery:      ManagerDeliveryState{Status: "compacting", ManagerPaneID: "%parent"},
		ActiveChild:   &ChildRunRef{ID: "child-1", Generation: 2, LifecycleStatus: "completed"},
	}
	status := ChildCompletionStatus{Validated: true, ChildID: "child-1", DeliveryID: "child-1:2:review-outline:complete:ready-for-plan:artifact", Result: ChildCompletionResult{Stage: "review-outline", Status: "complete", Outcome: "ready-for-plan", Artifact: "artifact"}}
	tmux := &recordingTmux{}
	queued, wake, err := queueOrDeliverWake(t.Context(), stateFile, state, status, deps{Tmux: tmux})
	if err != nil {
		t.Fatalf("queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "queue" || queued.Delivery.QueuedWake == nil || len(tmux.pastes) != 0 {
		t.Fatalf("wake=%+v state=%+v pastes=%#v", wake, queued.Delivery, tmux.pastes)
	}
	saveManagerState(t, stateFile, queued)
	if err := RunManagerReady(t.Context(), ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%new-parent"}, deps{Tmux: tmux}, &strings.Builder{}); err != nil {
		t.Fatalf("RunManagerReady error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil || loaded.Delivery.LastDeliveryID != status.DeliveryID || loaded.Delivery.Status != "ready" {
		t.Fatalf("loaded delivery = %+v", loaded.Delivery)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%new-parent" {
		t.Fatalf("pastes=%#v", tmux.pastes)
	}
}

func TestManagerReadySupersedesStaleQueuedWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		Delivery: ManagerDeliveryState{Status: "compacting", QueuedWake: &QueuedWake{DeliveryID: "old", ChildID: "child-1", ChildGeneration: 1, Payload: "wake"}},
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      2,
			LifecycleStatus: "running",
		},
	}
	saveManagerState(t, stateFile, state)
	tmux := &recordingTmux{}
	if err := RunManagerReady(t.Context(), ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%parent"}, deps{Tmux: tmux}, &strings.Builder{}); err != nil {
		t.Fatalf("RunManagerReady error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil || loaded.LastActionCard == nil || loaded.LastActionCard.Kind != "superseded_queued_wake" {
		t.Fatalf("loaded state = %+v", loaded)
	}
	if len(tmux.pastes) != 0 {
		t.Fatalf("pastes=%#v", tmux.pastes)
	}
}
