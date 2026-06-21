package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeHandleLauncherCommandIgnoresRuntimeArgs(t *testing.T) {
	var out bytes.Buffer
	handled, err := maybeHandleLauncherCommand(context.Background(), []string{"qrspi", "--help"}, &out)
	if err != nil {
		t.Fatalf("maybeHandleLauncherCommand: %v", err)
	}
	if handled {
		t.Fatalf("handled runtime args")
	}
}

func TestLauncherConfigureWritesConfigBeforeRuntimeStateExists(t *testing.T) {
	root := fakeRuntimeRoot(t)
	configPath := filepath.Join(t.TempDir(), "state", "launcher.json")
	var out bytes.Buffer

	handled, err := maybeHandleLauncherCommand(context.Background(), []string{"launcher", "configure", "--config", configPath, "--runtime-source-root", root}, &out)
	if err != nil {
		t.Fatalf("launcher configure: %v", err)
	}
	if !handled {
		t.Fatalf("launcher configure was not handled")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg LauncherConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.RuntimeSourceRoot != root {
		t.Fatalf("runtime_source_root = %q, want %q", cfg.RuntimeSourceRoot, root)
	}
	if !strings.Contains(out.String(), "configured launcher config") || !strings.Contains(out.String(), root) {
		t.Fatalf("configure output = %q", out.String())
	}
}

func TestConfigureLauncherStateUsesDefaultConfigPath(t *testing.T) {
	root := fakeRuntimeRoot(t)
	stateHome := t.TempDir()
	t.Setenv("VAMOS_LAUNCHER_CONFIG", "")
	t.Setenv("XDG_STATE_HOME", stateHome)

	if err := configureLauncherState("", root); err != nil {
		t.Fatalf("configureLauncherState: %v", err)
	}
	path := filepath.Join(stateHome, "vamos", "launcher.json")
	cfg, err := loadLauncherConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RuntimeSourceRoot != root {
		t.Fatalf("runtime_source_root = %q, want %q", cfg.RuntimeSourceRoot, root)
	}
}

func TestLauncherConfigureRejectsInvalidArgsAndRoots(t *testing.T) {
	var out bytes.Buffer
	_, err := maybeHandleLauncherCommand(context.Background(), []string{"launcher", "configure"}, &out)
	if err == nil || !strings.Contains(err.Error(), "--runtime-source-root") {
		t.Fatalf("missing runtime root err = %v", err)
	}

	out.Reset()
	_, err = maybeHandleLauncherCommand(context.Background(), []string{"launcher", "configure", "--runtime-source-root", "relative"}, &out)
	if err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("relative runtime root err = %v", err)
	}
}

func TestLauncherDoctorReportsValidConfigCacheFingerprintAndBuild(t *testing.T) {
	root := fakeRuntimeFingerprintRoot(t)
	configPath := filepath.Join(t.TempDir(), "launcher.json")
	if err := configureLauncherState(configPath, root); err != nil {
		t.Fatalf("configureLauncherState: %v", err)
	}
	cacheDir := t.TempDir()
	t.Setenv("VAMOS_LAUNCHER_CACHE", cacheDir)
	withBuildRuntimeFunc(t, func(_ context.Context, _ string, outputPath string) error {
		writeFile(t, outputPath, "runtime")
		return nil
	})

	var out bytes.Buffer
	handled, err := maybeHandleLauncherCommand(context.Background(), []string{"launcher", "doctor", "--config", configPath}, &out)
	if err != nil {
		t.Fatalf("launcher doctor: %v", err)
	}
	if !handled {
		t.Fatalf("launcher doctor was not handled")
	}
	got := out.String()
	for _, want := range []string{"config:", "runtime source root:", "fingerprint:", "cache:", "managed runtime:", "status: ok"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q: %s", want, got)
		}
	}
}

func TestLauncherDoctorMissingConfigIsActionable(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.json")
	var out bytes.Buffer
	_, err := maybeHandleLauncherCommand(context.Background(), []string{"launcher", "doctor", "--config", configPath}, &out)
	if err == nil {
		t.Fatalf("launcher doctor succeeded, want error")
	}
	if !strings.Contains(err.Error(), "vamos launcher configure --runtime-source-root") {
		t.Fatalf("doctor error missing configure guidance: %v", err)
	}
}

func TestLauncherHelpAndBadArgsAreHandledBeforeRuntime(t *testing.T) {
	var out bytes.Buffer
	handled, err := maybeHandleLauncherCommand(context.Background(), []string{"launcher", "help"}, &out)
	if err != nil {
		t.Fatalf("launcher help: %v", err)
	}
	if !handled || !strings.Contains(out.String(), "vamos launcher configure") {
		t.Fatalf("help handled=%v output=%q", handled, out.String())
	}

	out.Reset()
	handled, err = maybeHandleLauncherCommand(context.Background(), []string{"launcher", "unknown"}, &out)
	if err == nil || !handled {
		t.Fatalf("unknown command handled=%v err=%v", handled, err)
	}
}
