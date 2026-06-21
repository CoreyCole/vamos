package qrspicmd

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	out := &bytes.Buffer{}
	err = executeForErrorWithDeps(out, deps{
		StateRoot: func() (string, error) { return t.TempDir(), nil },
		Clock:     func() time.Time { return time.Unix(100, 123) },
	}, "init", "--plan-dir", "thoughts/example", "--project-root", t.TempDir())
	if err != nil {
		t.Fatalf("init with required flags error = %v", err)
	}
	if !strings.Contains(out.String(), `"type":"initialized"`) {
		t.Fatalf("init output = %q, want initialized event", out.String())
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
	assertErrorContains(t, err, "state-file is required")
}

func TestValidateResultRequiresStateAndPlan(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"stage", []string{"validate-result"}, "stage is required"},
		{"state", []string{"validate-result", "--stage", "design"}, "state-file is required"},
		{"plan", []string{"validate-result", "--stage", "design", "--state-file", "/tmp/state.json"}, "plan-dir is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("validate-result", "--stage", "design", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt", "--plan-dir", "thoughts/example")
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("validate-result should be implemented after required flags, got %v", err)
	}
}

func TestDecideNextRequiresStateAndPlan(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"state", []string{"decide-next"}, "state-file is required"},
		{"plan", []string{"decide-next", "--state-file", "/tmp/state.json"}, "plan-dir is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertErrorContains(t, executeForError(tc.args...), tc.want)
		})
	}

	err := executeForError("decide-next", "--state-file", "/tmp/state.json", "--result-file", "/tmp/result.txt", "--plan-dir", "thoughts/example")
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("decide-next should be implemented after required flags, got %v", err)
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

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	if err := (FileStateStore{}).Save(stateFile, ManagerState{SourceCwd: dir, Workflow: testWorkflowState(t, "design", nil)}); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	err := executeForErrorWithDeps(out, deps{}, "render-prompt", "--state-file", stateFile, "--node", "design", "--plan-dir", "thoughts/example")
	if err != nil {
		t.Fatalf("render-prompt with required flags error = %v", err)
	}
	if !strings.Contains(out.String(), ".pi/skills/q-design/SKILL.md") {
		t.Fatalf("render-prompt output = %q", out.String())
	}
}

func executeForError(args ...string) error {
	return executeForErrorWithDeps(&bytes.Buffer{}, deps{}, args...)
}

func executeForErrorWithDeps(out *bytes.Buffer, d deps, args ...string) error {
	cmd := newCommand(d)
	cmd.SetOut(out)
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
