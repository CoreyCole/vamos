package agentchat

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "threads.db"))
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
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "threads.db"))
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
