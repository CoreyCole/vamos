package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPlanBundleCopiesScreenshotsAndIndex(t *testing.T) {
	runDir := t.TempDir()
	shot := filepath.Join(runDir, "page.png")
	if err := os.WriteFile(shot, []byte("png"), 0o644); err != nil {
		t.Fatalf("write screenshot: %v", err)
	}
	planDir := t.TempDir()
	bundle, err := ExportPlanBundle(
		context.Background(),
		RunManifest{ID: "run-1", Screenshots: []string{shot}},
		PlanBundleOptions{PlanDir: planDir, RunDir: runDir},
	)
	if err != nil {
		t.Fatalf("ExportPlanBundle() error = %v", err)
	}
	if _, err := os.Stat(
		filepath.Join(
			planDir,
			"context",
			"implement",
			"e2e-runs",
			"run-1",
			"screenshots",
			"page.png",
		),
	); err != nil {
		t.Fatalf("copied screenshot missing: %v", err)
	}
	data, err := os.ReadFile(bundle.IndexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(data), "![page.png](screenshots/page.png)") {
		t.Fatalf("index missing screenshot link:\n%s", data)
	}
}
