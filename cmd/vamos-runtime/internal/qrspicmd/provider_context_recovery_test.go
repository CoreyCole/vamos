package qrspicmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
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

func TestProviderContextRecoveryPreservesBoundedValidGate(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "thoughts", "example")
	artifact := filepath.Join(planDir, "verify.md")
	writeFile(t, artifact, "verified")
	sessionDir := filepath.Join(dir, "sessions")
	sessionPath := filepath.Join(sessionDir, "session.jsonl")
	writeSessionTestFile(t, sessionPath, strings.Join([]string{
		sessionHeader("session-1", dir),
		assistantLine(testResultYAML(
			"verify", "needs_human", "", "thoughts/example/verify.md", "",
		)),
		providerContextErrorLine("Codex error: input exceeds the context window"),
	}, "\n")+"\n")
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		CanonicalPlanDir: planDir,
		ManagerPaneID:    "%parent",
		Workflow:         testWorkflowState(t, qrspi.NodeVerify, nil),
		ActiveChild: &ChildRunRef{
			ID: "child-1", Stage: "verify", Cwd: dir, TmuxPaneID: "%9",
			SessionID: "session-1", SessionDir: sessionDir, SessionPath: sessionPath,
			ValidationStatusPath: filepath.Join(dir, "validation-status.json"), Generation: 1,
		},
	})
	tmux := &recordingTmux{}
	gathered, gatherErr := GatherChildEvidence(
		loadManagerState(t, stateFile),
		ChildCompletionOptions{Boundary: ChildBoundaryAgentSettled, Interaction: ChildInteractionStageWork},
	)
	if gatherErr != nil || gathered.LatestGraphValidResult == nil {
		t.Fatalf("GatherChildEvidence() = %+v, err = %v", gathered, gatherErr)
	}
	var validate bytes.Buffer
	if err := RunValidateLatest(
		t.Context(),
		ValidateLatestOptions{StateFile: stateFile, Stage: "verify", ApplyRebind: true},
		deps{},
		&validate,
	); err != nil {
		t.Fatalf("RunValidateLatest() error = %v", err)
	}
	if !strings.Contains(validate.String(), "validated latest: verify needs_human") ||
		strings.Contains(validate.String(), "provider_context_error") {
		t.Fatalf("validate-latest output = %q", validate.String())
	}

	status, err := RunChildComplete(
		t.Context(),
		ChildCompletionOptions{StateFile: stateFile, ChildID: "child-1"},
		deps{Tmux: tmux},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("RunChildComplete() error = %v", err)
	}
	if !status.Validated || status.Intent != ChildIntentGraphValidResult ||
		status.Result.Status != "needs_human" || status.Result.Artifact != "thoughts/example/verify.md" ||
		!strings.Contains(status.Reason, "durable artifact proof") {
		t.Fatalf("status = %+v, want retained valid gate", status)
	}
	if status.Wake.Mode != "deliver" || len(tmux.pastes) != 1 {
		t.Fatalf("wake = %+v pastes = %#v", status.Wake, tmux.pastes)
	}
}

func TestResultHasDurableArtifactRejectsUntrustedPaths(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "thoughts", "example")
	writeFile(t, filepath.Join(planDir, "verify.md"), "verified")
	writeFile(t, filepath.Join(dir, "thoughts", "other.md"), "unrelated")
	if err := os.MkdirAll(filepath.Join(planDir, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.md")
	writeFile(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(planDir, "escape.md")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "bounded regular file", path: "thoughts/example/verify.md", want: true},
		{name: "missing", path: "thoughts/example/missing.md"},
		{name: "unrelated existing", path: "thoughts/other.md"},
		{name: "absolute", path: artifactAbsolutePath(t, filepath.Join(planDir, "verify.md"))},
		{name: "dot dot escape", path: "thoughts/example/../other.md"},
		{name: "directory", path: "thoughts/example/directory"},
		{name: "symlink escape", path: "thoughts/example/escape.md"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := &ResultEvidence{Parsed: &ParsedDecision{Result: wruntime.WorkflowResult{
				PrimaryArtifact: test.path,
			}}}
			if got := resultHasDurableArtifact(ManagerState{CanonicalPlanDir: planDir}, result); got != test.want {
				t.Fatalf("resultHasDurableArtifact(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func artifactAbsolutePath(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	return absolute
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
