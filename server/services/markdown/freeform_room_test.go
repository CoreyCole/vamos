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

func TestServeFreeformRoomListsEmptyKindRow(t *testing.T) {
	t.Parallel()
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/freeform", http.NoBody),
		rec,
	)
	if err := svc.ServeFreeformRoom(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/threads" {
		t.Fatalf("Location = %q, want /threads", got)
	}
	if strings.Contains(rec.Body.String(), `id="scoped-thread-list"`) {
		t.Fatal("GET /rooms/freeform must not render a body list")
	}
}

func TestServeFreeformRoomPreservesRawQuery(t *testing.T) {
	t.Parallel()
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/rooms/freeform?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md",
			http.NoBody,
		),
		rec,
	)
	if err := svc.ServeFreeformRoom(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	want := "/threads?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestServeFreeformRoomZeroThreadsComposerDoesNotInsert(t *testing.T) {
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
	before, err := dbSvc.Queries.ListAgentThreadsFreeform(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("precondition threads = %d", len(before))
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/threads", http.NoBody),
		rec,
	)
	c.Set("user_email", "t@example.com")
	if err := svc.ServeThreads(c); err != nil {
		t.Fatal(err)
	}
	after, err := dbSvc.Queries.ListAgentThreadsFreeform(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("GET inserted %d threads", len(after))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing composer: %s", body)
	}
	if strings.Contains(body, "/thoughts/chat/freeform/send") {
		t.Fatal("N=0 composer must not wire freeform send")
	}
	if strings.Contains(body, `id="scoped-thread-list"`) {
		t.Fatal("N=0 /threads must not render list")
	}
}

func writeJSONLUserMessage(t *testing.T, path, sessionID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","id":"` + sessionID + `"}` + "\n" +
		`{"type":"message","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseKindRejectsFreeform(t *testing.T) {
	t.Parallel()
	if _, ok := agenthome.ParseKind("freeform"); ok {
		t.Fatal("ParseKind must 404 unknown kinds including freeform")
	}
}
