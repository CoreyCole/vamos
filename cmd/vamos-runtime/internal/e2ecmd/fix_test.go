package e2ecmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/repair"
)

func TestFixCommandHasRunAndApplyFlags(t *testing.T) {
	cmd := NewFixCommand()
	for _, name := range []string{"run", "apply"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}

func TestRunFixRequiresRunDir(t *testing.T) {
	if err := RunFix(context.Background(), FixConfig{}); err == nil {
		t.Fatal("RunFix() error=nil want --run error")
	}
}

func TestRunFixNeedsHumanReturnsTypedError(t *testing.T) {
	runDir := t.TempDir()
	writeFixManifest(t, runDir)
	writeFixFailures(t, runDir, []repair.Failure{{Error: "unsupported story step"}})
	err := RunFix(context.Background(), FixConfig{RunDir: runDir})
	var needsHuman NeedsHumanError
	if !errors.As(err, &needsHuman) {
		t.Fatalf("RunFix() err=%T %[1]v want NeedsHumanError", err)
	}
}

func TestRunFixApplyReturnsNotEnabled(t *testing.T) {
	runDir := t.TempDir()
	writeFixManifest(t, runDir)
	writeFixFailures(t, runDir, []repair.Failure{{Error: "selector not visible"}})
	err := RunFix(context.Background(), FixConfig{RunDir: runDir, Apply: true})
	if err == nil {
		t.Fatal("RunFix(apply) error=nil want not enabled")
	}
}

func writeFixManifest(t *testing.T, dir string) {
	t.Helper()
	data, err := json.Marshal(map[string]string{"id": filepath.Base(dir)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFixFailures(t *testing.T, dir string, failures []repair.Failure) {
	t.Helper()
	data, err := json.Marshal(failures)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "failures.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
