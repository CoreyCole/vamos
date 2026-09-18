package markdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/agenthome"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestPairwiseRoomsShareThreadAndURL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "pair.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{
		threadPlanDir: "owner/plans/alpha",
	})

	ctx := context.Background()
	if _, err := svc.createAgent(ctx, createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.createAgent(ctx, createAgentInput{
		Slug: "research", Name: "Research", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	ab, err := svc.resolvePairwiseThread(ctx, "research", "nova", "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	ba, err := svc.resolvePairwiseThread(ctx, "nova", "research", "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ab == "" || ab != ba {
		t.Fatalf("pairwise threads %q vs %q", ab, ba)
	}
	dir := filepath.Join(root, "a2a", "nova__research", "sessions")
	if _, err := os.Stat(filepath.Join(dir, "current.jsonl")); err != nil {
		t.Fatalf("missing pairwise disk: %v", err)
	}
	if _, err := os.Stat(
		filepath.Join(root, "a2a", "nova__research", "MEMORY.md"),
	); err == nil {
		t.Fatal("pairwise seeded MEMORY.md")
	}

	e := echo.New()
	rooms := e.Group("/rooms")
	rooms.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_email", "t@example.com")
			return next(c)
		}
	})
	agenthome.RegisterRoomRoutes(rooms, svc.ServeAI470Room)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/rooms/a2a/research/nova", http.NoBody)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"GET /rooms/a2a/research/nova status %d body %s",
			rec.Code,
			rec.Body.String(),
		)
	}
	if strings.Contains(rec.Body.String(), `id="agent-chat-composer-form"`) {
		t.Fatal("pairwise composer was enabled")
	}
}

func TestPlanRoomWithoutLeadDisablesComposerAndBindingPersists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	planRel := "owner/plans/alpha"
	planAbs := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(planAbs, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(planAbs, "design.md"), []byte("# Design\n"))
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "lead.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		context.Background(),
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        planRel,
			ProjectID:         "p1",
			PlanDir:           planAbs,
			Label:             "alpha",
			ArtifactUpdatedAt: time.Now().UTC(),
			QrspiLifecycle:    "plan",
		},
	); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{
		ensureID:      "thread-plan",
		threadPlanDir: "thoughts/" + planRel,
	})
	ctx := context.Background()
	if _, err := svc.createAgent(ctx, createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	artifact := "thoughts/" + planRel + "/design.md"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/rooms/plan/alpha?artifact="+url.QueryEscape(artifact),
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "alpha")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Contains(body, `id="plan-lead-bind-form"`) {
		t.Fatalf("plan-lead bind UI must not gate plan rooms: %s", body)
	}
	if strings.Contains(body, "Pick a roster agent as this plan") {
		t.Fatal("pick-a-lead copy still present")
	}

	form := strings.NewReader("agent_slug=nova&artifact=" + url.QueryEscape(artifact))
	req := httptest.NewRequest(http.MethodPost, "/rooms/plan/alpha/lead", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	bindRec := httptest.NewRecorder()
	bc := echo.New().NewContext(req, bindRec)
	bc.SetParamNames("id")
	bc.SetParamValues("alpha")
	bc.Set("user_email", "t@example.com")
	if err := svc.HandleBindPlanLead(bc); err != nil {
		t.Fatal(err)
	}
	row, err := dbSvc.Queries.GetPlanWorkspace(ctx, planRel)
	if err != nil {
		t.Fatal(err)
	}
	if !row.LeadAgentSlug.Valid {
		t.Fatal("lead_agent_slug not persisted")
	}
	if row.LeadAgentSlug.String != "nova" {
		t.Fatalf("lead = %q want nova", row.LeadAgentSlug.String)
	}
}
