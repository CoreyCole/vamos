package markdown

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestServeThreadsIndexHasNoChatEnsure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/threads", nil), rec)
	if err := svc.ServeThreads(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, bad := range []string{
		`"Chat"`,
		">Chat<",
		`id="agent-chat-composer"`,
		`id="scoped-thread-list"`,
		`id="thread-chat"`,
		`data-testid="root-threads-index"`,
	} {
		if strings.Contains(body, bad) {
			t.Fatalf("bare GET /threads contains %q", bad)
		}
	}
	if !strings.Contains(body, "Pick a roster scope to see its threads.") {
		t.Fatalf("missing pick-scope copy: %s", body)
	}
}

func TestServeAI470RoomDocsZeroThreadsDoesNotInsert(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "vamos")
	mustMkdirAll(t, docs)
	mustWriteFile(t, filepath.Join(docs, "AGENTS.md"), []byte("# Vamos docs\n"))
	mustWriteFile(t, filepath.Join(docs, "index.html"), []byte("<html></html>"))
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "docs-zero.db"),
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
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	before, err := dbSvc.Queries.ListAgentThreadsByPlanDirRel(
		context.Background(),
		sql.NullString{String: "docs/vamos", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("precondition threads = %d", len(before))
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/plan/docs--vamos", http.NoBody),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "docs--vamos")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	after, err := dbSvc.Queries.ListAgentThreadsByPlanDirRel(
		context.Background(),
		sql.NullString{String: "docs/vamos", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("GET inserted %d threads", len(after))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing N=0 composer: %s", body)
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("N=0 must not render SharedThreadChat")
	}
}

func TestHandleCreateAgentListLandHasRosterDMHref(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
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
	if _, err := svc.roster.Create(roster.Bot{Slug: "nova", Name: "Nova"}); err != nil {
		t.Fatal(err)
	}
	form := strings.NewReader("slug=lead&name=Lead")
	req := httptest.NewRequest(http.MethodPost, "/agents", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "t@example.com")
	if err := svc.HandleCreateAgent(c); err != nil {
		t.Fatal(err)
	}
	if loc := rec.Header().Get("Location"); loc != "/rooms/dm/lead" {
		t.Fatalf("Location = %q want /rooms/dm/lead", loc)
	}
}
