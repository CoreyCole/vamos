package markdown

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

const htmlAppletRenderPrefix = "/thoughts/_render/html/"

type HTMLAppletRenderer struct{}

func (r HTMLAppletRenderer) Match(req DocumentRequest) bool {
	return req.Extension == ".html" || req.Extension == ".htm"
}

func (r HTMLAppletRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:        docPath,
		Title:       DocumentTitle(docPath, nil),
		Kind:        DocumentKindHTMLApplet,
		Component:   HTMLAppletFrame(docPath, iframeSrcForHTMLApplet(docPath)),
		CommentMode: CommentModeDocumentOnly,
	}, nil
}

func iframeSrcForHTMLApplet(docPath string) string {
	rel := NormalizeWorkspaceDocPath(docPath)
	return htmlAppletRenderPrefix + path.Clean("/" + rel)[1:]
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
	return c.Blob(http.StatusOK, "text/html; charset=utf-8", content)
}

func childHTMLHeaders(echo.Context) {}
