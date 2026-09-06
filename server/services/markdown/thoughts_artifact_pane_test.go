package markdown

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestThoughtsArtifactPaneUsesSharedThreadChrome(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docRel := filepath.Join("owner", "plans", "alpha", "design.md")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(docRel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, docRel),
		[]byte("# Design\n\nBody.\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "owner", "plans", "alpha", "notes.md")
	if err := os.WriteFile(sibling, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &Service{basePath: root}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/"+docRel, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	canonical, err := CanonicalThoughtsDocPath(docRel)
	if err != nil {
		t.Fatal(err)
	}
	page := &PageArgs{
		FilePath: canonical,
		ViewerArgs: ViewerArgs{
			RawMarkdown: "# Design\n",
		},
	}
	chatHref := "/threads/thread_demo?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	pane, err := svc.thoughtsArtifactPane(
		c,
		canonical,
		page,
		templ.Raw(`<article id="doc">design</article>`),
		chatHref,
	)
	if err != nil {
		t.Fatal(err)
	}

	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:    "t@example.com",
		Artifact:     pane,
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()

	for _, want := range []string{
		`id="thread-artifact-path-header"`,
		`aria-label="Toggle files"`,
		`id="thread-artifact-browser"`,
		`id="thread-artifact-document"`,
		`aria-label="Artifact actions"`,
		`data-testid="workbench-overflow-actions"`,
		`href="` + chatHref + `"`,
		`<span>Chat</span>`,
		`href="/thoughts/owner/plans/alpha/notes.md"`,
		`thoughts/owner/plans/alpha/design.md`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("thoughts workbench missing %q in:\n%s", want, html)
		}
	}
	// Old Chat-only thin bar (no Files toggle / path header chrome).
	if strings.Contains(html, "No thread for this plan") {
		t.Fatalf("thoughts workbench still has thin Chat-only empty state")
	}
	if !strings.Contains(html, `id="thread-artifact-path-header"`) ||
		!strings.Contains(html, `data-testid="workbench-overflow-actions"`) {
		t.Fatalf("expected shared thread artifact chrome, got thin bar only")
	}

	// Shared path header must not be paired with DocumentSurface WorkbenchActions bar.
	if strings.Contains(html, `id="document-header-actions"`) || strings.Contains(html, "Document actions") {
		t.Fatalf("legacy DocumentSurface WorkbenchActions bar present")
	}
}

func TestRemapThreadArtifactBrowserForThoughtsUsesThoughtsHrefs(t *testing.T) {
	t.Parallel()

	got := remapThreadArtifactBrowserForThoughts(ThreadArtifactBrowserArgs{
		DocPath:       "owner/plans/alpha/design.md",
		DirectoryPath: "owner/plans/alpha",
		ParentHref:    "/threads?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md&artifact_dir=thoughts%2Fowner%2Fplans",
		ParentEndpoint: "/threads/artifact-browser?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md&artifact_dir=thoughts%2Fowner%2Fplans",
		Entries: []ThreadArtifactEntry{
			{
				Name:           "docs",
				Path:           "owner/plans/alpha/docs",
				IsDir:          true,
				BrowseHref:     "/threads?x=1",
				BrowseEndpoint: "/threads/artifact-browser?x=1",
				Endpoint:       "/threads/artifact-directory?x=1",
				Children: []ThreadArtifactEntry{{
					Name: "nested.md",
					Path: "owner/plans/alpha/docs/nested.md",
					Href: "/threads?nested=1",
				}},
			},
			{
				Name: "design.md",
				Path: "owner/plans/alpha/design.md",
				Href: "/threads?design=1",
			},
		},
	})
	if got.ParentHref != "/thoughts/owner/plans" || got.ParentEndpoint != "" {
		t.Fatalf("parent nav = %#v", got)
	}
	if got.Entries[0].BrowseHref != "/thoughts/owner/plans/alpha/docs" ||
		got.Entries[0].BrowseEndpoint != "" ||
		got.Entries[0].Endpoint != "" {
		t.Fatalf("dir entry = %#v", got.Entries[0])
	}
	if got.Entries[0].Children[0].Href != "/thoughts/owner/plans/alpha/docs/nested.md" {
		t.Fatalf("child href = %q", got.Entries[0].Children[0].Href)
	}
	if got.Entries[1].Href != "/thoughts/owner/plans/alpha/design.md" {
		t.Fatalf("file href = %q", got.Entries[1].Href)
	}
}
