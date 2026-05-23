package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func maybeReexecManaged() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve launcher executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	launcherDir := filepath.Dir(exePath)
	version, err := readManagedVersion(filepath.Join(launcherDir, "managed_version.txt"))
	if err != nil {
		return err
	}
	binaryPath := filepath.Join(launcherDir, "vamos-"+version)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		if err := buildManagedRuntime(exePath, binaryPath); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat managed runtime %q: %w", binaryPath, err)
	}
	return syscall.Exec(
		binaryPath,
		append([]string{binaryPath}, os.Args[1:]...),
		os.Environ(),
	)
}

func readManagedVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read managed version: %w", err)
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", fmt.Errorf("managed version file %q is empty", path)
	}
	return version, nil
}

func buildManagedRuntime(exePath, binaryPath string) error {
	launcherDir := filepath.Dir(exePath)
	packageRoot := filepath.Clean(filepath.Join(launcherDir, "..", ".."))
	if root := strings.TrimSpace(os.Getenv("VAMOS_PACKAGE_ROOT")); root != "" {
		packageRoot = root
	}
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binaryPath, ".")
	cmd.Dir = filepath.Join(packageRoot, "cmd", "vamos-runtime")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build managed runtime: %w", err)
	}
	return nil
}
