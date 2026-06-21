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

func TestBuildChildCommandUsesPromptOutputAndResultEnv(t *testing.T) {
	req := ChildRunRequest{PromptFile: "/tmp/prompt.txt", OutputPath: "/tmp/output.txt", ResultPath: "/tmp/result.txt"}
	cmd := strings.Join(BuildChildCommand(req), " ")
	for _, want := range []string{"PROMPT_FILE=/tmp/prompt.txt", "OUTPUT_PATH=/tmp/output.txt", "RESULT_PATH=/tmp/result.txt", "pi --print", "tee"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command missing %q: %v", want, BuildChildCommand(req))
		}
	}
}

func TestResultAndOutputPathsOutsideRepo(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	root := filepath.Join(t.TempDir(), "state")
	result := ResultPath(root, "child")
	output := OutputPath(root, "child")
	for _, path := range []string{result, output} {
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
	if state.ActiveChild == nil || state.ActiveChild.TmuxPaneID != "%9" {
		t.Fatalf("active child = %+v", state.ActiveChild)
	}
	if !strings.Contains(out.String(), `"type":"child_started"`) || !strings.Contains(out.String(), `"type":"child_finished"`) {
		t.Fatalf("output = %q", out.String())
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

func TestRunChildWaitsForResultByDefault(t *testing.T) {
	fixture := newRunChildFixture(t, true)
	if err := RunChild(t.Context(), fixture.options(), deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunChild error = %v", err)
	}
	state := fixture.loadState(t)
	if _, err := os.Stat(state.ActiveChild.ResultPath); err != nil {
		t.Fatalf("result not written: %v", err)
	}
}

func TestRunChildTimeoutKeepsActiveChildRefs(t *testing.T) {
	fixture := newRunChildFixture(t, false)
	opts := fixture.options()
	opts.Timeout = time.Nanosecond
	err := RunChild(t.Context(), opts, deps{Clock: fixture.clock, Runner: fixture.runner}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for child result") {
		t.Fatalf("expected timeout, got %v", err)
	}
	state := fixture.loadState(t)
	if state.ActiveChild == nil || state.ActiveChild.ResultPath == "" || state.ActiveChild.OutputPath == "" {
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
	return ChildRun{ID: req.ID, Pane: TmuxPane{ID: "%9"}, OutputPath: req.OutputPath, ResultPath: req.ResultPath}, nil
}

func (f *fakeChildRunner) Wait(ctx context.Context, run ChildRun) (ChildRunResult, error) {
	if f.writeResult {
		if err := os.WriteFile(run.ResultPath, []byte("result"), 0o644); err != nil {
			return ChildRunResult{}, err
		}
	}
	return ChildRunResult{ID: run.ID, OutputPath: run.OutputPath, ResultPath: run.ResultPath}, nil
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
