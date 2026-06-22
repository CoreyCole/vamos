package pickleball

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

type fakeAppletEditor struct {
	calls []AppletEditInput
	err   error
}

func (f *fakeAppletEditor) ApplyPrompt(_ context.Context, input AppletEditInput) (AppletEditResult, error) {
	f.calls = append(f.calls, input)
	if f.err != nil {
		return AppletEditResult{FailureUserMessage: "I couldn't make that change safely. Your app is unchanged."}, f.err
	}
	if err := os.WriteFile(filepath.Join(input.IterationDir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		return AppletEditResult{}, err
	}
	return AppletEditResult{ChangedFiles: []string{"main.go"}, UserSummary: "Done — I updated the schedule."}, nil
}

type fakeAppletRuntime struct {
	starts []appletruntime.RuntimeConfig
	err    error
}

func (f *fakeAppletRuntime) Start(_ context.Context, cfg appletruntime.RuntimeConfig) (appletruntime.ProcessState, error) {
	f.starts = append(f.starts, cfg)
	if f.err != nil {
		return appletruntime.ProcessState{}, f.err
	}
	return appletruntime.ProcessState{AppID: cfg.AppID, SourceDir: cfg.SourceDir, BaseURL: "http://127.0.0.1:1", Healthy: true}, nil
}

func (f *fakeAppletRuntime) Stop(context.Context, string) error { return nil }
func (f *fakeAppletRuntime) Health(context.Context, string) (appletruntime.ProcessState, error) {
	return appletruntime.ProcessState{Healthy: true}, nil
}
func (f *fakeAppletRuntime) ProxyTarget(string) (string, bool) { return "http://127.0.0.1:1", true }

func TestSelfModifyWorkflowRunsIterationActivities(t *testing.T) {
	t.Parallel()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	var calls []string
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput) (IterationSpec, error) {
		calls = append(calls, "create")
		return IterationSpec{IterationID: "iter", SourceDir: "/tmp/iter", FilesRoot: "/tmp/files"}, nil
	}, activity.RegisterOptions{Name: ActivityPickleballCreateIteration})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput, IterationSpec) (AppletEditResult, error) {
		calls = append(calls, "edit")
		return AppletEditResult{UserSummary: "Done"}, nil
	}, activity.RegisterOptions{Name: ActivityPickleballRunAppletEdit})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput, IterationSpec) error {
		calls = append(calls, "build")
		return nil
	}, activity.RegisterOptions{Name: ActivityPickleballBuildIteration})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput, IterationSpec) (appletruntime.ProcessState, error) {
		calls = append(calls, "start")
		return appletruntime.ProcessState{Healthy: true}, nil
	}, activity.RegisterOptions{Name: ActivityPickleballStartIteration})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput, IterationSpec, AppletEditResult, appletruntime.ProcessState) error {
		calls = append(calls, "promote")
		return nil
	}, activity.RegisterOptions{Name: ActivityPickleballPromoteIteration})
	env.RegisterActivityWithOptions(func(SelfModifyWorkflowInput, string) error {
		calls = append(calls, "fail")
		return nil
	}, activity.RegisterOptions{Name: ActivityPickleballFailIteration})

	env.ExecuteWorkflow(SelfModifyWorkflow, SelfModifyWorkflowInput{SessionID: "default", Prompt: "Add skill totals"})

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow completed=%v err=%v", env.IsWorkflowCompleted(), env.GetWorkflowError())
	}
	if got := strings.Join(calls, ","); got != "create,edit,build,start,promote" {
		t.Fatalf("activity order = %s", got)
	}
}

func TestIterationActivitiesPromoteHiddenAppletAndPreserveCurrentOnFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	editor := &fakeAppletEditor{}
	runtime := &fakeAppletRuntime{}
	svc := newWorkflowTestService(t, editor, runtime)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	activities := &SelfModifyActivities{Service: svc}
	input := SelfModifyWorkflowInput{SessionID: session.ID, Prompt: "Make schedule prettier", UserEmail: "player@example.com"}

	spec, err := activities.CreateIteration(ctx, input)
	if err != nil {
		t.Fatalf("CreateIteration: %v", err)
	}
	if !strings.Contains(spec.SourceDir, filepath.Join("apps", "iterations")) {
		t.Fatalf("iteration should be hidden under apps/iterations: %+v", spec)
	}
	edit, err := activities.RunAppletEdit(ctx, input, spec)
	if err != nil {
		t.Fatalf("RunAppletEdit: %v", err)
	}
	if len(editor.calls) != 1 || editor.calls[0].IterationDir != spec.SourceDir {
		t.Fatalf("editor calls = %+v", editor.calls)
	}
	if err := activities.BuildIteration(ctx, input, spec); err != nil {
		t.Fatalf("BuildIteration: %v", err)
	}
	proc, err := activities.StartIteration(ctx, input, spec)
	if err != nil {
		t.Fatalf("StartIteration: %v", err)
	}
	if len(runtime.starts) != 1 || runtime.starts[0].SourceDir != spec.SourceDir {
		t.Fatalf("runtime starts = %+v", runtime.starts)
	}
	if err := activities.PromoteIteration(ctx, input, spec, edit, proc); err != nil {
		t.Fatalf("PromoteIteration: %v", err)
	}
	vm, err := svc.GetState(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if vm.State != AppStateSucceeded || vm.CurrentApplet != spec.IterationID || !strings.Contains(vm.UserMessage, "updated") {
		t.Fatalf("vm = %+v", vm)
	}

	if err := activities.FailIteration(ctx, input, "compiler exploded at /tmp/hidden"); err != nil {
		t.Fatalf("FailIteration: %v", err)
	}
	failed, err := svc.GetState(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if failed.State != AppStateFailed || failed.CurrentApplet != spec.IterationID || failed.UserMessage != "I couldn't make that change safely. Your app is unchanged." {
		t.Fatalf("failed vm = %+v", failed)
	}
}

func TestPromptPatchGeneratorUpdatesRepeatedPrompts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	seed := `package main

type Matchup struct { Reason string }
type Manifest struct { PromptSummary string }

func GenerateMatchups() []Matchup {
	return []Matchup{{Reason: "Balanced total skill by pairing high+low."}}
}

func WriteManifest() Manifest {
	return Manifest{PromptSummary: "Seed balanced matchup generator"}
}
`
	if err := os.WriteFile(mainPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	generator := PromptPatchGenerator{}
	if err := generator.ApplyPrompt(ctx, AIGenerateInput{WorkspacePath: dir, Prompt: "Add skill totals"}); err != nil {
		t.Fatalf("ApplyPrompt first: %v", err)
	}
	if err := generator.ApplyPrompt(ctx, AIGenerateInput{WorkspacePath: dir, Prompt: "Prioritize partners"}); err != nil {
		t.Fatalf("ApplyPrompt second: %v", err)
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{"Prioritize partners", "fresh partner pairings"} {
		if !strings.Contains(source, want) {
			t.Fatalf("patched source missing %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "Add skill totals") || strings.Contains(source, "Seed balanced") {
		t.Fatalf("old prompt text remained after second patch:\n%s", source)
	}
}

func newWorkflowTestService(t *testing.T, editor AppletEditor, runtime appletruntime.Manager) *Service {
	t.Helper()
	root := t.TempDir()
	filesRoot := filepath.Join(root, "files")
	current := filepath.Join(filesRoot, "apps", "current")
	for name, content := range map[string]string{
		"go.mod":  "module current\n\ngo 1.24\n",
		"main.go": "package main\nfunc main(){}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(current, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(current, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seedDir := filepath.Join(root, "seed")
	if err := copyDir(current, seedDir); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ThoughtsRoot: filepath.Join(root, "thoughts"), SeedBundleDir: seedDir, FilesRoot: filesRoot, CurrentAppDir: current, IterationsDir: filepath.Join(filesRoot, "apps", "iterations"), AppletEditor: editor, AppletRuntime: runtime, WorkflowStarter: &fakeWorkflowStarter{}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
