package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLockExcludesConcurrentAcquire(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lock := NewFileLock(
		filepath.Join(dir, "build.lock"),
		filepath.Join(dir, "build.lock.json"),
	)
	release, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire first lock: %v", err)
	}

	acquired := make(chan ReleaseFunc, 1)
	errs := make(chan error, 1)
	go func() {
		release, err := lock.Acquire(context.Background())
		if err != nil {
			errs <- err
			return
		}
		acquired <- release
	}()

	select {
	case secondRelease := <-acquired:
		_ = secondRelease()
		t.Fatal("second acquire succeeded before first release")
	case err := <-errs:
		t.Fatalf("second acquire failed unexpectedly: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := release(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	select {
	case secondRelease := <-acquired:
		if err := secondRelease(); err != nil {
			t.Fatalf("release second lock: %v", err)
		}
	case err := <-errs:
		t.Fatalf("second acquire failed after release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("second acquire did not complete after first release")
	}
}

func TestFileLockAcquireCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lock := NewFileLock(
		filepath.Join(dir, "build.lock"),
		filepath.Join(dir, "build.lock.json"),
	)
	release, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire first lock: %v", err)
	}
	defer func() { _ = release() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := lock.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire with canceled context error = %v, want deadline exceeded", err)
	}
}

func TestFileLockReleaseRemovesMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "build.lock")
	metadataPath := filepath.Join(dir, "build.lock.json")
	lock := NewFileLock(lockPath, metadataPath)
	release, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing after acquire: %v", err)
	}
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("metadata missing after acquire: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	for _, path := range []string{lockPath, metadataPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s exists after release or stat error = %v", path, err)
		}
	}
}
