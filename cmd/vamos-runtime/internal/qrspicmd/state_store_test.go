package qrspicmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultStateRootUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	got, err := DefaultStateRoot()
	if err != nil {
		t.Fatalf("DefaultStateRoot() error = %v", err)
	}
	want := filepath.Join("/tmp/xdg-state", "vamos", "q-manager")
	if got != want {
		t.Fatalf("DefaultStateRoot() = %q, want %q", got, want)
	}
}

func TestDefaultStateRootFallsBackToLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	got, err := DefaultStateRoot()
	if err != nil {
		t.Fatalf("DefaultStateRoot() error = %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	want := filepath.Join(home, ".local", "state", "vamos", "q-manager")
	if got != want {
		t.Fatalf("DefaultStateRoot() = %q, want %q", got, want)
	}
}

func TestCanonicalPlanDirStableAcrossCwd(t *testing.T) {
	root := t.TempDir()
	got, err := CanonicalPlanDir(root, "thoughts/../thoughts/example")
	if err != nil {
		t.Fatalf("CanonicalPlanDir() error = %v", err)
	}
	want := filepath.Join(root, "thoughts", "example")
	if got != want {
		t.Fatalf("CanonicalPlanDir() = %q, want %q", got, want)
	}

	abs := filepath.Join(root, "absolute", "plan")
	got, err = CanonicalPlanDir("/ignored", abs)
	if err != nil {
		t.Fatalf("CanonicalPlanDir(abs) error = %v", err)
	}
	if got != abs {
		t.Fatalf("CanonicalPlanDir(abs) = %q, want %q", got, abs)
	}
}

func TestStatePathAndLockPathAreOutsideRepoVamos(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state-root")
	repo := filepath.Join(t.TempDir(), "repo")
	key := LockKey{RepoID: repo, CanonicalPlanDir: filepath.Join(repo, "thoughts", "plan")}
	statePath := StatePath(root, key, "run-1")
	lockPath := LockPath(root, key)
	if !strings.HasPrefix(statePath, root+string(os.PathSeparator)) {
		t.Fatalf("state path %q is not under state root %q", statePath, root)
	}
	if !strings.HasPrefix(lockPath, root+string(os.PathSeparator)) {
		t.Fatalf("lock path %q is not under state root %q", lockPath, root)
	}
	if strings.Contains(statePath, filepath.Join(repo, ".vamos")) || strings.Contains(lockPath, filepath.Join(repo, ".vamos")) {
		t.Fatalf("paths must not live under repo .vamos: %q %q", statePath, lockPath)
	}
}

func TestFileStateStoreSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := FileStateStore{Root: root}
	path := filepath.Join(root, "state.json")
	state := ManagerState{SchemaVersion: schemaVersion, RepoID: "repo", CanonicalPlanDir: "plan", ManagerRunID: "run"}
	if err := store.Save(path, state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SchemaVersion != state.SchemaVersion || got.RepoID != state.RepoID || got.ManagerRunID != state.ManagerRunID {
		t.Fatalf("loaded state = %+v, want %+v", got, state)
	}
}

func TestAcquireLockAllowsSameOwner(t *testing.T) {
	clock := fixedClock(time.Unix(100, 0))
	store := FileStateStore{Root: t.TempDir(), Clock: clock}
	key := LockKey{RepoID: "repo", CanonicalPlanDir: "plan"}
	if _, err := store.AcquireLock(context.Background(), key, "owner", time.Hour); err != nil {
		t.Fatalf("first AcquireLock() error = %v", err)
	}
	if _, err := store.AcquireLock(context.Background(), key, "owner", time.Hour); err != nil {
		t.Fatalf("same-owner AcquireLock() error = %v", err)
	}
}

func TestAcquireLockRejectsDifferentActiveOwner(t *testing.T) {
	store := FileStateStore{Root: t.TempDir(), Clock: fixedClock(time.Unix(100, 0))}
	key := LockKey{RepoID: "repo", CanonicalPlanDir: "plan"}
	if _, err := store.AcquireLock(context.Background(), key, "owner-a", time.Hour); err != nil {
		t.Fatalf("first AcquireLock() error = %v", err)
	}
	_, err := store.AcquireLock(context.Background(), key, "owner-b", time.Hour)
	var conflict LockConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("AcquireLock() error = %v, want LockConflictError", err)
	}
	if conflict.Existing.Owner != "owner-a" {
		t.Fatalf("conflict owner = %q, want owner-a", conflict.Existing.Owner)
	}
}

func TestAcquireLockReplacesExpiredLock(t *testing.T) {
	now := time.Unix(100, 0)
	store := FileStateStore{Root: t.TempDir(), Clock: fixedClock(now)}
	key := LockKey{RepoID: "repo", CanonicalPlanDir: "plan"}
	if _, err := store.AcquireLock(context.Background(), key, "owner-a", time.Second); err != nil {
		t.Fatalf("first AcquireLock() error = %v", err)
	}
	store.Clock = fixedClock(now.Add(2 * time.Second))
	lock, err := store.AcquireLock(context.Background(), key, "owner-b", time.Second)
	if err != nil {
		t.Fatalf("expired AcquireLock() error = %v", err)
	}
	if lock.Owner != "owner-b" {
		t.Fatalf("lock owner = %q, want owner-b", lock.Owner)
	}
}

func TestAcquireLockAllowsOnlyOneConcurrentOwner(t *testing.T) {
	store := FileStateStore{Root: t.TempDir(), Clock: fixedClock(time.Unix(100, 0))}
	key := LockKey{RepoID: "repo", CanonicalPlanDir: "plan"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := make([]string, 0, 1)
	conflicts := 0
	for _, owner := range []string{"owner-a", "owner-b", "owner-c", "owner-d"} {
		owner := owner
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.AcquireLock(context.Background(), key, owner, time.Hour)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes = append(successes, owner)
				return
			}
			var conflict LockConflictError
			if errors.As(err, &conflict) {
				conflicts++
				return
			}
			t.Errorf("AcquireLock(%s) error = %v", owner, err)
		}()
	}
	close(start)
	wg.Wait()
	if len(successes) != 1 || conflicts != 3 {
		t.Fatalf("successes=%v conflicts=%d, want one success and three conflicts", successes, conflicts)
	}
}

func TestOperationLockSerializesSameStateFile(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	stateFile := filepath.Join(t.TempDir(), "state.json")
	first, err := store.AcquireOperationLock(t.Context(), stateFile)
	if err != nil {
		t.Fatalf("first AcquireOperationLock() error = %v", err)
	}
	acquired := make(chan StateOperationLock, 1)
	errs := make(chan error, 1)
	go func() {
		lock, lockErr := store.AcquireOperationLock(t.Context(), stateFile)
		if lockErr != nil {
			errs <- lockErr
			return
		}
		acquired <- lock
	}()
	select {
	case lock := <-acquired:
		_ = lock.Release()
		t.Fatal("second operation lock acquired before first released")
	case lockErr := <-errs:
		t.Fatalf("second AcquireOperationLock() error = %v", lockErr)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Release(); err != nil {
		t.Fatalf("first Release() error = %v", err)
	}
	select {
	case lock := <-acquired:
		if err := lock.Release(); err != nil {
			t.Fatalf("second Release() error = %v", err)
		}
	case lockErr := <-errs:
		t.Fatalf("second AcquireOperationLock() error = %v", lockErr)
	case <-time.After(time.Second):
		t.Fatal("second operation lock did not acquire after release")
	}
}

func TestOperationLockHonorsCancellation(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	stateFile := filepath.Join(t.TempDir(), "state.json")
	first, err := store.AcquireOperationLock(t.Context(), stateFile)
	if err != nil {
		t.Fatalf("first AcquireOperationLock() error = %v", err)
	}
	defer first.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, err = store.AcquireOperationLock(ctx, stateFile)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AcquireOperationLock() error = %v, want deadline exceeded", err)
	}
}

func TestOperationLockAllowsDifferentStateFiles(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	dir := t.TempDir()
	first, err := store.AcquireOperationLock(t.Context(), filepath.Join(dir, "one.json"))
	if err != nil {
		t.Fatalf("first AcquireOperationLock() error = %v", err)
	}
	defer first.Release()
	second, err := store.AcquireOperationLock(t.Context(), filepath.Join(dir, "two.json"))
	if err != nil {
		t.Fatalf("second AcquireOperationLock() error = %v", err)
	}
	defer second.Release()
}

func TestOperationLockReleaseIsIdempotent(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	lock, err := store.AcquireOperationLock(t.Context(), filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("AcquireOperationLock() error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first Release() error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}
}

func TestFileStateStoreMutateSerializesUpdates(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	path := filepath.Join(store.Root, "state.json")
	if err := store.Save(path, ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Generation: 1}}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Mutate(path, &ChildEpoch{ID: "child-1", Generation: 1}, func(state *ManagerState) error {
				current := state.SchemaVersion
				time.Sleep(time.Millisecond)
				state.SchemaVersion = current + 1

				return nil
			})
			if err != nil {
				t.Errorf("Mutate() error = %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	state, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 4 {
		t.Fatalf("SchemaVersion = %d, want 4 serialized updates", state.SchemaVersion)
	}
}

func TestFileStateStoreMutateRejectsStaleEpoch(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	path := filepath.Join(store.Root, "state.json")
	if err := store.Save(path, ManagerState{ActiveChild: &ChildRunRef{ID: "child-1", Generation: 2}}); err != nil {
		t.Fatal(err)
	}

	_, err := store.Mutate(path, &ChildEpoch{ID: "child-1", Generation: 1}, func(state *ManagerState) error {
		state.SchemaVersion = 99

		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "active child epoch changed") {
		t.Fatalf("Mutate() error = %v, want stale epoch refusal", err)
	}
	state, loadErr := store.Load(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if state.SchemaVersion != 0 {
		t.Fatalf("SchemaVersion = %d, stale mutation was saved", state.SchemaVersion)
	}
}

func TestFileStateStoreMutateDoesNotSaveFailedMutation(t *testing.T) {
	store := FileStateStore{Root: t.TempDir()}
	path := filepath.Join(store.Root, "state.json")
	if err := store.Save(path, ManagerState{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}

	_, err := store.Mutate(path, nil, func(state *ManagerState) error {
		state.SchemaVersion = 99

		return errors.New("stop")
	})
	if err == nil || err.Error() != "stop" {
		t.Fatalf("Mutate() error = %v, want stop", err)
	}
	state, loadErr := store.Load(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if state.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, failed mutation was saved", state.SchemaVersion)
	}
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }
