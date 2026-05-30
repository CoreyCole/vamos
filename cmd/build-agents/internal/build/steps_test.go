package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildStepsCommandSpecs(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	steps := BuildSteps(Options{RepoRoot: repoRoot, BinaryName: "agents-server"})
	byName := map[StepName]Step{}
	for _, step := range steps {
		byName[step.Name] = step
	}

	assertArgs(t, byName[StepProto].Command, []string{"buf", "generate"})
	assertArgs(t, byName[StepSQLC].Command, []string{"sqlc", "generate"})
	assertArgs(t, byName[StepTempl].Command, []string{"templ", "generate"})
	assertArgs(
		t,
		byName[StepGo].Command,
		[]string{"go", "build", "-o", "agents-server", "./cmd/server"},
	)
	if got, want := byName[StepGo].Command.Env["GOCACHE"], filepath.Join(
		repoRoot,
		".build-agents",
		"go-build-cache",
	); got != want {
		t.Fatalf("Go GOCACHE = %q, want %q", got, want)
	}
	if !filepath.IsAbs(byName[StepGo].Command.Env["GOCACHE"]) {
		t.Fatalf("Go GOCACHE is not absolute: %q", byName[StepGo].Command.Env["GOCACHE"])
	}
	if byName[StepTailwind].Command.Func == nil {
		t.Fatal("Tailwind command Func is nil")
	}
	if len(byName[StepTailwind].Command.Args) != 0 {
		t.Fatalf(
			"Tailwind command Args = %#v, want none",
			byName[StepTailwind].Command.Args,
		)
	}
	if got := byName[StepTSWorker].Command.Dir; got != "" {
		t.Fatalf("TS worker Dir = %q, want empty nested module cwd", got)
	}
	assertArgs(t, byName[StepTSWorker].Command, []string{"./node_modules/.bin/tsc"})
	if byName[StepDatastarAssets].Command.Func == nil {
		t.Fatal("Datastar assets command Func is nil")
	}
}

func TestBuildStepsUseStandaloneModulePaths(t *testing.T) {
	t.Parallel()

	steps := BuildSteps(Options{RepoRoot: t.TempDir(), BinaryName: "agents-server"})
	byName := map[StepName]Step{}
	for _, step := range steps {
		byName[step.Name] = step
	}

	assertContains(t, byName[StepSQLC].Inputs.Roots, "pkg/db/queries")
	assertContains(t, byName[StepSQLC].Inputs.Roots, "pkg/db/migrations/schema.sql")
	assertContains(
		t,
		byName[StepTSWorker].RestartOutputs.Roots,
		"dist/pkg/agents/temporal/workers/ts",
	)
	assertContains(t, byName[StepTSWorker].Inputs.Roots, "pkg/agents/temporal/workers/ts")
	assertContains(
		t,
		byName[StepTSWorker].RestartInputs.Roots,
		"pkg/agents/temporal/workers/ts",
	)
	assertContains(
		t,
		byName[StepTSWorker].RestartInputs.Includes,
		"pkg/agents/temporal/workers/ts/**/*.ts",
	)
	assertContains(
		t,
		byName[StepTSWorker].RestartInputs.Excludes,
		"pkg/agents/temporal/workers/ts/**/*.test.ts",
	)
	assertContains(t, byName[StepDatastarAssets].Inputs.Roots, "../datastar-pro/datastar-pro-v1.js")
}

func TestTailwindHashesCopiedDatastarUI(t *testing.T) {
	t.Parallel()

	steps := BuildSteps(Options{RepoRoot: t.TempDir(), BinaryName: "agents-server"})
	byName := map[StepName]Step{}
	for _, step := range steps {
		byName[step.Name] = step
	}

	tailwind := byName[StepTailwind]
	assertContains(t, tailwind.Inputs.Roots, "pkg/datastarui")
	assertContains(t, tailwind.Inputs.Includes, "pkg/datastarui/**/*.css")
	assertContains(t, tailwind.Inputs.Includes, "pkg/datastarui/**/*.go")
	assertContains(t, tailwind.Inputs.Includes, "pkg/datastarui/**/*.templ")
	assertContains(t, tailwind.Inputs.Includes, "pkg/datastarui/datastarui.lock.json")
	assertNotContains(t, byName[StepGo].Inputs.Excludes, "pkg/datastarui/**")
}

func TestBuildStepsUsesDatastarAssetEnvOverride(t *testing.T) {
	t.Setenv("VAMOS_DATASTAR_PRO_ASSET", "/opt/datastar/datastar-pro-v1.js")

	steps := BuildSteps(Options{RepoRoot: t.TempDir(), BinaryName: "agents-server"})
	byName := map[StepName]Step{}
	for _, step := range steps {
		byName[step.Name] = step
	}

	assertContains(t, byName[StepDatastarAssets].Inputs.Roots, "/opt/datastar/datastar-pro-v1.js")
}

func TestSyncDatastarAssetsReportsExactMissingPaths(t *testing.T) {
	repoRoot := t.TempDir()
	var stderr bytes.Buffer
	old := datastarAssetsWarningWriter
	datastarAssetsWarningWriter = &stderr
	t.Cleanup(func() { datastarAssetsWarningWriter = old })

	err := SyncDatastarAssets(DatastarAssetsOptions{
		RuntimeAsset: "static/js/datastar-pro-v1.js",
		HostAsset:    "../../datastar-pro/datastar-pro-v1.js",
	})(t.Context(), repoRoot, &fakeRunner{})
	if err != nil {
		t.Fatalf("SyncDatastarAssets: %v", err)
	}
	got := stderr.String()
	for _, want := range []string{"static/js/datastar-pro-v1.js", "../../datastar-pro/datastar-pro-v1.js"} {
		if !strings.Contains(got, want) {
			t.Fatalf("warning %q does not contain %q", got, want)
		}
	}
}

func TestSyncDatastarAssetsSilentWhenRuntimePresent(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "static", "js", "datastar-pro-v1.js"), "runtime")

	var stderr bytes.Buffer
	old := datastarAssetsWarningWriter
	datastarAssetsWarningWriter = &stderr
	t.Cleanup(func() { datastarAssetsWarningWriter = old })

	err := SyncDatastarAssets(DatastarAssetsOptions{
		RuntimeAsset: "static/js/datastar-pro-v1.js",
		HostAsset:    "../../datastar-pro/datastar-pro-v1.js",
	})(t.Context(), repoRoot, &fakeRunner{})
	if err != nil {
		t.Fatalf("SyncDatastarAssets: %v", err)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("warning = %q, want empty", got)
	}
}

func TestSyncDatastarAssetsCopiesHostAsset(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "host", "pkg", "agents")
	hostAsset := filepath.Clean(
		filepath.Join(repoRoot, "..", "..", "datastar-pro", "datastar-pro-v1.js"),
	)
	writeFile(t, hostAsset, "pro bundle")

	err := SyncDatastarAssets(DatastarAssetsOptions{
		RuntimeAsset: "static/js/datastar-pro-v1.js",
		HostAsset:    "../../datastar-pro/datastar-pro-v1.js",
	})(t.Context(), repoRoot, &fakeRunner{})
	if err != nil {
		t.Fatalf("SyncDatastarAssets: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repoRoot, "static", "js", "datastar-pro-v1.js"))
	if err != nil {
		t.Fatalf("read runtime asset: %v", err)
	}
	if string(got) != "pro bundle" {
		t.Fatalf("runtime asset = %q, want host asset", got)
	}
}

func TestTSWorkerSkipsWhenNodeModulesAbsent(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTSWorkerInputs(t, repoRoot)
	state := DefaultState(repoRoot)
	runner := &fakeRunner{}
	result, err := ExecuteStep(
		t.Context(),
		&state,
		TSWorkerStep(),
		nil,
		Options{},
		NewTreeHasher(repoRoot),
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionSkippedUnavailable {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionSkippedUnavailable)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want none", runner.calls)
	}
}

func TestTSWorkerRunsWhenNodeModulesPresent(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTSWorkerInputs(t, repoRoot)
	writeFile(t, filepath.Join(repoRoot, "node_modules", ".keep"), "")
	state := DefaultState(repoRoot)
	runner := &fakeRunner{}
	result, err := ExecuteStep(
		t.Context(),
		&state,
		TSWorkerStep(),
		nil,
		Options{},
		NewTreeHasher(repoRoot),
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionRunInputChanged {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionRunInputChanged)
	}
	if got, want := len(runner.calls), 1; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
	got := runner.calls[0]
	if got.Dir != "" {
		t.Fatalf("runner Dir = %q, want empty", got.Dir)
	}
	if diff := cmp.Diff([]string{"./node_modules/.bin/tsc"}, got.Args); diff != "" {
		t.Fatalf("runner Args mismatch (-want +got):\n%s", diff)
	}
}

func TestExecRunnerCreatesGoCache(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, ".build-agents", "go-build-cache")
	runner := NewExecRunner(repoRoot, nil, nil)
	if err := runner.Run(t.Context(), CommandSpec{
		Env:  map[string]string{"GOCACHE": cacheDir},
		Args: []string{"true"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := filepath.Abs(cacheDir); err != nil {
		t.Fatalf("abs cache path: %v", err)
	}
	if !pathExists(t, cacheDir) {
		t.Fatalf("GOCACHE dir was not created: %s", cacheDir)
	}
}

func assertArgs(t *testing.T, got CommandSpec, want []string) {
	t.Helper()
	if diff := cmp.Diff(want, got.Args); diff != "" {
		t.Fatalf("Args mismatch (-want +got):\n%s", diff)
	}
}

func assertContains(t *testing.T, got []string, want string) {
	t.Helper()
	for _, value := range got {
		if value == want {
			return
		}
	}
	t.Fatalf("%#v does not contain %q", got, want)
}

func assertNotContains(t *testing.T, got []string, want string) {
	t.Helper()
	for _, value := range got {
		if value == want {
			t.Fatalf("%#v unexpectedly contains %q", got, want)
		}
	}
}

func writeTSWorkerInputs(t *testing.T, repoRoot string) {
	t.Helper()
	writeFile(t, filepath.Join(repoRoot, "tsconfig.json"), `{"compilerOptions": {}}`)
	writeFile(t, filepath.Join(repoRoot, "package.json"), `{"scripts": {}}`)
	writeFile(
		t,
		filepath.Join(
			repoRoot,
			"pkg",
			"agents",
			"temporal",
			"workers",
			"ts",
			"worker.ts",
		),
		"export {};\n",
	)
}

func pathExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	_, err = filepath.EvalSymlinks(path)
	return err == nil
}
