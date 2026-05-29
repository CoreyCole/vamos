package artifacts

import (
	"os"
	"strings"
	"testing"
)

func TestWriteStaticIndexProducesPortableHTML(t *testing.T) {
	runDir := t.TempDir()
	manifest := RunManifest{
		ID:             "run-1",
		Command:        "go test ./pkg/e2e/generated",
		BaseURL:        "http://localhost:4200",
		ViewportFilter: "mobile,desktop-full",
		RepoCommit:     "abc123",
		Artifacts: []ArtifactEntry{
			{FeatureSlug: "thoughts-workbench", ScenarioSlug: "root-opens", Viewport: "mobile", Label: "page", Kind: ArtifactKindScreenshot, Path: "thoughts-workbench/root-opens/mobile/page.png"},
			{FeatureSlug: "thoughts-workbench", ScenarioSlug: "root-opens", Viewport: "mobile", Label: "page", Kind: ArtifactKindHTML, Path: "thoughts-workbench/root-opens/mobile/page.html"},
		},
	}
	path, err := WriteStaticIndex(manifest, runDir, StaticIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		"<style>",
		DefaultDatastarCDNURL(),
		`data-slot="card"`,
		`data-slot="button"`,
		`data-slot="badge"`,
		`thoughts-workbench/root-opens/mobile/page.png`,
		`thoughts-workbench/root-opens/mobile/page.html`,
		"HTML snapshot",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("index missing %q:\n%s", want, html)
		}
	}
	for _, forbidden := range []string{"/css/", "/js/datastar-pro-v1.js"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("index contains non-portable reference %q:\n%s", forbidden, html)
		}
	}
}

func TestGroupArtifactsByScenarioSortsDeterministically(t *testing.T) {
	entries := []ArtifactEntry{
		{FeatureSlug: "z-feature", ScenarioSlug: "second", Viewport: "desktop-full", Label: "page", Kind: ArtifactKindScreenshot, Path: "z-feature/second/desktop-full/page.png"},
		{FeatureSlug: "a-feature", ScenarioSlug: "second", Viewport: "mobile", Label: "page", Kind: ArtifactKindHTML, Path: "a-feature/second/mobile/page.html"},
		{FeatureSlug: "a-feature", ScenarioSlug: "first", Viewport: "desktop-half", Label: "page", Kind: ArtifactKindScreenshot, Path: "a-feature/first/desktop-half/page.png"},
	}
	groups := groupArtifactsByScenario(entries)
	if got, want := len(groups), 2; got != want {
		t.Fatalf("features=%d want %d", got, want)
	}
	if got, want := groups[0].Slug, "a-feature"; got != want {
		t.Fatalf("first feature=%q want %q", got, want)
	}
	if got, want := groups[0].Scenarios[0].Slug, "first"; got != want {
		t.Fatalf("first scenario=%q want %q", got, want)
	}
	if got, want := groups[0].Scenarios[0].Viewports[0].Name, "desktop-half"; got != want {
		t.Fatalf("first viewport=%q want %q", got, want)
	}
	if groups[0].Scenarios[0].Viewports[0].Screenshot == nil {
		t.Fatal("first viewport screenshot = nil")
	}
}
