package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureManagedRuntimeExistingBinarySkipsBuild(t *testing.T) {
	target := testManagedRuntime(t)
	writeFile(t, target.BinaryPath, "runtime")
	if err := os.Chmod(target.BinaryPath, 0o755); err != nil {
		t.Fatal(err)
	}

	var builds atomic.Int32
	withBuildRuntimeFunc(t, func(context.Context, string, string) error {
		builds.Add(1)
		return nil
	})

	if err := ensureManagedRuntime(context.Background(), RuntimeSource{Root: t.TempDir()}, target); err != nil {
		t.Fatalf("ensureManagedRuntime: %v", err)
	}
	if builds.Load() != 0 {
		t.Fatalf("build count = %d, want 0", builds.Load())
	}
}

func TestEnsureManagedRuntimeBuildsMissingTargetAtomically(t *testing.T) {
	target := testManagedRuntime(t)
	withBuildRuntimeFunc(t, func(_ context.Context, _ string, outputPath string) error {
		writeFile(t, outputPath, "runtime")
		return nil
	})

	if err := ensureManagedRuntime(context.Background(), RuntimeSource{Root: t.TempDir()}, target); err != nil {
		t.Fatalf("ensureManagedRuntime: %v", err)
	}
	info, err := os.Stat(target.BinaryPath)
	if err != nil {
		t.Fatalf("stat final binary: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("final binary mode %v is not executable", info.Mode())
	}
	if _, err := os.Stat(target.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lock path still exists: %v", err)
	}
	if _, err := os.Stat(target.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata path still exists: %v", err)
	}
}

func TestEnsureManagedRuntimeBuildFailureCleansTemp(t *testing.T) {
	target := testManagedRuntime(t)
	buildErr := errors.New("boom")
	withBuildRuntimeFunc(t, func(_ context.Context, _ string, outputPath string) error {
		writeFile(t, outputPath, "partial")
		return buildErr
	})

	err := ensureManagedRuntime(context.Background(), RuntimeSource{Root: t.TempDir()}, target)
	if !errors.Is(err, buildErr) {
		t.Fatalf("ensureManagedRuntime error = %v, want %v", err, buildErr)
	}
	if _, err := os.Stat(target.BinaryPath); !os.IsNotExist(err) {
		t.Fatalf("final binary exists after failed build: %v", err)
	}
	entries, err := os.ReadDir(target.TempDir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp dir contains %d entries after failed build", len(entries))
	}
}

func TestEnsureManagedRuntimeConcurrentCallersBuildOnce(t *testing.T) {
	target := testManagedRuntime(t)
	var builds atomic.Int32
	withBuildRuntimeFunc(t, func(_ context.Context, _ string, outputPath string) error {
		builds.Add(1)
		time.Sleep(150 * time.Millisecond)
		writeFile(t, outputPath, "runtime")
		return nil
	})

	const callers = 8
	sourceRoot := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- ensureManagedRuntime(context.Background(), RuntimeSource{Root: sourceRoot}, target)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensureManagedRuntime concurrent error: %v", err)
		}
	}
	if builds.Load() != 1 {
		t.Fatalf("build count = %d, want 1", builds.Load())
	}
}

func TestAcquireFileLockContextCancellation(t *testing.T) {
	target := testManagedRuntime(t)
	release, err := acquireFileLock(context.Background(), target.LockPath, target.MetadataPath)
	if err != nil {
		t.Fatalf("acquire initial lock: %v", err)
	}
	defer func() { _ = release() }()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = acquireFileLock(ctx, target.LockPath, target.MetadataPath)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireFileLock error = %v, want deadline exceeded", err)
	}
}

func TestAcquireFileLockReleaseRemovesFiles(t *testing.T) {
	target := testManagedRuntime(t)
	release, err := acquireFileLock(context.Background(), target.LockPath, target.MetadataPath)
	if err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}
	if _, err := os.Stat(target.LockPath); err != nil {
		t.Fatalf("stat lock path: %v", err)
	}
	if _, err := os.Stat(target.MetadataPath); err != nil {
		t.Fatalf("stat metadata path: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if _, err := os.Stat(target.LockPath); !os.IsNotExist(err) {
		t.Fatalf("lock path still exists: %v", err)
	}
	if _, err := os.Stat(target.MetadataPath); !os.IsNotExist(err) {
		t.Fatalf("metadata path still exists: %v", err)
	}
}

func testManagedRuntime(t *testing.T) ManagedRuntime {
	t.Helper()
	cache := t.TempDir()
	source := RuntimeSource{Root: filepath.Join(t.TempDir(), "source"), SourceKey: "sourcekey", SourceFrom: "test"}
	return managedRuntimePath(cache, source, Fingerprint{Value: "fingerprint"})
}

func withBuildRuntimeFunc(t *testing.T, fn func(context.Context, string, string) error) {
	t.Helper()
	prev := buildRuntimeFunc
	buildRuntimeFunc = fn
	t.Cleanup(func() { buildRuntimeFunc = prev })
}
