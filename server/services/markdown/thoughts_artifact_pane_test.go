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
		`<span>chat about this plan</span>`,
		`data-testid="view-chat"`,
		`title="View Chat"`,
		`href="/thoughts/owner/plans/alpha/notes.md"`,
		`thoughts/owner/plans/alpha/design.md`,
		`data-testid="artifact-browser-search"`,
		`placeholder="Search"`,
		`max-h-64`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("thoughts workbench missing %q in:\n%s", want, html)
		}
	}
	// Old Chat-only thin bar (no Files toggle / path header chrome).
	if strings.Contains(html, "No thread for this plan") {
		t.Fatalf("thoughts workbench still has thin Chat-only empty state")
	}
	overflowStart := strings.Index(html, `data-testid="workbench-overflow-actions"`)
	if overflowStart < 0 {
		t.Fatal("missing overflow actions")
	}
	overflow := html[overflowStart:]
	if end := strings.Index(overflow, `id="thread-artifact-browser"`); end > 0 {
		overflow = overflow[:end]
	}
	if strings.Contains(overflow, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot on thoughts:\n%s", overflow)
	}
	if !strings.Contains(html, `id="thread-artifact-path-header"`) ||
		!strings.Contains(html, `data-testid="workbench-overflow-actions"`) {
		t.Fatalf("expected shared thread artifact chrome, got thin bar only")
	}

	// Shared path header must not be paired with DocumentSurface WorkbenchActions bar.
	if strings.Contains(html, `id="document-header-actions"`) ||
		strings.Contains(html, "Document actions") {
		t.Fatalf("legacy DocumentSurface WorkbenchActions bar present")
	}
}

func TestRemapThreadArtifactBrowserForThoughtsUsesThoughtsHrefs(t *testing.T) {
	t.Parallel()

	selectedDoc := "owner/plans/alpha/design.md"
	got := remapThreadArtifactBrowserForThoughts(ThreadArtifactBrowserArgs{
		DocPath:        selectedDoc,
		DirectoryPath:  "owner/plans/alpha",
		ParentHref:     "/threads?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md&artifact_dir=thoughts%2Fowner%2Fplans",
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
	}, selectedDoc)
	wantParent := ThoughtsDocURLAtDirectory(selectedDoc, "owner/plans")
	wantParentEndpoint := thoughtsArtifactBrowserEndpoint(selectedDoc, "owner/plans")
	if got.ParentHref != wantParent || got.ParentEndpoint != wantParentEndpoint {
		t.Fatalf("parent nav = href %q endpoint %q", got.ParentHref, got.ParentEndpoint)
	}
	if !strings.Contains(got.ParentHref, "/thoughts/owner/plans/alpha/design.md") ||
		!strings.Contains(got.ParentHref, "artifact_dir=") {
		t.Fatalf("parent href dropped selected doc: %q", got.ParentHref)
	}
	wantBrowse := ThoughtsDocURLAtDirectory(selectedDoc, "owner/plans/alpha/docs")
	wantBrowseEndpoint := thoughtsArtifactBrowserEndpoint(
		selectedDoc,
		"owner/plans/alpha/docs",
	)
	wantExpand := thoughtsArtifactDirectoryEndpoint(
		"owner/plans/alpha/docs",
		selectedDoc,
		"owner/plans/alpha",
	)
	if got.Entries[0].BrowseHref != wantBrowse ||
		got.Entries[0].BrowseEndpoint != wantBrowseEndpoint ||
		got.Entries[0].Endpoint != wantExpand {
		t.Fatalf("dir entry = %#v", got.Entries[0])
	}
	if got.Entries[0].Children[0].Href != "/thoughts/owner/plans/alpha/docs/nested.md" {
		t.Fatalf("child href = %q", got.Entries[0].Children[0].Href)
	}
	if got.Entries[1].Href != "/thoughts/owner/plans/alpha/design.md" {
		t.Fatalf("file href = %q", got.Entries[1].Href)
	}
}

func TestThoughtsDirectoryWorkbenchUsesEmptyDocumentAndFilesSearch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design\n"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/owner/plans/alpha", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	dirArgs, err := svc.GetDirectoryListing("owner/plans/alpha")
	if err != nil {
		t.Fatal(err)
	}
	dirArgs.UserEmail = "t@example.com"
	state, err := svc.buildThoughtsDirectoryWorkbenchState(c, dirArgs)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`id="thread-artifact-browser"`,
		`data-testid="artifact-browser-search"`,
		`placeholder="Search"`,
		`max-h-64`,
		"Select a file from the artifact browser.",
		thoughtsArtifactDirectoryPath,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("directory workbench missing %q in:\n%s", want, html)
		}
	}
	for _, notWant := range []string{
		`id="thoughts-directory-primary"`,
		`id="thoughts-directory-scroll-region"`,
	} {
		if strings.Contains(html, notWant) {
			t.Fatalf("directory view still in document pane: %q", notWant)
		}
	}
}

func TestThoughtsArtifactPaneUpKeepsSelectedDoc(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design\n"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "notes.md"),
		[]byte("# Notes\n"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/owner/plans/alpha/design.md?artifact_dir=thoughts/owner/plans",
		nil,
	)
	c := e.NewContext(req, httptest.NewRecorder())
	pane, err := svc.thoughtsArtifactPane(
		c,
		"owner/plans/alpha/design.md",
		&PageArgs{FilePath: "owner/plans/alpha/design.md"},
		templ.Raw(`<article id="doc">design</article>`),
		"/rooms/plan/alpha",
	)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := pane.Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if !strings.Contains(html, `data-thread-artifact-cwd="thoughts/owner/plans"`) {
		t.Fatalf("cwd did not honor artifact_dir:\n%s", html)
	}
	if !strings.Contains(html, `data-testid="artifact-browser-file"`) ||
		!strings.Contains(html, `title="thoughts/owner/plans/alpha/design.md"`) {
		t.Fatalf("path header lost selected doc:\n%s", html)
	}
	if !strings.Contains(html, `data-testid="artifact-browser-search"`) {
		t.Fatalf("file page Files missing search:\n%s", html)
	}
	if !strings.Contains(
		html,
		`href="/thoughts/owner/plans/alpha/design.md?artifact_dir=thoughts%2Fowner"`,
	) {
		t.Fatalf("Up dropped selected doc:\n%s", html)
	}
	if !strings.Contains(html, `data-thread-artifact-up`) {
		t.Fatalf("missing Up:\n%s", html)
	}
}

func TestHandleThoughtsArtifactBrowserKeepsSelectedDoc(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/thoughts/_artifact-browser?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans",
			http.NoBody,
		),
		rec,
	)
	if err := svc.HandleThoughtsArtifactBrowser(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #thread-artifact-browser",
		"selector #thread-artifact-up-slot",
		`data-thread-artifact-cwd="thoughts/owner/plans"`,
		"window.history.pushState",
		`/thoughts/owner/plans/alpha/design.md`,
		`artifact_dir=thoughts%2Fowner%2Fplans`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("thoughts browser SSE missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{
		"selector #thread-artifact-document",
		"selector #workbench-v2-artifact-body",
		`id="thoughts-directory-primary"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("thoughts browser SSE patched %q: %s", forbidden, body)
		}
	}
}

func TestHandleThoughtsArtifactDirectoryExpandsWithThoughtsHrefs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs", "nested"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "docs", "note.md"),
		[]byte("# Note"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/thoughts/_artifact-directory?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha&directory=thoughts/owner/plans/alpha/docs",
			http.NoBody,
		),
		rec,
	)
	if err := svc.HandleThoughtsArtifactDirectory(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		threadArtifactDirectoryID("owner/plans/alpha/docs"),
		`data-loaded="true"`,
		"note.md",
		`href="/thoughts/owner/plans/alpha/docs/note.md"`,
		thoughtsArtifactBrowserPath,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("thoughts directory SSE missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "/threads/") {
		t.Fatalf("thoughts directory expand leaked thread hrefs: %s", body)
	}
}
