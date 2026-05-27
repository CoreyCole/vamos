//go:build !integration || unit

package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	querydb "github.com/CoreyCole/vamos/pkg/db"

	_ "modernc.org/sqlite"
)

func TestReadSchemaSQLUsesModuleLocalSchema(t *testing.T) {
	cwd := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	schemaPath := filepath.Join(cwd, "pkg", "db", "migrations", "schema.sql")
	if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	want := []byte("-- module-local schema\n")
	if err := os.WriteFile(schemaPath, want, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := readSchemaSQL()
	if err != nil {
		t.Fatalf("readSchemaSQL() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("readSchemaSQL() = %q, want %q", got, want)
	}
}

func TestNewServiceCreatesParentAndMigratesSchema(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "nested", "agents.db")
	svc, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("Stat(%q) error = %v", dbPath, err)
	}
	if !testTableExists(t, svc.DB(), "workspaces") {
		t.Fatal("workspaces table missing after schema migration")
	}
	if columnExists(t, svc.DB(), "agent_threads", "workspace_id") {
		t.Fatal("agent_threads.workspace_id exists after NewService")
	}
}

func TestNewServiceAgentThreadQueriesDoNotRequireWorkspaceIDColumn(t *testing.T) {
	t.Parallel()

	svc, err := NewService(filepath.Join(t.TempDir(), "vamos.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	if columnExists(t, svc.DB(), "agent_threads", "workspace_id") {
		t.Fatal("agent_threads.workspace_id exists after NewService")
	}

	ctx := t.Context()
	userEmail := "thread-query@example.com"
	workspaceID := "workspace-thread-query"
	threadID := "thread-query"
	rootDocPath := filepath.Join(t.TempDir(), "plan")

	if _, err := svc.Queries.CreateWorkspace(ctx, querydb.CreateWorkspaceParams{
		ID:                workspaceID,
		UserEmail:         userEmail,
		Title:             "Thread Query Workspace",
		RootDocPath:       rootDocPath,
		Cwd:               sql.NullString{String: rootDocPath, Valid: true},
		WorkflowType:      "freeform",
		WorkflowStateJson: sql.NullString{},
		Source:            "imported",
		SelectedThreadID:  sql.NullString{},
		SelectedDocPath:   sql.NullString{},
	}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	thread, err := svc.Queries.CreateAgentThread(ctx, querydb.CreateAgentThreadParams{
		ID:                threadID,
		UserEmail:         userEmail,
		Title:             "Thread Query",
		Cwd:               rootDocPath,
		LineageID:         "lineage-thread-query",
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		t.Fatalf("CreateAgentThread() error = %v", err)
	}
	if thread.ID != threadID {
		t.Fatalf("CreateAgentThread() ID = %q, want %q", thread.ID, threadID)
	}

	if err := svc.Queries.AttachThreadToWorkspace(ctx, querydb.AttachThreadToWorkspaceParams{
		ID:          threadID,
		WorkspaceID: sql.NullString{String: workspaceID, Valid: true},
	}); err != nil {
		t.Fatalf("AttachThreadToWorkspace() error = %v", err)
	}

	if _, err := svc.Queries.GetAgentThread(ctx, threadID); err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if _, err := svc.Queries.GetAgentThreadForUser(ctx, querydb.GetAgentThreadForUserParams{ID: threadID, UserEmail: userEmail}); err != nil {
		t.Fatalf("GetAgentThreadForUser() error = %v", err)
	}
	if _, err := svc.Queries.ListAgentThreads(ctx, querydb.ListAgentThreadsParams{UserEmail: userEmail, Limit: 10}); err != nil {
		t.Fatalf("ListAgentThreads() error = %v", err)
	}
	if _, err := svc.Queries.GetAgentThreadForWorkspaceUser(ctx, querydb.GetAgentThreadForWorkspaceUserParams{WorkspaceID: workspaceID, ThreadID: threadID, UserEmail: userEmail}); err != nil {
		t.Fatalf("GetAgentThreadForWorkspaceUser() error = %v", err)
	}
	if _, err := svc.Queries.ListAgentThreadsByWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("ListAgentThreadsByWorkspace() error = %v", err)
	}
	if _, err := svc.Queries.ListAgentThreadsForUserWithWorkspace(ctx, userEmail); err != nil {
		t.Fatalf("ListAgentThreadsForUserWithWorkspace() error = %v", err)
	}
	if _, err := svc.Queries.ListThreadsByPrimaryWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("ListThreadsByPrimaryWorkspace() error = %v", err)
	}
}

func TestNewServiceEnablesForeignKeys(t *testing.T) {
	t.Parallel()

	svc, err := NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	_, err = svc.DB().ExecContext(
		t.Context(),
		`INSERT INTO workspace_events (workspace_id, event_type, actor_type) VALUES ('missing-workspace', 'test', 'system')`,
	)
	if err == nil {
		t.Fatal("orphan workspace_events insert succeeded, want foreign-key failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign") {
		t.Fatalf("orphan insert error = %v, want foreign-key failure", err)
	}
}

func TestNewServiceConfiguresBusyTimeoutOnPooledConnections(t *testing.T) {
	t.Parallel()

	svc, err := NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	svc.DB().SetMaxOpenConns(2)

	ctx := t.Context()
	conn1, err := svc.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(1) error = %v", err)
	}
	defer conn1.Close()
	conn2, err := svc.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(2) error = %v", err)
	}
	defer conn2.Close()

	for idx, conn := range []*sql.Conn{conn1, conn2} {
		var timeoutMS int
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").
			Scan(&timeoutMS); err != nil {
			t.Fatalf("conn %d PRAGMA busy_timeout: %v", idx+1, err)
		}
		if timeoutMS != sqliteBusyTimeoutMS {
			t.Fatalf(
				"conn %d busy_timeout = %d, want %d",
				idx+1,
				timeoutMS,
				sqliteBusyTimeoutMS,
			)
		}
	}
}

func TestNewServiceEnablesForeignKeysOnPooledConnections(t *testing.T) {
	t.Parallel()

	svc, err := NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	svc.DB().SetMaxOpenConns(2)

	ctx := t.Context()
	conn1, err := svc.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(1) error = %v", err)
	}
	defer conn1.Close()
	conn2, err := svc.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(2) error = %v", err)
	}
	defer conn2.Close()

	for idx, conn := range []*sql.Conn{conn1, conn2} {
		var enabled int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").
			Scan(&enabled); err != nil {
			t.Fatalf("conn %d PRAGMA foreign_keys: %v", idx+1, err)
		}
		if enabled != 1 {
			t.Fatalf("conn %d foreign_keys = %d, want 1", idx+1, enabled)
		}
	}

	_, err = conn2.ExecContext(
		ctx,
		`INSERT INTO workspace_events (workspace_id, event_type, actor_type) VALUES ('missing-workspace', 'test', 'system')`,
	)
	if err == nil {
		t.Fatal(
			"orphan workspace_events insert on second connection succeeded, want foreign-key failure",
		)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign") {
		t.Fatalf("orphan insert error = %v, want foreign-key failure", err)
	}
}

func TestLegacyArtifactPathColumnsRenamed(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    selected_artifact_rel_path TEXT
);
CREATE TABLE workspace_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    artifact_rel_path TEXT
);
CREATE TABLE workspace_artifacts (
    workspace_id TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    PRIMARY KEY (workspace_id, rel_path)
);
CREATE TABLE artifact_comments (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    artifact_rel_path TEXT NOT NULL,
    user_email TEXT NOT NULL,
    comment_text TEXT NOT NULL,
    selected_text TEXT NOT NULL
);
INSERT INTO workspaces (id, selected_artifact_rel_path) VALUES ('workspace-1', 'selected.md');
INSERT INTO workspace_events (workspace_id, artifact_rel_path) VALUES ('workspace-1', 'event.md');
INSERT INTO workspace_artifacts (workspace_id, rel_path) VALUES ('workspace-1', 'artifact.md');
INSERT INTO artifact_comments (id, workspace_id, artifact_rel_path, user_email, comment_text, selected_text)
VALUES ('comment-1', 'workspace-1', 'comment.md', 'user@example.com', 'body', 'selected');`)
	if err != nil {
		t.Fatalf("seed legacy path columns: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	assertRenamedColumn(
		t,
		database,
		"workspaces",
		"selected_artifact_rel_path",
		"selected_doc_path",
	)
	assertRenamedColumn(
		t,
		database,
		"workspace_events",
		"artifact_rel_path",
		"doc_path",
	)
	assertRenamedColumn(t, database, "workspace_artifacts", "rel_path", "doc_path")
	assertRenamedColumn(
		t,
		database,
		"artifact_comments",
		"artifact_rel_path",
		"doc_path",
	)

	queries := map[string]string{
		"workspaces":          `SELECT selected_doc_path FROM workspaces WHERE id = 'workspace-1'`,
		"workspace_events":    `SELECT doc_path FROM workspace_events WHERE workspace_id = 'workspace-1'`,
		"workspace_artifacts": `SELECT doc_path FROM workspace_artifacts WHERE workspace_id = 'workspace-1'`,
		"artifact_comments":   `SELECT doc_path FROM artifact_comments WHERE id = 'comment-1'`,
	}
	wants := map[string]string{
		"workspaces":          "selected.md",
		"workspace_events":    "event.md",
		"workspace_artifacts": "artifact.md",
		"artifact_comments":   "comment.md",
	}
	for tableName, query := range queries {
		var documentPath string
		if err := database.QueryRowContext(t.Context(), query).
			Scan(&documentPath); err != nil {
			t.Fatalf("query migrated %s row: %v", tableName, err)
		}
		if documentPath != wants[tableName] {
			t.Fatalf(
				"%s doc_path = %q, want %q",
				tableName,
				documentPath,
				wants[tableName],
			)
		}
	}
}

func TestLegacyUserChatSelectionsMigratesToScopedTable(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE user_chat_selections (
    user_email TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    thread_id TEXT,
    run_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO user_chat_selections (user_email, workspace_id, thread_id, run_id, created_at, updated_at)
VALUES ('owner@example.com', 'workspace-1', 'thread-1', 'run-1', '2026-01-01 00:00:00', '2026-01-02 00:00:00');`)
	if err != nil {
		t.Fatalf("seed legacy user_chat_selections: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	for _, column := range []string{"scope", "scope_id"} {
		if !columnExists(t, database, "user_chat_selections", column) {
			t.Fatalf("user_chat_selections.%s missing after migration", column)
		}
	}
	if !indexExists(t, database, "idx_user_chat_selections_user_updated") {
		t.Fatal("idx_user_chat_selections_user_updated missing after migration")
	}

	row, err := querydb.New(database).GetUserChatSelection(
		t.Context(),
		querydb.GetUserChatSelectionParams{
			UserEmail: "owner@example.com",
			Scope:     "global",
			ScopeID:   "",
		},
	)
	if err != nil {
		t.Fatalf("GetUserChatSelection(global) error = %v", err)
	}
	if row.WorkspaceID != "workspace-1" || row.ThreadID.String != "thread-1" ||
		row.RunID.String != "run-1" {
		t.Fatalf("migrated row = %+v", row)
	}
}

func TestLegacyLayoutPreferencesMigratesToViewportClassKey(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE layout_preferences (
    user_email TEXT NOT NULL,
    page TEXT NOT NULL,
    view TEXT NOT NULL,
    config_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_email, page, view)
);
INSERT INTO layout_preferences (user_email, page, view, config_json)
VALUES ('agent@example.com', 'thoughts', 'split', '{"version":1,"page":"thoughts","view":"split","regions":[],"mobile":{"activeRegionID":""}}');`)
	if err != nil {
		t.Fatalf("seed legacy layout_preferences: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	if !columnExists(t, database, "layout_preferences", "viewport_class") {
		t.Fatal("layout_preferences.viewport_class missing after migration")
	}
	row, err := querydb.New(database).GetLayoutPreference(
		t.Context(),
		querydb.GetLayoutPreferenceParams{
			UserEmail:     "agent@example.com",
			Page:          "thoughts",
			View:          "split",
			ViewportClass: "desktop-full",
		},
	)
	if err != nil {
		t.Fatalf("GetLayoutPreference() error = %v", err)
	}
	if row.ViewportClass != "desktop-full" {
		t.Fatalf("viewport_class = %q, want desktop-full", row.ViewportClass)
	}
}

func TestPreAgentChatArtifactCommentsTableRenamed(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE artifact_comments (
    id TEXT PRIMARY KEY,
    artifact_path TEXT NOT NULL,
    body TEXT NOT NULL
);
INSERT INTO artifact_comments (id, artifact_path, body) VALUES ('comment-1', 'old.md', 'old body');`)
	if err != nil {
		t.Fatalf("seed pre-AgentChat artifact_comments: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	if testTableExists(t, database, "artifact_comments") {
		t.Fatal("old artifact_comments still exists, want renamed")
	}
	if !testTableExists(t, database, "artifact_comments_pre_agentchat") {
		t.Fatal("artifact_comments_pre_agentchat missing after rename")
	}
	var artifactPath, body string
	if err := database.QueryRowContext(
		t.Context(),
		`SELECT artifact_path, body FROM artifact_comments_pre_agentchat WHERE id = ?`,
		"comment-1",
	).Scan(&artifactPath, &body); err != nil {
		t.Fatalf("query renamed artifact_comments row: %v", err)
	}
	if artifactPath != "old.md" || body != "old body" {
		t.Fatalf(
			"renamed artifact_comments row = (%q, %q), want (old.md, old body)",
			artifactPath,
			body,
		)
	}
}

func TestPreAgentChatArtifactCommentsTableRenameConflictUsesSuffix(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE artifact_comments_pre_agentchat (id TEXT PRIMARY KEY);
CREATE TABLE artifact_comments (id TEXT PRIMARY KEY, artifact_path TEXT NOT NULL, body TEXT NOT NULL);`)
	if err != nil {
		t.Fatalf("seed conflict: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	if !testTableExists(t, database, "artifact_comments_pre_agentchat_2") {
		t.Fatal("artifact_comments_pre_agentchat_2 missing after conflict rename")
	}
}

func TestOpenDBAfterPreAgentChatCompatibilityPreparation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "agents.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open seed: %v", err)
	}
	_, err = database.ExecContext(t.Context(), `
CREATE TABLE artifact_comments (id TEXT PRIMARY KEY, artifact_path TEXT NOT NULL, body TEXT NOT NULL);`)
	if closeErr := database.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("seed old db: %v", err)
	}

	svc, err := NewService(path)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if !testTableExists(t, svc.db, "artifact_comments_pre_agentchat") {
		t.Fatal("artifact_comments_pre_agentchat missing after DB open")
	}
}

func TestRuntimeMigrationsAddWorkspaceSelectedDocPath(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	createOldShapeAgentChatTables(t, database)

	if err := runRuntimeMigrations(t.Context(), database); err != nil {
		t.Fatalf("runRuntimeMigrations() error = %v", err)
	}
	if !columnExists(t, database, "workspaces", "selected_doc_path") {
		t.Fatal("selected_doc_path column was not added")
	}
}

func TestPrepareSchemaCompatibilityMigrationsAddWorkspaceSelectedDocPath(
	t *testing.T,
) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL,
    root_doc_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);`)
	if err != nil {
		t.Fatalf("seed old workspaces: %v", err)
	}

	if err := prepareSchemaCompatibilityMigrations(t.Context(), database); err != nil {
		t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
	}
	if !columnExists(t, database, "workspaces", "selected_doc_path") {
		t.Fatal("selected_doc_path column was not added")
	}
}

func TestPrepareSchemaCompatibilityMigrationsAddsWorkspaceEventDocColumns(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspace_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor_email TEXT,
    actor_type TEXT NOT NULL DEFAULT 'system',
    thread_id TEXT,
    session_id TEXT,
    run_id TEXT,
    payload_json TEXT,
    event_key TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`)
	if err != nil {
		t.Fatalf("seed old workspace_events: %v", err)
	}

	for range 2 {
		if err := prepareSchemaCompatibilityMigrations(
			t.Context(),
			database,
		); err != nil {
			t.Fatalf("prepareSchemaCompatibilityMigrations() error = %v", err)
		}
	}
	for _, column := range []string{"doc_path", "comment_id"} {
		if !columnExists(t, database, "workspace_events", column) {
			t.Fatalf("%s column was not added", column)
		}
	}
}

func TestRunRuntimeMigrationsAddsWorkspaceEventDocColumns(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	createOldShapeAgentChatTables(t, database)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspace_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL DEFAULT 'system',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`)
	if err != nil {
		t.Fatalf("seed old workspace_events: %v", err)
	}

	if err := runRuntimeMigrations(t.Context(), database); err != nil {
		t.Fatalf("runRuntimeMigrations() error = %v", err)
	}
	for _, column := range []string{"doc_path", "comment_id"} {
		if !columnExists(t, database, "workspace_events", column) {
			t.Fatalf("%s column was not added", column)
		}
	}
}

func TestRunRuntimeMigrationsRemovesAgentThreadWorkspaceID(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	createOldShapeAgentChatTables(t, database)

	if err := runRuntimeMigrations(t.Context(), database); err != nil {
		t.Fatalf("runRuntimeMigrations() error = %v", err)
	}

	if columnExists(t, database, "agent_threads", "workspace_id") {
		t.Fatal("agent_threads.workspace_id still exists after runtime migrations")
	}
	for _, tc := range []struct {
		table  string
		column string
	}{
		{table: "agent_runs", column: "workspace_id"},
		{table: "agent_runs", column: "session_id"},
		{table: "agent_entries", column: "origin_session_id"},
	} {
		if !columnExists(t, database, tc.table, tc.column) {
			t.Fatalf("columnExists(%s, %s) = false, want true", tc.table, tc.column)
		}
	}
	if indexExists(t, database, "idx_agent_threads_workspace_updated") {
		t.Fatal("idx_agent_threads_workspace_updated still exists after runtime migrations")
	}
	for _, indexName := range []string{
		"idx_agent_sessions_workspace_updated",
		"idx_agent_runs_workspace_created",
		"idx_agent_entries_origin_session",
		"idx_agent_runs_thread_running",
	} {
		if !indexExists(t, database, indexName) {
			t.Fatalf("indexExists(%s) = false, want true", indexName)
		}
	}
}

func TestAgentSessionImportingStatusMigrationAllowsImporting(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'New Workspace',
    root_doc_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_threads (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'New Chat',
    cwd TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    head_entry_id TEXT,
    parent_thread_id TEXT REFERENCES agent_threads(id),
    forked_from_entry_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_sessions (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL CHECK (source IN ('terminal', 'web', 'adopted')),
    session_path TEXT,
    session_id TEXT,
    parent_session_id TEXT,
    cwd TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'imported', 'unassigned', 'ambiguous', 'diverged', 'failed')),
    inferred_workspace_id TEXT,
    inferred_plan_dir TEXT,
    imported_head_entry_id TEXT,
    last_imported_at DATETIME,
    last_error TEXT,
    metadata_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_agent_sessions_path
    ON agent_sessions(session_path)
    WHERE session_path IS NOT NULL;
CREATE TABLE agent_entries (
    lineage_id TEXT NOT NULL,
    entry_id TEXT NOT NULL,
    parent_entry_id TEXT,
    entry_type TEXT NOT NULL,
    origin_order INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    origin_thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    origin_run_id TEXT,
    session_timestamp DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (lineage_id, entry_id)
);
CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    restore_head_entry_id TEXT,
    result_head_entry_id TEXT,
    workflow_id TEXT NOT NULL,
    temporal_run_id TEXT,
    root_doc_path TEXT NOT NULL,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);
INSERT INTO agent_sessions (id, source, session_path, status)
VALUES ('session-1', 'terminal', '/tmp/session.jsonl', 'pending');`)
	if err != nil {
		t.Fatalf("seed old agent_sessions: %v", err)
	}

	if err := runRuntimeMigrations(t.Context(), database); err != nil {
		t.Fatalf("runRuntimeMigrations() error = %v", err)
	}
	if _, err := database.ExecContext(
		t.Context(),
		`UPDATE agent_sessions SET status = 'importing' WHERE id = 'session-1'`,
	); err != nil {
		t.Fatalf("update status importing after migration: %v", err)
	}
	for _, column := range []string{"workspace_id", "thread_id", "user_email"} {
		if !columnExists(t, database, "agent_sessions", column) {
			t.Fatalf("agent_sessions.%s missing after migration", column)
		}
	}
	if !indexExists(t, database, "idx_agent_sessions_path") {
		t.Fatal("idx_agent_sessions_path missing after migration")
	}
}

func TestAgentSessionUserEmailMigrationBackfillsAssignedRows(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'New Workspace',
    root_doc_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_threads (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    workspace_id TEXT,
    title TEXT NOT NULL DEFAULT 'New Chat',
    cwd TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    head_entry_id TEXT,
    parent_thread_id TEXT REFERENCES agent_threads(id),
    forked_from_entry_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_sessions (
    id TEXT PRIMARY KEY,
    workspace_id TEXT REFERENCES workspaces(id),
    thread_id TEXT REFERENCES agent_threads(id),
    user_email TEXT,
    source TEXT NOT NULL CHECK (source IN ('terminal', 'web', 'adopted')),
    session_path TEXT,
    session_id TEXT,
    parent_session_id TEXT,
    cwd TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'imported', 'unassigned', 'ambiguous', 'diverged', 'failed')),
    inferred_workspace_id TEXT,
    inferred_plan_dir TEXT,
    imported_head_entry_id TEXT,
    last_imported_at DATETIME,
    last_error TEXT,
    metadata_json TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_agent_sessions_path
    ON agent_sessions(session_path)
    WHERE session_path IS NOT NULL;
CREATE TABLE agent_entries (
    lineage_id TEXT NOT NULL,
    entry_id TEXT NOT NULL,
    parent_entry_id TEXT,
    entry_type TEXT NOT NULL,
    origin_order INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    origin_thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    origin_run_id TEXT,
    session_timestamp DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (lineage_id, entry_id)
);
CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    restore_head_entry_id TEXT,
    result_head_entry_id TEXT,
    workflow_id TEXT NOT NULL,
    temporal_run_id TEXT,
    root_doc_path TEXT NOT NULL,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);
INSERT INTO workspaces (id, user_email, title, root_doc_path)
VALUES ('workspace-1', 'workspace@example.com', 'Workspace', '/tmp/workspace');
INSERT INTO agent_threads (id, user_email, workspace_id, title, cwd, lineage_id)
VALUES ('thread-1', 'thread@example.com', NULL, 'Thread', '/tmp/thread', 'lineage-1');
INSERT INTO agent_sessions (id, workspace_id, thread_id, user_email, source, session_path, status)
VALUES
    ('workspace-owned', 'workspace-1', NULL, NULL, 'adopted', '/tmp/workspace.jsonl', 'imported'),
    ('thread-only', NULL, 'thread-1', NULL, 'adopted', '/tmp/thread.jsonl', 'imported'),
    ('preserved-owner', 'workspace-1', NULL, 'existing@example.com', 'adopted', '/tmp/preserved.jsonl', 'imported'),
    ('ownerless-unassigned', NULL, NULL, NULL, 'adopted', '/tmp/unassigned.jsonl', 'unassigned'),
    ('ownerless-failed', NULL, NULL, NULL, 'adopted', '/tmp/failed.jsonl', 'failed');`)
	if err != nil {
		t.Fatalf("seed historical agent_sessions: %v", err)
	}

	for range 2 {
		if err := runRuntimeMigrations(t.Context(), database); err != nil {
			t.Fatalf("runRuntimeMigrations() error = %v", err)
		}
	}

	assertSessionOwner := func(id, want string, wantValid bool) {
		t.Helper()
		var owner sql.NullString
		if err := database.QueryRowContext(
			t.Context(),
			`SELECT user_email FROM agent_sessions WHERE id = ?`,
			id,
		).Scan(&owner); err != nil {
			t.Fatalf("query %s owner: %v", id, err)
		}
		if owner.Valid != wantValid || owner.String != want {
			t.Fatalf("%s owner = %v, want valid=%v value=%q", id, owner, wantValid, want)
		}
	}

	assertSessionOwner("workspace-owned", "workspace@example.com", true)
	assertSessionOwner("thread-only", "thread@example.com", true)
	assertSessionOwner("preserved-owner", "existing@example.com", true)
	assertSessionOwner("ownerless-unassigned", "", false)
	assertSessionOwner("ownerless-failed", "", false)

	if _, err := database.ExecContext(
		t.Context(),
		`UPDATE agent_sessions SET status = 'importing' WHERE id = 'workspace-owned'`,
	); err != nil {
		t.Fatalf("importing status after migration: %v", err)
	}
	if !indexExists(t, database, "idx_agent_sessions_path") {
		t.Fatal("idx_agent_sessions_path missing after migration")
	}
}

func TestReconcileRunningRunIndexPreflightFailsOlderDuplicateRuns(t *testing.T) {
	t.Parallel()

	database := openMigratorTestDB(t)
	createOldShapeAgentChatTables(t, database)
	_, err := database.ExecContext(t.Context(), `
INSERT INTO agent_threads (id, user_email, title, cwd, lineage_id) VALUES ('thread-1', 'user@example.com', 'Thread', '/tmp/project', 'lineage-1');
INSERT INTO agent_runs (id, thread_id, trigger, status, prompt_text, workflow_id, root_doc_path, created_at)
VALUES
    ('run-old', 'thread-1', 'send', 'running', 'old', 'workflow-old', '/tmp/project', '2026-04-30T00:00:00Z'),
    ('run-new', 'thread-1', 'resume', 'running', 'new', 'workflow-new', '/tmp/project', '2026-04-30T00:00:01Z');`)
	if err != nil {
		t.Fatalf("seed duplicate runs: %v", err)
	}

	if err := runRuntimeMigrations(t.Context(), database); err != nil {
		t.Fatalf("runRuntimeMigrations() error = %v", err)
	}

	var oldStatus, newStatus string
	if err := database.QueryRowContext(t.Context(), `SELECT status FROM agent_runs WHERE id = 'run-old'`).
		Scan(&oldStatus); err != nil {
		t.Fatalf("query old status: %v", err)
	}
	if err := database.QueryRowContext(t.Context(), `SELECT status FROM agent_runs WHERE id = 'run-new'`).
		Scan(&newStatus); err != nil {
		t.Fatalf("query new status: %v", err)
	}
	if oldStatus != "failed" || newStatus != "running" {
		t.Fatalf(
			"statuses = old:%s new:%s, want old failed/new running",
			oldStatus,
			newStatus,
		)
	}
	if !indexExists(t, database, "idx_agent_runs_thread_running") {
		t.Fatal("idx_agent_runs_thread_running missing after migration")
	}
}

func openMigratorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(
		t.Context(),
		"PRAGMA foreign_keys = ON",
	); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return database
}

func createOldShapeAgentChatTables(t *testing.T, database *sql.DB) {
	t.Helper()
	_, err := database.ExecContext(t.Context(), `
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'New Workspace',
    root_doc_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_threads (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'New Chat',
    cwd TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    head_entry_id TEXT,
    parent_thread_id TEXT REFERENCES agent_threads(id),
    forked_from_entry_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME
);
CREATE TABLE agent_sessions (
    id TEXT PRIMARY KEY,
    workspace_id TEXT REFERENCES workspaces(id),
    thread_id TEXT REFERENCES agent_threads(id),
    source TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE agent_entries (
    lineage_id TEXT NOT NULL,
    entry_id TEXT NOT NULL,
    parent_entry_id TEXT,
    entry_type TEXT NOT NULL,
    origin_order INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    origin_thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    origin_run_id TEXT,
    session_timestamp DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (lineage_id, entry_id)
);
CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL REFERENCES agent_threads(id),
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    restore_head_entry_id TEXT,
    result_head_entry_id TEXT,
    workflow_id TEXT NOT NULL,
    temporal_run_id TEXT,
    root_doc_path TEXT NOT NULL,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);`)
	if err != nil {
		t.Fatalf("create old-shape tables: %v", err)
	}
}

func assertRenamedColumn(
	t *testing.T,
	database *sql.DB,
	tableName, legacyName, newName string,
) {
	t.Helper()
	if !columnExists(t, database, tableName, newName) {
		t.Fatalf("%s.%s missing after compatibility preparation", tableName, newName)
	}
	if columnExists(t, database, tableName, legacyName) {
		t.Fatalf(
			"%s.%s still exists after compatibility preparation",
			tableName,
			legacyName,
		)
	}
}

func columnExists(t *testing.T, database *sql.DB, tableName, columnName string) bool {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), "PRAGMA table_info("+tableName+")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", tableName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		if name == columnName {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("column rows error: %v", err)
	}
	return false
}

func testTableExists(t *testing.T, database *sql.DB, tableName string) bool {
	t.Helper()
	var name string
	err := database.QueryRowContext(
		t.Context(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableName,
	).Scan(&name)
	if err == nil {
		return true
	}
	if err == sql.ErrNoRows {
		return false
	}
	t.Fatalf("query table %s: %v", tableName, err)
	return false
}

func indexExists(t *testing.T, database *sql.DB, indexName string) bool {
	t.Helper()
	var name string
	err := database.QueryRowContext(
		t.Context(),
		`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`,
		indexName,
	).Scan(&name)
	if err == nil {
		return true
	}
	if err == sql.ErrNoRows {
		return false
	}
	t.Fatalf("query index %s: %v", indexName, err)
	return false
}
