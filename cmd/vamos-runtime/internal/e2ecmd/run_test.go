package e2ecmd

import (
	"bytes"
	"testing"
)

func TestRunCommandHasBrowserFlags(t *testing.T) {
	cmd := NewRunCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	for _, name := range []string{"story", "scenario", "viewport", "base-url", "artifacts-dir", "plan-dir", "no-restart"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
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
	got := BuildGoTestArgs(RunConfig{Story: "thoughts-workbench", Scenario: "root-opens"})
	want := []string{
		"test",
		"./pkg/e2e/generated",
		"-run",
		"ThoughtsWorkbench.*RootOpens",
	}
	if len(got) != len(want) {
		t.Fatalf("BuildGoTestArgs()=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BuildGoTestArgs()=%v want %v", got, want)
		}
	}
}

func TestShouldPreflight(t *testing.T) {
	if !ShouldPreflight(RunConfig{}) {
		t.Fatal("ShouldPreflight()=false want true")
	}
}
