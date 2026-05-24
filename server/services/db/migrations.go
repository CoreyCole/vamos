package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func enableSQLiteForeignKeys(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	return err
}

func prepareSchemaCompatibilityMigrations(ctx context.Context, database *sql.DB) error {
	if err := ensureAgentRunsWorkflowColumnsIfTableExists(ctx, database); err != nil {
		return err
	}
	if err := ensureColumnIfTableExists(
		ctx,
		database,
		"agent_sessions",
		"workspace_id",
		"TEXT REFERENCES workspaces(id)",
	); err != nil {
		return err
	}
	if err := ensureColumnIfTableExists(
		ctx,
		database,
		"agent_sessions",
		"thread_id",
		"TEXT REFERENCES agent_threads(id)",
	); err != nil {
		return err
	}
	if err := ensureColumnIfTableExists(
		ctx,
		database,
		"agent_sessions",
		"user_email",
		"TEXT",
	); err != nil {
		return err
	}
	for _, column := range []struct {
		tableName  string
		legacyName string
		newName    string
	}{
		{tableName: "workspaces", legacyName: "artifact_root", newName: "root_doc_path"},
		{tableName: "agent_runs", legacyName: "artifact_root", newName: "root_doc_path"},
		{tableName: "workspaces", legacyName: "selected_artifact_rel_path", newName: "selected_doc_path"},
		{tableName: "workspaces", legacyName: "selected_document_path", newName: "selected_doc_path"},
		{tableName: "workspace_events", legacyName: "artifact_rel_path", newName: "doc_path"},
		{tableName: "workspace_artifacts", legacyName: "rel_path", newName: "doc_path"},
	} {
		if err := renameColumnIfTableExists(
			ctx,
			database,
			column.tableName,
			column.legacyName,
			column.newName,
		); err != nil {
			return err
		}
	}
	if err := ensureColumnIfTableExists(
		ctx,
		database,
		"workspaces",
		"root_doc_path",
		"TEXT NOT NULL DEFAULT ''",
	); err != nil {
		return err
	}
	if err := ensureColumnIfTableExists(
		ctx,
		database,
		"workspaces",
		"selected_doc_path",
		"TEXT",
	); err != nil {
		return err
	}
	if err := dropColumnIfExists(
		ctx,
		database,
		"agent_runs",
		"artifact_root",
	); err != nil {
		return err
	}
	if err := ensureWorkspaceEventsDocColumnsIfTableExists(
		ctx,
		database,
	); err != nil {
		return err
	}
	if err := ensurePlanWorkspacesImplMetadataColumnsIfTableExists(
		ctx,
		database,
	); err != nil {
		return err
	}
	if err := ensureScopedUserChatSelections(ctx, database); err != nil {
		return err
	}
	if err := handlePreAgentChatArtifactCommentsTable(ctx, database); err != nil {
		return err
	}
	return ensureArtifactCommentsDocPathColumn(ctx, database)
}

func runRuntimeMigrations(ctx context.Context, database *sql.DB) error {
	if err := renameColumnIfTableExists(
		ctx,
		database,
		"agent_runs",
		"artifact_root",
		"root_doc_path",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_sessions",
		"workspace_id",
		"TEXT REFERENCES workspaces(id)",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_sessions",
		"thread_id",
		"TEXT REFERENCES agent_threads(id)",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_sessions",
		"user_email",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensureAgentSessionsImportingStatus(ctx, database); err != nil {
		return err
	}
	if err := ensureAgentSessionsUserEmailBackfill(ctx, database); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_threads",
		"workspace_id",
		"TEXT REFERENCES workspaces(id)",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"workspaces",
		"selected_doc_path",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"workspaces",
		"current_session_id",
		"TEXT REFERENCES chat_sessions(id)",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"workspaces",
		"current_branch_id",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensurePlanWorkspacesImplMetadataColumnsIfTableExists(
		ctx,
		database,
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"workspace_id",
		"TEXT REFERENCES workspaces(id)",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"session_id",
		"TEXT REFERENCES agent_sessions(id)",
	); err != nil {
		return err
	}
	if err := ensureAgentRunsWorkflowColumns(ctx, database); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"workflow_node_id",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"workflow_attempt",
		"INTEGER NOT NULL DEFAULT 0",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"workflow_result_status",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"workflow_result_json",
		"TEXT",
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_runs",
		"root_doc_path",
		"TEXT NOT NULL DEFAULT ''",
	); err != nil {
		return err
	}
	if err := dropColumnIfExists(
		ctx,
		database,
		"agent_runs",
		"artifact_root",
	); err != nil {
		return err
	}
	if err := ensureWorkspaceEventsDocColumnsIfTableExists(
		ctx,
		database,
	); err != nil {
		return err
	}
	if err := ensureColumn(
		ctx,
		database,
		"agent_entries",
		"origin_session_id",
		"TEXT REFERENCES agent_sessions(id)",
	); err != nil {
		return err
	}
	if err := reconcileRunningRunIndexPreflight(ctx, database); err != nil {
		return err
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_agent_threads_workspace_updated ON agent_threads(workspace_id, updated_at DESC) WHERE workspace_id IS NOT NULL AND archived_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_workspace_updated ON agent_sessions(workspace_id, updated_at DESC) WHERE workspace_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_workspace_created ON agent_runs(workspace_id, created_at DESC) WHERE workspace_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_runs_workspace_node_created ON agent_runs(workspace_id, workflow_node_id, created_at DESC) WHERE workflow_node_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_entries_origin_session ON agent_entries(origin_session_id) WHERE origin_session_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runs_thread_running ON agent_runs(thread_id) WHERE status = 'running'`,
	}
	for _, indexSQL := range indexes {
		if err := ensureIndex(ctx, database, indexSQL); err != nil {
			return err
		}
	}
	return nil
}

func ensureAgentRunsWorkflowColumnsIfTableExists(
	ctx context.Context,
	database *sql.DB,
) error {
	exists, err := tableExists(ctx, database, "agent_runs")
	if err != nil || !exists {
		return err
	}
	return ensureAgentRunsWorkflowColumns(ctx, database)
}

func ensurePlanWorkspacesImplMetadataColumnsIfTableExists(
	ctx context.Context,
	database *sql.DB,
) error {
	exists, err := tableExists(ctx, database, "plan_workspaces")
	if err != nil || !exists {
		return err
	}
	return ensurePlanWorkspacesImplMetadataColumns(ctx, database)
}

func ensureWorkspaceEventsDocColumnsIfTableExists(
	ctx context.Context,
	database *sql.DB,
) error {
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "doc_path", definition: "TEXT"},
		{name: "comment_id", definition: "TEXT"},
	} {
		if err := ensureColumnIfTableExists(
			ctx,
			database,
			"workspace_events",
			column.name,
			column.definition,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureScopedUserChatSelections(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "user_chat_selections")
	if err != nil || !exists {
		return err
	}
	hasScope, err := tableColumnExists(ctx, database, "user_chat_selections", "scope")
	if err != nil || hasScope {
		return err
	}
	if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() { _, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE user_chat_selections_new (
			user_email TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('global', 'freeform', 'workspace')),
			scope_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL,
			thread_id TEXT,
			run_id TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_email, scope, scope_id)
		)`,
		`INSERT INTO user_chat_selections_new (
			user_email,
			scope,
			scope_id,
			workspace_id,
			thread_id,
			run_id,
			created_at,
			updated_at
		)
		SELECT
			user_email,
			'global',
			'',
			workspace_id,
			thread_id,
			run_id,
			created_at,
			updated_at
		FROM user_chat_selections`,
		`DROP TABLE user_chat_selections`,
		`ALTER TABLE user_chat_selections_new RENAME TO user_chat_selections`,
		`CREATE INDEX IF NOT EXISTS idx_user_chat_selections_user_updated
			ON user_chat_selections (user_email, updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensurePlanWorkspacesImplMetadataColumns(
	ctx context.Context,
	database *sql.DB,
) error {
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "workspace_slug", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "impl_workspace_path", definition: "TEXT"},
		{name: "impl_workspace_url", definition: "TEXT"},
		{name: "impl_workspace_discovered_at", definition: "DATETIME"},
		{name: "impl_workspace_state", definition: "TEXT NOT NULL DEFAULT 'none' CHECK (impl_workspace_state IN ('none', 'active', 'merged', 'missing'))"},
		{name: "impl_workspace_merged_at", definition: "DATETIME"},
		{name: "impl_workspace_missing_at", definition: "DATETIME"},
	} {
		if err := ensureColumn(
			ctx,
			database,
			"plan_workspaces",
			column.name,
			column.definition,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureAgentRunsWorkflowColumns(ctx context.Context, database *sql.DB) error {
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "workflow_node_id", definition: "TEXT"},
		{name: "workflow_attempt", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "workflow_result_status", definition: "TEXT"},
		{name: "workflow_result_json", definition: "TEXT"},
	} {
		if err := ensureColumn(
			ctx,
			database,
			"agent_runs",
			column.name,
			column.definition,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(
	ctx context.Context,
	database *sql.DB,
	tableName, columnName, definition string,
) error {
	exists, err := tableColumnExists(ctx, database, tableName, columnName)
	if err != nil || exists {
		return err
	}
	_, err = database.ExecContext(
		ctx,
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition),
	)
	return err
}

func ensureColumnIfTableExists(
	ctx context.Context,
	database *sql.DB,
	tableName, columnName, definition string,
) error {
	exists, err := tableExists(ctx, database, tableName)
	if err != nil || !exists {
		return err
	}
	return ensureColumn(ctx, database, tableName, columnName, definition)
}

func renameColumnIfTableExists(
	ctx context.Context,
	database *sql.DB,
	tableName, legacyName, newName string,
) error {
	exists, err := tableExists(ctx, database, tableName)
	if err != nil || !exists {
		return err
	}
	hasNewColumn, err := tableColumnExists(ctx, database, tableName, newName)
	if err != nil || hasNewColumn {
		return err
	}
	hasLegacyColumn, err := tableColumnExists(ctx, database, tableName, legacyName)
	if err != nil || !hasLegacyColumn {
		return err
	}
	_, err = database.ExecContext(
		ctx,
		fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s",
			tableName,
			legacyName,
			newName,
		),
	)
	return err
}

func dropColumnIfExists(
	ctx context.Context,
	database *sql.DB,
	tableName, columnName string,
) error {
	exists, err := tableColumnExists(ctx, database, tableName, columnName)
	if err != nil || !exists {
		return err
	}
	_, err = database.ExecContext(
		ctx,
		fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tableName, columnName),
	)
	return err
}

func tableColumnExists(
	ctx context.Context,
	database *sql.DB,
	tableName, columnName string,
) (bool, error) {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info("+tableName+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, columnName) {
			return true, rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func ensureIndex(ctx context.Context, database *sql.DB, indexSQL string) error {
	_, err := database.ExecContext(ctx, indexSQL)
	return err
}

func handlePreAgentChatArtifactCommentsTable(
	ctx context.Context,
	database *sql.DB,
) error {
	exists, err := tableExists(ctx, database, "artifact_comments")
	if err != nil || !exists {
		return err
	}
	hasWorkspaceID, err := tableColumnExists(
		ctx,
		database,
		"artifact_comments",
		"workspace_id",
	)
	if err != nil || hasWorkspaceID {
		return err
	}
	name, err := preAgentChatArtifactCommentsTableName(ctx, database)
	if err != nil {
		return err
	}
	_, err = database.ExecContext(
		ctx,
		"ALTER TABLE artifact_comments RENAME TO "+name,
	)
	return err
}

func ensureArtifactCommentsDocPathColumn(
	ctx context.Context,
	database *sql.DB,
) error {
	exists, err := tableExists(ctx, database, "artifact_comments")
	if err != nil || !exists {
		return err
	}
	hasDocPath, err := tableColumnExists(
		ctx,
		database,
		"artifact_comments",
		"doc_path",
	)
	if err != nil || hasDocPath {
		return err
	}
	for _, legacyName := range []string{"artifact_rel_path", "artifact_path"} {
		if err := renameColumnIfTableExists(
			ctx,
			database,
			"artifact_comments",
			legacyName,
			"doc_path",
		); err != nil {
			return err
		}
		hasDocPath, err := tableColumnExists(
			ctx,
			database,
			"artifact_comments",
			"doc_path",
		)
		if err != nil || hasDocPath {
			return err
		}
	}
	return nil
}

func preAgentChatArtifactCommentsTableName(
	ctx context.Context,
	database *sql.DB,
) (string, error) {
	for i := range 100 {
		name := "artifact_comments_pre_agentchat"
		if i > 0 {
			name = fmt.Sprintf("artifact_comments_pre_agentchat_%d", i+1)
		}
		nameTaken, err := tableExists(ctx, database, name)
		if err != nil {
			return "", err
		}
		if !nameTaken {
			return name, nil
		}
	}
	return "", errors.New("no available pre-AgentChat artifact_comments table name")
}

func ensureAgentSessionsImportingStatus(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "agent_sessions")
	if err != nil || !exists {
		return err
	}

	var createSQL string
	if err := database.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'agent_sessions'`,
	).Scan(&createSQL); err != nil {
		return err
	}
	if strings.Contains(createSQL, "'importing'") ||
		!strings.Contains(strings.ToUpper(createSQL), "CHECK") ||
		!strings.Contains(createSQL, "'imported'") {
		return nil
	}

	if _, err := database.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() { _, _ = database.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE agent_sessions_new (
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
				CHECK (status IN ('pending', 'importing', 'imported', 'unassigned', 'ambiguous', 'diverged', 'failed')),
			inferred_workspace_id TEXT,
			inferred_plan_dir TEXT,
			imported_head_entry_id TEXT,
			last_imported_at DATETIME,
			last_error TEXT,
			metadata_json TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO agent_sessions_new (
			id,
			workspace_id,
			thread_id,
			user_email,
			source,
			session_path,
			session_id,
			parent_session_id,
			cwd,
			status,
			inferred_workspace_id,
			inferred_plan_dir,
			imported_head_entry_id,
			last_imported_at,
			last_error,
			metadata_json,
			created_at,
			updated_at
		)
		SELECT
			id,
			workspace_id,
			thread_id,
			user_email,
			source,
			session_path,
			session_id,
			parent_session_id,
			cwd,
			status,
			inferred_workspace_id,
			inferred_plan_dir,
			imported_head_entry_id,
			last_imported_at,
			last_error,
			metadata_json,
			created_at,
			updated_at
		FROM agent_sessions`,
		`DROP TABLE agent_sessions`,
		`ALTER TABLE agent_sessions_new RENAME TO agent_sessions`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_sessions_path
			ON agent_sessions(session_path)
			WHERE session_path IS NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureAgentSessionsUserEmailBackfill(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "agent_sessions")
	if err != nil || !exists {
		return err
	}
	hasUserEmail, err := tableColumnExists(ctx, database, "agent_sessions", "user_email")
	if err != nil || !hasUserEmail {
		return err
	}

	statements := []string{
		`UPDATE agent_sessions
SET user_email = (
    SELECT workspaces.user_email
    FROM workspaces
    WHERE workspaces.id = agent_sessions.workspace_id
)
WHERE (user_email IS NULL OR TRIM(user_email) = '')
  AND workspace_id IS NOT NULL
  AND TRIM(workspace_id) != ''
  AND EXISTS (
      SELECT 1 FROM workspaces WHERE workspaces.id = agent_sessions.workspace_id
  )`,
		`UPDATE agent_sessions
SET user_email = (
    SELECT agent_threads.user_email
    FROM agent_threads
    WHERE agent_threads.id = agent_sessions.thread_id
)
WHERE (user_email IS NULL OR TRIM(user_email) = '')
  AND thread_id IS NOT NULL
  AND TRIM(thread_id) != ''
  AND EXISTS (
      SELECT 1 FROM agent_threads WHERE agent_threads.id = agent_sessions.thread_id
  )`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func reconcileRunningRunIndexPreflight(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "agent_runs")
	if err != nil || !exists {
		return err
	}
	_, err = database.ExecContext(ctx, `
WITH ranked_running_agent_runs AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY thread_id ORDER BY created_at DESC, id DESC) AS row_num
    FROM agent_runs
    WHERE status = 'running'
)
UPDATE agent_runs
SET status = 'failed',
    error_message = 'superseded by active-run guard',
    completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP)
WHERE id IN (SELECT id FROM ranked_running_agent_runs WHERE row_num > 1)`)
	return err
}

func tableExists(ctx context.Context, database *sql.DB, tableName string) (bool, error) {
	var name string
	err := database.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).
		Scan(&name)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}
