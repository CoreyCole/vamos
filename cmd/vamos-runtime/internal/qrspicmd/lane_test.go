package qrspicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testLaneSpec(t *testing.T) LaneSpec {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"review", "sessions", "prompts"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prompt := filepath.Join(root, "prompts", "lane.md")
	if err := os.WriteFile(prompt, []byte("review"), 0o644); err != nil {
		t.Fatal(err)
	}
	return LaneSpec{
		ID:             "scout",
		CoordinatorID:  "child-1",
		CoordinatorGen: 1,
		Role:           LaneRoleReviewScout,
		PromptFile:     prompt,
		Cwd:            root,
		PlanDir:        root,
		ReviewDir:      filepath.Join(root, "review"),
		ReportPath:     filepath.Join(root, "review", "scout.md"),
		SessionID:      "session-1",
		SessionDir:     filepath.Join(root, "sessions"),
		Attempt:        1,
		Timeout:        time.Minute,
	}
}

func TestLaneSpec(t *testing.T) {
	spec := testLaneSpec(t)
	if err := ValidateLaneSpec(spec); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*LaneSpec){
		"unsafe ID":      func(s *LaneSpec) { s.ID = "../escape" },
		"unknown role":   func(s *LaneSpec) { s.Role = "writer" },
		"bad attempt":    func(s *LaneSpec) { s.Attempt = 2 },
		"outside report": func(s *LaneSpec) { s.ReportPath = filepath.Join(t.TempDir(), "report.md") },
		"zero timeout":   func(s *LaneSpec) { s.Timeout = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			copy := spec
			mutate(&copy)
			if err := ValidateLaneSpec(copy); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	duplicate := spec
	duplicate.ID, duplicate.SessionID = "reviewer", "session-2"
	if err := ValidateLaneSpecs(
		[]LaneSpec{spec, duplicate},
	); err == nil ||
		!strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate result: %v", err)
	}
}

func TestLaneSpecRejectsSymlinkEscape(t *testing.T) {
	spec := testLaneSpec(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(spec.ReviewDir, "escape")); err != nil {
		t.Fatal(err)
	}
	spec.ReportPath = filepath.Join(spec.ReviewDir, "escape", "report.md")
	if err := ValidateLaneSpec(spec); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}

func TestLaneRecord(t *testing.T) {
	spec := testLaneSpec(t)
	path := LaneRecordPath(spec)
	running := LaneRecord{
		Spec:      spec,
		State:     LaneRunning,
		ErrorTail: strings.Repeat("x", maxLaneDiagnosticBytes+1),
	}
	if err := WriteLaneRecord(path, running); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLaneRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ErrorTail) != maxLaneDiagnosticBytes {
		t.Fatalf("tail length = %d", len(got.ErrorTail))
	}
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneSuccess},
	); err != nil {
		t.Fatal(err)
	}
	for _, state := range []LaneState{LaneQueued, LaneRunning, LaneSuccess, LaneFailed} {
		if err := WriteLaneRecord(
			path,
			LaneRecord{Spec: spec, State: state},
		); err != nil {
			t.Fatal(err)
		}
		got, err = ReadLaneRecord(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != LaneSuccess {
			t.Fatalf("terminal success replaced with %s", got.State)
		}
	}
	failed := testLaneSpec(t)
	failedPath := LaneRecordPath(failed)
	if err := WriteLaneRecord(
		failedPath,
		LaneRecord{Spec: failed, State: LaneFailed},
	); err != nil {
		t.Fatal(err)
	}
	if err := WriteLaneRecord(
		failedPath,
		LaneRecord{Spec: failed, State: LaneSuccess},
	); err != nil {
		t.Fatal(err)
	}
	got, err = ReadLaneRecord(failedPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != LaneFailed {
		t.Fatalf("terminal failure replaced with %s", got.State)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLaneRecord(path); err == nil {
		t.Fatal("expected malformed record error")
	}
}

type fakeLaneProcess struct {
	pid    int
	runner *fakeLaneProcessRunner
}

func (p *fakeLaneProcess) PID() int { return p.pid }

func (p *fakeLaneProcess) Terminate(_ context.Context, _ time.Duration) error {
	p.runner.terminated = true
	p.runner.order = append(p.runner.order, "terminate")
	return p.runner.terminateErr
}

func (p *fakeLaneProcess) Wait(ctx context.Context) (LaneExit, error) {
	p.runner.waitCalls++
	if p.runner.wait != nil {
		return p.runner.wait(ctx, p.runner.waitCalls)
	}
	return p.runner.exit, p.runner.waitErr
}

type fakeLaneProcessRunner struct {
	command      []string
	exit         LaneExit
	waitErr      error
	terminateErr error
	terminated   bool
	waitCalls    int
	order        []string
	wait         func(context.Context, int) (LaneExit, error)
}

func (r *fakeLaneProcessRunner) Start(
	_ context.Context,
	command []string,
	_ string,
) (LaneProcess, error) {
	r.command = command
	return &fakeLaneProcess{pid: 42, runner: r}, nil
}

func TestBuildLaneCommandIsNonInteractive(t *testing.T) {
	command := strings.Join(BuildLaneCommand(testLaneSpec(t)), " ")
	for _, want := range []string{"pi --print --no-extensions", "--session-id", "--session-dir", "--name", `-- "$(cat "$PROMPT_FILE")`, "non-interactive", "read-only"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
	for _, forbidden := range []string{"tmux", "Q_MANAGER_", "child-complete", "--extension", "wake", "result-record"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("command exposes %q: %s", forbidden, command)
		}
	}
}

type selectiveLaneProcess struct {
	failed bool
}

func (p *selectiveLaneProcess) PID() int { return 42 }

func (p *selectiveLaneProcess) Terminate(
	context.Context,
	time.Duration,
) error {
	return nil
}

func (p *selectiveLaneProcess) Wait(context.Context) (LaneExit, error) {
	if p.failed {
		return LaneExit{ExitCode: 1}, errors.New("lane failed")
	}
	return LaneExit{}, nil
}

type selectiveLaneProcessRunner struct{}

func (selectiveLaneProcessRunner) Start(
	_ context.Context,
	command []string,
	_ string,
) (LaneProcess, error) {
	return &selectiveLaneProcess{
		failed: strings.Contains(strings.Join(command, "\x00"), "/failed/"),
	}, nil
}

func TestDetachedLaneRunnerPersistsTerminalState(t *testing.T) {
	spec := testLaneSpec(t)
	if err := os.WriteFile(spec.ReportPath, []byte("# report"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeLaneProcessRunner{exit: LaneExit{ExitCode: 0}}
	runner := &DetachedLaneRunner{ProcessRunner: fake}
	record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != LaneRunning || record.PID != 42 {
		t.Fatalf("start record = %#v", record)
	}
	record, err = runner.Wait(context.Background(), record)
	if err != nil || record.State != LaneSuccess {
		t.Fatalf("wait = %#v, %v", record, err)
	}
	persisted, err := ReadLaneRecord(LaneRecordPath(spec))
	if err != nil || persisted.State != LaneSuccess {
		t.Fatalf("persisted = %#v, %v", persisted, err)
	}
}

func TestDetachedLaneRunnerFailureIsTerminal(t *testing.T) {
	spec := testLaneSpec(t)
	fake := &fakeLaneProcessRunner{waitErr: errors.New("stopped")}
	runner := &DetachedLaneRunner{ProcessRunner: fake}
	record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	record, err = runner.Wait(context.Background(), record)
	if err == nil || record.State != LaneFailed ||
		!strings.Contains(record.ErrorTail, "stopped") {
		t.Fatalf("wait = %#v, %v", record, err)
	}
}

func TestDetachedLaneRunnerTerminatesBeforeReapAndFailureRecord(t *testing.T) {
	spec := testLaneSpec(t)
	spec.Timeout = time.Millisecond
	fake := &fakeLaneProcessRunner{}
	fake.wait = func(ctx context.Context, call int) (LaneExit, error) {
		switch call {
		case 1:
			<-ctx.Done()
			fake.order = append(fake.order, "timeout")
			return LaneExit{}, ctx.Err()
		case 2:
			fake.order = append(fake.order, "reap")
			return LaneExit{ExitCode: 143}, nil
		default:
			t.Fatalf("unexpected wait call %d", call)
			return LaneExit{}, nil
		}
	}
	runner := &DetachedLaneRunner{ProcessRunner: fake}
	record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	record, err = runner.Wait(context.Background(), record)
	if err == nil || record.State != LaneFailed {
		t.Fatalf("wait = %#v, %v", record, err)
	}
	if got, want := strings.Join(fake.order, ","), "timeout,terminate,reap"; got != want {
		t.Fatalf("lifecycle order = %q, want %q", got, want)
	}
	persisted, readErr := ReadLaneRecord(LaneRecordPath(spec))
	if readErr != nil || persisted.State != LaneFailed {
		t.Fatalf("persisted = %#v, %v", persisted, readErr)
	}
}

func TestDetachedLaneRunnerRejectsSuccessfulExitWithoutReport(t *testing.T) {
	spec := testLaneSpec(t)
	runner := &DetachedLaneRunner{ProcessRunner: &fakeLaneProcessRunner{exit: LaneExit{}}}
	record, err := runner.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	record, err = runner.Wait(context.Background(), record)
	if err == nil || record.State != LaneFailed ||
		!strings.Contains(record.ErrorTail, "no such file") {
		t.Fatalf("wait = %#v, %v", record, err)
	}
}

type fakeLaneRunner struct {
	mu       sync.Mutex
	starts   []LaneSpec
	attempts map[string]int
}

func (r *fakeLaneRunner) Start(_ context.Context, spec LaneSpec) (LaneRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, spec)
	if r.attempts == nil {
		r.attempts = map[string]int{}
	}
	r.attempts[spec.ID]++
	return LaneRecord{Spec: spec, State: LaneRunning}, nil
}

func (r *fakeLaneRunner) Wait(_ context.Context, record LaneRecord) (LaneRecord, error) {
	if record.Spec.ID == "failed" {
		record.State = LaneFailed
		record.ErrorTail = "lane failed"
		return record, errors.New("lane failed")
	}
	if err := os.WriteFile(
		record.Spec.ReportPath,
		[]byte("# report"),
		0o644,
	); err != nil {
		return record, err
	}
	record.State = LaneSuccess
	return record, nil
}

func TestLaneCoordinatorContinuesAfterLaneFailure(t *testing.T) {
	failed := testLaneSpec(t)
	failed.ID, failed.SessionID = "failed", "session-failed"
	active := failed
	active.ID, active.SessionID = "active", "session-active"
	active.ReportPath = filepath.Join(active.ReviewDir, "active.md")
	queued := failed
	queued.ID, queued.SessionID = "queued", "session-queued"
	queued.ReportPath = filepath.Join(queued.ReviewDir, "queued.md")
	runner := &fakeLaneRunner{}
	records, err := (LaneCoordinator{Runner: runner, MaxParallel: 2}).Run(
		context.Background(),
		[]LaneSpec{failed, active, queued},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].State != LaneFailed ||
		records[1].State != LaneSuccess || records[2].State != LaneSuccess {
		t.Fatalf("records = %#v", records)
	}
	for _, id := range []string{"failed", "active", "queued"} {
		if runner.attempts[id] != 1 {
			t.Fatalf("attempts = %#v", runner.attempts)
		}
	}
	data, err := os.ReadFile(LaneFailureLogPath(failed.PlanDir, failed.CoordinatorID))
	if err != nil {
		t.Fatal(err)
	}
	var event LaneFailureEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatal(err)
	}
	if event.LaneID != failed.ID || event.Attempt != failed.Attempt ||
		event.RecordPath != LaneRecordPath(failed) ||
		event.OutputPath != LaneDiagnosticOutputPath(failed) {
		t.Fatalf("failure event = %#v", event)
	}
}

func TestRunLaneReturnsDegradedBatchResult(t *testing.T) {
	failed := testLaneSpec(t)
	failed.ID, failed.SessionID = "failed", "session-failed"
	good := failed
	good.ID, good.SessionID = "good", "session-good"
	good.ReportPath = filepath.Join(good.ReviewDir, "good.md")
	if err := os.WriteFile(good.ReportPath, []byte("# report"), 0o644); err != nil {
		t.Fatal(err)
	}
	specsPath := filepath.Join(t.TempDir(), "specs.json")
	data, err := json.Marshal([]LaneSpec{failed, good})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	if err := RunLane(context.Background(), LaneRunOptions{
		SpecsFile:   specsPath,
		MaxParallel: 2,
	}, laneDependencies{LaneProcessRunner: selectiveLaneProcessRunner{}}, out); err != nil {
		t.Fatal(err)
	}
	var result LaneRunResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 || result.Records[0].State != LaneFailed ||
		result.Records[1].State != LaneSuccess {
		t.Fatalf("records = %#v", result.Records)
	}
	if got, want := result.Reports, []string{
		good.ReportPath,
	}; len(got) != 1 ||
		got[0] != want[0] {
		t.Fatalf("reports = %#v, want %#v", got, want)
	}
}

func TestLaneCoordinatorValidatesBeforeStart(t *testing.T) {
	good := testLaneSpec(t)
	bad := good
	bad.ID = "../bad"
	runner := &fakeLaneRunner{}
	if _, err := (LaneCoordinator{Runner: runner, MaxParallel: 1}).Run(
		context.Background(),
		[]LaneSpec{good, bad},
	); err == nil {
		t.Fatal("expected validation error")
	}
	if len(runner.starts) != 0 {
		t.Fatalf("started invalid set: %#v", runner.starts)
	}
}

func TestInspectLaneRecordRejectsUnsafePaths(t *testing.T) {
	spec := testLaneSpec(t)
	path := LaneRecordPath(spec)
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneSuccess},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLaneRecord(spec.PlanDir, path); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLaneRecord(
		spec.PlanDir,
		filepath.Join(t.TempDir(), "record.json"),
	); err == nil {
		t.Fatal("expected outside path rejection")
	}
	if _, err := InspectLaneRecord(spec.PlanDir, filepath.Dir(path)); err == nil {
		t.Fatal("expected directory rejection")
	}
	link := filepath.Join(filepath.Dir(path), "linked.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLaneRecord(spec.PlanDir, link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestLaneDiagnosticsStaySeparateFromReports(t *testing.T) {
	spec := testLaneSpec(t)
	diagnostic := LaneDiagnosticOutputPath(spec)
	if err := requireContained(spec.PlanDir, diagnostic); err != nil {
		t.Fatalf("diagnostic path = %q: %v", diagnostic, err)
	}
	if err := requireContained(filepath.Dir(diagnostic), spec.ReportPath); err == nil {
		t.Fatalf(
			"report path %q is inside diagnostic tree %q",
			spec.ReportPath,
			diagnostic,
		)
	}
	if err := os.MkdirAll(filepath.Dir(diagnostic), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diagnostic, []byte("full lane output"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := LaneRecordPath(spec)
	if err := WriteLaneRecord(path, LaneRecord{
		Spec:       spec,
		State:      LaneFailed,
		OutputPath: diagnostic,
		ErrorTail:  "bounded summary",
	}); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	if err := RunLaneStatus(
		LaneStatusOptions{PlanDir: spec.PlanDir, Record: path},
		out,
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "full lane output") {
		t.Fatalf("status read full diagnostic output: %s", out.String())
	}
}

func TestLaneStatusIsReadOnly(t *testing.T) {
	spec := testLaneSpec(t)
	path := LaneRecordPath(spec)
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneFailed, ErrorTail: "failure"},
	); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	if err := RunLaneStatus(
		LaneStatusOptions{PlanDir: spec.PlanDir, Record: path},
		out,
	); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"retry", "terminate", "attach", "wake", "result", "graph", "workspace"} {
		if strings.Contains(strings.ToLower(out.String()), forbidden) {
			t.Fatalf("status output exposes %q: %s", forbidden, out.String())
		}
	}
}

func TestLaneReport(t *testing.T) {
	spec := testLaneSpec(t)
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected missing report error")
	}
	if err := os.Mkdir(spec.ReportPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected directory report error")
	}
	if err := os.Remove(spec.ReportPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.ReportPath, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected empty report error")
	}
	if err := os.WriteFile(
		spec.ReportPath,
		[]byte("# Findings\n\nSafe."),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err != nil {
		t.Fatal(err)
	}
	spec.ReportPath = filepath.Join(spec.ReviewDir, "not-markdown.txt")
	if err := os.WriteFile(spec.ReportPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected extension error")
	}
}
