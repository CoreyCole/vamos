package markdown

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRewriteThoughtsImageSrcRelativeFromDoc(t *testing.T) {
	t.Parallel()
	src, ok := rewriteThoughtsImageSrc(
		"./shots/desktop/02-thread-open.png",
		"vamos/docs/guide.md",
	)
	if !ok {
		t.Fatal("expected rewrite")
	}
	if src != "/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png" {
		t.Fatalf("src=%q", src)
	}
	src, ok = rewriteThoughtsImageSrc("shots/mobile/01-closed.png", "vamos/docs/guide.md")
	if !ok || src != "/thoughts/raw/vamos/docs/shots/mobile/01-closed.png" {
		t.Fatalf("shots/ rewrite src=%q ok=%v", src, ok)
	}
}

func TestRewriteThoughtsImageSrcExistingAppPath(t *testing.T) {
	t.Parallel()
	src, ok := rewriteThoughtsImageSrc(
		"/thoughts/vamos/docs/shots/desktop/02-thread-open.png",
		"other/doc.md",
	)
	if !ok || src != "/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png" {
		t.Fatalf("src=%q ok=%v", src, ok)
	}
	src, ok = rewriteThoughtsImageSrc(
		"/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png",
		"other/doc.md",
	)
	if !ok || src != "/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png" {
		t.Fatalf("double prefix src=%q ok=%v", src, ok)
	}
}

func TestRewriteThoughtsImageSrcHTTPSAndEscape(t *testing.T) {
	t.Parallel()
	src, ok := rewriteThoughtsImageSrc("https://example.com/a.png", "vamos/docs/guide.md")
	if !ok || src != "https://example.com/a.png" {
		t.Fatalf("https src=%q ok=%v", src, ok)
	}
	if _, ok := rewriteThoughtsImageSrc(
		"javascript:alert(1)",
		"vamos/docs/guide.md",
	); ok {
		t.Fatal("javascript should not rewrite")
	}
	src, ok = rewriteThoughtsImageSrc("../shots/x.png", "vamos/docs/guide.md")
	if !ok || src != "/thoughts/raw/vamos/shots/x.png" {
		t.Fatalf("parent join src=%q ok=%v", src, ok)
	}
	if _, ok := rewriteThoughtsImageSrc("./icon.svg", "vamos/docs/guide.md"); ok {
		t.Fatal("svg should not rewrite")
	}
}

func TestRewriteThoughtsRasterHrefTextLink(t *testing.T) {
	t.Parallel()
	href, ok := rewriteThoughtsRasterHref("./shot.png", "owner/docs/a.md")
	if !ok || href != "/thoughts/raw/owner/docs/shot.png" {
		t.Fatalf("href=%q ok=%v", href, ok)
	}
}

func TestMarkdownBytesToHTMLForDocRewritesImages(t *testing.T) {
	t.Parallel()
	r, err := NewRenderer("github-dark")
	if err != nil {
		t.Fatal(err)
	}
	html, err := r.MarkdownBytesToHTMLForDoc(
		[]byte(
			"See ![shot](./shots/desktop/02-thread-open.png) and [open](./shots/desktop/02-thread-open.png).",
		),
		"vamos/docs/guide.md",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		html,
		`src="/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png"`,
	) {
		t.Fatalf("missing img src: %s", html)
	}
	if !strings.Contains(html, `alt="shot"`) {
		t.Fatalf("missing alt: %s", html)
	}
	if !strings.Contains(
		html,
		`href="/thoughts/raw/vamos/docs/shots/desktop/02-thread-open.png"`,
	) {
		t.Fatalf("missing raster text link: %s", html)
	}
	if strings.Contains(html, `src="./shots/`) {
		t.Fatalf("relative src leaked: %s", html)
	}
	html, err = r.MarkdownBytesToHTMLForDoc(
		[]byte("![x](./icon.svg)"),
		"vamos/docs/guide.md",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "icon.svg") {
		t.Fatalf("svg should be omitted: %s", html)
	}
}

func TestServeThoughtsRawHappyPathAndRejects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "shots"))
	mustWriteFile(t, filepath.Join(root, "shots", "a.png"), []byte("\x89PNG"))
	mustWriteFile(t, filepath.Join(root, "shots", "a.svg"), []byte("<svg></svg>"))
	mustWriteFile(t, filepath.Join(root, "note.md"), []byte("# n"))
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "secret.png"), []byte("secret"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/raw/shots/a.png", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("*")
	c.SetParamValues("shots/a.png")
	if err := svc.ServeThoughtsRaw(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().
		Get(echo.HeaderContentType); !strings.Contains(
		ct,
		"image/png",
	) {
		t.Fatalf("content-type=%q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}

	for _, tc := range []struct {
		path string
		url  string
		want int
	}{
		{path: "shots/a.svg", url: "/thoughts/raw/shots/a.svg", want: http.StatusForbidden},
		{path: "note.md", url: "/thoughts/raw/note.md", want: http.StatusForbidden},
		{path: "../secret.png", url: "/thoughts/raw/../secret.png", want: http.StatusBadRequest},
		{path: "escape/secret.png", url: "/thoughts/raw/escape/secret.png", want: http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("*")
		c.SetParamValues(tc.path)
		err := svc.ServeThoughtsRaw(c)
		httpErr := &echo.HTTPError{}
		if !errors.As(err, &httpErr) {
			t.Fatalf("path %q err=%v", tc.path, err)
		}
		if httpErr.Code != tc.want {
			t.Fatalf("path %q code=%d want=%d", tc.path, httpErr.Code, tc.want)
		}
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/thoughts/raw/shots/a.png?url=https://evil.example/x.png",
		nil,
	)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("*")
	c.SetParamValues("shots/a.png")
	err = svc.ServeThoughtsRaw(c)
	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("remote url query err=%v", err)
	}
}

func TestImageRendererDirectOpen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "hero.png"), []byte("png"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RenderThoughtsDocument(t.Context(), "hero.png")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindImage {
		t.Fatalf("kind=%q", page.ViewerArgs.DocumentKind)
	}
	html := page.ViewerArgs.HTMLContent
	for _, want := range []string{
		`id="thoughts-image-preview"`,
		`src="/thoughts/raw/hero.png"`,
		"hero.png",
		"max-w-full",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
}
