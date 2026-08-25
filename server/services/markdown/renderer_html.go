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
	htmlAppletRenderPrefix = "/thoughts/_render/html/"
	thoughtsAssetPrefix    = "/thoughts/_assets/"
	htmlAppletBridgeScript = `<script src="/js/frame-comment-bridge.js?v=4" data-commentui-mode="child"></script>`
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
		injectHTMLAppletBridge(content),
	)
}

func injectHTMLAppletBridge(content []byte) []byte {
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
