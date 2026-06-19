package markdown

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestResolveHTMLAppletAssetStaysUnderDocumentDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "plans", "demo", "assets"))
	mustWriteFile(t, filepath.Join(root, "plans", "demo", "assets", "app.js"), []byte("console.log('ok')"))

	got, err := resolveHTMLAppletAsset(root, "thoughts/plans/demo/app.html", "plans/demo/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("plans", "demo", "assets", "app.js")) {
		t.Fatalf("asset=%q", got)
	}
}

func TestResolveHTMLAppletAssetRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "plans", "demo"))
	mustWriteFile(t, filepath.Join(root, "secret.js"), []byte("bad"))

	for _, asset := range []string{"../secret.js", "secret.js", "plans/other/app.js"} {
		if _, err := resolveHTMLAppletAsset(root, "thoughts/plans/demo/app.html", asset); err == nil {
			t.Fatalf("asset %q unexpectedly allowed", asset)
		}
	}
}

func TestChildHTMLHeadersSetContainmentHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	childHTMLHeaders(c)

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("bad CSP: %s", csp)
	}
}

func TestServeHTMLAppletStreamsRawHTML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/_render/html/demo.html", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("*")
	c.SetParamValues("demo.html")

	if err := svc.ServeHTMLApplet(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "<h1>Demo</h1>" {
		t.Fatalf("body=%q", got)
	}
	if strings.Contains(rec.Body.String(), "doc-workbench") || strings.Contains(rec.Body.String(), "thoughts-markdown-scroll-region") {
		t.Fatalf("child route returned workbench: %s", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type=%q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
}

func TestHTMLAppletRendererReturnsSandboxedFrame(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RenderThoughtsDocument(t.Context(), "demo.html")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindHTMLApplet {
		t.Fatalf("kind=%q", page.ViewerArgs.DocumentKind)
	}
	if page.ViewerArgs.CommentMode != CommentModeDocumentOnly {
		t.Fatalf("comment mode=%q", page.ViewerArgs.CommentMode)
	}

	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/thoughts/_render/html/demo.html"`) {
		t.Fatalf("missing iframe src: %s", html)
	}
	if !strings.Contains(html, `sandbox="allow-scripts allow-forms allow-downloads"`) {
		t.Fatalf("missing sandbox: %s", html)
	}
	if strings.Contains(html, "allow-same-origin") {
		t.Fatalf("sandbox permits same-origin: %s", html)
	}
}
