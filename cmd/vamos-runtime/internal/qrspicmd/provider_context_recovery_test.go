package qrspicmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestProviderContextRecoveryOriginalBugSequence(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	repo := filepath.Join(dir, "repo")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	validationPath := filepath.Join(dir, "runs", "child-1", "validation-status.json")
	oldDeliveryID := "child-1:1:verify:blocked::thoughts/example/verify.md"

	writeSessionWithBlockedResultThenProviderError(t, sessionPath, "session-1", repo)
	writeFile(t, filepath.Join(dir, "status.json"), `{"exitCode":0}`)
	writeFile(t, filepath.Join(dir, "done"), "")
	writeValidationStatusFixture(t, validationPath, ChildCompletionStatus{
		Validated:     true,
		ManagerNeeded: true,
		ChildID:       "child-1",
		DeliveryID:    oldDeliveryID,
		Result: ChildCompletionResult{
			Stage:        "verify",
			Status:       "blocked",
			Artifact:     "thoughts/example/verify.md",
			PlanGoal:     "stale plan goal",
			KeyDecisions: "stale key decisions",
		},
	})

	state := ManagerState{
		CanonicalPlanDir: "thoughts/example",
		SourceCwd:        repo,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeVerify, nil),
		Delivery: ManagerDeliveryState{
			LastDeliveryID: oldDeliveryID,
		},
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			Cwd:                  repo,
			TmuxPaneID:           "%9",
			SessionID:            "session-1",
			SessionDir:           sessionDir,
			SessionPath:          sessionPath,
			StatusPath:           filepath.Join(dir, "status.json"),
			DonePath:             filepath.Join(dir, "done"),
			ValidationStatusPath: validationPath,
			Generation:           1,
		},
	}
	saveManagerState(t, stateFile, state)

	var inspect bytes.Buffer
	if err := RunInspect(
		t.Context(),
		InspectOptions{StateFile: stateFile, Sessions: true, Latest: true},
		deps{Tmux: &recordingTmux{}},
		&inspect,
	); err != nil {
		t.Fatalf("RunInspect error = %v", err)
	}
	assertProviderContextOutput(t, inspect.String(), stateFile, sessionPath)

	var validate bytes.Buffer
	if err := RunValidateLatest(
		t.Context(),
		ValidateLatestOptions{
			StateFile:   stateFile,
			Stage:       "verify",
			ApplyRebind: true,
			Output:      "text",
		},
		deps{},
		&validate,
	); err != nil {
		t.Fatalf("RunValidateLatest error = %v", err)
	}
	assertProviderContextOutput(t, validate.String(), stateFile, sessionPath)
	if !strings.Contains(validate.String(), "action: child_context_exhausted") {
		t.Fatalf("validate-latest output missing action card:\n%s", validate.String())
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeVerify {
		t.Fatalf("validate-latest advanced workflow to %q", loaded.Workflow.CurrentNodeID)
	}

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
	assertProviderContextRecoveryStatus(t, *status)
	if status.DeliveryID == oldDeliveryID ||
		!strings.Contains(status.DeliveryID, ":provider_context_error:") {
		t.Fatalf(
			"delivery ID = %q, want distinct provider context identity",
			status.DeliveryID,
		)
	}
	if status.Wake.Mode != "deliver" {
		t.Fatalf("wake = %+v, want deliver despite old blocked delivery", status.Wake)
	}
	if len(tmux.pastes) != 1 ||
		!strings.Contains(tmux.pastes[0].text, "terminal_evidence:") {
		t.Fatalf("pastes = %#v, want provider evidence wake", tmux.pastes)
	}

	var disk ChildCompletionStatus
	data, err := os.ReadFile(validationPath)
	if err != nil {
		t.Fatalf("read validation status: %v", err)
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("decode validation status: %v", err)
	}
	assertProviderContextRecoveryStatus(t, disk)

	var cont bytes.Buffer
	if err := RunContinue(
		t.Context(),
		ContinueOptions{
			StateFile: stateFile,
			PlanDir:   "thoughts/example",
			Output:    "text",
		},
		deps{Tmux: &recordingTmux{}},
		&cont,
	); err != nil {
		t.Fatalf("RunContinue error = %v", err)
	}
	assertProviderContextOutput(t, cont.String(), stateFile, sessionPath)
	if !strings.Contains(cont.String(), "action: child_context_exhausted") {
		t.Fatalf("continue output missing action card:\n%s", cont.String())
	}
	loaded = loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeVerify ||
		loaded.LastActionCard == nil ||
		loaded.LastActionCard.Kind != ActionChildContextExhausted {
		t.Fatalf("continue advanced or missed card: %+v", loaded)
	}
}

func assertProviderContextRecoveryStatus(t *testing.T, status ChildCompletionStatus) {
	t.Helper()
	if status.Validated || !status.ManagerNeeded ||
		status.Result.Status != ActionChildContextExhausted ||
		status.TerminalEvidence == nil ||
		!status.TerminalEvidence.ContextWindowError {
		t.Fatalf("status = %+v, want provider context recovery", status)
	}
}

func assertProviderContextOutput(t *testing.T, text, stateFile, sessionPath string) {
	t.Helper()
	for _, want := range []string{
		"provider_context_error",
		"input exceeds the context window",
		"evidence id:",
		sessionPath,
		"vamos qrspi inspect --state-file " + stateFile + " --sessions --latest",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
