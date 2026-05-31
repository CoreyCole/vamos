package repair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/artifacts"
)

func TestBuildPlanSelectorFailure(t *testing.T) {
	dir := t.TempDir()
	writeFailures(
		t,
		dir,
		[]artifacts.Failure{
			{Error: "selector for thoughts.workbench.sidebar not visible"},
		},
	)
	plan, err := BuildPlan(
		context.Background(),
		Request{FailuresPath: filepath.Join(dir, "failures.json")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 {
		t.Fatalf("changes=%v want one selector change", plan.Changes)
	}
	if got, want := plan.Changes[0].Scope, FixScopeVamosHelpers; got != want {
		t.Fatalf("scope=%s want %s", got, want)
	}
	if got, want := plan.Changes[0].Path, "pkg/e2e/vamos"; got != want {
		t.Fatalf("path=%s want %s", got, want)
	}
}

func TestBuildPlanUnsupportedStoryStepNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	writeFailures(
		t,
		dir,
		[]artifacts.Failure{{Error: "unsupported story step: I change production UI"}},
	)
	plan, err := BuildPlan(
		context.Background(),
		Request{FailuresPath: filepath.Join(dir, "failures.json")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.NeedsHuman) == 0 {
		t.Fatalf("NeedsHuman empty, plan=%+v", plan)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("changes=%v want none for unsupported story language", plan.Changes)
	}
}

func TestValidatePlanRejectsProductionUIPaths(t *testing.T) {
	err := ValidatePlan(
		Plan{
			Changes: []Change{
				{
					Scope: FixScopeVamosHelpers,
					Path:  "server/layouts/root.templ",
					Why:   "test",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("ValidatePlan() error=nil want forbidden path error")
	}
}

func TestValidatePlanRejectsArbitrarySourcePaths(t *testing.T) {
	err := ValidatePlan(
		Plan{
			Changes: []Change{
				{
					Scope: FixScopeDatastarUI,
					Path:  "pkg/agents/workflows/runtime/runtime.go",
					Why:   "test",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("ValidatePlan() error=nil want arbitrary source path error")
	}
}

func TestValidatePlanAllowsBoundedPaths(t *testing.T) {
	plan := Plan{Changes: []Change{
		{Scope: FixScopeVamosHelpers, Path: "pkg/e2e/vamos", Why: "selector drift"},
		{
			Scope: FixScopeTests,
			Path:  "pkg/e2e/tests",
			Why:   "repair authored Go Story test",
		},
	}}
	if err := ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
}

func writeFailures(t *testing.T, dir string, failures []artifacts.Failure) {
	t.Helper()
	data, err := json.Marshal(failures)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "failures.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
