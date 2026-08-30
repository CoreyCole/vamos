package fixtures

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	serverdb "github.com/CoreyCole/vamos/server/services/db"
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
	if _, err := db.Exec(
		`INSERT INTO workspaces (id, root_doc_path, current_session_id) VALUES ('ws_1', ?, 'stale-thread-id')`,
		t.TempDir(),
	); err != nil {
		t.Fatal(err)
	}
	_, err = BuildThoughtsWorkbenchBasic(
		context.Background(),
		db,
		Input{
			Workspace: WorkspaceIdentity{
				Slug:         "feature-a",
				CheckoutPath: t.TempDir(),
				DBPath:       filepath.Join(t.TempDir(), "agents.db"),
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildThoughtsWorkbenchBasic() error = %v", err)
	}
	var selectedThread sql.NullString
	var currentSession sql.NullString
	if err := db.QueryRow(`SELECT selected_thread_id, current_session_id FROM workspaces WHERE id = 'ws_1'`).
		Scan(&selectedThread, &currentSession); err != nil {
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
	if _, err := db.Exec(
		`INSERT INTO workspaces (id, root_doc_path) VALUES ('ws_1', ?)`,
		shared,
	); err != nil {
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
	if err := db.QueryRow(`SELECT root_doc_path FROM workspaces WHERE id = 'ws_1'`).
		Scan(&rootDocPath); err != nil {
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
		`CREATE TABLE agent_thread_drafts (user_email TEXT, thread_id TEXT, content TEXT, operation_order INTEGER, PRIMARY KEY(user_email, thread_id))`,
		`CREATE TABLE agent_runs (id TEXT PRIMARY KEY, workspace_id TEXT, thread_id TEXT, trigger TEXT, status TEXT, prompt_text TEXT, workflow_id TEXT, root_doc_path TEXT)`,
		`CREATE TABLE document_comments (id TEXT PRIMARY KEY, doc_path TEXT)`,
		`CREATE TABLE document_comment_replies (id TEXT PRIMARY KEY, comment_id TEXT)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildWorkbenchV2ResetsFixtureOwnedState(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/agents.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createThoughtsWorkbenchFixtureSchema(t, db)
	input := Input{
		Workspace: WorkspaceIdentity{
			Slug:         "feature",
			CheckoutPath: t.TempDir(),
			DBPath:       "fixture.db",
		},
	}
	if _, err := BuildWorkbenchV2(t.Context(), db, input); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(
		filepath.Join(
			input.Workspace.CheckoutPath,
			"thoughts",
			"v2-streamlit",
			"AGENTS.md",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(
		manifest,
	) != "---\nvamos_artifact: applet\napplet:\n  id: e2e-v2-streamlit\n  title: E2E v2-streamlit\n  kind: http\n  source_dir: "+filepath.ToSlash(
		filepath.Join(input.Workspace.CheckoutPath, "examples", "streamlit"),
	)+"\n  files_root: files\n  health_path: /_stcore/health\n  start_command: [./start.sh]\n---\n# E2E v2-streamlit\n" {
		t.Fatalf(
			"streamlit fixture manifest unexpectedly claims root aliases:\n%s",
			manifest,
		)
	}
	if _, err := db.Exec(
		`INSERT INTO document_comments (id, doc_path) VALUES ('fixture-comment', 'thoughts/workbench-v2/root.md')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildWorkbenchV2(t.Context(), db, input); err != nil {
		t.Fatal(err)
	}
	var runs, comments, drafts int
	if err := db.QueryRow(`SELECT count(*) FROM agent_runs WHERE thread_id = 'wb2_rejected' AND status = 'running'`).
		Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM document_comments WHERE doc_path LIKE 'thoughts/workbench-v2/%'`).
		Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM agent_thread_drafts WHERE thread_id = 'wb2_rejected' AND content = 'rejected thread draft'`).
		Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || comments != 0 || drafts != 1 {
		t.Fatalf("runs=%d comments=%d drafts=%d", runs, comments, drafts)
	}
}

func TestBuildWorkbenchV2ResetsRealSchemaDependencyGraph(t *testing.T) {
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	input := Input{
		Workspace: WorkspaceIdentity{
			Slug:         "feature",
			CheckoutPath: t.TempDir(),
			DBPath:       "fixture.db",
		},
	}
	if _, err := BuildWorkbenchV2(t.Context(), database.DB(), input); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO agent_sessions (id, agent, projected_thread_id) VALUES ('wb2_web_session', 'pi', 'wb2_alpha')`,
		`INSERT INTO agent_runs (id, workspace_id, thread_id, session_id, trigger, status, prompt_text, workflow_id, root_doc_path) VALUES ('wb2_accepted_run', 'ws_1', 'wb2_alpha', 'wb2_web_session', 'resume', 'complete', 'fixture', 'fixture', 'thoughts/workbench-v2/root.md')`,
		`INSERT INTO workspace_events (workspace_id, event_type, thread_id, run_id) VALUES ('ws_1', 'fixture', 'wb2_alpha', 'wb2_accepted_run')`,
		`INSERT INTO agent_run_attachments (id, run_id, thread_id, path, basename) VALUES ('wb2_attachment', 'wb2_accepted_run', 'wb2_alpha', 'fixture', 'fixture')`,
		`INSERT INTO agent_entries (lineage_id, entry_id, entry_type, origin_order, payload_json, origin_thread_id, origin_run_id, session_timestamp) VALUES ('wb2_alpha', 'fixture-entry', 'message', 1, '{}', 'wb2_alpha', 'wb2_accepted_run', CURRENT_TIMESTAMP)`,
		`INSERT INTO document_comments (id, doc_path, user_email, comment_text, selected_text) VALUES ('wb2_comment', 'thoughts/workbench-v2/root.md', 'fixture@example.test', 'fixture', '')`,
		`INSERT INTO document_comment_replies (id, comment_id, user_email, reply_text) VALUES ('wb2_reply', 'wb2_comment', 'fixture@example.test', 'fixture')`,
		`INSERT INTO document_comments (id, doc_path, user_email, comment_text, selected_text) VALUES ('nearby-comment', 'thoughts/owner/plans/nearby/design.md', 'fixture@example.test', 'keep', '')`,
	} {
		if _, err := database.DB().ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := BuildWorkbenchV2(t.Context(), database.DB(), input); err != nil {
		t.Fatal(err)
	}
	var rejected, fixtureComments, nearby int
	if err := database.DB().
		QueryRow(`SELECT count(*) FROM agent_runs WHERE id = 'wb2_rejected_active_run' AND status = 'running'`).
		Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().
		QueryRow(`SELECT count(*) FROM document_comments WHERE id = 'wb2_comment'`).
		Scan(&fixtureComments); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().
		QueryRow(`SELECT count(*) FROM document_comments WHERE id = 'nearby-comment'`).
		Scan(&nearby); err != nil {
		t.Fatal(err)
	}
	if rejected != 1 || fixtureComments != 0 || nearby != 1 {
		t.Fatalf(
			"rejected=%d fixtureComments=%d nearby=%d",
			rejected,
			fixtureComments,
			nearby,
		)
	}
	rows, err := database.DB().Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
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
