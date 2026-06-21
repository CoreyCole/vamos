package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ManagedRuntime struct {
	BinaryPath   string
	LockPath     string
	MetadataPath string
	TempDir      string
}

func defaultLauncherCacheDir() (string, error) {
	if path := strings.TrimSpace(os.Getenv("VAMOS_LAUNCHER_CACHE")); path != "" {
		return path, nil
	}
	if cacheHome := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); cacheHome != "" {
		return filepath.Join(cacheHome, "vamos", "launcher"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for launcher cache: %w", err)
	}
	return filepath.Join(home, ".cache", "vamos", "launcher"), nil
}

func managedRuntimePath(cacheDir string, source RuntimeSource, fp Fingerprint) ManagedRuntime {
	binaryPath := filepath.Join(cacheDir, "runtimes", source.SourceKey, "vamos-runtime-"+fp.Value)
	return ManagedRuntime{
		BinaryPath:   binaryPath,
		LockPath:     binaryPath + ".lock",
		MetadataPath: binaryPath + ".lock.json",
		TempDir:      filepath.Join(filepath.Dir(binaryPath), "tmp"),
	}
}

func ensureManagedRuntime(ctx context.Context, source RuntimeSource, target ManagedRuntime) error {
	if ok, err := usableRuntimeBinary(target.BinaryPath); ok || err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target.BinaryPath), 0o755); err != nil {
		return fmt.Errorf("create managed runtime dir %q: %w", filepath.Dir(target.BinaryPath), err)
	}
	if err := os.MkdirAll(target.TempDir, 0o755); err != nil {
		return fmt.Errorf("create managed runtime temp dir %q: %w", target.TempDir, err)
	}

	release, err := acquireFileLock(ctx, target.LockPath, target.MetadataPath)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	if ok, err := usableRuntimeBinary(target.BinaryPath); ok || err != nil {
		return err
	}

	tempPath := tempRuntimePath(target)
	if err := buildRuntimeFunc(ctx, source.Root, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("ensure managed runtime %q from %q: %w", target.BinaryPath, source.Root, err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod managed runtime temp %q: %w", tempPath, err)
	}
	if err := atomicInstallRuntime(tempPath, target.BinaryPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func usableRuntimeBinary(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat managed runtime %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("managed runtime target %q is not a regular file", path)
	}
	if info.Mode()&0o111 == 0 {
		return false, fmt.Errorf("managed runtime target %q is not executable", path)
	}
	return true, nil
}

type lockMetadata struct {
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	StartedAt time.Time `json:"started_at"`
}

func acquireFileLock(ctx context.Context, lockPath, metadataPath string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir %q: %w", filepath.Dir(lockPath), err)
	}
	metadata := lockMetadata{PID: os.Getpid(), Hostname: hostname(), StartedAt: time.Now().UTC()}
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			encoded, encodeErr := json.MarshalIndent(metadata, "", "  ")
			if encodeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("encode lock metadata for %q: %w", lockPath, encodeErr)
			}
			if _, err := file.Write(append(encoded, '\n')); err != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write lock %q: %w", lockPath, err)
			}
			if err := file.Close(); err != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("close lock %q: %w", lockPath, err)
			}
			if err := os.WriteFile(metadataPath, append(encoded, '\n'), 0o644); err != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write lock metadata %q: %w", metadataPath, err)
			}
			return func() error {
				metadataErr := os.Remove(metadataPath)
				lockErr := os.Remove(lockPath)
				if metadataErr != nil && !os.IsNotExist(metadataErr) {
					return metadataErr
				}
				if lockErr != nil && !os.IsNotExist(lockErr) {
					return lockErr
				}
				return nil
			}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock %q: %w", lockPath, err)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("acquire lock %q: %w", lockPath, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "unknown"
	}
	return host
}

func tempRuntimePath(target ManagedRuntime) string {
	name := fmt.Sprintf(".vamos-runtime-%d-%d.tmp", os.Getpid(), time.Now().UnixNano())
	return filepath.Join(target.TempDir, name)
}

func atomicInstallRuntime(tempPath, finalPath string) error {
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("create managed runtime dir %q: %w", filepath.Dir(finalPath), err)
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return fmt.Errorf("install managed runtime %q -> %q: %w", tempPath, finalPath, err)
	}
	return nil
}
