package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFileStateStoreMissingLoadReturnsDefault(t *testing.T) {
	t.Parallel()

	store, _ := newTestStore(t)
	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load missing state: %v", err)
	}
	if diff := cmp.Diff(DefaultState(""), state); diff != "" {
		t.Fatalf("Load missing state mismatch (-want +got):\n%s", diff)
	}
}

func TestFileStateStoreSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	store, _ := newTestStore(t)
	want := DefaultState("")
	want.Steps[string(StepGo)] = StepState{InputHash: "in", OutputHash: "out"}
	want.PendingRestarts = PendingRestartState{Web: true, TSWorker: true}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("state mismatch (-want +got):\n%s", diff)
	}
}

func TestFileStateStoreMalformedStateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `{not-json}`},
		{name: "truncated", data: `{"version": 1,`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store, dir := newTestStore(t)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(dir, "state.json"),
				[]byte(test.data),
				0o644,
			); err != nil {
				t.Fatalf("write state: %v", err)
			}
			if _, err := store.Load(context.Background()); err == nil {
				t.Fatal("Load malformed state succeeded, want error")
			}
		})
	}
}

func TestFileStateStoreCleanRemovesSafeStateOnly(t *testing.T) {
	t.Parallel()

	store, dir := newTestStore(t)
	writeFile(t, filepath.Join(dir, "state.json"), "{}")
	writeFile(t, filepath.Join(dir, "random.tmp"), "tmp")
	writeFile(t, filepath.Join(dir, "go-build-cache", "cachefile"), "cache")
	writeFile(t, filepath.Join(dir, "build.lock"), "lock")
	writeFile(t, filepath.Join(dir, "build.lock.json"), "metadata")

	if err := store.Clean(context.Background()); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	for _, path := range []string{"state.json", "random.tmp", filepath.Join("go-build-cache", "cachefile")} {
		if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after Clean or stat error = %v", path, err)
		}
	}
	for _, path := range []string{"build.lock", "build.lock.json"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("%s missing after Clean: %v", path, err)
		}
	}
}

func TestRunCleanPreservesPendingRestarts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, ".build-agents")
	store := NewFileStateStore(filepath.Join(stateDir, "state.json"))
	state := DefaultState(repoRoot)
	state.PendingRestarts = PendingRestartState{Web: true, TSWorker: true}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeFile(t, filepath.Join(stateDir, "go-build-cache", "cachefile"), "cache")

	if err := runWithDeps(
		context.Background(),
		Options{
			RepoRoot:  repoRoot,
			StateDir:  ".build-agents",
			Clean:     true,
			NoRestart: true,
		},
		NewFileLock(
			filepath.Join(stateDir, "build.lock"),
			filepath.Join(stateDir, "build.lock.json"),
		),
		store,
		&fakeHasher{},
		&fakeRunner{},
	); err != nil {
		t.Fatalf("Run clean: %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after clean: %v", err)
	}
	if diff := cmp.Diff(state.PendingRestarts, got.PendingRestarts); diff != "" {
		t.Fatalf("pending restarts mismatch (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(
		filepath.Join(stateDir, "go-build-cache", "cachefile"),
	); !os.IsNotExist(
		err,
	) {
		t.Fatalf("go-build-cache survived clean or stat error = %v", err)
	}
}

func newTestStore(t *testing.T) (*FileStateStore, string) {
	t.Helper()
	dir := t.TempDir()
	return NewFileStateStore(filepath.Join(dir, "state.json")), dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
