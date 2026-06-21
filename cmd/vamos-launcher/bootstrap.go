package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func maybeReexecManaged(ctx context.Context) error {
	source, err := resolveRuntimeSource(ctx)
	if err != nil {
		return err
	}
	fp, err := computeRuntimeFingerprint(ctx, source)
	if err != nil {
		return err
	}
	cacheDir, err := defaultLauncherCacheDir()
	if err != nil {
		return err
	}
	target := managedRuntimePath(cacheDir, source, fp)
	if err := ensureManagedRuntime(ctx, source, target); err != nil {
		return err
	}
	return execRuntime(target.BinaryPath, os.Args[1:], os.Environ())
}

func buildRuntime(ctx context.Context, sourceRoot, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create managed runtime output dir for %q: %w", outputPath, err)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", outputPath, "./cmd/vamos-runtime")
	cmd.Dir = sourceRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build managed runtime from %q with go build -o %q ./cmd/vamos-runtime: %w", sourceRoot, outputPath, err)
	}
	return nil
}

func execRuntime(binaryPath string, args []string, env []string) error {
	return syscall.Exec(binaryPath, append([]string{binaryPath}, args...), env)
}
