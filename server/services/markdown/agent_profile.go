package markdown

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func (s *Service) HandleUpdateAgentProfile(c echo.Context) error {
	kind, ok := agenthome.ParseKind(c.Param("kind"))
	if !ok || kind != agenthome.KindDM {
		return echo.NewHTTPError(http.StatusNotFound, "unknown room kind")
	}
	slug := strings.TrimSpace(c.Param("id"))
	if err := validateAgentSlug(slug); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rel := strings.TrimSpace(c.FormValue("file"))
	body := c.FormValue("body")
	if err := s.writeAgentProfileFile(slug, rel, []byte(body)); err != nil {
		return err
	}
	return c.Redirect(
		http.StatusSeeOther,
		agenthome.ProfileGETPath(kind, slug, rel),
	)
}

func (s *Service) writeAgentProfileFile(slug, rel string, body []byte) error {
	path, err := s.agentProfileAbsPath(slug, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func (s *Service) agentProfileAbsPath(slug, rel string) (string, error) {
	if err := validateAgentSlug(slug); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	clean, err := canonicalAgentProfileRel(rel)
	if err != nil {
		return "", err
	}
	root := filepath.Join(s.basePath, "agents", slug)
	abs := filepath.Join(root, filepath.FromSlash(clean))
	if !pathWithinRoot(abs, root) {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid profile path")
	}
	return abs, nil
}

func canonicalAgentProfileRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid profile path")
	}
	switch rel {
	case "AGENTS.md", "MEMORY.md", "USER.md":
		return rel, nil
	}
	if strings.HasPrefix(rel, "skills/") && rel != "skills/" {
		base := strings.TrimPrefix(rel, "skills/")
		if base == "" || strings.Contains(base, "/") {
			return "", echo.NewHTTPError(http.StatusBadRequest, "invalid profile path")
		}
		return rel, nil
	}
	if strings.HasPrefix(rel, "sessions/handoffs/") && rel != "sessions/handoffs/" {
		base := strings.TrimPrefix(rel, "sessions/handoffs/")
		if base == "" || strings.Contains(base, "/") {
			return "", echo.NewHTTPError(http.StatusBadRequest, "invalid profile path")
		}
		return rel, nil
	}
	return "", echo.NewHTTPError(http.StatusBadRequest, "invalid profile path")
}
