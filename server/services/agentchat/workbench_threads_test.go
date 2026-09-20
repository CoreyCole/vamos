package agentchat

import (
	"database/sql"
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
	serverdb "github.com/CoreyCole/vamos/server/services/db"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

func TestResolveSharedThreadPlanDirReturnsThoughtsIdentity(t *testing.T) {
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	if err := os.MkdirAll(
		filepath.Join(thoughtsRoot, "owner", "plans", "alpha"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "threads.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
	}

	for _, tc := range []struct {
		name, id, cwd string
	}{
		{name: "absolute runtime cwd", id: "absolute", cwd: filepath.Join(thoughtsRoot, "owner", "plans", "alpha")},
		{name: "durable cwd", id: "durable", cwd: "thoughts/owner/plans/alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			threadID := "thread-" + tc.id
			if _, err := database.Queries.CreateAgentThread(
				t.Context(),
				db.CreateAgentThreadParams{
					ID:        threadID,
					UserEmail: "owner@example.com",
					Title:     "Alpha",
					Cwd:       tc.cwd,
					LineageID: threadID,
					ProjectID: "project-1",
				},
			); err != nil {
				t.Fatal(err)
			}
			got, err := service.ResolveSharedThreadPlanDir(t.Context(), threadID)
			if err != nil {
				t.Fatal(err)
			}
			if got != "thoughts/owner/plans/alpha" {
				t.Fatalf("ResolveSharedThreadPlanDir() = %q", got)
			}
		})
	}
}

func TestResolveSharedThreadPlanDirAgentsDesk(t *testing.T) {
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	deskAbs := filepath.Join(thoughtsRoot, "docs", "vamos")
	if err := os.MkdirAll(deskAbs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(deskAbs, "AGENTS.md"),
		[]byte("# desk\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "threads.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
	}
	threadID, err := service.EnsureSharedThreadForDoc(
		t.Context(),
		"thoughts/docs/vamos/index.html",
		"owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if threadID == "" {
		t.Fatal("expected desk thread")
	}
	got, err := service.ResolveSharedThreadPlanDir(t.Context(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "thoughts/docs/vamos" {
		t.Fatalf("ResolveSharedThreadPlanDir() = %q", got)
	}
}

func TestServeThreadDocsDeskIndexHTMLCommentsPanel(t *testing.T) {
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	deskAbs := filepath.Join(thoughtsRoot, "docs", "vamos")
	if err := os.MkdirAll(deskAbs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(deskAbs, "AGENTS.md"),
		[]byte("# desk\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(deskAbs, "index.html"),
		[]byte("<html><body>docs index</body></html>"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "threads.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	renderer := &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
		db:           database.DB(),
	}
	markdownService, err := markdown.NewService(thoughtsRoot, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	markdownService.WithWorkbenchThreadRenderer(renderer)
	threadID, err := renderer.EnsureSharedThreadForDoc(
		t.Context(),
		"thoughts/docs/vamos/index.html",
		"owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	target := "/threads/" + threadID + "?artifact=" + url.QueryEscape(
		"thoughts/docs/vamos/index.html",
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, target, nil), rec)
	c.SetParamNames("threadID")
	c.SetParamValues(threadID)
	c.Set("user_email", "owner@example.com")
	if err := markdownService.ServeThread(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeThread status = %d body=%q", rec.Code, rec.Body.String())
	}
	html := rec.Body.String()
	for _, unwanted := range []string{
		"Comments are unavailable for this artifact.",
		"Comments are unavailable for directories.",
		"Select an artifact to view comments.",
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("comments unavailable: %q", unwanted)
		}
	}
	if !strings.Contains(html, `id="comments-context-panel"`) {
		t.Fatalf("missing comments panel: %s", html)
	}
}

func TestServeThreadDefaultArtifactAcceptsSharedThreadPlanIdentity(t *testing.T) {
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	planDir := filepath.Join(thoughtsRoot, "owner", "plans", "alpha")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "design.md"),
		[]byte("# Alpha design"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "threads.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	renderer := &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
	}
	markdownService, err := markdown.NewService(thoughtsRoot, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	markdownService.WithWorkbenchThreadRenderer(renderer)

	for _, tc := range []struct {
		name, id, cwd string
	}{
		{name: "absolute runtime cwd", id: "absolute", cwd: planDir},
		{name: "durable cwd", id: "durable", cwd: "thoughts/owner/plans/alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			threadID := "thread-" + tc.id
			if _, err := database.Queries.CreateAgentThread(
				t.Context(),
				db.CreateAgentThreadParams{
					ID:        threadID,
					UserEmail: "owner@example.com",
					Title:     "Alpha",
					Cwd:       tc.cwd,
					LineageID: threadID,
					ProjectID: "project-1",
				},
			); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			c := echo.New().
				NewContext(httptest.NewRequest(http.MethodGet, "/threads/"+threadID, nil), rec)
			c.SetParamNames("threadID")
			c.SetParamValues(threadID)
			if err := markdownService.ServeThread(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("ServeThread status = %d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestServeThreadRendersComposerDraftAndTranscript(t *testing.T) {
	projectRoot := t.TempDir()
	thoughtsRoot := filepath.Join(projectRoot, "thoughts")
	planDir := filepath.Join(thoughtsRoot, "owner", "plans", "alpha")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "design.md"),
		[]byte("# Alpha design"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "notes.md"),
		[]byte("# Alpha notes"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "threads.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	renderer := &Service{
		projectRoot:  projectRoot,
		thoughtsRoot: thoughtsRoot,
		queries:      database.Queries,
		db:           database.DB(),
	}
	markdownService, err := markdown.NewService(thoughtsRoot, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	markdownService.WithWorkbenchThreadRenderer(renderer)

	threadID := "thread-ssr-chat"
	if _, err := database.Queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        threadID,
			UserEmail: "owner@example.com",
			Title:     "Alpha",
			Cwd:       "thoughts/owner/plans/alpha",
			LineageID: threadID,
			ProjectID: "project-1",
		},
	); err != nil {
		t.Fatal(err)
	}
	entryID := "entry-ssr-1"
	if err := database.Queries.CreateAgentEntry(
		t.Context(),
		db.CreateAgentEntryParams{
			LineageID:        threadID,
			EntryID:          entryID,
			EntryType:        "message",
			OriginOrder:      1,
			PayloadJson:      `{"type":"message","id":"entry-ssr-1","message":{"role":"user","content":"SSR transcript chestnut leftover"}}`,
			OriginThreadID:   threadID,
			SessionTimestamp: time.Now().UTC(),
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Queries.UpdateAgentThreadHead(
		t.Context(),
		db.UpdateAgentThreadHeadParams{
			HeadEntryID: sql.NullString{String: entryID, Valid: true},
			ID:          threadID,
		},
	); err != nil {
		t.Fatal(err)
	}
	draft := "composer draft chestnut leftover"
	if err := renderer.SaveThreadDraft(
		t.Context(),
		"owner@example.com",
		threadID,
		draft,
		1,
	); err != nil {
		t.Fatal(err)
	}

	bodies := []string{}
	for _, target := range []string{
		"/threads/" + threadID,
		"/threads/" + threadID + "?artifact=thoughts/owner/plans/alpha/notes.md",
	} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, target, nil), rec)
		c.SetParamNames("threadID")
		c.SetParamValues(threadID)
		c.Set("user_email", "owner@example.com")
		if err := markdownService.ServeThread(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("ServeThread status = %d body=%q", rec.Code, rec.Body.String())
		}
		html := rec.Body.String()
		bodies = append(bodies, html)
		for _, want := range []string{
			draft,
			"SSR transcript chestnut leftover",
			`id="agent-chat-composer-form"`,
			`id="agent-chat-stable-transcript"`,
			`id="agent-chat-messages"`,
			`data-thread-artifact-file`,
			"/threads/" + threadID + "?artifact=",
		} {
			if !strings.Contains(html, want) {
				t.Fatalf("ServeThread %s missing %q in %s", target, want, html)
			}
		}
		if strings.Contains(html, "threadArtifactFileClickAction") ||
			strings.Contains(html, "/threads/"+threadID+"/artifact?") {
			t.Fatalf("file rows still use patch select: %s", html)
		}
	}
	if !strings.Contains(bodies[0], draft) || !strings.Contains(bodies[1], draft) {
		t.Fatal("composer draft missing across artifact URLs")
	}
}
