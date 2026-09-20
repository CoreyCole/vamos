package markdown

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

func TestResolveHTMLAppletAssetStaysUnderDocumentDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "plans", "demo", "assets"))
	mustWriteFile(
		t,
		filepath.Join(root, "plans", "demo", "assets", "app.js"),
		[]byte("console.log('ok')"),
	)

	got, err := resolveHTMLAppletAsset(
		root,
		"thoughts/plans/demo/app.html",
		"plans/demo/assets/app.js",
	)
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
		if _, err := resolveHTMLAppletAsset(
			root,
			"thoughts/plans/demo/app.html",
			asset,
		); err == nil {
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
	for _, want := range []string{
		"frame-ancestors 'self'",
		"script-src 'self' 'unsafe-inline' 'unsafe-eval' blob: https://cdn.jsdelivr.net",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP missing %q: %s", want, csp)
		}
	}
}

func TestServeHTMLAppletInjectsCommentBridgeWithoutChangingArtifact(t *testing.T) {
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
	got := rec.Body.String()
	if !strings.Contains(got, "<h1>Demo</h1>") {
		t.Fatalf("body=%q", got)
	}
	if !strings.Contains(got, htmlAppletBridgeScript) {
		t.Fatalf("missing bridge: %q", got)
	}
	if !strings.Contains(got, `data-vamos-theme-boot`) {
		t.Fatalf("missing theme boot: %q", got)
	}
	saved, err := os.ReadFile(filepath.Join(root, "demo.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(saved); got != "<h1>Demo</h1>" {
		t.Fatalf("saved artifact=%q", got)
	}
	if strings.Contains(rec.Body.String(), "doc-workbench") ||
		strings.Contains(rec.Body.String(), "thoughts-markdown-scroll-region") {
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
		{
			"dark",
			"thoughts/demo.html",
			"dark",
			"/thoughts/_render/html/demo.html?theme=dark",
		},
		{
			"light",
			"thoughts/demo.html",
			"light",
			"/thoughts/_render/html/demo.html?theme=light",
		},
		{
			"default",
			"thoughts/demo.html",
			"",
			"/thoughts/_render/html/demo.html?theme=dark",
		},
		{
			"nested",
			"thoughts/plans/demo report.html",
			"light",
			"/thoughts/_render/html/plans/demo%20report.html?theme=light",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := iframeSrcForHTMLApplet(tt.docPath, tt.theme); got != tt.want {
				t.Fatalf("iframeSrcForHTMLApplet()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestServeHTMLAppletAppliesThemeQuery(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/_render/html/demo.html?theme=light",
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("*")
	c.SetParamValues("demo.html")

	if err := svc.ServeHTMLApplet(c); err != nil {
		t.Fatal(err)
	}
	got := rec.Body.String()
	if strings.Contains(got, `class="dark"`) {
		t.Fatalf("light theme still has dark class: %q", got)
	}
	if !strings.Contains(got, "color-scheme: light") {
		t.Fatalf("missing light color-scheme: %q", got)
	}
	if !strings.Contains(got, htmlAppletBridgeScript) {
		t.Fatalf("missing bridge: %q", got)
	}
}

func TestInjectHTMLAppletBridgeIncludesThemeSyncModule(t *testing.T) {
	got := string(injectHTMLAppletBridge([]byte("<h1>Demo</h1>")))
	if !strings.Contains(got, `src="/js/frame-comment-bridge.js?v=4"`) {
		t.Fatalf("missing comment bridge: %q", got)
	}
	if !strings.Contains(
		got,
		`<script type="module" src="/js/vamos-html-applet.js?v=2"></script>`,
	) {
		t.Fatalf("missing theme module: %q", got)
	}
}

func TestInjectHTMLAppletBridgeStillInjectsWhenAuthorImportsAppletJS(t *testing.T) {
	src := []byte(
		`<html><body><script type="module">import("/js/vamos-html-applet.js?v=2")</script></body></html>`,
	)
	got := string(injectHTMLAppletBridge(src))
	if !strings.Contains(got, `src="/js/frame-comment-bridge.js?v=4"`) {
		t.Fatalf("author applet import skipped comment bridge: %q", got)
	}
	if !strings.Contains(got, `data-commentui-mode="child"`) {
		t.Fatalf("missing child comment mode: %q", got)
	}
	if n := strings.Count(got, "/js/frame-comment-bridge.js"); n != 1 {
		t.Fatalf("comment bridge count=%d body=%q", n, got)
	}
	twice := string(injectHTMLAppletBridge([]byte(got)))
	if twice != got {
		t.Fatalf("double inject changed output\nonce=%q\ntwice=%q", got, twice)
	}
}

func TestHTMLAppletJSPromotesThoughtsPageClicksToParent(t *testing.T) {
	body, err := os.ReadFile(
		filepath.Join("..", "..", "..", "static", "js", "vamos-html-applet.js"),
	)
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, want := range []string{
		"thoughtsPageHrefForParent",
		`window.top.location.assign(href)`,
		`"/thoughts/raw/"`,
		`"/thoughts/_render/"`,
		`"/thoughts/_assets/"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("missing %q in vamos-html-applet.js", want)
		}
	}
}

func TestInjectHTMLAppletBridgeFindsRealClosingBody(t *testing.T) {
	original := []byte(
		`<html><body><!-- </body> --><script>const marker = "</body>";</script><p>Demo</p></body></html>`,
	)
	got := injectHTMLAppletBridge(original)
	want := []byte(
		`<html><body><!-- </body> --><script>const marker = "</body>";</script><p>Demo</p>` + htmlAppletBridgeScript + `</body></html>`,
	)
	if !bytes.Equal(got, want) {
		t.Fatalf("injected HTML=%q want %q", got, want)
	}
	if restored := bytes.Replace(
		got,
		[]byte(htmlAppletBridgeScript),
		nil,
		1,
	); !bytes.Equal(
		restored,
		original,
	) {
		t.Fatalf("original bytes changed: got %q want %q", restored, original)
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
	for _, want := range []string{
		`data-commentui-frame`,
		`data-commentui-bridge-id="` + commentui.FrameCommentBridgeID("thoughts/demo.html") + `"`,
		`data-commentui-transport="opaque-postmessage"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing frame comment marker %q: %s", want, html)
		}
	}
	if !strings.Contains(html, `src="/thoughts/_render/html/demo.html?theme=light"`) {
		t.Fatalf("missing iframe src: %s", html)
	}
	if !strings.Contains(html, `sandbox="allow-scripts allow-forms allow-downloads`) {
		t.Fatalf("missing sandbox: %s", html)
	}
	if !strings.Contains(html, `allow-popups allow-popups-to-escape-sandbox`) {
		t.Fatalf("sandbox does not permit new tabs: %s", html)
	}
	if !strings.Contains(html, "allow-same-origin") {
		t.Fatalf("sandbox missing allow-same-origin: %s", html)
	}
	if !strings.Contains(html, "allow-top-navigation-by-user-activation") {
		t.Fatalf("sandbox missing allow-top-navigation-by-user-activation: %s", html)
	}
	if strings.Contains(html, "allow-top-navigation") &&
		!strings.Contains(html, "allow-top-navigation-by-user-activation") {
		t.Fatalf("sandbox broadened with top-navigation: %s", html)
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
	if !strings.Contains(
		html,
		`class="h-full min-h-0 w-full flex-1 border-0 bg-background"`,
	) {
		t.Fatalf(
			"iframe must fill the surface with shell background: %s",
			html,
		)
	}
	if strings.Contains(html, "bg-white") {
		t.Fatalf("iframe still uses bg-white: %s", html)
	}
	if strings.Contains(html, "min-h-[70vh]") || strings.Contains(html, "rounded-lg") {
		t.Fatalf("HTML renderer keeps inset/card sizing: %s", html)
	}
}

func TestRenderThoughtsDocumentDocsIndexHTMLIsCommentable(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html><body>docs</body></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, requestPath := range []string{
		"docs/vamos/index.html",
		"thoughts/docs/vamos/index.html",
		"docs/vamos",
	} {
		page, err := svc.RenderThoughtsDocument(t.Context(), requestPath)
		if err != nil {
			t.Fatalf("%s: %v", requestPath, err)
		}
		if page == nil {
			t.Fatalf("%s: nil page", requestPath)
		}
		if page.ViewerArgs.CommentMode != CommentModeDocumentOnly {
			t.Fatalf(
				"%s CommentMode=%q want %q",
				requestPath,
				page.ViewerArgs.CommentMode,
				CommentModeDocumentOnly,
			)
		}
		if page.ViewerArgs.DocumentKind != DocumentKindHTMLApplet {
			t.Fatalf("%s kind=%q", requestPath, page.ViewerArgs.DocumentKind)
		}
	}
}

func TestPrepareHTMLAppletDocumentDarkTheme(t *testing.T) {
	src := []byte(
		`<!doctype html><html lang="en"><head><link rel="stylesheet" href="app.css"></head><body>Hi</body></html>`,
	)
	got := string(prepareHTMLAppletDocument(src, "dark"))
	if !strings.Contains(got, `class="dark"`) && !strings.Contains(got, `class="dark `) &&
		!strings.Contains(got, ` class="dark"`) {
		if !strings.Contains(got, "dark") {
			t.Fatalf("missing dark class: %q", got)
		}
	}
	if !strings.Contains(got, `lang="en"`) {
		t.Fatalf("lost lang: %q", got)
	}
	if !strings.Contains(got, "color-scheme: dark") {
		t.Fatalf("missing dark color-scheme: %q", got)
	}
	boot := strings.Index(got, "data-vamos-theme-boot")
	style := strings.Index(got, `rel="stylesheet"`)
	if boot < 0 || style < 0 || boot > style {
		t.Fatalf(
			"boot must precede stylesheet: boot=%d style=%d body=%q",
			boot,
			style,
			got,
		)
	}
	if !strings.Contains(got, htmlAppletBridgeScript) {
		t.Fatalf("missing module bridge: %q", got)
	}
}

func TestPrepareHTMLAppletDocumentLightTheme(t *testing.T) {
	src := []byte(`<html class="dark foo"><head></head><body></body></html>`)
	got := string(prepareHTMLAppletDocument(src, "light"))
	if strings.Contains(got, "class=\"dark\"") || strings.Contains(got, " dark") {
		// foo must remain; dark must not
	}
	if !strings.Contains(got, "foo") {
		t.Fatalf("lost existing class: %q", got)
	}
	if strings.Contains(got, "class=\"dark") || strings.Contains(got, " dark\"") ||
		strings.Contains(got, "class=\"dark foo\"") {
		t.Fatalf("dark class should be removed for light: %q", got)
	}
	if !strings.Contains(got, "color-scheme: light") {
		t.Fatalf("missing light color-scheme: %q", got)
	}
}

func TestPrepareHTMLAppletDocumentPreservesHTMLClasses(t *testing.T) {
	src := []byte(`<html lang="en" class="foo bar"><head></head><body></body></html>`)
	got := string(prepareHTMLAppletDocument(src, "dark"))
	if !strings.Contains(got, "foo") || !strings.Contains(got, "bar") {
		t.Fatalf("lost classes: %q", got)
	}
	if !strings.Contains(got, "dark") {
		t.Fatalf("missing dark: %q", got)
	}
	if !strings.Contains(got, `lang="en"`) {
		t.Fatalf("lost lang: %q", got)
	}
}

func TestPrepareHTMLAppletDocumentIdempotent(t *testing.T) {
	src := []byte(
		`<html><head><link rel="stylesheet" href="a.css"></head><body>x</body></html>`,
	)
	once := prepareHTMLAppletDocument(src, "dark")
	twice := prepareHTMLAppletDocument(once, "dark")
	if string(once) != string(twice) {
		t.Fatalf("double prepare changed output\nonce=%q\ntwice=%q", once, twice)
	}
	if n := strings.Count(string(once), "data-vamos-theme-boot"); n != 1 {
		t.Fatalf("boot count=%d body=%q", n, once)
	}
	if n := strings.Count(string(once), "/js/vamos-html-applet.js?v=2"); n != 1 {
		t.Fatalf("bridge count=%d body=%q", n, once)
	}
}
