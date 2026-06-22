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

func TestResolveHTMLAppletAssetAllowsRootDocumentAssets(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "app.css"), []byte("body { color: red; }"))

	got, err := resolveHTMLAppletAsset(root, "thoughts/demo.html", "app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "app.css") {
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

func TestIframeSrcForHTMLAppletAddsNormalizedTheme(t *testing.T) {
	tests := []struct {
		name    string
		docPath string
		theme   string
		want    string
	}{
		{"dark", "thoughts/demo.html", "dark", "/thoughts/_render/html/demo.html?theme=dark"},
		{"light", "thoughts/demo.html", "light", "/thoughts/_render/html/demo.html?theme=light"},
		{"default", "thoughts/demo.html", "", "/thoughts/_render/html/demo.html?theme=dark"},
		{"nested", "thoughts/plans/demo report.html", "light", "/thoughts/_render/html/plans/demo%20report.html?theme=light"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := iframeSrcForHTMLApplet(tt.docPath, tt.theme); got != tt.want {
				t.Fatalf("iframeSrcForHTMLApplet()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestServeHTMLAppletIgnoresThemeQueryAndStreamsRawHTML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/_render/html/demo.html?theme=light", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("*")
	c.SetParamValues("demo.html")

	if err := svc.ServeHTMLApplet(c); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.String(); got != "<h1>Demo</h1>" {
		t.Fatalf("body=%q", got)
	}
}

func TestHTMLAppletRendererReturnsSandboxedFrame(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RenderThoughtsDocumentWithOptions(
		t.Context(),
		"demo.html",
		DocumentRenderOptions{CurrentTheme: "light"},
	)
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
	if !strings.Contains(html, `data-vamos-html-applet`) {
		t.Fatalf("missing applet marker: %s", html)
	}
	if !strings.Contains(html, `src="/thoughts/_render/html/demo.html?theme=light"`) {
		t.Fatalf("missing iframe src: %s", html)
	}
	if !strings.Contains(html, `sandbox="allow-scripts allow-forms allow-downloads"`) {
		t.Fatalf("missing sandbox: %s", html)
	}
	if strings.Contains(html, "allow-same-origin") {
		t.Fatalf("sandbox permits same-origin: %s", html)
	}
	if strings.Contains(html, "HTML applet:") {
		t.Fatalf("HTML renderer includes duplicate chrome: %s", html)
	}
	if strings.Contains(html, "max-w-6xl") || strings.Contains(html, "mx-auto") {
		t.Fatalf("HTML renderer keeps capped wrapper classes: %s", html)
	}
	if !strings.Contains(html, `referrerpolicy="same-origin"`) {
		t.Fatalf("missing referrer policy: %s", html)
	}
	if !strings.Contains(html, `class="h-full min-h-0 w-full flex-1 border-0 bg-background"`) {
		t.Fatalf("iframe no longer fills available width/height: %s", html)
	}
	if strings.Contains(html, "min-h-[70vh]") || strings.Contains(html, "rounded-lg") {
		t.Fatalf("HTML renderer keeps inset/card sizing: %s", html)
	}
}
