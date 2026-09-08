package markdown

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		`ArrowUp`,
		`data-thread-artifact-hit`,
		`aria-current=page`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("browser missing %q: %s", want, html)
		}
	}
}

func TestThreadArtifactBrowserShowsResultsWhenSearchHasQuery(t *testing.T) {
	t.Parallel()

	var body strings.Builder
	if err := ThreadArtifactBrowser(ThreadArtifactBrowserArgs{
		DocPath:     "owner/note.md",
		BrowserOpen: false,
	}).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`$_artifactBrowserOpen || $dirSearch`,
		`id="thread-artifact-browser-results"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("collapsed browser missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `style="display: none;"`) {
		t.Fatalf("SSR display:none blocks search from showing results: %s", html)
	}
}

func TestArtifactSearchQueryMatchUsesFileNameNotPathSegments(t *testing.T) {
	t.Parallel()

	path := "owner/plans/alpha/unique-target.md"
	if !artifactSearchQueryMatch("unique-target", "unique-target.md", path) {
		t.Fatal("filename unique-target should match unique-target.md")
	}
	if !artifactSearchQueryMatch("unique-target.md", "unique-target", path) {
		t.Fatal("query unique-target.md should match display name unique-target")
	}
	if !artifactSearchQueryMatch("design.md", "design", "owner/plans/alpha/design.md") {
		t.Fatal("query design.md should match display name design")
	}
	if artifactSearchQueryMatch("alpha", "unique-target.md", path) {
		t.Fatal("parent dir name should not match a file in that path")
	}
	if !artifactSearchQueryMatch("alpha", "alpha", "owner/plans/alpha") {
		t.Fatal("directory name alpha should match")
	}
}

func TestHandleThoughtsArtifactSearchFindsFileByNameNotPathDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for i := 0; i < 40; i++ {
		mustMkdirAll(
			t,
			filepath.Join(root, "owner", "plans", "alpha", fmt.Sprintf("dir-%02d", i)),
		)
	}
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "dir-39", "unique-target.md"),
		[]byte("# Target"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"unique-target",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	if !strings.Contains(
		body,
		`href="/thoughts/owner/plans/alpha/dir-39/unique-target.md"`,
	) {
		t.Fatalf("filename search missed unique-target.md: %s", body)
	}
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", body)
	}
	if !strings.Contains(body[dirStart:globalStart], "unique-target.md") {
		t.Fatalf(
			"this directory should include nested unique-target.md: %s",
			body[dirStart:globalStart],
		)
	}

	design := thoughtsArtifactSearchBody(
		t,
		svc,
		"design.md",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	dirStart = strings.Index(design, `data-testid="artifact-search-this-directory"`)
	globalStart = strings.Index(design, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", design)
	}
	dirSection := design[dirStart:globalStart]
	if !strings.Contains(dirSection, `href="/thoughts/owner/plans/alpha/design.md"`) {
		t.Fatalf("this directory missed design.md by filename: %s", dirSection)
	}

	alpha := thoughtsArtifactSearchBody(
		t,
		svc,
		"alpha",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	if strings.Contains(alpha, "unique-target.md") {
		t.Fatalf("dir-name search should not return files under that path: %s", alpha)
	}
	if strings.Contains(alpha, `>design<`) {
		t.Fatalf("dir-name search should not return design.md: %s", alpha)
	}
	if !strings.Contains(alpha, `>alpha/<`) {
		t.Fatalf("dir-name search missed the alpha directory: %s", alpha)
	}
}

func TestHandleThoughtsArtifactSearchThisDirectoryWalksNestedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustMkdirAll(t, filepath.Join(root, "owner", "shared"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "shared", "notebook.md"),
		[]byte("# Notebook"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"design.md",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans",
	)
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", body)
	}
	dirSection := body[dirStart:globalStart]
	if !strings.Contains(dirSection, `href="/thoughts/owner/plans/alpha/design.md"`) {
		t.Fatalf("this directory missed nested design.md: %s", dirSection)
	}
	if strings.Contains(body[globalStart:], "design.md") &&
		strings.Contains(body[globalStart:], ">design<") {
		t.Fatalf("nested design.md leaked into all thoughts: %s", body[globalStart:])
	}
}

func TestHandleThoughtsArtifactSearchThisDirectoryShowsFullPath(t *testing.T) {
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
		filepath.Join(root, "owner", "plans", "alpha", "docs", "design.md"),
		[]byte("# Nested"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"design.md",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", body)
	}
	dirSection := body[dirStart:globalStart]
	for _, want := range []string{
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha/docs/design.md",
		`text-[10px]`,
		`focus:ring-2`,
	} {
		if !strings.Contains(dirSection, want) {
			t.Fatalf("this directory missing path %q: %s", want, dirSection)
		}
	}
}

func TestHandleThoughtsArtifactSearchDirHitChangesCwd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"docs",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", body)
	}
	dirSection := body[dirStart:globalStart]
	for _, want := range []string{
		`data-thread-artifact-enter`,
		`data-thread-artifact-hit`,
		`data-on:click`,
		`$dirSearch =`,
		thoughtsArtifactBrowserPath,
		"thoughts/owner/plans/alpha/docs",
		">alpha/docs/<",
	} {
		if !strings.Contains(dirSection, want) {
			t.Fatalf("dir hit missing cwd open %q: %s", want, dirSection)
		}
	}
	if strings.Contains(dirSection, `data-thread-artifact-toggle`) ||
		strings.Contains(dirSection, `<details`) {
		t.Fatalf("dir hit should change cwd, not toggle: %s", dirSection)
	}
	if strings.Contains(dirSection, `data-thread-artifact-file`) {
		t.Fatalf("dir hit should not be a file link: %s", dirSection)
	}
}

func TestHandleThoughtsArtifactSearchRootUsesSingleAllThoughtsSection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "design"))
	mustMkdirAll(t, filepath.Join(root, "owner", "shared", "design"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "shared", "notebook.md"),
		[]byte("# Notebook"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"design",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts",
	)
	if strings.Contains(body, `data-testid="artifact-search-this-directory"`) {
		t.Fatalf("root search should not split this directory: %s", body)
	}
	if strings.Contains(body, "No matches") {
		t.Fatalf("root search should not show empty All thoughts: %s", body)
	}
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if globalStart < 0 {
		t.Fatalf("root search missing All thoughts: %s", body)
	}
	section := body[globalStart:]
	for _, want := range []string{
		`href="/thoughts/owner/plans/alpha/design.md"`,
		">alpha/design.md<",
		">alpha/design/<",
		">shared/design/<",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha/design",
		"thoughts/owner/shared/design",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("root All thoughts missing %q: %s", want, section)
		}
	}
}

func TestHandleThoughtsArtifactSearchHitTitleUsesPlanDirAndFilename(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "pi-config-cleanup"))
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "pkg-state-refactor"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "pi-config-cleanup", "design.md"),
		[]byte("# One"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "pkg-state-refactor", "design.md"),
		[]byte("# Two"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"design",
		"thoughts/owner/plans/pi-config-cleanup/design.md",
		"thoughts",
	)
	for _, want := range []string{
		">pi-config-cleanup/design.md<",
		">pkg-state-refactor/design.md<",
		"thoughts/owner/plans/pi-config-cleanup/design.md",
		"thoughts/owner/plans/pkg-state-refactor/design.md",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("search title missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `truncate">design<`) {
		t.Fatalf("search title should not be naked design: %s", body)
	}
}

func TestHandleThoughtsArtifactSearchOrdersHitsByModTime(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	alpha := filepath.Join(root, "owner", "plans", "alpha")
	mustMkdirAll(t, alpha)
	olderPath := filepath.Join(alpha, "recency-alpha.md")
	newerPath := filepath.Join(alpha, "recency-zeta.md")
	mustWriteFile(t, olderPath, []byte("# Older"))
	mustWriteFile(t, newerPath, []byte("# Newer"))
	mustWriteFile(
		t,
		filepath.Join(alpha, "design.md"),
		[]byte("# Design"),
	)
	older := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)
	newer := time.Date(2026, 9, 7, 18, 30, 0, 0, time.Local)
	mustChtimes(t, olderPath, older)
	mustChtimes(t, newerPath, newer)

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := thoughtsArtifactSearchBody(
		t,
		svc,
		"recency",
		"thoughts/owner/plans/alpha/design.md",
		"thoughts/owner/plans/alpha",
	)
	dirStart := strings.Index(body, `data-testid="artifact-search-this-directory"`)
	globalStart := strings.Index(body, `data-testid="artifact-search-all-thoughts"`)
	if dirStart < 0 || globalStart < 0 {
		t.Fatalf("missing search sections: %s", body)
	}
	dirSection := body[dirStart:globalStart]
	newerHref := `href="/thoughts/owner/plans/alpha/recency-zeta.md"`
	olderHref := `href="/thoughts/owner/plans/alpha/recency-alpha.md"`
	newerAt := strings.Index(dirSection, newerHref)
	olderAt := strings.Index(dirSection, olderHref)
	if newerAt < 0 || olderAt < 0 || newerAt > olderAt {
		t.Fatalf(
			"expected newer recency-zeta.md before recency-alpha.md: newer=%d older=%d %s",
			newerAt,
			olderAt,
			dirSection,
		)
	}
	for _, want := range []string{
		artifactSearchHitTimestamp(newer),
		artifactSearchHitTimestamp(older),
	} {
		if !strings.Contains(dirSection, want) {
			t.Fatalf("search hit missing timestamp %q: %s", want, dirSection)
		}
	}
}

func mustChtimes(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func thoughtsArtifactSearchBody(
	t *testing.T,
	svc *Service,
	query, artifact, artifactDir string,
) string {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/thoughts/_artifact-search?q="+query+
				"&artifact="+artifact+
				"&artifact_dir="+artifactDir,
			http.NoBody,
		),
		rec,
	)
	if err := svc.HandleThoughtsArtifactSearch(c); err != nil {
		t.Fatal(err)
	}
	return rec.Body.String()
}
