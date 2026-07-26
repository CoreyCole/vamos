package qrspicmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		"bad attempt":    func(s *LaneSpec) { s.Attempt = 3 },
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
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneRunning},
	); err != nil {
		t.Fatal(err)
	}
	got, err = ReadLaneRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != LaneSuccess {
		t.Fatalf("terminal record replaced with %s", got.State)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLaneRecord(path); err == nil {
		t.Fatal("expected malformed record error")
	}
}

type fakeLaneProcess struct{ pid int }

func (p fakeLaneProcess) PID() int { return p.pid }

type fakeLaneProcessRunner struct {
	command   []string
	exit      LaneExit
	waitErr   error
	cancelled bool
}

func (r *fakeLaneProcessRunner) Start(
	_ context.Context,
	command []string,
	_ string,
) (LaneProcess, error) {
	r.command = command
	return fakeLaneProcess{pid: 42}, nil
}

func (r *fakeLaneProcessRunner) Wait(_ context.Context, _ LaneProcess) (LaneExit, error) {
	return r.exit, r.waitErr
}

func (r *fakeLaneProcessRunner) Cancel(_ context.Context, _ LaneProcess) error {
	r.cancelled = true
	return nil
}

func TestBuildLaneCommandIsNonInteractive(t *testing.T) {
	command := strings.Join(BuildLaneCommand(testLaneSpec(t)), " ")
	for _, want := range []string{"pi --print --no-extensions", "--session-id", "--session-dir", "--name", "non-interactive", "read-only"} {
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

func TestDetachedLaneRunnerPersistsTerminalState(t *testing.T) {
	spec := testLaneSpec(t)
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
