package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRuntimeSourceUsesPackageRootBeforeConfig(t *testing.T) {
	root := fakeRuntimeRoot(t)
	other := fakeRuntimeRoot(t)
	configPath := writeLauncherConfig(t, other)
	t.Setenv("VAMOS_PACKAGE_ROOT", root)
	t.Setenv("VAMOS_LAUNCHER_CONFIG", configPath)

	source, err := resolveRuntimeSource(context.Background())
	if err != nil {
		t.Fatalf("resolveRuntimeSource: %v", err)
	}
	if source.Root != root {
		t.Fatalf("Root = %q, want %q", source.Root, root)
	}
	if source.SourceFrom != "VAMOS_PACKAGE_ROOT" {
		t.Fatalf("SourceFrom = %q, want VAMOS_PACKAGE_ROOT", source.SourceFrom)
	}
	if source.SourceKey == "" {
		t.Fatalf("SourceKey is empty")
	}
}

func TestResolveRuntimeSourceLoadsConfigOverride(t *testing.T) {
	root := fakeRuntimeRoot(t)
	configPath := writeLauncherConfig(t, root)
	t.Setenv("VAMOS_PACKAGE_ROOT", "")
	t.Setenv("VAMOS_LAUNCHER_CONFIG", configPath)

	source, err := resolveRuntimeSource(context.Background())
	if err != nil {
		t.Fatalf("resolveRuntimeSource: %v", err)
	}
	if source.Root != root {
		t.Fatalf("Root = %q, want %q", source.Root, root)
	}
	if source.SourceFrom != configPath {
		t.Fatalf("SourceFrom = %q, want %q", source.SourceFrom, configPath)
	}
}

func TestDefaultLauncherConfigPathUsesXDGStateHome(t *testing.T) {
	t.Setenv("VAMOS_LAUNCHER_CONFIG", "")
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)

	path, err := defaultLauncherConfigPath()
	if err != nil {
		t.Fatalf("defaultLauncherConfigPath: %v", err)
	}
	want := filepath.Join(xdg, "vamos", "launcher.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestDefaultLauncherConfigPathUsesHomeFallback(t *testing.T) {
	t.Setenv("VAMOS_LAUNCHER_CONFIG", "")
	t.Setenv("XDG_STATE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := defaultLauncherConfigPath()
	if err != nil {
		t.Fatalf("defaultLauncherConfigPath: %v", err)
	}
	want := filepath.Join(home, ".local", "state", "vamos", "launcher.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestResolveRuntimeSourceMissingConfigIsActionable(t *testing.T) {
	t.Setenv("VAMOS_PACKAGE_ROOT", "")
	t.Setenv("VAMOS_LAUNCHER_CONFIG", filepath.Join(t.TempDir(), "missing.json"))

	_, err := resolveRuntimeSource(context.Background())
	if err == nil {
		t.Fatalf("resolveRuntimeSource succeeded, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "set VAMOS_PACKAGE_ROOT") || !strings.Contains(msg, "vamos launcher configure --runtime-source-root") {
		t.Fatalf("error %q missing actionable configure guidance", msg)
	}
}

func TestValidateRuntimeSourceRootRejectsRelativeRoot(t *testing.T) {
	_, err := validateRuntimeSourceRoot("relative/path")
	if err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("validateRuntimeSourceRoot relative err = %v, want absolute error", err)
	}
}

func TestValidateRuntimeSourceRootRejectsMissingGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "vamos-runtime"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := validateRuntimeSourceRoot(root)
	if err == nil || !strings.Contains(err.Error(), "missing go.mod") {
		t.Fatalf("validateRuntimeSourceRoot err = %v, want missing go.mod", err)
	}
}

func TestValidateRuntimeSourceRootRejectsMissingRuntimeDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := validateRuntimeSourceRoot(root)
	if err == nil || !strings.Contains(err.Error(), "missing cmd/vamos-runtime") {
		t.Fatalf("validateRuntimeSourceRoot err = %v, want missing cmd/vamos-runtime", err)
	}
}

func TestValidateRuntimeSourceRootAcceptsValidRoot(t *testing.T) {
	root := fakeRuntimeRoot(t)
	validated, err := validateRuntimeSourceRoot("  " + root + "  ")
	if err != nil {
		t.Fatalf("validateRuntimeSourceRoot: %v", err)
	}
	if validated != root {
		t.Fatalf("validated = %q, want %q", validated, root)
	}
	if sourceRootKey(validated) == "" {
		t.Fatalf("sourceRootKey is empty")
	}
}

func fakeRuntimeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "vamos-runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func writeLauncherConfig(t *testing.T, runtimeSourceRoot string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "launcher.json")
	data, err := json.Marshal(LauncherConfig{RuntimeSourceRoot: runtimeSourceRoot})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
