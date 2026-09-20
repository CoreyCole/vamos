package markdown

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/server/services/agenthome"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func withTestRoster(t *testing.T, svc *Service) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agents.yml")
	svc.WithRoster(&roster.Store{Path: path})
	return path
}

func TestCreateAgentSeedsDiskAndEnsuresHomeThread(t *testing.T) {
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
	rosterPath := withTestRoster(t, svc)

	ctx := context.Background()
	a, err := svc.createAgent(ctx, createAgentInput{
		Slug:      "nova",
		Name:      "Nova",
		UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.createAgent(ctx, createAgentInput{
		Slug:      "research",
		Name:      "Research",
		UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		filepath.Join(root, "agents", "nova", "AGENTS.md"),
		filepath.Join(root, "agents", "nova", "MEMORY.md"),
		filepath.Join(root, "agents", "nova", "USER.md"),
		filepath.Join(root, "agents", "nova", "sessions", "current.jsonl"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	threadA, err := svc.ensureBotHomeThread(ctx, a, "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := svc.ensureBotHomeThread(ctx, b, "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if threadA == threadB {
		t.Fatalf("two slugs shared thread %s", threadA)
	}
	body, err := os.ReadFile(rosterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "slug: nova") ||
		!strings.Contains(string(body), "slug: research") {
		t.Fatalf("roster yaml missing slugs: %s", body)
	}

	gotA, err := svc.resolveBotHomeThread(ctx, "nova", "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if gotA != threadA {
		t.Fatalf("resolve nova = %q want %q", gotA, threadA)
	}
	gotB, err := svc.resolveBotHomeThread(ctx, "research", "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if gotB != threadB {
		t.Fatalf(
			"resolve research = %q want %q (fixture index must not win)",
			gotB,
			threadB,
		)
	}
}

func TestCreateAgentRejectsReservedSlug(t *testing.T) {
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
	_, err = svc.createAgent(
		context.Background(),
		createAgentInput{Slug: "a2a", Name: "Nope"},
	)
	if err == nil {
		t.Fatal("a2a slug accepted")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
	_, err = svc.createAgent(
		context.Background(),
		createAgentInput{Slug: "_lead", Name: "Nope"},
	)
	if err == nil {
		t.Fatal("_ prefix accepted")
	}
}

func TestCreateAgentRejectsArchivedSlug(t *testing.T) {
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
	rosterPath := withTestRoster(t, svc)
	if err := os.WriteFile(
		rosterPath,
		[]byte("bots:\n  - slug: nova\n    name: Nova\n    archived: true\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	_, err = svc.createAgent(
		context.Background(),
		createAgentInput{Slug: "nova", Name: "Nova"},
	)
	if err == nil {
		t.Fatal("archived slug accepted")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusConflict {
		t.Fatalf("err = %v", err)
	}
	body, err := os.ReadFile(rosterPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "slug: nova") != 1 {
		t.Fatalf("un-archived or duplicated slug: %s", body)
	}
}

func TestServeAI470RoomUsesBotHomeThreadNotFixtureIndex(t *testing.T) {
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
	nova, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := svc.ensureBotHomeThread(context.Background(), nova, "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{findID: "fixture-0"})
	got, err := svc.resolveAI470Thread(
		context.Background(),
		agenthome.KindDM,
		"nova",
		"",
		"",
		"",
		"t@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != threadID {
		t.Fatalf("resolve = %q want %q (not fixture-0)", got, threadID)
	}
}

func TestHandleCreateAgentRedirectsToBotListLand(t *testing.T) {
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

	form := strings.NewReader("slug=hermes&name=Hermes")
	req := httptest.NewRequest(http.MethodPost, "/agents", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "t@example.com")
	if err := svc.HandleCreateAgent(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/rooms/dm/hermes" {
		t.Fatalf("Location = %q", loc)
	}
	rows, err := dbSvc.Queries.ListAgentThreadsByAgentSlug(
		context.Background(),
		sql.NullString{String: "hermes", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("create agent inserted %d bot_home rows", len(rows))
	}
}

func TestServeAI470RoomProfileViewListsDiskFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	if err := seedBotHomeTree(root, "nova", "Nova"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "agents", "nova", "USER.md")); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/dm/nova?view=profile", nil),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("dm", "nova")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"AGENTS.md", "MEMORY.md", `id="agent-profile-files"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "USER.md") {
		t.Fatalf("invented missing USER.md: %s", body)
	}
}

func TestHandleUpdateAgentProfileWritesDiskFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedBotHomeTree(root, "nova", "Nova"); err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	form.Set("file", "MEMORY.md")
	form.Set("body", "# updated memory\n")
	req := httptest.NewRequest(
		http.MethodPost,
		"/rooms/dm/nova/profile",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("kind", "id")
	c.SetParamValues("dm", "nova")
	if err := svc.HandleUpdateAgentProfile(c); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "agents", "nova", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# updated memory\n" {
		t.Fatalf("disk body = %q", got)
	}
}
