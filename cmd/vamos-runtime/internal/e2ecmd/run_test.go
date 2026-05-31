package e2ecmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	duiappconfig "github.com/coreycole/datastarui/e2e/appconfig"
)

func TestRunCommandHasBrowserFlags(t *testing.T) {
	cmd := NewRunCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	for _, name := range []string{"story", "scenario", "viewport", "base-url", "artifacts-dir", "plan-dir", "no-restart", "config"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}

func TestRunCommandHasConfigFlag(t *testing.T) {
	cmd := NewRunCommand()
	if flag := cmd.Flags().Lookup("config"); flag == nil {
		t.Fatal("missing flag --config")
	}
}

func TestSlugToTestFragment(t *testing.T) {
	if got, want := slugToTestFragment(
		"thoughts-workbench",
	), "ThoughtsWorkbench"; got != want {
		t.Fatalf("slugToTestFragment()=%q want %q", got, want)
	}
}

func TestBuildGoTestArgs(t *testing.T) {
	got := BuildGoTestArgs(RunConfig{Story: "thoughts-workbench", Scenario: "root-opens"}, duiappconfig.Config{})
	want := []string{
		"test",
		"./pkg/e2e/generated",
		"-run",
		"ThoughtsWorkbench.*RootOpens",
	}
	assertStrings(t, got, want)
}

func TestBuildGoTestArgsUsesConfiguredRunPackage(t *testing.T) {
	got := BuildGoTestArgs(RunConfig{Story: "select-component"}, duiappconfig.Config{RunPackage: "./tests/e2e"})
	want := []string{"test", "./tests/e2e", "-run", "SelectComponent"}
	assertStrings(t, got, want)
}

func TestSelectedViewportEnvDefaultsToVerifyViewports(t *testing.T) {
	if got, want := selectedViewportEnv(RunConfig{}), "mobile,desktop-half,desktop-full"; got != want {
		t.Fatalf("selectedViewportEnv()=%q want %q", got, want)
	}
}

func TestSelectedViewportEnvPreservesExplicitCommaList(t *testing.T) {
	if got, want := selectedViewportEnv(RunConfig{Viewport: "mobile,desktop-half"}), "mobile,desktop-half"; got != want {
		t.Fatalf("selectedViewportEnv()=%q want %q", got, want)
	}
}

func TestEnsureSelectedTestsExistRejectsNoMatch(t *testing.T) {
	err := ensureSelectedTestsExist(context.Background(), []string{
		"test",
		"./pkg/e2e/generated",
		"-run",
		"DefinitelyNoGeneratedE2ETestMatchesThis",
	}, ".")
	if err == nil || !strings.Contains(err.Error(), "no E2E tests matched") {
		t.Fatalf("ensureSelectedTestsExist() error = %v", err)
	}
}

func TestShouldPreflightSkipsForNone(t *testing.T) {
	if ShouldPreflight(RunConfig{}, duiappconfig.Config{Preflight: duiappconfig.PreflightConfig{Mode: "none"}}) {
		t.Fatal("ShouldPreflight()=true want false")
	}
}

func TestShouldPreflightRunsForVamosWorkspace(t *testing.T) {
	if !ShouldPreflight(RunConfig{}, duiappconfig.Config{Preflight: duiappconfig.PreflightConfig{Mode: "vamos_workspace"}}) {
		t.Fatal("ShouldPreflight()=false want true")
	}
}

func TestBuildServerCommandUsesConfiguredCommand(t *testing.T) {
	got := BuildServerCommand(RunConfig{}, duiappconfig.Config{Server: duiappconfig.ServerConfig{Command: "just build-local"}})
	want := []string{"just", "build-local"}
	assertStrings(t, got, want)
}

func TestBuildServerCommandSkipsWhenBaseURLSet(t *testing.T) {
	t.Setenv("E2E_BASE_URL", "")
	got := BuildServerCommand(
		RunConfig{BaseURL: "http://localhost:4242"},
		duiappconfig.Config{Server: duiappconfig.ServerConfig{Command: "just build-local", SkipWhenBaseURLSet: true}},
	)
	if got != nil {
		t.Fatalf("BuildServerCommand()=%v want nil", got)
	}
}

func TestBuildServerCommandSkipsWhenEnvBaseURLSet(t *testing.T) {
	t.Setenv("E2E_BASE_URL", "http://localhost:4242")
	got := BuildServerCommand(
		RunConfig{},
		duiappconfig.Config{Server: duiappconfig.ServerConfig{Command: "just build-local", SkipWhenBaseURLSet: true}},
	)
	if got != nil {
		t.Fatalf("BuildServerCommand()=%v want nil", got)
	}
}

func TestLoadAppConfigUsesExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vamos-e2e.yaml"
	if err := os.WriteFile(path, []byte("app: test\nbase_url: http://localhost:4242\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadAppConfig(RunConfig{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath=%q want %q", cfg.ConfigPath, path)
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
