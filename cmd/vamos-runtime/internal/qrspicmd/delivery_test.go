package qrspicmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestDeliveryReadyManagerReceivesOneWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine(
			testResultYAML(
				"review-outline",
				"complete",
				"complete",
				"thoughts/example/reviews/outline/review.md",
				"",
			),
		),
	)
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
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Wake.Mode != "deliver" || len(tmux.pastes) != 1 || len(tmux.keys) != 1 {
		t.Fatalf("status = %+v pastes=%#v keys=%#v", status, tmux.pastes, tmux.keys)
	}
	if tmux.pastes[0].pane.ID != "%parent" ||
		!strings.Contains(tmux.pastes[0].text, "q_manager_child_wake:") {
		t.Fatalf("paste = %#v", tmux.pastes[0])
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.LastDeliveryID != status.DeliveryID {
		t.Fatalf(
			"last delivery ID = %q, want %q",
			loaded.Delivery.LastDeliveryID,
			status.DeliveryID,
		)
	}
	status, err = RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("duplicate RunChildComplete error = %v", err)
	}
	if status.Wake.Mode != "suppress" || status.Wake.Reason != "duplicate_delivery" ||
		len(tmux.pastes) != 1 {
		t.Fatalf("duplicate status = %+v pastes=%#v", status, tmux.pastes)
	}
}

func TestDeliveryProviderContextErrorBypassesOlderBlockedDelivery(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	state := ManagerState{
		ManagerPaneID: "%parent",
		Delivery: ManagerDeliveryState{
			LastDeliveryID: "child-1:1:verify:blocked::thoughts/example/verify.md",
		},
		ActiveChild: &ChildRunRef{ID: "child-1", Generation: 1},
	}
	status := ChildCompletionStatus{
		Validated:     false,
		ManagerNeeded: true,
		ChildID:       "child-1",
		DeliveryID:    "child-1:1:provider_context_error:abc123",
		Reason:        "provider_context_error",
		Result: ChildCompletionResult{
			Stage:  "verify",
			Status: ActionChildContextExhausted,
		},
	}
	tmux := &recordingTmux{}
	delivered, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		state,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatalf("queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "deliver" || delivered.Delivery.LastDeliveryID != status.DeliveryID ||
		len(tmux.pastes) != 1 {
		t.Fatalf("wake=%+v state=%+v pastes=%#v", wake, delivered, tmux.pastes)
	}
	again, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		delivered,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatalf("second queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "suppress" || wake.Reason != "duplicate_delivery" ||
		again.Delivery.LastDeliveryID != status.DeliveryID || len(tmux.pastes) != 1 {
		t.Fatalf("second wake=%+v state=%+v pastes=%#v", wake, again, tmux.pastes)
	}
}

func TestDeliveryQueuesWhileCompactingAndManagerReadyFlushes(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		ManagerPaneID: "%parent",
		Delivery: ManagerDeliveryState{
			Status:        "compacting",
			ManagerPaneID: "%parent",
		},
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      2,
			LifecycleStatus: "completed",
		},
	}
	status := ChildCompletionStatus{
		Validated:  true,
		ChildID:    "child-1",
		DeliveryID: "child-1:2:review-outline:complete:ready-for-plan:artifact",
		Result: ChildCompletionResult{
			Stage:    "review-outline",
			Status:   "complete",
			Outcome:  "ready-for-plan",
			Artifact: "artifact",
		},
	}
	tmux := &recordingTmux{}
	queued, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		state,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatalf("queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "queue" || queued.Delivery.QueuedWake == nil ||
		len(tmux.pastes) != 0 {
		t.Fatalf("wake=%+v state=%+v pastes=%#v", wake, queued.Delivery, tmux.pastes)
	}
	saveManagerState(t, stateFile, queued)
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%new-parent"},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatalf("RunManagerReady error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil ||
		loaded.Delivery.LastDeliveryID != status.DeliveryID ||
		loaded.Delivery.Status != "ready" {
		t.Fatalf("loaded delivery = %+v", loaded.Delivery)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%new-parent" {
		t.Fatalf("pastes=%#v", tmux.pastes)
	}
	var out strings.Builder
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%new-parent"},
		deps{Tmux: tmux},
		&out,
	); err != nil {
		t.Fatalf("second RunManagerReady error = %v", err)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("second manager-ready pasted again: %#v", tmux.pastes)
	}
	if !strings.Contains(out.String(), "manager ready: no queued wake") {
		t.Fatalf("second output = %q", out.String())
	}
}

func TestDeliveryQueuesWhenSelectedManagerPaneUnavailable(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		ManagerPaneID: "%dead",
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      1,
			LifecycleStatus: "completed",
		},
	}
	status := ChildCompletionStatus{
		Validated:  true,
		ChildID:    "child-1",
		DeliveryID: "child-1:1:plan:complete:complete:artifact",
		Result: ChildCompletionResult{
			Stage:    "plan",
			Status:   "complete",
			Outcome:  "complete",
			Artifact: "artifact",
		},
	}
	tmux := &recordingTmux{missingPanes: map[string]bool{"%dead": true}}
	queued, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		state,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatalf("queueOrDeliverWake error = %v", err)
	}
	if wake.Mode != "queue" || wake.Reason != "manager_pane_unavailable" ||
		queued.Delivery.QueuedWake == nil || len(tmux.pastes) != 0 {
		t.Fatalf("wake=%+v state=%+v pastes=%#v", wake, queued, tmux.pastes)
	}
	if queued.LastActionCard == nil ||
		queued.LastActionCard.Kind != ActionManagerPaneUnavailable ||
		!strings.Contains(queued.LastActionCard.SafeCommand, "manager-ready") {
		t.Fatalf("action card = %+v", queued.LastActionCard)
	}
}

func TestManagerReadyCurrentPaneAdoptsUnavailableDeliveryAndFlushes(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		ManagerPaneID: "%dead",
		Delivery: ManagerDeliveryState{
			Status:        "compacting",
			ManagerPaneID: "%dead",
			QueuedWake: &QueuedWake{
				DeliveryID:      "wake-1",
				ChildID:         "child-1",
				ChildGeneration: 1,
				Payload:         "wake",
			},
		},
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      1,
			LifecycleStatus: "completed",
		},
	})
	t.Setenv("TMUX_PANE", "%new")
	tmux := &recordingTmux{missingPanes: map[string]bool{"%dead": true}}
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatal(err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ManagerPaneID != "%new" || loaded.Delivery.ManagerPaneID != "%new" ||
		loaded.Delivery.QueuedWake != nil || loaded.Delivery.LastDeliveryID != "wake-1" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%new" {
		t.Fatalf("pastes = %#v", tmux.pastes)
	}
}

func TestManagerReadyCurrentPaneAdoptsCompactingLiveDeliveryAndFlushes(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		ManagerPaneID: "%old",
		Delivery: ManagerDeliveryState{
			Status:        "compacting",
			ManagerPaneID: "%old",
			QueuedWake: &QueuedWake{
				DeliveryID:      "wake-1",
				ChildID:         "child-1",
				ChildGeneration: 1,
				Payload:         "wake",
			},
		},
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      1,
			LifecycleStatus: "completed",
		},
	})
	t.Setenv("TMUX_PANE", "%new")
	tmux := &recordingTmux{}
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatal(err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ManagerPaneID != "%new" || loaded.Delivery.ManagerPaneID != "%new" ||
		loaded.Delivery.QueuedWake != nil || loaded.Delivery.LastDeliveryID != "wake-1" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%new" {
		t.Fatalf("pastes = %#v", tmux.pastes)
	}
}

func TestDeliveryFailureQueuesPhaseAndManagerReadyDoesNotDuplicatePaste(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	state := ManagerState{
		ManagerPaneID: "%parent",
		ActiveChild: &ChildRunRef{
			ID:                     "replacement",
			Generation:             1,
			LifecycleStatus:        "running",
			LaunchKind:             ChildLaunchResumeHandoff,
			ContinuationDeliveryID: "wake-1",
		},
	}
	status := ChildCompletionStatus{
		Validated:           true,
		ContinuationStarted: true,
		ChildID:             "source",
		DeliveryID:          "wake-1",
		Reason:              "handoff_auto_resumed",
		Result: ChildCompletionResult{
			Stage:    "research",
			Status:   "handoff",
			Artifact: "thoughts/example/handoffs/research.md",
		},
		NextChild: NextChildInfo{Stage: "research", Skill: ".pi/skills/q-resume/SKILL.md"},
	}
	tmux := &recordingTmux{sendErr: errors.New("submit failed")}
	queued, wake, err := queueOrDeliverWake(
		t.Context(),
		stateFile,
		state,
		status,
		deps{Tmux: tmux},
	)
	if err != nil {
		t.Fatal(err)
	}
	if wake.Mode != "queue" || wake.Reason != "manager_delivery_failed" ||
		queued.Delivery.QueuedWake == nil ||
		queued.Delivery.QueuedWake.Delivery != QueuedWakeSubmitOnly ||
		queued.Delivery.QueuedWake.PastedPaneID != "%parent" ||
		len(tmux.pastes) != 1 || len(tmux.keys) != 1 {
		t.Fatalf("wake=%+v queued=%+v pastes=%d keys=%d", wake, queued.Delivery.QueuedWake, len(tmux.pastes), len(tmux.keys))
	}
	if strings.Contains(queued.Delivery.QueuedWake.Payload, "action: run_command") {
		t.Fatalf("continuation wake contains continue action: %s", queued.Delivery.QueuedWake.Payload)
	}
	saveManagerState(t, stateFile, queued)
	tmux.sendErr = nil
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%parent"},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatal(err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil || loaded.Delivery.LastDeliveryID != "wake-1" ||
		len(tmux.pastes) != 1 || len(tmux.keys) != 2 {
		t.Fatalf("loaded=%+v pastes=%d keys=%d", loaded.Delivery, len(tmux.pastes), len(tmux.keys))
	}
}

func TestManagerReadyAdoptsPaneAndRepastesSubmitOnlyWake(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	saveManagerState(t, stateFile, ManagerState{
		Delivery: ManagerDeliveryState{QueuedWake: &QueuedWake{
			DeliveryID: "wake-1", ChildID: "replacement", ChildGeneration: 1,
			Payload: "wake", Delivery: QueuedWakeSubmitOnly, PastedPaneID: "%old",
		}},
		ActiveChild: &ChildRunRef{
			ID: "replacement", Generation: 1, LifecycleStatus: "running",
			LaunchKind: ChildLaunchResumeHandoff, ContinuationDeliveryID: "wake-1",
		},
	})
	tmux := &recordingTmux{}
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%new"},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatal(err)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%new" ||
		len(tmux.keys) != 1 || tmux.keys[0].pane.ID != "%new" {
		t.Fatalf("pastes=%#v keys=%#v", tmux.pastes, tmux.keys)
	}
}

func TestDeliveryPasteFailureRetriesPasteAndSubmit(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	state := ManagerState{ManagerPaneID: "%parent", ActiveChild: &ChildRunRef{ID: "child", Generation: 1, LifecycleStatus: "completed"}}
	status := ChildCompletionStatus{ChildID: "child", DeliveryID: "wake-1", Result: ChildCompletionResult{Stage: "plan", Status: "complete"}}
	tmux := &recordingTmux{pasteErr: errors.New("paste failed")}
	queued, wake, err := queueOrDeliverWake(t.Context(), stateFile, state, status, deps{Tmux: tmux})
	if err != nil {
		t.Fatal(err)
	}
	if wake.Mode != "queue" || queued.Delivery.QueuedWake == nil ||
		queued.Delivery.QueuedWake.Delivery != QueuedWakePasteAndSubmit || len(tmux.keys) != 0 {
		t.Fatalf("wake=%+v queued=%+v keys=%d", wake, queued.Delivery.QueuedWake, len(tmux.keys))
	}
	saveManagerState(t, stateFile, queued)
	tmux.pasteErr = nil
	if err := RunManagerReady(t.Context(), ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%parent"}, deps{Tmux: tmux}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if len(tmux.pastes) != 2 || len(tmux.keys) != 1 {
		t.Fatalf("pastes=%d keys=%d", len(tmux.pastes), len(tmux.keys))
	}
}

func TestManagerReadySupersedesStaleQueuedWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		Delivery: ManagerDeliveryState{
			Status: "compacting",
			QueuedWake: &QueuedWake{
				DeliveryID:      "old",
				ChildID:         "child-1",
				ChildGeneration: 1,
				Payload:         "wake",
			},
		},
		ActiveChild: &ChildRunRef{
			ID:              "child-1",
			Generation:      2,
			LifecycleStatus: "running",
		},
	}
	saveManagerState(t, stateFile, state)
	tmux := &recordingTmux{}
	if err := RunManagerReady(
		t.Context(),
		ManagerReadyOptions{StateFile: stateFile, ManagerPane: "%parent"},
		deps{Tmux: tmux},
		&strings.Builder{},
	); err != nil {
		t.Fatalf("RunManagerReady error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake != nil || loaded.LastActionCard == nil ||
		loaded.LastActionCard.Kind != "superseded_queued_wake" {
		t.Fatalf("loaded state = %+v", loaded)
	}
	if len(tmux.pastes) != 0 {
		t.Fatalf("pastes=%#v", tmux.pastes)
	}
}
