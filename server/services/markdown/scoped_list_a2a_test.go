package markdown

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestScopedListsExcludePairwiseA2A(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	planDir := filepath.Join(root, "owner", "plans", "alpha")
	mustMkdirAll(t, planDir)
	mustWriteFile(t, filepath.Join(planDir, "design.md"), []byte("# Alpha\n"))
	mustWriteFile(t, filepath.Join(planDir, "AGENTS.md"), []byte("# agents\n"))

	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents.db"),
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
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{
		threadPlanDir: "thoughts/owner/plans/alpha",
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

	botID := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:          botID,
		UserEmail:   "t@example.com",
		Title:       "Nova chat",
		Cwd:         filepath.Join(root, "agents", "nova"),
		LineageID:   uuid.NewString(),
		PiSessionID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.Queries.BindAgentThreadBotHome(ctx, db.BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "nova", Valid: true},
		Cwd:       filepath.Join(root, "agents", "nova"),
		Title:     "Nova chat",
		ID:        botID,
	}); err != nil {
		t.Fatal(err)
	}

	pairID, err := svc.resolvePairwiseThread(ctx, "research", "nova", "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if pairID == "" {
		t.Fatal("expected pairwise thread")
	}

	freeID := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:          freeID,
		UserEmail:   "t@example.com",
		Title:       "Freeform",
		Cwd:         filepath.Join(root, "freeform"),
		LineageID:   uuid.NewString(),
		PiSessionID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}

	planRel := "owner/plans/alpha"
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		ctx,
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:     planRel,
			ProjectID:      "vamos",
			PlanDir:        "thoughts/owner/plans/alpha",
			Label:          "Alpha",
			QrspiLifecycle: "design",
		},
	); err != nil {
		t.Fatal(err)
	}
	planID := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:          planID,
		UserEmail:   "t@example.com",
		Title:       "Plan chat",
		Cwd:         "thoughts/owner/plans/alpha",
		LineageID:   uuid.NewString(),
		PlanDirRel:  sql.NullString{String: planRel, Valid: true},
		PiSessionID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.Queries.BindAgentThreadPlan(ctx, db.BindAgentThreadPlanParams{
		Cwd:   "thoughts/owner/plans/alpha",
		Title: "Plan chat",
		ID:    planID,
	}); err != nil {
		t.Fatal(err)
	}

	assertNoPair := func(t *testing.T, body string) {
		t.Helper()
		if strings.Contains(body, `href="/threads/`+pairID+`"`) {
			t.Fatalf("pairwise thread listed: %s", body)
		}
		if strings.Contains(body, "Messaged 2 Bots") {
			t.Fatalf("fixture chips leaked into scoped list: %s", body)
		}
		if strings.Contains(body, `id="workbench-v2-interagent-chips"`) {
			t.Fatalf("fixture InterAgentMessageChips in scoped list: %s", body)
		}
	}

	t.Run("bot", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(
			httptest.NewRequest(http.MethodGet, "/rooms/dm/nova", http.NoBody),
			rec,
		)
		c.SetParamNames("kind", "id")
		c.SetParamValues("dm", "nova")
		c.Set("user_email", "t@example.com")
		if err := svc.ServeAI470Room(c); err != nil {
			t.Fatal(err)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `href="/threads/`+botID+`"`) {
			t.Fatalf("missing bot thread: %s", body)
		}
		assertNoPair(t, body)
	})

	t.Run("freeform", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(
			httptest.NewRequest(http.MethodGet, "/rooms/freeform", http.NoBody),
			rec,
		)
		c.Set("user_email", "t@example.com")
		if err := svc.ServeFreeformRoom(c); err != nil {
			t.Fatal(err)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `href="/threads/`+freeID+`"`) {
			t.Fatalf("missing freeform thread: %s", body)
		}
		assertNoPair(t, body)
	})

	t.Run("plan", func(t *testing.T) {
		artifact := "thoughts/owner/plans/alpha/design.md"
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
		if !strings.Contains(body, `href="/threads/`+planID+`"`) {
			t.Fatalf("missing plan thread: %s", body)
		}
		assertNoPair(t, body)
	})
}
