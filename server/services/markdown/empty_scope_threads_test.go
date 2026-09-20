package markdown

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestEmptyScopeBotComposerCreatesThreadAndRedirects(t *testing.T) {
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
	if _, err := svc.createAgent(context.Background(), createAgentInput{
		Slug:      "nova",
		Name:      "Nova",
		UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	form := strings.NewReader("prompt=hello+empty+scope")
	req := httptest.NewRequest(http.MethodPost, "/rooms/dm/nova/threads", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("nova")
	c.Set("user_email", "t@example.com")
	if err := svc.HandleCreateBotScopeThread(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "#doc-right-chat-panel") {
		t.Fatal("POST patched leftover right rail")
	}
	if strings.Contains(body, "/thoughts/chat/freeform/send") {
		t.Fatal("wired leftover freeform send")
	}
	if !strings.Contains(body, `window.location.href = "/threads/`) {
		t.Fatalf("missing Datastar redirect: %s", body)
	}
	threads, err := dbSvc.Queries.ListAgentThreadsByAgentSlug(
		context.Background(),
		sql.NullString{String: "nova", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 {
		t.Fatalf("threads = %d", len(threads))
	}
	thread := threads[0]
	if !strings.Contains(body, `/threads/`+thread.ID) {
		t.Fatalf("redirect missing thread id %s: %s", thread.ID, body)
	}
	if strings.TrimSpace(thread.PiSessionID) == "" {
		t.Fatal("missing pi_session_id")
	}
	piPath := botPiJSONLPath(root, "nova", thread.PiSessionID)
	raw, err := os.ReadFile(piPath)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	var header map[string]any
	if err := json.Unmarshal([]byte(line), &header); err != nil {
		t.Fatal(err)
	}
	if header["id"] != thread.PiSessionID {
		t.Fatalf("header id = %#v want %q", header["id"], thread.PiSessionID)
	}
	if header["id"] == header["run_id"] && header["run_id"] != nil {
		t.Fatal("header stamped run_id as session id")
	}
}

func TestEmptyScopeFreeformComposerCreatesThreadAndRedirects(t *testing.T) {
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

	form := strings.NewReader("prompt=freeform+hello")
	req := httptest.NewRequest(http.MethodPost, "/rooms/freeform/threads", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "t@example.com")
	if err := svc.HandleCreateFreeformScopeThread(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "#doc-right-chat-panel") {
		t.Fatal("POST patched leftover right rail")
	}
	threads, err := dbSvc.Queries.ListAgentThreadsFreeform(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 {
		t.Fatalf("threads = %d", len(threads))
	}
	if !strings.Contains(body, `/threads/`+threads[0].ID) {
		t.Fatalf("missing redirect: %s", body)
	}
	piPath := filepath.Join(
		root,
		".vamos",
		"sessions",
		"pi",
		threads[0].PiSessionID+".jsonl",
	)
	if _, err := os.Stat(piPath); err != nil {
		t.Fatal(err)
	}
}

func TestServeAI470RoomEmptyComposerPostsNamedThreadRoute(t *testing.T) {
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
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	withTestRoster(t, svc)
	if _, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova",
		Name: "Nova",
	}); err != nil {
		t.Fatal(err)
	}
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
	if !strings.Contains(body, "/rooms/dm/nova/threads") {
		t.Fatalf("composer missing named POST: %s", body)
	}
	if strings.Contains(body, "/thoughts/chat/freeform/send") {
		t.Fatal("wired leftover freeform send")
	}
}
