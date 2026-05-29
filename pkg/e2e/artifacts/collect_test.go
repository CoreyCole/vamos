package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectRunArtifactsPreservesFeatureScenarioViewport(t *testing.T) {
	runDir := t.TempDir()
	writeFile(t, filepath.Join(runDir, "thoughts-workbench", "root-opens", "mobile", "page.png"), "png")
	writeFile(t, filepath.Join(runDir, "thoughts-workbench", "root-opens", "mobile", "page.html"), "html")
	writeFile(t, filepath.Join(runDir, "thoughts-workbench", "root-opens", "desktop-full", "trace.zip"), "zip")
	writeFile(t, filepath.Join(runDir, "manifest.json"), "{}")

	result, err := CollectRunArtifacts(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(result.Entries), 3; got != want {
		t.Fatalf("entries=%d want %d", got, want)
	}
	wantEntry := ArtifactEntry{
		FeatureSlug:  "thoughts-workbench",
		ScenarioSlug: "root-opens",
		Viewport:     "desktop-full",
		Label:        "trace",
		Kind:         ArtifactKindTrace,
		Path:         "thoughts-workbench/root-opens/desktop-full/trace.zip",
	}
	if result.Entries[0] != wantEntry {
		t.Fatalf("first entry=%#v want %#v", result.Entries[0], wantEntry)
	}
	if len(result.Screenshots) != 1 || len(result.HTMLSnapshots) != 1 || len(result.Traces) != 1 {
		t.Fatalf("legacy arrays not populated: %#v", result)
	}
}

func TestEntryFromRunRelativePathRejectsUnknownShapes(t *testing.T) {
	for _, rel := range []string{
		"page.png",
		"feature/scenario/page.png",
		"feature/scenario/mobile/page.txt",
		"feature//mobile/page.png",
	} {
		if entry, ok := EntryFromRunRelativePath(rel); ok {
			t.Fatalf("EntryFromRunRelativePath(%q)=%#v, true; want false", rel, entry)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
