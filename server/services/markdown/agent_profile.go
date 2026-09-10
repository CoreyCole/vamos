package markdown

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func profileView(c echo.Context) bool {
	return agenthome.ProfileView(c)
}

func mobileActiveRegionForRoom(_ string) string {
	return "workbench-v2-artifact"
}

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

func (s *Service) agentProfilePane(
	kind agenthome.RoomKind,
	slug, selected string,
) templ.Component {
	files, err := s.listAgentProfileFiles(slug)
	if err != nil {
		return WorkbenchUnavailable("Agent profile files are unavailable.")
	}
	args := agenthome.ProfilePaneArgs{
		Kind:     kind,
		Slug:     slug,
		Files:    files,
		Selected: strings.TrimSpace(selected),
	}
	if args.Selected != "" {
		body, err := s.readAgentProfileFile(slug, args.Selected)
		if err != nil {
			args.Selected = ""
		} else {
			args.Body = string(body)
		}
	}
	return agenthome.ProfilePane(args)
}

func (s *Service) listAgentProfileFiles(slug string) ([]agenthome.ProfileFile, error) {
	if err := validateAgentSlug(slug); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	root := filepath.Join(s.basePath, "agents", slug)
	var out []agenthome.ProfileFile
	for _, name := range []string{"AGENTS.md", "MEMORY.md", "USER.md"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		out = append(out, agenthome.ProfileFile{RelPath: name})
	}
	out = append(out, listAgentProfileDir(root, "skills")...)
	out = append(out, listAgentProfileDir(root, filepath.Join("sessions", "handoffs"))...)
	return out, nil
}

func listAgentProfileDir(root, rel string) []agenthome.ProfileFile {
	dir := filepath.Join(root, rel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []agenthome.ProfileFile
	prefix := filepath.ToSlash(rel)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		out = append(out, agenthome.ProfileFile{
			RelPath: prefix + "/" + entry.Name(),
		})
	}
	return out
}

func (s *Service) readAgentProfileFile(slug, rel string) ([]byte, error) {
	path, err := s.agentProfileAbsPath(slug, rel)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "profile file not found")
		}
		return nil, err
	}
	return body, nil
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
