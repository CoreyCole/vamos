package markdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agenthome"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestCreateAgentSeedsDiskAndEnsuresHomeThread(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)

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
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
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

func TestServeAI470RoomUsesBotHomeThreadNotFixtureIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
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

func TestHandleCreateAgentRedirectsToBotHome(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)

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
}
