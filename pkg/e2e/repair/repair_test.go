package repair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanSelectorFailure(t *testing.T) {
	dir := t.TempDir()
	writeFailures(
		t,
		dir,
		[]Failure{
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
		[]Failure{{Error: "unsupported story step: I change production UI"}},
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
		{Scope: FixScopeDatastarUI, Path: "../datastarui/e2e", Why: "artifact behavior"},
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

func TestValidatePlanRejectsDeletedLegacyE2EPaths(t *testing.T) {
	for _, path := range []string{"pkg/e2e/selectors", "pkg/e2e/steps", "pkg/e2e/runtime"} {
		err := ValidatePlan(Plan{Changes: []Change{{Scope: FixScopeVamosHelpers, Path: path, Why: "stale scope"}}})
		if err == nil {
			t.Fatalf("ValidatePlan(%s) error=nil want rejected stale path", path)
		}
	}
}

func writeFailures(t *testing.T, dir string, failures []Failure) {
	t.Helper()
	data, err := json.Marshal(failures)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "failures.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
