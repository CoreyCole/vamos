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

func TestRecoverySummaryPathUsesPlanContextRecovery(t *testing.T) {
	path := RecoverySummaryPath(
		"/tmp/plan",
		"review-implementation",
		"verify/child:1",
		time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC),
	)
	want := filepath.Join(
		"/tmp/plan",
		"context",
		"recovery",
		"2026-07-05_01-02-03_review-implementation_verify-child-1_context-recovery.md",
	)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestRunRecoverSummaryDryRunWritesPromptAndNote(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "thoughts", "example")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(planDir, "AGENTS.md"), "plan memory")
	sessionPath := filepath.Join(dir, "sessions", "child.jsonl")
	writeSessionTestFile(t, sessionPath, strings.Join([]string{
		sessionHeader("session-1", dir),
		providerContextErrorLine(
			"Codex error: Your input exceeds the context window of this model. Please adjust your input and try again.",
		),
	}, "\n")+"\n")
	validationPath := filepath.Join(dir, "validation-status.json")
	writeValidationStatusFixture(
		t,
		validationPath,
		ChildCompletionStatus{
			Result: ChildCompletionResult{Artifact: "thoughts/example/verify.md"},
		},
	)
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		CanonicalPlanDir:  planDir,
		ImplementationCwd: filepath.Join(dir, "repo"),
		ActiveChild: &ChildRunRef{
			ID:                   "child-1",
			Stage:                "verify",
			SessionPath:          sessionPath,
			ValidationStatusPath: validationPath,
		},
		Workflow: testWorkflowState(t, qrspi.NodeVerify, nil),
	})

	out, err := executeManagerCommand(
		deps{
			Clock: func() time.Time { return time.Date(2026, 7, 5, 1, 2, 3, 4, time.UTC) },
		},
		"recover-summary",
		"--state-file",
		stateFile,
		"--session-file",
		sessionPath,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("recover-summary error = %v", err)
	}
	promptPath := filepath.Join(
		dir,
		"prompts",
		"recover-summary-20260705T010203.000000004Z.md",
	)
	notePath := filepath.Join(
		planDir,
		"context",
		"recovery",
		"2026-07-05_01-02-03_verify_child-1_context-recovery.md",
	)
	if !strings.Contains(out, "recovery prompt: "+promptPath) ||
		!strings.Contains(out, "recovery note: "+notePath) {
		t.Fatalf("output = %s", out)
	}
	prompt := readFile(t, promptPath)
	note := readFile(t, notePath)
	for _, want := range []string{sessionPath, notePath, "Evidence ID:", "Provider error:", "Do not emit qrspi_result", "Same-stage relaunch"} {
		if !strings.Contains(prompt+"\n"+note, want) {
			t.Fatalf("prompt/note missing %q\nprompt:\n%s\nnote:\n%s", want, prompt, note)
		}
	}
	if strings.Contains(note, "advance graph.") &&
		!strings.Contains(note, "Do not advance graph") {
		t.Fatalf("note should prohibit graph advance: %s", note)
	}
}

func TestWriteRecoverySummaryPromptIncludesProviderEvidence(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	req := RecoverySummaryRequest{
		StateFile:   filepath.Join(dir, "state.json"),
		PlanDir:     filepath.Join(dir, "plan"),
		Stage:       "verify",
		ChildID:     "child-1",
		SessionFile: filepath.Join(dir, "session.jsonl"),
		NotePath:    filepath.Join(dir, "note.md"),
		Evidence: AssistantTerminalEvidence{
			SessionID:          "session-1",
			Line:               42,
			StopReason:         "error",
			ErrorMessage:       "context window exceeded",
			ContextWindowError: true,
			EvidenceID:         "abc123",
		},
	}
	if err := WriteRecoverySummaryPrompt(req, promptPath); err != nil {
		t.Fatal(err)
	}
	prompt := readFile(t, promptPath)
	for _, want := range []string{req.SessionFile, req.NotePath, "abc123", "context window exceeded", "Do not emit qrspi_result", "Do not advance graph"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRunRecoverSummaryJSONOutput(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "session.jsonl")
	writeSessionTestFile(
		t,
		sessionPath,
		sessionHeader(
			"session-1",
			dir,
		)+"\n"+providerContextErrorLine(
			"context window exceeded",
		)+"\n",
	)
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		CanonicalPlanDir: planDir,
		ActiveChild: &ChildRunRef{
			ID:          "child-1",
			Stage:       "verify",
			SessionPath: sessionPath,
		},
		Workflow: testWorkflowState(t, qrspi.NodeVerify, nil),
	})
	out, err := executeManagerCommand(
		deps{
			Clock: func() time.Time { return time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC) },
		},
		"recover-summary",
		"--state-file",
		stateFile,
		"--session-file",
		sessionPath,
		"--dry-run",
		"--output",
		"json",
	)
	if err != nil {
		t.Fatal(err)
	}
	var got RecoverySummaryRequest
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if got.NotePath == "" || got.PromptPath == "" || !got.Evidence.ContextWindowError {
		t.Fatalf("request = %+v", got)
	}
}

func TestRunRecoverSummaryNonDryRunRequiresWrittenNote(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "session.jsonl")
	writeSessionTestFile(
		t,
		sessionPath,
		sessionHeader(
			"session-1",
			dir,
		)+"\n"+providerContextErrorLine(
			"context window exceeded",
		)+"\n",
	)
	stateFile := filepath.Join(dir, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		CanonicalPlanDir: planDir,
		ActiveChild: &ChildRunRef{
			ID:          "child-1",
			Stage:       "verify",
			SessionPath: sessionPath,
		},
		Workflow: testWorkflowState(t, qrspi.NodeVerify, nil),
	})
	promptPath := filepath.Join(
		dir,
		"prompts",
		"recover-summary-20260705T010203.000000000Z.md",
	)
	_, err := executeManagerCommand(
		deps{
			Clock: func() time.Time { return time.Date(2026, 7, 5, 1, 2, 3, 0, time.UTC) },
			CommandRunner: fakeCommandRunner{results: map[string]CommandResult{
				"pi @" + promptPath: {ExitCode: 0},
			}, errs: map[string]error{}},
		},
		"recover-summary",
		"--state-file",
		stateFile,
		"--session-file",
		sessionPath,
	)
	if err == nil || !strings.Contains(err.Error(), "did not write recovery note") {
		t.Fatalf("expected missing note error, got %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeValidationStatusFixture(
	t *testing.T,
	path string,
	status ChildCompletionStatus,
) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}
