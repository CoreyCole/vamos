package pickleball

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/generatedgo"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

type fakeAIGenerator struct {
	calls []AIGenerateInput
	err   error
}

func (f *fakeAIGenerator) ApplyPrompt(_ context.Context, input AIGenerateInput) error {
	f.calls = append(f.calls, input)
	return f.err
}

type fakeRunner struct {
	calls []generatedgo.RunnerInput
	err   error
}

func (f *fakeRunner) BuildAndRun(_ context.Context, input generatedgo.RunnerInput) (generatedgo.RunnerResult, error) {
	f.calls = append(f.calls, input)
	writeTestFile := func(name, content string) {
		if err := os.WriteFile(filepath.Join(input.OutputDir, name), []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	writeTestFile("app.html", "<h1>Generated</h1>")
	writeTestFile("results.csv", "court,team_a,team_b,reason\n1,A+B,C+D,prompted\n")
	writeTestFile("manifest.json", `{"schema_version":1,"build_id":"`+input.EnvAllowlist["VAMOS_GENERATED_BUILD_ID"]+`","mode":"one_shot","prompt_summary":"Generated prompt","artifacts":{"html":"app.html","csv":"results.csv"}}`)
	return generatedgo.RunnerResult{
		Status:     generatedgo.BuildStatusSucceeded,
		Manifest:   generatedgo.GeneratedManifest{SchemaVersion: 1, BuildID: input.EnvAllowlist["VAMOS_GENERATED_BUILD_ID"], Mode: generatedgo.RunnerModeOneShot, PromptSummary: "Generated prompt", Artifacts: generatedgo.GeneratedArtifacts{HTML: "app.html", CSV: "results.csv"}},
		SourceHash: "sha256:source",
		ArtifactHashes: map[string]string{
			"app.html":      "sha256:html",
			"results.csv":   "sha256:csv",
			"manifest.json": "sha256:manifest",
		},
	}, f.err
}

func TestSelfModifyWorkflowRunsEditThenBuildActivities(t *testing.T) {
	t.Parallel()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	var editCalls, buildCalls int
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput) error {
		editCalls++
		return nil
	}, activity.RegisterOptions{Name: ActivityPickleballRunAIEdits})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput) error {
		buildCalls++
		return nil
	}, activity.RegisterOptions{Name: ActivityPickleballBuildSnapshot})

	env.ExecuteWorkflow(SelfModifyWorkflow, SelfModifyWorkflowInput{SessionID: "default", Prompt: "Add skill totals"})

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow completed=%v err=%v", env.IsWorkflowCompleted(), env.GetWorkflowError())
	}
	if editCalls != 1 || buildCalls != 1 {
		t.Fatalf("activity calls edit=%d build=%d", editCalls, buildCalls)
	}
}

func TestSelfModifyWorkflowActivityNamesMatchStructRegistration(t *testing.T) {
	t.Parallel()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivity(&SelfModifyActivities{})

	env.ExecuteWorkflow(SelfModifyWorkflow, SelfModifyWorkflowInput{SessionID: "default", Prompt: "Add skill totals"})

	err := env.GetWorkflowError()
	if err == nil || !strings.Contains(err.Error(), "pickleball service is required") {
		t.Fatalf("workflow error = %v; activity names may not match struct registration", err)
	}
}

func TestSelfModifyActivitiesEditAndPromoteSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ai := &fakeAIGenerator{}
	runner := &fakeRunner{}
	svc := newWorkflowTestService(t, ai, runner)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	activities := &SelfModifyActivities{Service: svc}
	input := SelfModifyWorkflowInput{SessionID: session.ID, Prompt: "Add a CSV column explaining skill totals", UserEmail: "player@example.com"}

	if err := activities.RunAIEdits(ctx, input); err != nil {
		t.Fatalf("RunAIEdits: %v", err)
	}
	if len(ai.calls) != 1 || !strings.Contains(ai.calls[0].Prompt, "CSV column") || ai.calls[0].WorkspacePath != session.WorkspacePath {
		t.Fatalf("ai calls = %+v", ai.calls)
	}
	if err := activities.BuildAndSnapshot(ctx, input); err != nil {
		t.Fatalf("BuildAndSnapshot: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].WorkspaceDir != session.WorkspacePath {
		t.Fatalf("runner calls = %+v", runner.calls)
	}
	vm, err := svc.GetState(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if vm.State != AppStateSucceeded || vm.Current == nil || vm.LastGood == nil {
		t.Fatalf("vm = %+v", vm)
	}
	for _, rel := range []string{"app.html", "results.csv", "manifest.json", filepath.Join("source", "main.go")} {
		if _, err := os.Stat(filepath.Join(svc.store.Root(), "sessions", session.ID, "snapshots", vm.Current.BuildID, rel)); err != nil {
			t.Fatalf("snapshot file %s missing: %v", rel, err)
		}
	}
}

func TestBuildAndSnapshotFailurePreservesLastGood(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runner := &fakeRunner{err: os.ErrInvalid}
	svc := newWorkflowTestService(t, &fakeAIGenerator{}, runner)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	lastGood := BuildSnapshot{BuildID: "last-good", Status: "succeeded", HTMLThoughtsPath: "creative-mode-agent/examples/pickleball/sessions/player/snapshots/last-good/app.html", CreatedAt: time.Now().UTC()}
	if err := svc.PromoteSnapshot(ctx, session.ID, lastGood); err != nil {
		t.Fatalf("PromoteSnapshot: %v", err)
	}
	activities := &SelfModifyActivities{Service: svc}
	if err := activities.BuildAndSnapshot(ctx, SelfModifyWorkflowInput{SessionID: session.ID, Prompt: "break it"}); err == nil {
		t.Fatal("BuildAndSnapshot error = nil")
	}
	vm, err := svc.GetState(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if vm.State != AppStateFailed || vm.LastGood == nil || vm.LastGood.BuildID != "last-good" {
		t.Fatalf("vm = %+v", vm)
	}
}

func newWorkflowTestService(t *testing.T, ai AIGenerator, runner Runner) *Service {
	t.Helper()
	seedDir := filepath.Join(t.TempDir(), "seed")
	for name, content := range map[string]string{
		"go.mod":      "module seed\n\ngo 1.24\n",
		"main.go":     "package main\nfunc main(){}\n",
		"players.csv": "name,skill\nAvery,5\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(seedDir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(Options{ThoughtsRoot: t.TempDir(), SeedBundleDir: seedDir, AIGenerator: ai, Runner: runner, WorkflowStarter: &fakeWorkflowStarter{}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
