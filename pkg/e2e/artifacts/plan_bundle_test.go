package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPlanBundleCopiesArtifactsAndIndex(t *testing.T) {
	runDir := t.TempDir()
	shot := filepath.Join(runDir, "page.png")
	if err := os.WriteFile(shot, []byte("png"), 0o644); err != nil {
		t.Fatalf("write screenshot: %v", err)
	}
	html := filepath.Join(runDir, "page.html")
	if err := os.WriteFile(html, []byte("<main></main>"), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}
	failures := filepath.Join(runDir, "failures.json")
	if err := os.WriteFile(failures, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write failures: %v", err)
	}
	planDir := filepath.Join(t.TempDir(), "thoughts", "agent", "plans", "example")
	bundle, err := ExportPlanBundle(
		context.Background(),
		RunManifest{
			ID:            "run-1",
			Stories:       []string{"durable-session-chat"},
			Screenshots:   []string{shot},
			HTMLSnapshots: []string{html},
			Traces:        []string{filepath.Join(runDir, "trace.zip")},
			FailuresPath:  failures,
		},
		PlanBundleOptions{
			PlanDir:      planDir,
			RunDir:       runDir,
			Command:      "go test ./pkg/e2e/generated",
			IncludeHTML:  true,
			IncludeTrace: true,
		},
	)
	if err != nil {
		t.Fatalf("ExportPlanBundle() error = %v", err)
	}
	runBundleDir := filepath.Join(planDir, "context", "implement", "e2e-runs", "run-1")
	for _, path := range []string{
		filepath.Join(runBundleDir, "screenshots", "page.png"),
		filepath.Join(runBundleDir, "html", "page.html"),
		filepath.Join(runBundleDir, "manifest.json"),
		filepath.Join(runBundleDir, "failures.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("bundle artifact missing %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(bundle.IndexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	index := string(data)
	for _, want := range []string{
		"`go test ./pkg/e2e/generated`",
		"[`manifest.json`](manifest.json)",
		"[`failures.json`](failures.json)",
		"![page.png](screenshots/page.png)",
		"[`page.html`](html/page.html)",
		"trace.zip",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("index missing %q:\n%s", want, index)
		}
	}
}

func TestExportPlanBundleRejectsUnsafePlanDirs(t *testing.T) {
	for _, planDir := range []string{"tmp/not-thoughts", filepath.Join(t.TempDir(), "not-thoughts")} {
		_, err := ExportPlanBundle(
			context.Background(),
			RunManifest{ID: "run-1"},
			PlanBundleOptions{PlanDir: planDir},
		)
		if err == nil {
			t.Fatalf("ExportPlanBundle(%q) error = nil, want unsafe plan dir rejection", planDir)
		}
	}
}
