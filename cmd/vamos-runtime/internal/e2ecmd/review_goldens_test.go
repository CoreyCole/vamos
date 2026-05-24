package e2ecmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/artifacts"
)

func TestReviewCommandHasRunFlag(t *testing.T) {
	cmd := NewReviewCommand()
	for _, name := range []string{"run", "baseline", "plan-dir"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}

func TestRunReviewRequiresRunDir(t *testing.T) {
	if err := RunReview(context.Background(), ReviewConfig{}); err == nil {
		t.Fatal("RunReview() error=nil want --run error")
	}
}

func TestRunReviewWritesRunArtifact(t *testing.T) {
	runDir := t.TempDir()
	writeManifest(t, runDir, artifacts.RunManifest{ID: "run-1"})
	if err := RunReview(
		context.Background(),
		ReviewConfig{RunDir: runDir, Baseline: "main"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "e2e-visual.md")); err != nil {
		t.Fatal(err)
	}
}

func TestGoldensAcceptCommandHasHumanApprovalFlag(t *testing.T) {
	cmd := NewGoldensCommand()
	accept, _, err := cmd.Find([]string{"accept"})
	if err != nil {
		t.Fatal(err)
	}
	if accept.Flags().Lookup("human-approved") == nil {
		t.Fatal("missing --human-approved")
	}
}

func TestRunGoldensAcceptRequiresHumanApproval(t *testing.T) {
	runDir := t.TempDir()
	writeManifest(t, runDir, artifacts.RunManifest{ID: "run-1"})
	err := RunGoldensAccept(
		context.Background(),
		GoldensConfig{RunDir: runDir, GoldenRoot: t.TempDir()},
	)
	if err == nil {
		t.Fatal("RunGoldensAccept() error=nil want approval error")
	}
}

func writeManifest(t *testing.T, dir string, manifest artifacts.RunManifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
