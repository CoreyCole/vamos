package markdown

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHandleThoughtsArtifactSearchSplitsDirectoryAndGlobal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "notes.md"),
		[]byte("# Notes"),
	)
	mustMkdirAll(t, filepath.Join(root, "owner", "shared"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "shared", "notebook.md"),
		[]byte("# Notebook"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/thoughts/_artifact-search?q=note&artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha",
			http.NoBody,
		),
		rec,
	)
	if err := svc.HandleThoughtsArtifactSearch(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #thread-artifact-browser-results",
		`data-testid="artifact-search-this-directory"`,
		`data-testid="artifact-search-all-thoughts"`,
		"This directory",
		"All thoughts",
		`href="/thoughts/owner/plans/alpha/notes.md"`,
		`href="/thoughts/owner/shared/notebook.md"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("search SSE missing %q: %s", want, body)
		}
	}
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 || dirStart > globalStart {
		t.Fatalf("section order dir=%d global=%d", dirStart, globalStart)
	}
	dirSection := body[dirStart:globalStart]
	if !strings.Contains(dirSection, "/thoughts/owner/plans/alpha/notes.md") {
		t.Fatalf("notes.md should be in this directory: %s", dirSection)
	}
	if strings.Contains(dirSection, "/thoughts/owner/shared/notebook.md") {
		t.Fatalf("notebook.md leaked into this directory: %s", dirSection)
	}
	globalSection := body[globalStart:]
	if !strings.Contains(globalSection, "/thoughts/owner/shared/notebook.md") {
		t.Fatalf("notebook.md should be in all thoughts: %s", globalSection)
	}
	if strings.Contains(globalSection, "/thoughts/owner/plans/alpha/notes.md") {
		t.Fatalf("cwd hit duplicated in all thoughts: %s", globalSection)
	}
}

func TestHandleThoughtsArtifactSearchEmptyQueryReturnsListing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/thoughts/_artifact-search?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha",
			http.NoBody,
		),
		rec,
	)
	if err := svc.HandleThoughtsArtifactSearch(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "This directory") ||
		strings.Contains(body, "All thoughts") {
		t.Fatalf("empty query should not section results: %s", body)
	}
	if !strings.Contains(body, `href="/thoughts/owner/plans/alpha/design.md"`) {
		t.Fatalf("empty query missing cwd listing: %s", body)
	}
}

func TestThreadArtifactBrowserWiresSearchEndpoint(t *testing.T) {
	t.Parallel()

	var body strings.Builder
	if err := ThreadArtifactBrowser(ThreadArtifactBrowserArgs{
		DocPath:       "owner/note.md",
		DirectoryPath: "owner",
		BrowserOpen:   true,
	}).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`id="thread-artifact-browser-results"`,
		thoughtsArtifactSearchPath,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("browser missing %q: %s", want, html)
		}
	}
}
