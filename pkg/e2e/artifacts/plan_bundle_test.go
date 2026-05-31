package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPlanBundlePreservesStructuredArtifactHierarchy(t *testing.T) {
	runDir := t.TempDir()
	paths := []string{
		"thoughts-workbench/root-opens/mobile/page.png",
		"thoughts-workbench/root-opens/desktop-full/page.png",
		"thoughts-workbench/root-opens/mobile/page.html",
	}
	for _, rel := range paths {
		writePlanBundleTestFile(t, filepath.Join(runDir, filepath.FromSlash(rel)), rel)
	}
	writePlanBundleTestFile(t, filepath.Join(runDir, "index.html"), "<html></html>")
	failures := filepath.Join(runDir, "failures.json")
	writePlanBundleTestFile(t, failures, `[]`)

	planDir := filepath.Join(t.TempDir(), "thoughts", "agent", "plans", "example")
	bundle, err := ExportPlanBundle(
		context.Background(),
		RunManifest{
			ID:           "run-1",
			Stories:      []string{"thoughts-workbench"},
			FailuresPath: failures,
			Artifacts: []ArtifactEntry{
				{FeatureSlug: "thoughts-workbench", ScenarioSlug: "root-opens", Viewport: "mobile", Label: "page", Kind: ArtifactKindScreenshot, Path: "thoughts-workbench/root-opens/mobile/page.png"},
				{FeatureSlug: "thoughts-workbench", ScenarioSlug: "root-opens", Viewport: "desktop-full", Label: "page", Kind: ArtifactKindScreenshot, Path: "thoughts-workbench/root-opens/desktop-full/page.png"},
				{FeatureSlug: "thoughts-workbench", ScenarioSlug: "root-opens", Viewport: "mobile", Label: "page", Kind: ArtifactKindHTML, Path: "thoughts-workbench/root-opens/mobile/page.html"},
			},
		},
		PlanBundleOptions{
			PlanDir:     planDir,
			RunDir:      runDir,
			Command:     "go test ./pkg/e2e/tests",
			IncludeHTML: true,
		},
	)
	if err != nil {
		t.Fatalf("ExportPlanBundle() error = %v", err)
	}
	runBundleDir := filepath.Join(planDir, "context", "implement", "e2e-runs", "run-1")
	for _, rel := range append(paths, "index.html", "manifest.json", "failures.json") {
		path := filepath.Join(runBundleDir, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("bundle artifact missing %s: %v", path, err)
		}
	}
	if got, want := len(bundle.ScreenshotPaths), 2; got != want {
		t.Fatalf("ScreenshotPaths len=%d want %d: %#v", got, want, bundle.ScreenshotPaths)
	}
	data, err := os.ReadFile(bundle.IndexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	index := string(data)
	for _, want := range []string{
		"`go test ./pkg/e2e/tests`",
		"[`manifest.json`](manifest.json)",
		"[`failures.json`](failures.json)",
		"[`index.html`](index.html)",
		"thoughts-workbench/root-opens/mobile/page.png",
		"thoughts-workbench/root-opens/desktop-full/page.png",
		"thoughts-workbench/root-opens/mobile/page.html",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("index missing %q:\n%s", want, index)
		}
	}
}

func writePlanBundleTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
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
