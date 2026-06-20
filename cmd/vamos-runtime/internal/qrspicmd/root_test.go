package qrspicmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRootRequiresExplicitSubcommand(t *testing.T) {
	cmd := NewCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "use an explicit subcommand") {
		t.Fatalf("expected explicit subcommand error, got %v", err)
	}
}

func TestSubcommandsExist(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"init", "run-child", "validate-result", "decide-next", "render-prompt"} {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing subcommand %s", name)
		}
	}
}

func TestInitRequiresPlanDir(t *testing.T) {
	err := executeForError("init")
	assertErrorContains(t, err, "plan-dir is required")

	err = executeForError("init", "--plan-dir", "thoughts/example")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented after required flags, got %v", err)
	}
}

func TestRunChildRequiresStageCwdPrompt(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"plan", []string{"run-child"}, "plan-dir is required"},
		{"stage", []string{"run-child", "--plan-dir", "thoughts/example"}, "stage is required"},
		{"cwd", []string{"run-child", "--plan-dir", "thoughts/example", "--stage", "design"}, "cwd is required"},
		{"prompt", []string{"run-child", "--plan-dir", "thoughts/example", "--stage", "design", "--cwd", "."}, "prompt-file is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("run-child", "--plan-dir", "thoughts/example", "--stage", "design", "--cwd", ".", "--prompt-file", "/tmp/prompt.txt")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented after required flags, got %v", err)
	}
}

func TestValidateResultRequiresStateAndResult(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"stage", []string{"validate-result"}, "stage is required"},
		{"state", []string{"validate-result", "--stage", "design"}, "state-file is required"},
		{"result", []string{"validate-result", "--stage", "design", "--state-file", "/tmp/state.json"}, "result-file is required"},
		{"plan", []string{"validate-result", "--stage", "design", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt"}, "plan-dir is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("validate-result", "--stage", "design", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt", "--plan-dir", "thoughts/example")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented after required flags, got %v", err)
	}
}

func TestDecideNextRequiresStateAndResult(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"state", []string{"decide-next"}, "state-file is required"},
		{"result", []string{"decide-next", "--state-file", "/tmp/state.json"}, "result-file is required"},
		{"plan", []string{"decide-next", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt"}, "plan-dir is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("decide-next", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt", "--plan-dir", "thoughts/example")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented after required flags, got %v", err)
	}
}

func TestRenderPromptRequiresStateAndNode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"state", []string{"render-prompt"}, "state-file is required"},
		{"node", []string{"render-prompt", "--state-file", "/tmp/state.json"}, "node is required"},
		{"plan", []string{"render-prompt", "--state-file", "/tmp/state.json", "--node", "design"}, "plan-dir is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("render-prompt", "--state-file", "/tmp/state.json", "--node", "design", "--plan-dir", "thoughts/example")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented after required flags, got %v", err)
	}
}

func executeForError(args ...string) error {
	cmd := NewCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	return cmd.Execute()
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %v", want, err)
	}
}
