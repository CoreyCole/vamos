package qrspicmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestChildCompleteWritesValidatedStatus(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
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
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1", Output: "json"},
		deps{Tmux: tmux},
		&out,
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-plan" ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Normalizations) != 1 ||
		status.Normalizations[0].Canonical != "ready-for-plan" {
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
	if !disk.Validated || disk.DeliveryID == "" ||
		disk.Result.Outcome != "ready-for-plan" {
		t.Fatalf("disk status = %+v", disk)
	}
	if !strings.Contains(out.String(), `"validated": true`) {
		t.Fatalf("json output = %q", out.String())
	}

	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewOutline {
		t.Fatalf(
			"child-complete advanced workflow to %q; want still review-outline",
			loaded.Workflow.CurrentNodeID,
		)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.LifecycleStatus != "completed" ||
		loaded.ActiveChild.LastDeliveryID == "" {
		t.Fatalf("loaded active child = %+v", loaded.ActiveChild)
	}

	status, err = RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete duplicate error = %v", err)
	}
	if status.DeliveryID != loaded.ActiveChild.LastDeliveryID ||
		status.Wake.Mode != "suppress" ||
		status.Wake.Reason != "duplicate_delivery" {
		t.Fatalf("duplicate status = %+v", status)
	}
}

func TestChildCompleteQueuesValidatedWakeWhileManagerCompacting(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine(
			testResultYAML(
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Delivery: ManagerDeliveryState{
			Status:        "compacting",
			ManagerPaneID: "%parent",
		},
		Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "review-plan",
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
	if !status.Validated || status.Wake.Mode != "queue" ||
		status.Wake.Reason != "manager_compacting" {
		t.Fatalf("status = %+v, want validated queued wake", status)
	}
	if len(tmux.pastes) != 0 || len(tmux.keys) != 0 {
		t.Fatalf(
			"tmux pastes=%#v keys=%#v, want no parent paste while compacting",
			tmux.pastes,
			tmux.keys,
		)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Delivery.QueuedWake == nil ||
		loaded.Delivery.QueuedWake.DeliveryID != status.DeliveryID {
		t.Fatalf(
			"loaded delivery = %+v, want queued wake %q",
			loaded.Delivery,
			status.DeliveryID,
		)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if disk.Wake.Mode != "queue" || disk.Wake.Reason != "manager_compacting" {
		t.Fatalf("disk wake = %+v, want queued manager_compacting", disk.Wake)
	}
}

func TestLenientPositiveOutcomeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := writePiSession(
		t,
		sessionDir,
		"session.jsonl",
		"session-1",
		repo,
		assistantLine(
			testResultYAML(
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
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

	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Result.Outcome != "ready-for-workspace" ||
		len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
	if status.Normalizations[0].Original != "complete" ||
		status.Normalizations[0].Canonical != "ready-for-workspace" {
		t.Fatalf("normalization = %+v", status.Normalizations[0])
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewPlan {
		t.Fatalf(
			"child-complete advanced workflow to %q; want still review-plan",
			loaded.Workflow.CurrentNodeID,
		)
	}
}

func TestChildCompleteManagerAwareReviewPlanNormalization(t *testing.T) {
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
				"review-plan",
				"complete",
				"complete",
				"thoughts/example/reviews/plan/review.md",
				"",
			),
		),
	)
	state := ManagerState{
		CanonicalPlanDir: filepath.Join(
			dir,
			"thoughts",
			"plan",
			"reviews",
			"impl-review",
		),
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

	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: &recordingTmux{}},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Result.Outcome != "ready-for-implement" || len(status.Normalizations) != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestChildCompleteProviderContextErrorDeliversManagerWake(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionWithBlockedResultThenProviderError(t, sessionPath, "session-1", repo)
	oldDeliveryID := "child-1:1:verify:blocked::thoughts/example/verify.md"
	if err := writeValidationStatus(validationPath, ChildCompletionStatus{
		Validated:     true,
		ChildID:       "child-1",
		DeliveryID:    oldDeliveryID,
		ManagerNeeded: true,
		Result: ChildCompletionResult{
			Stage:        "verify",
			Status:       "blocked",
			Artifact:     "thoughts/example/verify.md",
			PlanGoal:     "stale plan goal",
			KeyDecisions: "stale key decisions",
		},
	}); err != nil {
		t.Fatalf("write prior validation status: %v", err)
	}
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Delivery: ManagerDeliveryState{
			LastDeliveryID: oldDeliveryID,
		},
		Workflow: testWorkflowState(t, qrspi.NodeVerify, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			Cwd:                  repo,
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	tmux := &recordingTmux{}
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1", Output: "json"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if status.Validated || !status.ManagerNeeded || status.RetryExhausted ||
		status.Result.Status != ActionChildContextExhausted ||
		status.TerminalEvidence == nil || !status.TerminalEvidence.ContextWindowError ||
		!strings.Contains(status.DeliveryID, ":provider_context_error:") ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("status = %+v", status)
	}
	if status.Result.Artifact != "thoughts/example/verify.md" ||
		status.Result.PlanGoal != "stale plan goal" {
		t.Fatalf("status result did not preserve prior context = %+v", status.Result)
	}
	if len(tmux.pastes) != 1 ||
		!strings.Contains(tmux.pastes[0].text, "q_manager_child_wake:") ||
		!strings.Contains(tmux.pastes[0].text, "terminal_evidence:") ||
		!strings.Contains(tmux.pastes[0].text, "context_window_error: true") ||
		!strings.Contains(tmux.pastes[0].text, "evidence_id:") ||
		!strings.Contains(tmux.pastes[0].text, "Your input exceeds the context window") {
		t.Fatalf("pastes = %#v", tmux.pastes)
	}
	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	if disk.TerminalEvidence == nil || !disk.TerminalEvidence.ContextWindowError ||
		disk.DeliveryID != status.DeliveryID {
		t.Fatalf("disk status = %+v", disk)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastActionCard == nil ||
		loaded.LastActionCard.Kind != ActionChildContextExhausted ||
		loaded.ActiveChild == nil ||
		loaded.ActiveChild.LifecycleStatus != "awaiting_manager" ||
		loaded.Delivery.LastDeliveryID != status.DeliveryID {
		t.Fatalf("loaded state = %+v", loaded)
	}
}

func TestChildCompleteProviderContextErrorSuppressesSameEvidenceDuplicate(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionWithBlockedResultThenProviderError(t, sessionPath, "session-1", repo)
	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeVerify, nil),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			Cwd:                  repo,
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"),
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)
	tmux := &recordingTmux{}
	first, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("first RunChildComplete error = %v", err)
	}
	second, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("second RunChildComplete error = %v", err)
	}
	if first.DeliveryID == "" || second.DeliveryID != first.DeliveryID ||
		second.Wake.Mode != "suppress" || second.Wake.Reason != "duplicate_delivery" ||
		len(tmux.pastes) != 1 {
		t.Fatalf("first=%+v second=%+v pastes=%#v", first, second, tmux.pastes)
	}
}

func TestTerminalEvidenceRefreshDoesNotReturnEarlyForOlderResult(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionTestFile(t, sessionPath, strings.Join([]string{
		sessionHeader("session-1", repo),
		assistantLine(
			testResultYAML(
				"verify",
				"blocked",
				"",
				"thoughts/example/verify.md",
				"stale blocked",
			),
		),
	}, "\n")+"\n")
	state := ManagerState{ActiveChild: &ChildRunRef{
		ID:          "child-1",
		Stage:       "verify",
		Cwd:         repo,
		SessionID:   "session-1",
		SessionDir:  sessionDir,
		SessionPath: sessionPath,
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(25 * time.Millisecond)
		file, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer file.Close()
		_, _ = file.WriteString(
			providerContextErrorLine(
				"Codex error: Your input exceeds the context window of this model.",
			) + "\n",
		)
	}()
	evidence, ok, err := terminalEvidenceForActiveChildWithRefresh(state)
	<-done
	if err != nil || !ok || !evidence.ContextWindowError || evidence.Line != 3 {
		t.Fatalf("evidence=%+v ok=%v err=%v", evidence, ok, err)
	}
}

func TestChildCompleteInvalidResultSuppressesThenExhausts(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	donePath := filepath.Join(dir, "done")
	sessionPath := writePiSession(
		t,
		filepath.Join(dir, "sessions"),
		"session.jsonl",
		"session-1",
		filepath.Join(dir, "repo"),
		assistantLine("not yaml"),
	)
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
	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete retry error = %v", err)
	}
	if status.Validated || status.ManagerNeeded || status.Wake.Mode != "suppress" ||
		status.Reason != "retryable_invalid_result" {
		t.Fatalf("retry status = %+v", status)
	}
	if len(tmux.pastes) != 1 {
		t.Fatalf("pastes = %#v, want reprompt", tmux.pastes)
	}

	status, err = RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete exhausted error = %v", err)
	}
	if !status.ManagerNeeded || !status.RetryExhausted ||
		status.Result.Status != "invalid_result" ||
		status.Wake.Mode != "deliver" {
		t.Fatalf("exhausted status = %+v", status)
	}
}
