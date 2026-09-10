package markdown

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) HandleCreateAgent(c echo.Context) error {
	if s.queries == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"database is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	agent, err := s.createAgent(c.Request().Context(), createAgentInput{
		Slug:        strings.TrimSpace(c.FormValue("slug")),
		Name:        strings.TrimSpace(c.FormValue("name")),
		Label:       strings.TrimSpace(c.FormValue("label")),
		Description: strings.TrimSpace(c.FormValue("description")),
		UserEmail:   userEmail,
	})
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/rooms/dm/"+agent.Slug)
}

type createAgentInput struct {
	Slug        string
	Name        string
	Label       string
	Description string
	UserEmail   string
}

func (s *Service) createAgent(
	ctx context.Context,
	in createAgentInput,
) (db.Agent, error) {
	slug := strings.TrimSpace(in.Slug)
	if err := validateAgentSlug(slug); err != nil {
		return db.Agent{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = slug
	}
	if s.queries == nil {
		return db.Agent{}, echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"database is not configured",
		)
	}
	if _, err := s.queries.GetAgentBySlug(ctx, slug); err == nil {
		return db.Agent{}, echo.NewHTTPError(
			http.StatusConflict,
			"agent slug already exists",
		)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.Agent{}, err
	}
	agent, err := s.queries.CreateAgent(ctx, db.CreateAgentParams{
		ID:          uuid.NewString(),
		Slug:        slug,
		Name:        name,
		Label:       strings.TrimSpace(in.Label),
		Description: strings.TrimSpace(in.Description),
	})
	if err != nil {
		return db.Agent{}, err
	}
	if err := seedBotHomeTree(s.basePath, slug, name); err != nil {
		return db.Agent{}, err
	}
	if _, err := s.ensureBotHomeThread(
		ctx,
		agent,
		strings.TrimSpace(in.UserEmail),
	); err != nil {
		return db.Agent{}, err
	}
	return agent, nil
}

func (s *Service) ensureBotHomeThread(
	ctx context.Context,
	agent db.Agent,
	userEmail string,
) (string, error) {
	if s.queries == nil {
		return "", nil
	}
	existing, err := s.queries.GetBotHomeThreadByAgentID(ctx, sql.NullString{
		String: agent.ID,
		Valid:  true,
	})
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if strings.TrimSpace(userEmail) == "" {
		userEmail = "shared"
	}
	if err := seedBotHomeTree(s.basePath, agent.Slug, agent.Name); err != nil {
		return "", err
	}
	cwd := filepath.Join(s.basePath, "agents", agent.Slug)
	threadID := uuid.NewString()
	if _, err := s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        threadID,
		UserEmail: userEmail,
		Title:     agent.Name,
		Cwd:       cwd,
		LineageID: uuid.NewString(),
		ProjectID: "",
	}); err != nil {
		return "", err
	}
	if err := s.queries.BindAgentThreadBotHome(ctx, db.BindAgentThreadBotHomeParams{
		AgentID: sql.NullString{String: agent.ID, Valid: true},
		Cwd:     cwd,
		Title:   agent.Name,
		ID:      threadID,
	}); err != nil {
		return "", err
	}
	return threadID, nil
}

func validateAgentSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\\`) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	if slug == "a2a" || strings.HasPrefix(slug, "_") {
		return fmt.Errorf("reserved slug %q", slug)
	}
	return nil
}

func seedBotHomeTree(thoughtsRoot, slug, name string) error {
	if err := validateAgentSlug(slug); err != nil {
		return err
	}
	root := filepath.Join(thoughtsRoot, "agents", slug)
	for _, dir := range []string{
		filepath.Join(root, "skills"),
		filepath.Join(root, "sessions", "history"),
		filepath.Join(root, "sessions", "handoffs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = slug
	}
	files := map[string]string{
		"AGENTS.md": "# " + display + "\n\nRole memory lives in MEMORY.md. Do not embed the live roster here.\n",
		"MEMORY.md": "# Memory\n",
		"USER.md":   "# User\n",
	}
	for fileName, body := range files {
		path := filepath.Join(root, fileName)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	current := filepath.Join(root, "sessions", "current.jsonl")
	_, err := os.Stat(current)
	if os.IsNotExist(err) {
		return os.WriteFile(current, nil, 0o644)
	}
	return err
}
