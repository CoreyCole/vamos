package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if info, err := os.Stat(target.BinaryPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("managed runtime target %q is a directory", target.BinaryPath)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat managed runtime %q: %w", target.BinaryPath, err)
	}
	if err := buildRuntime(ctx, source.Root, target.BinaryPath); err != nil {
		return fmt.Errorf("ensure managed runtime %q: %w", target.BinaryPath, err)
	}
	return nil
}
