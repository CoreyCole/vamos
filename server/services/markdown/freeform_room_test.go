package markdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/agenthome"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
)

func TestServeFreeformRoomListsEmptyKindRow(t *testing.T) {
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
	threadID := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(
		context.Background(),
		db.CreateAgentThreadParams{
			ID:          threadID,
			UserEmail:   "t@example.com",
			Title:       "Empty kind",
			Cwd:         filepath.Join(root, "freeform"),
			LineageID:   uuid.NewString(),
			PiSessionID: uuid.NewString(),
		},
	); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	renderer := &threadWorkbenchTestRenderer{}
	svc.WithWorkbenchThreadRenderer(renderer)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/freeform", http.NoBody),
		rec,
	)
	c.Set("user_email", "t@example.com")
	if err := svc.ServeFreeformRoom(c); err != nil {
		t.Fatal(err)
	}
	if renderer.chatThreadID != "" {
		t.Fatalf("GET must not auto click-in, got %q", renderer.chatThreadID)
	}
	if renderer.ensureFreeform {
		t.Fatal("GET /rooms/freeform must not ensure a thread")
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="scoped-thread-list"`,
		`href="/threads/` + threadID + `"`,
		"Empty kind",
		`id="roster-row-freeform"`,
		"roster-row-selected",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatal("N=1 must show the list, not composer")
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
		httptest.NewRequest(http.MethodGet, "/rooms/freeform", http.NoBody),
		rec,
	)
	c.Set("user_email", "t@example.com")
	if err := svc.ServeFreeformRoom(c); err != nil {
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
}

func TestParseKindRejectsFreeform(t *testing.T) {
	t.Parallel()
	if _, ok := agenthome.ParseKind("freeform"); ok {
		t.Fatal("ParseKind must 404 unknown kinds including freeform")
	}
}
