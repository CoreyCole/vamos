package fixtures

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDefaultRegistryResolvesBasicFixture(t *testing.T) {
	registry := DefaultRegistry()
	builder, err := registry.Resolve("thoughts-workbench.basic")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	db, err := sql.Open("sqlite", t.TempDir()+"/agents.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state, err := builder(context.Background(), db, Input{Workspace: WorkspaceIdentity{
		Slug:         "feature-a",
		CheckoutPath: t.TempDir(),
		DBPath:       t.TempDir() + "/agents.db",
	}})
	if err != nil {
		t.Fatalf("builder() error = %v", err)
	}
	if got, want := state.Name, "thoughts-workbench.basic"; got != want {
		t.Fatalf("state.Name=%q want %q", got, want)
	}
	if got, want := state.Data["workspace_slug"], "feature-a"; got != want {
		t.Fatalf("workspace_slug=%v want %q", got, want)
	}
}

func TestBuildThoughtsWorkbenchBasicClearsLegacyCurrentSession(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createThoughtsWorkbenchFixtureSchema(t, db)
	if _, err := db.Exec(`INSERT INTO workspaces (id, root_doc_path, current_session_id) VALUES ('ws_1', ?, 'stale-thread-id')`, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_, err = BuildThoughtsWorkbenchBasic(
		context.Background(),
		db,
		Input{Workspace: WorkspaceIdentity{Slug: "feature-a", CheckoutPath: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "agents.db")}},
	)
	if err != nil {
		t.Fatalf("BuildThoughtsWorkbenchBasic() error = %v", err)
	}
	var selectedThread sql.NullString
	var currentSession sql.NullString
	if err := db.QueryRow(`SELECT selected_thread_id, current_session_id FROM workspaces WHERE id = 'ws_1'`).Scan(&selectedThread, &currentSession); err != nil {
		t.Fatal(err)
	}
	if !selectedThread.Valid || selectedThread.String != "th_1" {
		t.Fatalf("selected_thread_id=%+v want th_1", selectedThread)
	}
	if currentSession.Valid {
		t.Fatalf("current_session_id=%q want NULL", currentSession.String)
	}
}

func TestBuildThoughtsWorkbenchBasicKeepsWorkspaceRelativeThoughtsRoot(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(t.TempDir(), "shared-thoughts")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(root, "checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(checkout, "thoughts")); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	createThoughtsWorkbenchFixtureSchema(t, db)
	if _, err := db.Exec(`INSERT INTO workspaces (id, root_doc_path) VALUES ('ws_1', ?)`, shared); err != nil {
		t.Fatal(err)
	}
	_, err = BuildThoughtsWorkbenchBasic(
		context.Background(),
		db,
		Input{
			Workspace: WorkspaceIdentity{
				Slug:         "feature-a",
				CheckoutPath: checkout,
				DBPath:       filepath.Join(t.TempDir(), "agents.db"),
			},
			ThoughtsRoot: filepath.Join(checkout, "thoughts"),
		},
	)
	if err != nil {
		t.Fatalf("BuildThoughtsWorkbenchBasic() error = %v", err)
	}
	var rootDocPath string
	if err := db.QueryRow(`SELECT root_doc_path FROM workspaces WHERE id = 'ws_1'`).Scan(&rootDocPath); err != nil {
		t.Fatal(err)
	}
	if got, want := rootDocPath, filepath.Join(checkout, "thoughts"); got != want {
		t.Fatalf("root_doc_path=%q want workspace-relative symlink path %q", got, want)
	}
}

func createThoughtsWorkbenchFixtureSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, user_email TEXT, title TEXT, root_doc_path TEXT, workflow_type TEXT, source TEXT, selected_thread_id TEXT, current_session_id TEXT, updated_at TEXT DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE agent_threads (id TEXT PRIMARY KEY, user_email TEXT, title TEXT, cwd TEXT, lineage_id TEXT)`,
		`CREATE TABLE agent_thread_workspaces (thread_id TEXT, workspace_id TEXT, is_primary INTEGER, role TEXT, adopted_from TEXT, adopted_at TEXT, PRIMARY KEY(thread_id, workspace_id))`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildThoughtsWorkbenchBasicRequiresWorkspace(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/agents.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = BuildThoughtsWorkbenchBasic(
		context.Background(),
		db,
		Input{Workspace: WorkspaceIdentity{Slug: "main"}},
	)
	if err == nil {
		t.Fatal("expected main workspace rejection")
	}
}
