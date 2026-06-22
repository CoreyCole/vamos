package qrspicmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

type fakeCommandRunner struct {
	results map[string]CommandResult
	errs    map[string]error
}

func (f fakeCommandRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	return f.results[key], f.errs[key]
}

func TestCheckPiCompatibilityAcceptsCurrentChildFlags(t *testing.T) {
	runner := fakeCommandRunner{results: map[string]CommandResult{
		"pi --help":    {Stdout: "Usage:\n  --session-id <id>\n  --session-dir <dir>\n  --name <name>\n  --extension <path>\n"},
		"pi --version": {Stdout: "pi 1.2.3\n"},
	}, errs: map[string]error{}}
	report, err := CheckPiCompatibility(t.Context(), PiCompatibilityRequest{UsesExtension: true}, runner)
	if err != nil {
		t.Fatalf("CheckPiCompatibility error = %v", err)
	}
	if !report.OK || report.Version != "pi 1.2.3" || len(report.Problems) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestCheckPiCompatibilityReportsMissingFlags(t *testing.T) {
	runner := fakeCommandRunner{results: map[string]CommandResult{
		"pi --help": {Stdout: "Usage:\n  --session-dir <dir>\n  --name <name>\n"},
	}, errs: map[string]error{}}
	report, err := CheckPiCompatibility(t.Context(), PiCompatibilityRequest{}, runner)
	if err != nil {
		t.Fatalf("CheckPiCompatibility error = %v", err)
	}
	if report.OK || len(report.Problems) == 0 {
		t.Fatalf("report = %+v, want missing flag", report)
	}
	found := false
	for _, problem := range report.Problems {
		found = found || strings.Contains(problem.Evidence, "--session-id")
	}
	if !found {
		t.Fatalf("problems = %+v, want --session-id evidence", report.Problems)
	}
}

func TestRunDoctorTextIncludesSafeCommand(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "doctor-state.json")
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, ManagerRunID: "run", SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodePlan, nil)}
	saveManagerState(t, stateFile, state)
	runner := fakeCommandRunner{results: map[string]CommandResult{
		"pi --help":    {Stdout: "--session-id\n--session-dir\n--name\n"},
		"pi --version": {Stdout: "pi test\n"},
	}, errs: map[string]error{}}
	out, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, CommandRunner: runner}, "doctor", "--state-file", stateFile)
	if err != nil {
		t.Fatalf("doctor command error = %v", err)
	}
	for _, want := range []string{"pi: ok", "state root: ok", "safe command:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %q", want, out)
		}
	}
}

func TestStartNextInitialPreflightFailureDoesNotCreateLockOrState(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := fakeCommandRunner{results: map[string]CommandResult{
		"pi --help": {Stdout: "--session-dir\n--name\n"},
	}, errs: map[string]error{}}
	child := &fakeChildRunner{}
	out, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, CommandRunner: runner, Runner: child}, "start-next", "--plan-dir", fixture.planDir, "--project-root", fixture.projectRoot)
	if err != nil {
		t.Fatalf("start-next command error = %v", err)
	}
	if !strings.Contains(out, "action: pi_compatibility_failed") || !strings.Contains(out, "--session-id") {
		t.Fatalf("output = %q", out)
	}
	if len(child.started) != 0 {
		t.Fatalf("started = %d, want 0", len(child.started))
	}
	matches, err := filepath.Glob(filepath.Join(fixture.stateRoot, "**", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("state files = %v, want none", matches)
	}
}

func TestStartNextExistingStatePreflightFailureDoesNotMutateActiveChild(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "existing-state.json")
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, ManagerRunID: "run", SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodePlan, nil)}
	saveManagerState(t, stateFile, state)
	runner := fakeCommandRunner{results: map[string]CommandResult{
		"pi --help": {Stdout: "--session-dir\n--name\n"},
	}, errs: map[string]error{"pi --version": errors.New("no version")}}
	child := &fakeChildRunner{}
	out, err := executeManagerCommand(deps{Clock: fixture.clock, CommandRunner: runner, Runner: child}, "start-next", "--state-file", stateFile)
	if err != nil {
		t.Fatalf("start-next command error = %v", err)
	}
	if !strings.Contains(out, "action: pi_compatibility_failed") {
		t.Fatalf("output = %q", out)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild != nil || loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionPiCompatibilityFailed {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(child.started) != 0 {
		t.Fatalf("started = %d, want 0", len(child.started))
	}
}
