package markdown

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func TestThreadArtifactRoutesKeepThreadContext(t *testing.T) {
	t.Parallel()

	if got := ThreadArtifactHref(
		"thread_1",
		"thoughts/owner/plans/alpha/design.md",
	); got != "/threads/thread_1?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md" {
		t.Fatalf("href = %q", got)
	}
	if got := ThreadArtifactDirectoryEndpoint(
		"thread_1",
		"thoughts/owner/plans/alpha/docs",
	); got != "/threads/thread_1/artifact-directory?directory=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs" {
		t.Fatalf("directory endpoint = %q", got)
	}
	if got := ThreadArtifactHrefAtDirectory(
		"thread_1",
		"thoughts/owner/plans/alpha/docs/note.md",
		"thoughts/owner/plans/alpha",
	); got != "/threads/thread_1?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs%2Fnote.md&artifact_dir=thoughts%2Fowner%2Fplans%2Falpha" {
		t.Fatalf("artifact directory href = %q", got)
	}
	if got := ThreadArtifactBrowserEndpoint(
		"thread_1",
		"thoughts/owner/plans/alpha/docs/note.md",
		"thoughts/owner/plans/alpha/docs",
	); got != "/threads/thread_1/artifact-browser?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs%2Fnote.md&artifact_dir=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs" {
		t.Fatalf("artifact browser endpoint = %q", got)
	}
	if got := ThreadArtifactHrefAtDirectory(
		"thread_1",
		"thoughts/owner/plans/alpha/docs/note.md",
		"thoughts",
	); got != "/threads/thread_1?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs%2Fnote.md&artifact_dir=thoughts" {
		t.Fatalf("root artifact directory href = %q", got)
	}
}

func TestThreadArtifactRoutesRoundTripCanonicalIdentity(t *testing.T) {
	t.Parallel()

	for _, artifact := range []string{
		"owner/quote'file.md",
		`owner/back\\slash.md`,
		"owner/line\nbreak.md",
		"owner/query?fragment#.md",
		"owner/unicode-🌰.md",
	} {
		href := ThreadArtifactHref("thread_1", artifact)
		parsed, err := url.Parse(href)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", href, err)
		}
		if got, want := parsed.Query().
			Get("artifact"),
			"thoughts/"+artifact; got != want {
			t.Fatalf("artifact round trip = %q, want %q", got, want)
		}
	}
}

func TestThreadArtifactRoutesRejectInvalidPaths(t *testing.T) {
	t.Parallel()

	if got := ThreadArtifactHref(
		"thread_1",
		"../../etc/passwd",
	); got != "/threads/thread_1" {
		t.Fatalf("href = %q", got)
	}
	if got := ThreadArtifactDirectoryEndpoint("thread_1", "../../etc"); got != "" {
		t.Fatalf("directory endpoint = %q", got)
	}
	if got := ThreadArtifactHrefAtDirectory(
		"thread_1",
		"safe.md",
		"../../etc",
	); got != "/threads/thread_1" {
		t.Fatalf("artifact directory href = %q", got)
	}
	if got := ThreadArtifactBrowserEndpoint(
		"thread_1",
		"safe.md",
		"../../etc",
	); got != "" {
		t.Fatalf("artifact browser endpoint = %q", got)
	}
}

func TestThreadArtifactDirectoryIDsDoNotCollapsePunctuation(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}
	for _, path := range []string{"a-b", "a_b", "a/b", "a b", "a?b"} {
		id := threadArtifactDirectoryID(path)
		if prior, ok := seen[id]; ok {
			t.Fatalf("%q and %q share directory ID %q", prior, path, id)
		}
		seen[id] = path
	}
}

func TestThreadArtifactPaneScopesStaticHandlersToBrowserRows(t *testing.T) {
	t.Parallel()

	malicious := "quote'\\line\n<script>.md"
	entry := ThreadArtifactEntry{
		Name:     malicious,
		Path:     malicious,
		Href:     "/threads/thread_1?artifact=thoughts%2Fsafe.md",
		Endpoint: "/threads/thread_1/artifact?artifact=thoughts%2Fquote%27%5Cline%0A%3Cscript%3E.md",
	}
	var body bytes.Buffer
	if err := ThreadArtifactPane(
		ThreadArtifactBrowserArgs{
			ThreadID:       "thread_1",
			DocPath:        malicious,
			DirectoryPath:  "owner",
			ParentHref:     "/threads/thread_1?artifact=thoughts%2Fsafe.md&artifact_dir=thoughts",
			ParentEndpoint: "/threads/thread_1/artifact-browser?artifact=thoughts%2Fsafe.md&artifact_dir=thoughts",
			Entries:        []ThreadArtifactEntry{entry},
			BrowserOpen:    true,
		},
		templ.Raw(`<p><a href="/thoughts/fullscreen.md">Fullscreen</a></p>`),
	).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`data-thread-artifact-browser`,
		`data-thread-artifact-file`,
		`data-thread-artifact-up`,
		`data-thread-artifact-cwd="thoughts/owner"`,
		`href="/threads/thread_1?artifact=thoughts%2Fsafe.md"`,
		`data-on:click="if (!$_threadArtifactLoading`,
		`href="/thoughts/fullscreen.md"`,
		`_artifactBrowserOpen: true`,
		`data-show="$_artifactBrowserOpen"`,
		`aria-controls="thread-artifact-browser"`,
		`id="thread-artifact-path-header"`,
		`aria-label="Toggle files"`,
		`title="Toggle files"`,
		`wb2_artifact_browser=`,
		`document.cookie`,
		`sessionStorage.setItem('workbench-v2:artifact-browser-open'`,
		`data-attr:d=`,
		`data-artifact-browser-open="1"`,
		`d="M15 19l-7-7 7-7"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("artifact pane missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, ">Up</span>") || strings.Contains(html, ">←</span>") {
		t.Fatalf("Up control must be an icon aligned with files toggle: %s", html)
	}
	if strings.Contains(html, "threadArtifactFileClickAction") ||
		strings.Contains(html, `data-thread-artifact-file" data-artifact-endpoint`) ||
		strings.Contains(html, `data-thread-artifact-file" data-on:click`) ||
		strings.Contains(html, "/threads/thread_1/artifact?") ||
		strings.Contains(html, `href="/thoughts/fullscreen.md" data-on:click`) {
		t.Fatalf(
			"file row was intercepted or document link intercepted: %s",
			html,
		)
	}
}

func TestThreadArtifactBrowserDirectoryCanonicalizesRootFileParent(t *testing.T) {
	t.Parallel()

	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/threads/thread_1", http.NoBody),
		httptest.NewRecorder(),
	)
	got, err := threadArtifactBrowserDirectory(c, "root.md")
	if err != nil || got != "" {
		t.Fatalf("root artifact browser directory = %q, %v", got, err)
	}
}

func TestThreadArtifactBrowserAtThoughtsRootOmitsUpAction(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := ThreadArtifactBrowser(ThreadArtifactBrowserArgs{
		ThreadID:      "thread_1",
		DocPath:       "owner/note.md",
		DirectoryPath: "",
	}).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body.String(), "data-thread-artifact-up") ||
		!strings.Contains(body.String(), `data-thread-artifact-cwd="thoughts"`) {
		t.Fatalf("root browser has invalid Up state: %s", body.String())
	}
}

func TestThreadArtifactDirectoryRendersDisclosureWithoutNavigation(t *testing.T) {
	t.Parallel()

	entry := ThreadArtifactEntry{
		Name:           "docs",
		Path:           "owner/docs",
		Endpoint:       "/threads/thread_1/artifact-directory?directory=thoughts%2Fowner%2Fdocs",
		BrowseHref:     "/threads/thread_1?artifact=thoughts%2Fowner%2Fnote.md&artifact_dir=thoughts%2Fowner%2Fdocs",
		BrowseEndpoint: "/threads/thread_1/artifact-browser?artifact=thoughts%2Fowner%2Fnote.md&artifact_dir=thoughts%2Fowner%2Fdocs",
		TargetID:       threadArtifactDirectoryID("owner/docs"),
		IsDir:          true,
		IsExpanded:     true,
		IsLoaded:       true,
		Children: []ThreadArtifactEntry{
			{
				Name:     "note",
				Path:     "owner/docs/note.md",
				Href:     "/threads/thread_1?artifact=thoughts%2Fowner%2Fdocs%2Fnote.md",
				Endpoint: "/threads/thread_1/artifact?artifact=thoughts%2Fowner%2Fdocs%2Fnote.md",
			},
		},
	}
	var body bytes.Buffer
	if err := ThreadArtifactDirectory(entry).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`<details class="group" open`,
		`data-on:toggle="if (el.open &amp;&amp; el.dataset.loaded !== &#39;true&#39; &amp;&amp; el.dataset.artifactEndpoint)`,
		`aria-controls="` + threadArtifactChildrenID(entry) + `"`,
		`data-thread-artifact-toggle`,
		`data-thread-artifact-enter`,
		`aria-label="Open docs folder"`,
		`href="/threads/thread_1?artifact=thoughts%2Fowner%2Fdocs%2Fnote.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("directory disclosure missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `href="/thoughts/owner/docs"`) ||
		strings.Contains(html, `<summary`) && strings.Contains(
			html[strings.Index(html, `<summary`):strings.Index(html, `</summary>`)],
			`href=`,
		) {
		t.Fatalf("directory summary still navigates: %s", html)
	}
}

func TestArtifactBrowserOpenFromRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/threads/t", http.NoBody)
	if !ArtifactBrowserOpenFromRequest(req) {
		t.Fatal("missing cookie should default open")
	}
	req.AddCookie(&http.Cookie{Name: "wb2_artifact_browser", Value: "0"})
	if ArtifactBrowserOpenFromRequest(req) {
		t.Fatal("cookie 0 should be closed")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/threads/t", http.NoBody)
	req2.AddCookie(&http.Cookie{Name: "wb2_artifact_browser", Value: "1"})
	if !ArtifactBrowserOpenFromRequest(req2) {
		t.Fatal("cookie 1 should be open")
	}
}

func TestThreadArtifactPathHeaderLockedH10(t *testing.T) {
	t.Parallel()
	var body strings.Builder
	if err := ThreadArtifactPane(
		ThreadArtifactBrowserArgs{DocPath: "x.md", BrowserOpen: true},
		templ.Raw("<p>doc</p>"),
	).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	out := body.String()
	idx := strings.Index(out, `id="thread-artifact-path-header"`)
	if idx < 0 {
		t.Fatal("missing path header")
	}
	end := strings.Index(out[idx:], ">")
	tag := out[idx : idx+end]
	for _, want := range []string{"h-10", "min-h-10", "max-h-10"} {
		if !strings.Contains(tag, want) {
			t.Fatalf("path header missing %s: %s", want, tag)
		}
	}
}

func TestSetViewDocumentToggle(t *testing.T) {
	t.Parallel()
	browser := ThreadArtifactBrowserArgs{DocPath: "owner/plans/alpha/design.md"}
	setViewDocumentToggle(&browser, false, "")
	if browser.ViewDocumentHref != "/thoughts/owner/plans/alpha/design.md" ||
		browser.DocumentViewActive {
		t.Fatalf("chat -> thoughts = %#v", browser)
	}
	setViewDocumentToggle(&browser, true, "/rooms/plan/alpha?artifact=x")
	if browser.ViewDocumentHref != "/rooms/plan/alpha?artifact=x" ||
		!browser.DocumentViewActive {
		t.Fatalf("thoughts -> chat = %#v", browser)
	}
}

func TestViewDocumentButtonSitsLeftOfOverflow(t *testing.T) {
	t.Parallel()
	var body strings.Builder
	if err := ThreadArtifactPane(
		ThreadArtifactBrowserArgs{
			DocPath:            "owner/plans/alpha/design.md",
			ViewDocumentHref:   "/thoughts/owner/plans/alpha/design.md",
			DocumentViewActive: false,
			HeaderActions: templ.Raw(
				`<div data-testid="workbench-overflow-actions"></div>`,
			),
		},
		templ.Raw("<p>doc</p>"),
	).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	out := body.String()
	view := strings.Index(out, `data-testid="view-document"`)
	overflow := strings.Index(out, `data-testid="workbench-overflow-actions"`)
	if view < 0 || overflow < 0 || view > overflow {
		t.Fatalf(
			"view document should be left of 3-dot: view=%d overflow=%d\n%s",
			view,
			overflow,
			out,
		)
	}
	if !strings.Contains(out, `title="View Document"`) ||
		!strings.Contains(out, `aria-pressed="false"`) {
		t.Fatalf("missing View Document tooltip/pressed: %s", out)
	}
}
