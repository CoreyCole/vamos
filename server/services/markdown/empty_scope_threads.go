package markdown

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) HandleCreateBotScopeThread(c echo.Context) error {
	slug := strings.TrimSpace(c.Param("slug"))
	if err := s.requireKnownBot(slug); err != nil {
		return err
	}
	if err := seedBotHomeTree(s.basePath, slug, slug); err != nil {
		return err
	}
	cwd := filepath.Join(s.basePath, "agents", slug)
	title := emptyScopeTitle(c.FormValue("prompt"), slug)
	thread, err := s.createEmptyScopeThread(c.Request().Context(), emptyScopeCreate{
		UserEmail: userEmailFromContext(c),
		Title:     title,
		Cwd:       cwd,
	})
	if err != nil {
		return err
	}
	if err := s.queries.BindAgentThreadBotHome(
		c.Request().Context(),
		db.BindAgentThreadBotHomeParams{
			AgentSlug: sql.NullString{String: slug, Valid: true},
			Cwd:       cwd,
			Title:     title,
			ID:        thread.ID,
		},
	); err != nil {
		return err
	}
	if err := s.writeEmptyScopePiSession(
		c.Request().Context(),
		thread,
		cwd,
		botPiJSONLPath(s.basePath, slug, thread.PiSessionID),
	); err != nil {
		return err
	}
	return datastarRedirectToThread(c, thread.ID)
}

func (s *Service) HandleCreatePlanScopeThread(c echo.Context) error {
	roomID := strings.TrimSpace(c.Param("id"))
	artifact, _, err := optionalThreadArtifact(c.FormValue("artifact"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rel, err := s.resolvePlanDirRelForRoom(c.Request().Context(), roomID, artifact)
	if err != nil {
		return err
	}
	if rel == "" {
		rel = strings.ReplaceAll(roomID, "--", "/")
	}
	rel = strings.Trim(strings.TrimPrefix(filepath.ToSlash(rel), "thoughts/"), "/")
	if rel == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plan scope is required")
	}
	cwd := filepath.Join(s.basePath, filepath.FromSlash(rel))
	title := emptyScopeTitle(c.FormValue("prompt"), filepath.Base(rel))
	thread, err := s.createEmptyScopeThread(c.Request().Context(), emptyScopeCreate{
		UserEmail:  userEmailFromContext(c),
		Title:      title,
		Cwd:        cwd,
		PlanDirRel: sql.NullString{String: rel, Valid: true},
	})
	if err != nil {
		return err
	}
	lead := sql.NullString{}
	if row, err := s.queries.GetPlanWorkspace(c.Request().Context(), rel); err == nil {
		lead = row.LeadAgentSlug
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := s.queries.BindAgentThreadPlan(
		c.Request().Context(),
		db.BindAgentThreadPlanParams{
			AgentSlug: lead,
			Cwd:       cwd,
			Title:     title,
			ID:        thread.ID,
		},
	); err != nil {
		return err
	}
	piPath := filepath.Join(cwd, ".vamos", "sessions", "pi", thread.PiSessionID+".jsonl")
	if err := s.writeEmptyScopePiSession(
		c.Request().Context(),
		thread,
		cwd,
		piPath,
	); err != nil {
		return err
	}
	return datastarRedirectToThread(c, thread.ID)
}

func (s *Service) HandleCreateFreeformScopeThread(c echo.Context) error {
	cwd := strings.TrimSpace(c.FormValue("cwd"))
	if cwd == "" {
		cwd = s.basePath
	}
	title := emptyScopeTitle(c.FormValue("prompt"), "Untitled")
	thread, err := s.createEmptyScopeThread(c.Request().Context(), emptyScopeCreate{
		UserEmail: userEmailFromContext(c),
		Title:     title,
		Cwd:       cwd,
	})
	if err != nil {
		return err
	}
	piPath := filepath.Join(cwd, ".vamos", "sessions", "pi", thread.PiSessionID+".jsonl")
	if err := s.writeEmptyScopePiSession(
		c.Request().Context(),
		thread,
		cwd,
		piPath,
	); err != nil {
		return err
	}
	return datastarRedirectToThread(c, thread.ID)
}

type emptyScopeCreate struct {
	UserEmail  string
	Title      string
	Cwd        string
	PlanDirRel sql.NullString
}

func (s *Service) createEmptyScopeThread(
	ctx context.Context,
	in emptyScopeCreate,
) (db.AgentThread, error) {
	if s.queries == nil {
		return db.AgentThread{}, echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"database is not configured",
		)
	}
	userEmail := strings.TrimSpace(in.UserEmail)
	if userEmail == "" {
		userEmail = "shared"
	}
	return s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:          uuid.NewString(),
		UserEmail:   userEmail,
		Title:       in.Title,
		Cwd:         in.Cwd,
		LineageID:   uuid.NewString(),
		ProjectID:   "",
		PlanDirRel:  in.PlanDirRel,
		PiSessionID: uuid.NewString(),
	})
}

func (s *Service) writeEmptyScopePiSession(
	ctx context.Context,
	thread db.AgentThread,
	cwd, absPath string,
) error {
	if err := writeEmptyScopePiHeader(absPath, thread.PiSessionID, cwd); err != nil {
		return err
	}
	_, err := s.queries.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
		ID:           uuid.NewString(),
		IdentityKind: "global_pi",
		ArtifactPath: sql.NullString{String: absPath, Valid: absPath != ""},
		Agent:        "pi",
		ExternalSessionID: sql.NullString{
			String: thread.PiSessionID,
			Valid:  thread.PiSessionID != "",
		},
		Cwd:               sql.NullString{String: cwd, Valid: cwd != ""},
		ProjectionState:   "unassigned",
		ProjectedThreadID: sql.NullString{String: thread.ID, Valid: true},
	})
	return err
}

func datastarRedirectToThread(c echo.Context, threadID string) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect("/threads/" + threadID)
}

func userEmailFromContext(c echo.Context) string {
	userEmail, _ := c.Get("user_email").(string)
	return strings.TrimSpace(userEmail)
}

func emptyScopeTitle(prompt, fallback string) string {
	title := strings.TrimSpace(prompt)
	if title == "" {
		title = strings.TrimSpace(fallback)
	}
	if title == "" {
		title = "Untitled"
	}
	const max = 80
	if utf8.RuneCountInString(title) <= max {
		return title
	}
	runes := []rune(title)
	return string(runes[:max])
}

func botPiJSONLPath(thoughtsRoot, slug, piSessionID string) string {
	return filepath.Join(
		thoughtsRoot,
		"agents",
		slug,
		"sessions",
		"pi",
		piSessionID+".jsonl",
	)
}

func emptyScopeComposerAction(kind, id string) string {
	switch kind {
	case "dm":
		return "@post('/rooms/dm/" + id + "/threads', {contentType: 'form'})"
	case "plan":
		return "@post('/rooms/plan/" + id + "/threads', {contentType: 'form'})"
	case "freeform":
		return "@post('/rooms/freeform/threads', {contentType: 'form'})"
	default:
		return ""
	}
}

func emptyScopeModeLabel(kind, id, attachedDoc string) string {
	switch kind {
	case "dm":
		return "agent"
	case "freeform":
		return "freeform"
	case "plan":
		if emptyScopeIsDocsDesk(id, attachedDoc) {
			return "docs"
		}
		return "plan"
	default:
		return "freeform"
	}
}

func emptyScopeIsDocsDesk(id, attachedDoc string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if strings.HasPrefix(id, "docs--") {
		return true
	}
	doc := strings.ToLower(filepath.ToSlash(strings.TrimSpace(attachedDoc)))
	doc = strings.TrimPrefix(doc, "thoughts/")
	return strings.HasPrefix(doc, "docs/") || strings.Contains(doc, "/docs/")
}

func writeEmptyScopePiHeader(absPath, sessionID, cwd string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(absPath); err == nil && info.Size() > 0 {
		return nil
	}
	raw, err := json.Marshal(map[string]string{
		"type":      "session",
		"id":        sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"cwd":       cwd,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, append(raw, '\n'), 0o644)
}
