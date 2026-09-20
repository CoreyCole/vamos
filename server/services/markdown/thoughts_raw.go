package markdown

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

const thoughtsRawURLPrefix = "/thoughts/raw/"

var (
	errThoughtsRawNotAllowed = errors.New(
		"thoughts raw path is not an allowed raster image",
	)
	errThoughtsRawNotFound = errors.New("thoughts raw file not found")
)

func isRasterImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func rasterContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func escapeThoughtsRelURL(rel string) string {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return ""
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func thoughtsRawURL(rel string) string {
	escaped := escapeThoughtsRelURL(rel)
	if escaped == "" {
		return ""
	}
	return thoughtsRawURLPrefix + escaped
}

func stripThoughtsAppPrefix(dest string) string {
	dest = strings.TrimSpace(dest)
	switch {
	case strings.HasPrefix(dest, thoughtsRawURLPrefix):
		return strings.TrimPrefix(dest, thoughtsRawURLPrefix)
	case strings.HasPrefix(dest, "/thoughts/"):
		return strings.TrimPrefix(dest, "/thoughts/")
	case strings.HasPrefix(dest, "thoughts/"):
		return strings.TrimPrefix(dest, "thoughts/")
	default:
		return dest
	}
}

func resolveMarkdownMediaRel(dest, docRelPath string) (string, error) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.ContainsAny(dest, "\x00\r\n") {
		return "", errInvalidThoughtsDocumentPath
	}
	if i := strings.IndexAny(dest, "?#"); i >= 0 {
		dest = dest[:i]
	}
	parsed, err := url.Parse(dest)
	if err != nil {
		return "", errInvalidThoughtsDocumentPath
	}
	if parsed.Scheme != "" || parsed.Host != "" {
		return "", errInvalidThoughtsDocumentPath
	}
	original := strings.TrimSpace(dest)
	appAbsolute := strings.HasPrefix(original, "/thoughts/") ||
		strings.HasPrefix(original, "thoughts/") ||
		strings.HasPrefix(original, thoughtsRawURLPrefix) ||
		strings.HasPrefix(parsed.Path, "/thoughts/") ||
		strings.HasPrefix(parsed.Path, thoughtsRawURLPrefix)
	rel := stripThoughtsAppPrefix(parsed.Path)
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errInvalidThoughtsDocumentPath
	}
	if !appAbsolute {
		docDir := path.Dir(filepath.ToSlash(strings.Trim(docRelPath, "/")))
		if docDir == "." {
			docDir = ""
		}
		if docDir != "" {
			rel = path.Join(docDir, rel)
		}
	}
	return CanonicalThoughtsDocPath(rel)
}

func rewriteThoughtsRasterHref(dest, docRelPath string) (string, bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", false
	}
	parsed, err := url.Parse(dest)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "http" || scheme == "https" {
		if !isSafeMarkdownLinkDestination(dest) {
			return "", false
		}
		return dest, true
	}
	rel, err := resolveMarkdownMediaRel(dest, docRelPath)
	if err != nil {
		return "", false
	}
	if !isRasterImageExt(path.Ext(rel)) {
		return "", false
	}
	href := thoughtsRawURL(rel)
	return href, href != ""
}

func rewriteThoughtsImageSrc(dest, docRelPath string) (string, bool) {
	return rewriteThoughtsRasterHref(dest, docRelPath)
}

func (s *Service) resolveThoughtsRawFile(
	requestPath string,
) (abs, contentType string, err error) {
	clean, err := CanonicalThoughtsDocPath(requestPath)
	if err != nil {
		return "", "", errInvalidThoughtsDocumentPath
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if !isRasterImageExt(ext) {
		return "", "", errThoughtsRawNotAllowed
	}
	fullPath := filepath.Join(s.basePath, filepath.FromSlash(clean))
	if !pathWithinRoot(filepath.Clean(fullPath), filepath.Clean(s.basePath)) {
		return "", "", errInvalidThoughtsDocumentPath
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", errThoughtsRawNotFound
		}
		return "", "", err
	}
	if info.IsDir() {
		return "", "", errThoughtsRawNotFound
	}
	resolved, resolveErr := filepath.EvalSymlinks(fullPath)
	if resolveErr == nil {
		fullPath = resolved
	}
	if !pathWithinRoot(filepath.Clean(fullPath), filepath.Clean(s.basePath)) {
		return "", "", errInvalidThoughtsDocumentPath
	}
	return fullPath, rasterContentType(ext), nil
}

func (s *Service) ServeThoughtsRaw(c echo.Context) error {
	if remote := strings.TrimSpace(c.QueryParam("url")); remote != "" {
		return echo.NewHTTPError(http.StatusBadRequest, "remote urls are not allowed")
	}
	requestPath := strings.TrimPrefix(c.Param("*"), "/")
	abs, contentType, err := s.resolveThoughtsRawFile(requestPath)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidThoughtsDocumentPath):
			return echo.NewHTTPError(http.StatusBadRequest, "invalid Thoughts path")
		case errors.Is(err, errThoughtsRawNotAllowed):
			return echo.NewHTTPError(http.StatusForbidden, "file type is not allowed")
		case errors.Is(err, errThoughtsRawNotFound):
			return echo.NewHTTPError(http.StatusNotFound, "Thoughts image not found")
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "invalid Thoughts path")
		}
	}
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	if contentType != "" {
		c.Response().Header().Set(echo.HeaderContentType, contentType)
	}
	return c.File(abs)
}
