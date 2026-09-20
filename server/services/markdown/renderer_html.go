package markdown

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	xhtml "golang.org/x/net/html"
)

const (
	htmlAppletRenderPrefix    = "/thoughts/_render/html/"
	thoughtsAssetPrefix       = "/thoughts/_assets/"
	htmlAppletBridgeScript    = `<script src="/js/frame-comment-bridge.js?v=4" data-commentui-mode="child"></script><script type="module" src="/js/vamos-html-applet.js?v=2"></script>`
	htmlAppletThemeBootScript = `<script data-vamos-theme-boot>
(function () {
  try {
    var t = new URLSearchParams(location.search).get("theme");
    if (t !== "dark" && t !== "light") return;
    var r = document.documentElement;
    r.classList.toggle("dark", t === "dark");
    r.style.colorScheme = t;
  } catch (e) {}
})();
</script>`
)

type HTMLAppletRenderer struct{}

func (r HTMLAppletRenderer) Match(req DocumentRequest) bool {
	return req.Extension == ".html" || req.Extension == ".htm"
}

func (r HTMLAppletRenderer) Render(
	_ context.Context,
	req DocumentRequest,
) (RenderedDocument, error) {
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:  docPath,
		Title: DocumentTitle(docPath, nil),
		Kind:  DocumentKindHTMLApplet,
		Component: HTMLAppletFrame(
			docPath,
			iframeSrcForHTMLApplet(docPath, req.CurrentTheme),
		),
		CommentMode: CommentModeDocumentOnly,
	}, nil
}

func normalizeHTMLAppletTheme(theme string) string {
	switch strings.TrimSpace(theme) {
	case "dark":
		return "dark"
	case "light":
		return "light"
	default:
		return "dark"
	}
}

func iframeSrcForHTMLApplet(docPath, theme string) string {
	rel := NormalizeWorkspaceDocPath(docPath)
	src := htmlAppletRenderPrefix + escapeHTMLAppletPath(path.Clean("/" + rel)[1:])
	values := url.Values{"theme": []string{normalizeHTMLAppletTheme(theme)}}
	return src + "?" + values.Encode()
}

func escapeHTMLAppletPath(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (s *Service) ServeHTMLApplet(c echo.Context) error {
	requestPath := strings.TrimPrefix(c.Param("*"), "/")
	req, err := s.resolveThoughtsDocumentRequest(requestPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if !(HTMLAppletRenderer{}).Match(req) {
		return echo.NewHTTPError(http.StatusUnsupportedMediaType, "not an HTML applet")
	}
	content, err := os.ReadFile(req.FullPath)
	if err != nil {
		return fmt.Errorf("read HTML applet: %w", err)
	}
	childHTMLHeaders(c)
	return c.Blob(
		http.StatusOK,
		"text/html; charset=utf-8",
		prepareHTMLAppletDocument(content, c.QueryParam("theme")),
	)
}

func prepareHTMLAppletDocument(content []byte, theme string) []byte {
	theme = normalizeHTMLAppletTheme(theme)
	content = applyHTMLAppletThemeToRoot(content, theme)
	content = injectHTMLAppletThemeBoot(content)
	return injectHTMLAppletBridge(content)
}

func applyHTMLAppletThemeToRoot(content []byte, theme string) []byte {
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(content))
	offset := 0
	for {
		tokenType := tokenizer.Next()
		raw := tokenizer.Raw()
		if tokenType == xhtml.ErrorToken {
			break
		}
		if tokenType == xhtml.StartTagToken || tokenType == xhtml.DoctypeToken {
			name, _ := tokenizer.TagName()
			if tokenType == xhtml.StartTagToken && bytes.EqualFold(name, []byte("html")) {
				rewritten := rewriteHTMLRootStartTag(raw, theme)
				out := make([]byte, 0, len(content)-len(raw)+len(rewritten))
				out = append(out, content[:offset]...)
				out = append(out, rewritten...)
				out = append(out, content[offset+len(raw):]...)
				return out
			}
		}
		offset += len(raw)
	}
	open := []byte(
		fmt.Sprintf(
			`<html class="%s" style="color-scheme: %s">`,
			htmlRootClassForTheme("", theme),
			theme,
		),
	)
	out := make([]byte, 0, len(open)+len(content)+7)
	out = append(out, open...)
	out = append(out, content...)
	if !bytes.Contains(bytes.ToLower(content), []byte("</html>")) {
		out = append(out, []byte("</html>")...)
	}
	return out
}

func rewriteHTMLRootStartTag(raw []byte, theme string) []byte {
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(raw))
	if tokenizer.Next() != xhtml.StartTagToken {
		return raw
	}
	var (
		classVal string
		styleVal string
		hasClass bool
		hasStyle bool
		other    []string
	)
	for {
		key, val, more := tokenizer.TagAttr()
		if len(key) > 0 {
			k := strings.ToLower(string(key))
			switch k {
			case "class":
				hasClass = true
				classVal = string(val)
			case "style":
				hasStyle = true
				styleVal = string(val)
			default:
				other = append(
					other,
					fmt.Sprintf(`%s="%s"`, string(key), string(val)),
				)
			}
		}
		if !more {
			break
		}
	}
	classVal = htmlRootClassForTheme(classVal, theme)
	styleVal = mergeColorSchemeStyle(styleVal, theme)
	var b strings.Builder
	b.WriteString("<html")
	for _, attr := range other {
		b.WriteByte(' ')
		b.WriteString(attr)
	}
	if hasClass || classVal != "" {
		b.WriteString(` class="`)
		b.WriteString(classVal)
		b.WriteByte('"')
	}
	if hasStyle || styleVal != "" {
		b.WriteString(` style="`)
		b.WriteString(styleVal)
		b.WriteByte('"')
	}
	b.WriteByte('>')
	return []byte(b.String())
}

func htmlRootClassForTheme(existing, theme string) string {
	parts := strings.Fields(existing)
	out := make([]string, 0, len(parts)+1)
	for _, p := range parts {
		if p == "dark" {
			continue
		}
		out = append(out, p)
	}
	if theme == "dark" {
		out = append(out, "dark")
	}
	return strings.Join(out, " ")
}

func mergeColorSchemeStyle(style, theme string) string {
	parts := strings.Split(style, ";")
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		name, _, ok := strings.Cut(p, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "color-scheme") {
			continue
		}
		out = append(out, p)
	}
	out = append(out, "color-scheme: "+theme)
	return strings.Join(out, "; ")
}

func injectHTMLAppletThemeBoot(content []byte) []byte {
	if bytes.Contains(content, []byte("data-vamos-theme-boot")) {
		return content
	}
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(content))
	offset := 0
	headEnd := -1
	htmlEnd := -1
	for {
		tokenType := tokenizer.Next()
		raw := tokenizer.Raw()
		if tokenType == xhtml.ErrorToken {
			break
		}
		if tokenType == xhtml.StartTagToken {
			name, _ := tokenizer.TagName()
			switch {
			case bytes.EqualFold(name, []byte("html")):
				htmlEnd = offset + len(raw)
			case bytes.EqualFold(name, []byte("head")):
				headEnd = offset + len(raw)
			}
		}
		offset += len(raw)
	}
	insertAt := 0
	snippet := htmlAppletThemeBootScript
	switch {
	case headEnd >= 0:
		insertAt = headEnd
	case htmlEnd >= 0:
		insertAt = htmlEnd
		snippet = "<head>" + htmlAppletThemeBootScript + "</head>"
	default:
		snippet = "<head>" + htmlAppletThemeBootScript + "</head>"
	}
	out := make([]byte, 0, len(content)+len(snippet))
	out = append(out, content[:insertAt]...)
	out = append(out, snippet...)
	out = append(out, content[insertAt:]...)
	return out
}

func injectHTMLAppletBridge(content []byte) []byte {
	if bytes.Contains(content, []byte("/js/frame-comment-bridge.js")) ||
		bytes.Contains(content, []byte(`data-commentui-mode="child"`)) {
		return content
	}
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(content))
	offset := 0
	insertAt := -1
	for {
		tokenType := tokenizer.Next()
		raw := tokenizer.Raw()
		if tokenType == xhtml.ErrorToken {
			break
		}
		if tokenType == xhtml.EndTagToken {
			name, _ := tokenizer.TagName()
			if bytes.EqualFold(name, []byte("body")) {
				insertAt = offset
			}
		}
		offset += len(raw)
	}
	if insertAt < 0 {
		insertAt = len(content)
	}
	injected := make([]byte, 0, len(content)+len(htmlAppletBridgeScript))
	injected = append(injected, content[:insertAt]...)
	injected = append(injected, htmlAppletBridgeScript...)
	injected = append(injected, content[insertAt:]...)
	return injected
}

func resolveHTMLAppletAsset(basePath, docPath, assetPath string) (string, error) {
	docRel := NormalizeWorkspaceDocPath(docPath)
	if docRel == "" {
		return "", fmt.Errorf("doc is required")
	}
	docDir := path.Dir(docRel)
	assetRel := strings.TrimPrefix(path.Clean("/"+assetPath), "/")
	if assetRel == "." || assetRel == "" || strings.HasPrefix(assetRel, "../") {
		return "", fmt.Errorf("invalid asset path")
	}
	if docDir != "." && assetRel != docDir && !strings.HasPrefix(assetRel, docDir+"/") {
		return "", fmt.Errorf("asset escapes applet directory")
	}
	abs := filepath.Join(basePath, filepath.FromSlash(assetRel))
	if !pathWithinRoot(filepath.Clean(abs), filepath.Clean(basePath)) {
		return "", fmt.Errorf("asset escapes thoughts root")
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("asset not found")
	}
	return abs, nil
}

func (s *Service) ServeThoughtsAsset(c echo.Context) error {
	assetPath := strings.TrimPrefix(c.Param("*"), "/")
	abs, err := resolveHTMLAppletAsset(s.basePath, c.QueryParam("doc"), assetPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.File(abs)
}

func childHTMLHeaders(c echo.Context) {
	h := c.Response().Header()
	h.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self' data: blob:",
		"script-src 'self' 'unsafe-inline' 'unsafe-eval' blob: https://cdn.jsdelivr.net",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"connect-src 'self'",
		"frame-ancestors 'self'",
		"base-uri 'self'",
	}, "; "))
	h.Set("X-Content-Type-Options", "nosniff")
}
