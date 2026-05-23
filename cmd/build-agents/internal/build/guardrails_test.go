package build

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformPreflightRunsBeforeStateMutation(t *testing.T) {
	t.Parallel()

	lock := &recordingLock{events: &eventRecorder{}}
	store := &recordingStore{events: &eventRecorder{}, state: DefaultState("")}
	hasher := &recordingHasher{events: &eventRecorder{}}
	runner := failingRunner{err: errors.New("no systemd")}

	err := runWithDeps(t.Context(), Options{}, lock, store, hasher, runner)
	if err == nil {
		t.Fatal("runWithDeps succeeded, want guardrail error")
	}
	if got := store.events.events; len(got) != 0 {
		t.Fatalf("store was touched before guardrail failed: %#v", got)
	}
	if got := hasher.events.events; len(got) != 0 {
		t.Fatalf("hasher was touched before guardrail failed: %#v", got)
	}
}

func TestRequireLinuxSystemdCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	if err := RequireLinuxSystemd(t.Context(), runner); err != nil {
		t.Fatalf("RequireLinuxSystemd: %v", err)
	}
	if got, want := len(runner.calls), 1; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
	want := []string{"systemctl", "--user", "show-environment"}
	if got := strings.Join(runner.calls[0].Args, " "); got != strings.Join(want, " ") {
		t.Fatalf("guardrail command = %q, want %q", got, strings.Join(want, " "))
	}
}

func TestRequireLaunchdCommand(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	if err := RequireLaunchd(t.Context(), runner, 501); err != nil {
		t.Fatalf("RequireLaunchd: %v", err)
	}
	want := []string{"launchctl", "print", "gui/501"}
	if got := len(runner.calls); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
	if got := strings.Join(runner.calls[0].Args, " "); got != strings.Join(want, " ") {
		t.Fatalf("launchd guardrail command = %q, want %q", got, strings.Join(want, " "))
	}
}

func TestNewPlatformServiceManagerDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		wantErr string
	}{
		{name: "linux", goos: "linux"},
		{name: "darwin", goos: "darwin"},
		{
			name:    "unsupported",
			goos:    "freebsd",
			wantErr: "unsupported smart build platform",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewPlatformServiceManager(
				t.Context(),
				test.goos,
				&fakeRunner{},
				func(context.Context, CommandSpec) (string, error) { return "loaded\n", nil },
				nil,
				nil,
			)
			if test.wantErr == "" && err != nil {
				t.Fatalf("NewPlatformServiceManager: %v", err)
			}
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
			}
		})
	}
}

func TestJustBuildForwardsArgs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cmd := exec.CommandContext(
		t.Context(),
		"just",
		"--dry-run",
		"build",
		"--clean",
		"--no-restart",
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("just dry-run failed: %v\n%s", err, out)
	}
	want := "go run ./cmd/build-agents --clean --no-restart"
	if got := string(out); !strings.Contains(got, want) {
		t.Fatalf("dry-run output %q does not contain %q", got, want)
	}
}

type failingRunner struct{ err error }

func (r failingRunner) Run(context.Context, CommandSpec) error { return r.err }

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}
