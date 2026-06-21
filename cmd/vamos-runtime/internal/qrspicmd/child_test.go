package qrspicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildChildCommandUsesInteractivePiSessionAndDoneEnv(t *testing.T) {
	req := ChildRunRequest{
		PromptFile:  "/tmp/prompt.txt",
		OutputPath:  "/tmp/output.txt",
		SessionID:   "question-1",
		SessionDir:  "/tmp/sessions",
		SessionName: "q-manager question",
		DonePath:    "/tmp/done",
		StatusPath:  "/tmp/status.json",
	}
	cmd := strings.Join(BuildChildCommand(req), " ")
	for _, want := range []string{"PROMPT_FILE=/tmp/prompt.txt", "OUTPUT_PATH=/tmp/output.txt", "SESSION_ID=question-1", "SESSION_DIR=/tmp/sessions", "--session-id", "--session-dir", "--name", "@$PROMPT_FILE", "capture-pane", "STATUS_PATH", "DONE_PATH", "interactive Pi"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command missing %q: %v", want, BuildChildCommand(req))
		}
	}
	if strings.Contains(cmd, "--print") || strings.Contains(cmd, "tee") || strings.Contains(cmd, "RESULT_PATH") || strings.Contains(cmd, "cp \"$OUTPUT_PATH\"") {
		t.Fatalf("command kept authoritative result file: %s", cmd)
	}
}

func TestResultAndOutputPathsOutsideRepo(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	root := filepath.Join(t.TempDir(), "state")
	result := ResultPath(root, "child")
	output := OutputPath(root, "child")
	sessionDir := SessionDir(root, "child")
	done := DonePath(root, "child")
	status := StatusPath(root, "child")
	for _, path := range []string{result, output, sessionDir, done, status} {
		if !strings.HasPrefix(path, root) {
			t.Fatalf("path %s not under state root %s", path, root)
		}
		if strings.HasPrefix(path, repo) {
			t.Fatalf("path %s unexpectedly under repo %s", path, repo)
		}
	}
}

func TestRunChildStartsRightSplitAndSavesActiveChild(t *testing.T) {
	fixture := newRunChildFixture(t, true)
	out := &bytes.Buffer{}
	err := RunChild(t.Context(), fixture.options(), deps{Clock: fixture.clock, Runner: fixture.runner}, out)
	if err != nil {
		t.Fatalf("RunChild error = %v", err)
	}
	if got := len(fixture.runner.started); got != 1 {
		t.Fatalf("started = %d, want 1", got)
	}
	req := fixture.runner.started[0]
	if req.Split != "right" || req.Cwd != fixture.cwd {
		t.Fatalf("request = %+v", req)
	}
	state := fixture.loadState(t)
	if state.ActiveChild == nil || state.ActiveChild.TmuxPaneID != "%9" || state.ActiveChild.SessionPath == "" {
		t.Fatalf("active child = %+v", state.ActiveChild)
	}
	for _, want := range []string{`"type":"child_started"`, `"type":"child_finished"`, `"outputPath"`, `"sessionId"`, `"sessionDir"`, `"sessionPath"`, `"donePath"`, `"statusPath"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %s: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), `"resultPath"`) {
		t.Fatalf("output exposed default resultPath: %q", out.String())
	}
}

func TestRunChildUsesImplementationCwdWhenRequestedExplicitly(t *testing.T) {
	fixture := newRunChildFixture(t, true)
	impl := filepath.Join(fixture.dir, "impl")
	fixture.cwd = impl
	state := fixture.loadState(t)
	state.ImplementationCwd = impl
	fixture.saveState(t, state)
	if err := os.MkdirAll(impl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunChild(t.Context(), fixture.options(), deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunChild error = %v", err)
	}
	if got := fixture.runner.started[0].Cwd; got != impl {
		t.Fatalf("cwd = %q, want %q", got, impl)
	}
}

func TestRunChildWaitsForDoneByDefault(t *testing.T) {
	fixture := newRunChildFixture(t, true)
	if err := RunChild(t.Context(), fixture.options(), deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunChild error = %v", err)
	}
	state := fixture.loadState(t)
	if _, err := os.Stat(state.ActiveChild.DonePath); err != nil {
		t.Fatalf("done marker not written: %v", err)
	}
	if _, err := os.Stat(state.ActiveChild.StatusPath); err != nil {
		t.Fatalf("status not written: %v", err)
	}
}

func TestRunChildTimeoutKeepsActiveChildRefs(t *testing.T) {
	fixture := newRunChildFixture(t, false)
	opts := fixture.options()
	opts.Timeout = time.Nanosecond
	err := RunChild(t.Context(), opts, deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for child done marker") {
		t.Fatalf("expected timeout, got %v", err)
	}
	state := fixture.loadState(t)
	if state.ActiveChild == nil || state.ActiveChild.DonePath == "" || state.ActiveChild.StatusPath == "" || state.ActiveChild.OutputPath == "" || state.ActiveChild.SessionID == "" || state.ActiveChild.SessionDir == "" {
		t.Fatalf("active child refs not preserved: %+v", state.ActiveChild)
	}
}

func TestRunChildRejectsMissingPromptFile(t *testing.T) {
	fixture := newRunChildFixture(t, true)
	opts := fixture.options()
	opts.PromptFile = filepath.Join(fixture.dir, "missing.txt")
	err := RunChild(t.Context(), opts, deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "prompt-file does not exist") {
		t.Fatalf("expected prompt missing error, got %v", err)
	}
}

type fakeChildRunner struct {
	writeResult bool
	started     []ChildRunRequest
}

func (f *fakeChildRunner) Start(ctx context.Context, req ChildRunRequest) (ChildRun, error) {
	f.started = append(f.started, req)
	return ChildRun{ID: req.ID, Pane: TmuxPane{ID: "%9"}, OutputPath: req.OutputPath, SessionID: req.SessionID, SessionDir: req.SessionDir, DonePath: req.DonePath, StatusPath: req.StatusPath}, nil
}

func (f *fakeChildRunner) Wait(ctx context.Context, run ChildRun) (ChildRunResult, error) {
	if f.writeResult {
		sessionPath := filepath.Join(run.SessionDir, "session.jsonl")
		session := sessionHeader(run.SessionID, f.started[len(f.started)-1].Cwd) + "\n" + assistantLine("```yaml\nqrspi_result:\n  stage: plan\n```") + "\n"
		if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
			return ChildRunResult{}, err
		}
		if err := os.WriteFile(sessionPath, []byte(session), 0o644); err != nil {
			return ChildRunResult{}, err
		}
		if err := os.WriteFile(run.StatusPath, []byte(`{"exitCode":0,"finishedAt":"1970-01-01T00:00:00Z"}`), 0o644); err != nil {
			return ChildRunResult{}, err
		}
		if err := os.WriteFile(run.DonePath, []byte(""), 0o644); err != nil {
			return ChildRunResult{}, err
		}
	}
	return ChildRunResult{ID: run.ID, OutputPath: run.OutputPath, SessionID: run.SessionID, SessionDir: run.SessionDir, DonePath: run.DonePath, StatusPath: run.StatusPath}, nil
}

type runChildFixture struct {
	dir       string
	cwd       string
	stateFile string
	prompt    string
	runner    *fakeChildRunner
}

func newRunChildFixture(t *testing.T, writeResult bool) runChildFixture {
	t.Helper()
	dir := t.TempDir()
	cwd := filepath.Join(dir, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(prompt, []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, "state", "key", "run.json")
	state := ManagerState{RepoID: cwd, CanonicalPlanDir: filepath.Join(cwd, "thoughts/example"), ManagerRunID: "run", SourceCwd: cwd}
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	return runChildFixture{dir: dir, cwd: cwd, stateFile: stateFile, prompt: prompt, runner: &fakeChildRunner{writeResult: writeResult}}
}

func (f runChildFixture) options() RunChildOptions {
	return RunChildOptions{PlanDir: "thoughts/example", Stage: "plan", Cwd: f.cwd, PromptFile: f.prompt, StateFile: f.stateFile, Timeout: time.Second}
}

func (f runChildFixture) clock() time.Time { return time.Unix(100, 123) }

func (f runChildFixture) loadState(t *testing.T) ManagerState {
	t.Helper()
	data, err := os.ReadFile(f.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state ManagerState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (f runChildFixture) saveState(t *testing.T, state ManagerState) {
	t.Helper()
	if err := (FileStateStore{}).Save(f.stateFile, state); err != nil {
		t.Fatal(err)
	}
}
