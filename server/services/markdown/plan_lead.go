package markdown

import (
	"context"
	"database/sql"
	"errors"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func planLeadRoomID(docPath string) string {
	path := filepath.ToSlash(strings.TrimSpace(docPath))
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	if i := strings.Index(path, "thoughts/"); i >= 0 {
		path = path[i:]
	} else if !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + path
	}
	parts := strings.Split(path, "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "thoughts" || parts[i+2] != "plans" {
			continue
		}
		id := strings.TrimSpace(parts[i+3])
		if id == "" || id == "." {
			return ""
		}
		return id
	}
	return ""
}

func planLeadChatHref(docPath string) string {
	return thoughtsChatHref("", docPath)
}

// thoughtsChatHref builds a plain GET /rooms/plan/{id}?artifact=… link for the
// nearest AGENTS.md ancestor of docPath. Classic thoughts/…/plans/{id} roots keep
// id = that plan folder. Other AGENTS roots (docs desks) slug the relative dir
// with "/" → "--" so the existing /rooms/:kind/:id route still matches. Empty
// basePath keeps the plan-only fallback used by roster/overflow unit tests.
func thoughtsChatHref(basePath, docPath string) string {
	id := ""
	if strings.TrimSpace(basePath) != "" {
		root, ok := InferWorkspaceRoot(basePath, docPath)
		if !ok {
			return ""
		}
		id = thoughtsAgentsRoomID(root)
	} else {
		id = planLeadRoomID(docPath)
	}
	if id == "" {
		return ""
	}
	href := "/rooms/plan/" + url.PathEscape(id)
	canonical, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		canonical, err = CanonicalThoughtsDirPath(docPath)
		if err != nil {
			return href
		}
	}
	if canonical == "" {
		return href
	}
	return href + "?artifact=" + url.QueryEscape("thoughts/"+canonical)
}

func (s *Service) rosterSelectionForThoughtsDoc(
	docPath string,
) agenthome.RosterSelection {
	roomID := ""
	if s != nil && strings.TrimSpace(s.basePath) != "" {
		if root, ok := InferWorkspaceRoot(s.basePath, docPath); ok {
			roomID = thoughtsAgentsRoomID(root)
		}
	}
	if roomID == "" {
		roomID = planLeadRoomID(docPath)
	}
	if roomID == "" {
		return agenthome.RosterSelection{}
	}
	return agenthome.RosterSelection{Kind: agenthome.KindPlan, ID: roomID}
}

func thoughtsAgentsRoomID(agentsRoot string) string {
	root := filepath.ToSlash(strings.TrimSpace(agentsRoot))
	root = strings.Trim(root, "/")
	root = strings.TrimPrefix(root, "thoughts/")
	if root == "" {
		return ""
	}
	if id := planLeadRoomID("thoughts/" + root); id != "" {
		return id
	}
	return strings.ReplaceAll(root, "/", "--")
}

func rosterPlanTitle(label, planDirRel string) string {
	title := strings.TrimSpace(label)
	if title != "" {
		return title
	}
	return filepath.Base(filepath.ToSlash(strings.TrimSpace(planDirRel)))
}

func rosterPlanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Mon 3:04 PM")
}

func thoughtsPlanDocPath(planDirRel, name string) string {
	rel := filepath.ToSlash(strings.TrimSpace(planDirRel))
	rel = strings.Trim(rel, "/")
	rel = strings.TrimPrefix(rel, "thoughts/")
	name = strings.TrimSpace(name)
	if rel == "" || name == "" {
		return ""
	}
	return "thoughts/" + rel + "/" + name
}

func thoughtsDesignDocPath(planDirRel string) string {
	return thoughtsPlanDocPath(planDirRel, "design.md")
}

func thoughtsAgentsDocPath(planDirRel string) string {
	return thoughtsPlanDocPath(planDirRel, "AGENTS.md")
}

func thoughtsPlanMdDocPath(planDirRel string) string {
	return thoughtsPlanDocPath(planDirRel, "plan.md")
}

func (s *Service) planDocExists(thoughtsRel string) bool {
	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return false
	}
	rel := filepath.ToSlash(strings.TrimSpace(thoughtsRel))
	rel = strings.Trim(rel, "/")
	rel = strings.TrimPrefix(rel, "thoughts/")
	if rel == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.basePath, filepath.FromSlash(rel)))
	return err == nil
}

func (s *Service) thoughtsPlanArtifactPath(planDirRel string) string {
	design := thoughtsDesignDocPath(planDirRel)
	if s.planDocExists(design) {
		return design
	}
	agents := thoughtsAgentsDocPath(planDirRel)
	if s.planDocExists(agents) {
		return agents
	}
	plan := thoughtsPlanMdDocPath(planDirRel)
	if s.planDocExists(plan) {
		return plan
	}
	return design
}

func (s *Service) rosterPlanRowFromDirRel(
	planDirRel string,
	updatedAt time.Time,
	label string,
) agenthome.RosterPlanRow {
	rel := filepath.ToSlash(strings.TrimSpace(planDirRel))
	rel = strings.Trim(rel, "/")
	rel = strings.TrimPrefix(rel, "thoughts/")
	doc := s.thoughtsPlanArtifactPath(rel)
	id := planLeadRoomID(doc)
	if id == "" {
		id = filepath.Base(rel)
	}
	return agenthome.RosterPlanRow{
		ID:    id,
		Title: rosterPlanTitle(label, rel),
		Href:  planLeadChatHref(doc),
		Time:  rosterPlanTime(updatedAt),
	}
}

func (s *Service) liveRosterDocs() []agenthome.RosterDocRow {
	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return nil
	}
	docsRoot := filepath.Join(s.basePath, "docs")
	entries, err := os.ReadDir(docsRoot)
	if err != nil {
		return nil
	}
	out := make([]agenthome.RosterDocRow, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		href := "/thoughts/docs/" + name + "/"
		if _, err := os.Stat(filepath.Join(docsRoot, name, "index.html")); err == nil {
			href = "/thoughts/docs/" + name + "/index.html"
		}
		out = append(out, agenthome.RosterDocRow{
			ID:    name,
			Title: name,
			Href:  href,
		})
	}
	return out
}

func (s *Service) liveRosterPlans(ctx context.Context) []agenthome.RosterPlanRow {
	if s != nil && s.queries != nil {
		rows, err := s.queries.ListCurrentPlanWorkspaces(ctx, "")
		if err == nil && len(rows) > 0 {
			out := make([]agenthome.RosterPlanRow, 0, len(rows))
			for _, row := range rows {
				out = append(out, s.rosterPlanRowFromDirRel(
					row.PlanDirRel,
					row.ArtifactUpdatedAt,
					row.Label,
				))
			}
			return out
		}
	}
	return s.globRosterPlans()
}

func (s *Service) globRosterPlans() []agenthome.RosterPlanRow {
	if s == nil || strings.TrimSpace(s.basePath) == "" {
		return nil
	}
	var order []string
	byDir := map[string]time.Time{}
	for _, name := range []string{"design.md", "AGENTS.md", "plan.md"} {
		matches, err := filepath.Glob(
			filepath.Join(s.basePath, "*", "plans", "*", name),
		)
		if err != nil {
			continue
		}
		for _, abs := range matches {
			rel, err := filepath.Rel(s.basePath, abs)
			if err != nil {
				continue
			}
			dir := strings.TrimSuffix(filepath.ToSlash(rel), "/"+name)
			if _, seen := byDir[dir]; seen {
				continue
			}
			var updated time.Time
			if st, err := os.Stat(abs); err == nil {
				updated = st.ModTime()
			}
			byDir[dir] = updated
			order = append(order, dir)
		}
	}
	out := make([]agenthome.RosterPlanRow, 0, len(order))
	for _, dir := range order {
		out = append(out, s.rosterPlanRowFromDirRel(dir, byDir[dir], ""))
	}
	return out
}

func (s *Service) HandleBindPlanLead(c echo.Context) error {
	if s.queries == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"database is not configured",
		)
	}
	slug := strings.TrimSpace(c.FormValue("agent_slug"))
	if slug == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent_slug required")
	}
	if s.roster == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"database is not configured",
		)
	}
	agent, err := s.roster.Get(slug)
	if errors.Is(err, roster.ErrNotFound) || errors.Is(err, roster.ErrArchived) {
		return echo.NewHTTPError(http.StatusNotFound, "agent not found")
	}
	if err != nil {
		return err
	}
	artifact := strings.TrimSpace(c.FormValue("artifact"))
	roomID := strings.TrimSpace(c.Param("id"))
	planRel, err := s.resolvePlanDirRelForRoom(c.Request().Context(), roomID, artifact)
	if err != nil {
		return err
	}
	if planRel == "" {
		return echo.NewHTTPError(http.StatusNotFound, "plan workspace not found")
	}
	if err := s.queries.SetPlanWorkspaceLeadAgent(
		c.Request().Context(),
		db.SetPlanWorkspaceLeadAgentParams{
			LeadAgentSlug: sql.NullString{String: agent.Slug, Valid: true},
			PlanDirRel:    planRel,
		},
	); err != nil {
		return err
	}
	userEmail, _ := c.Get("user_email").(string)
	if s.workbenchThreadsRenderer != nil && artifact != "" && userEmail != "" {
		threadID, err := s.workbenchThreadsRenderer.EnsureSharedThreadForDoc(
			c.Request().Context(), artifact, userEmail,
		)
		if err != nil {
			return err
		}
		if threadID != "" {
			row, err := s.queries.GetPlanWorkspace(c.Request().Context(), planRel)
			if err != nil {
				return err
			}
			if err := s.queries.BindAgentThreadPlan(
				c.Request().Context(),
				db.BindAgentThreadPlanParams{
					AgentSlug: sql.NullString{String: agent.Slug, Valid: true},
					Cwd:       row.PlanDir,
					Title:     row.Label,
					ID:        threadID,
				},
			); err != nil {
				return err
			}
		}
	}
	target := "/rooms/plan/" + roomID
	if artifact != "" {
		target += "?artifact=" + artifact
	}
	return c.Redirect(http.StatusSeeOther, target)
}

func (s *Service) planRoomNeedsLead(
	ctx context.Context,
	roomID, artifact string,
) (bool, error) {
	if s.queries == nil {
		return false, nil
	}
	planRel, err := s.resolvePlanDirRelForRoom(ctx, roomID, artifact)
	if err != nil {
		return false, err
	}
	if planRel == "" {
		return true, nil
	}
	row, err := s.queries.GetPlanWorkspace(ctx, planRel)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !row.LeadAgentSlug.Valid ||
		strings.TrimSpace(row.LeadAgentSlug.String) == "", nil
}

func (s *Service) resolvePlanDirRelForRoom(
	ctx context.Context,
	roomID, artifact string,
) (string, error) {
	if s.queries == nil {
		return "", nil
	}
	roomID = strings.TrimSpace(roomID)
	roomSlash := strings.ReplaceAll(roomID, "--", "/")
	for _, candidate := range []string{
		strings.TrimSpace(artifact),
		roomID,
		roomSlash,
	} {
		if candidate == "" {
			continue
		}
		if row, err := s.queries.GetPlanWorkspace(ctx, candidate); err == nil {
			return row.PlanDirRel, nil
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		if !strings.HasPrefix(candidate, "thoughts/") {
			if row, err := s.queries.GetPlanWorkspace(
				ctx,
				"thoughts/"+candidate,
			); err == nil {
				return row.PlanDirRel, nil
			} else if err != nil &&
				!errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
		}
	}
	if artifact != "" {
		if rel, err := s.lookupPlanDirRelFromArtifact(ctx, artifact); err != nil {
			return "", err
		} else if rel != "" {
			return rel, nil
		}
	}
	if roomID != "" {
		rows, err := s.queries.ListCurrentPlanWorkspaces(ctx, "")
		if err != nil {
			return "", err
		}
		for _, row := range rows {
			if row.Label == roomID || strings.HasSuffix(row.PlanDirRel, "/"+roomID) {
				return row.PlanDirRel, nil
			}
		}
	}
	return "", nil
}

func (s *Service) lookupPlanDirRelFromArtifact(
	ctx context.Context,
	artifact string,
) (string, error) {
	artifact = strings.TrimSpace(artifact)
	if artifact == "" {
		return "", nil
	}
	candidates := []string{artifact}
	if strings.HasPrefix(artifact, "thoughts/") {
		candidates = append(candidates, strings.TrimPrefix(artifact, "thoughts/"))
	}
	for _, c := range candidates {
		c = strings.TrimSuffix(c, "/")
		if i := strings.LastIndex(c, "/"); i > 0 {
			dir := c[:i]
			if row, err := s.queries.GetPlanWorkspace(ctx, dir); err == nil {
				return row.PlanDirRel, nil
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
		}
	}
	return "", nil
}

// planLeadBindComponent is retained for HandleBindPlanLead tests only.
// ServeAI470Room must not render it — plan lead is not a roster persona bind gate.
func (s *Service) planLeadBindComponent(
	ctx context.Context,
	roomID, artifact string,
) templ.Component {
	var b strings.Builder
	b.WriteString(`<div id="plan-lead-bind" class="space-y-3 p-4">`)
	b.WriteString(
		`<p class="text-sm text-muted-foreground">Pick a roster agent as this plan&apos;s lead. Composer stays disabled until a lead is bound.</p>`,
	)
	b.WriteString(`<form id="plan-lead-bind-form" method="post" action="/rooms/plan/`)
	b.WriteString(html.EscapeString(roomID))
	b.WriteString(`/lead" class="flex flex-col gap-2">`)
	if artifact != "" {
		b.WriteString(`<input type="hidden" name="artifact" value="`)
		b.WriteString(html.EscapeString(artifact))
		b.WriteString(`"/>`)
	}
	b.WriteString(`<label class="text-sm" for="plan-lead-agent-slug">Lead agent</label>`)
	b.WriteString(
		`<select id="plan-lead-agent-slug" name="agent_slug" class="rounded-md border border-border bg-background px-2 py-1 text-sm">`,
	)
	if s.roster != nil {
		agents, err := s.roster.List()
		if err == nil {
			for _, agent := range agents {
				b.WriteString(`<option value="`)
				b.WriteString(html.EscapeString(agent.Slug))
				b.WriteString(`">`)
				b.WriteString(html.EscapeString(agent.Name))
				b.WriteString(`</option>`)
			}
		}
	}
	b.WriteString(`</select>`)
	b.WriteString(
		`<button type="submit" class="rounded-md border border-border px-3 py-1 text-sm">Bind lead</button>`,
	)
	b.WriteString(`</form></div>`)
	return templ.Raw(b.String())
}
