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
	if err := ensureImplWorkspaceCleanupProofColumnsIfTableExists(ctx, database); err != nil {
		return err
	}
	if err := ensureScopedUserChatSelections(ctx, database); err != nil {
		return err
	}
	if err := handlePreAgentChatArtifactCommentsTable(ctx, database); err != nil {
		return err
	}
	if err := ensureLayoutPreferencesViewportClass(ctx, database); err != nil {
		return err
	}
	if err := ensureAgentThreadWorkspaces(ctx, database); err != nil {
		return err
	}
	if err := ensureAgentThreadProjectColumnsIfTableExists(ctx, database); err != nil {
		return err
	}
	return ensureArtifactCommentsDocPathColumn(ctx, database)
}

func runRuntimeMigrations(ctx context.Context, database *sql.DB) error {
	if err := ensureLayoutPreferencesViewportClass(ctx, database); err != nil {
		return err
	}
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
	if err := ensureAgentThreadWorkspaces(ctx, database); err != nil {
		return err
	}
	if err := ensureAgentThreadProjectColumnsIfTableExists(ctx, database); err != nil {
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
	if err := ensureImplWorkspaceCleanupProofColumnsIfTableExists(ctx, database); err != nil {
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
	if err := ensureWorkspaceErrorEvents(ctx, database); err != nil {
		return err
	}

	indexes := []string{
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

func ensureWorkspaceErrorEvents(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS workspace_error_events (
id INTEGER PRIMARY KEY AUTOINCREMENT,
workspace_slug TEXT NOT NULL,
source TEXT NOT NULL CHECK (source IN ('switch', 'manager', 'log')),
severity TEXT NOT NULL CHECK (severity IN ('warn', 'error')),
message TEXT NOT NULL,
detail TEXT NOT NULL DEFAULT '',
dedupe_key TEXT NOT NULL,
occurrence_count INTEGER NOT NULL DEFAULT 1,
payload_json TEXT NOT NULL DEFAULT '{}',
first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return err
	}
	for _, indexSQL := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_error_events_dedupe ON workspace_error_events(dedupe_key)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_error_events_recent ON workspace_error_events(last_seen_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_error_events_workspace_recent ON workspace_error_events(workspace_slug, last_seen_at DESC, id DESC)`,
	} {
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

func ensureImplWorkspaceCleanupProofColumnsIfTableExists(
	ctx context.Context,
	database *sql.DB,
) error {
	exists, err := tableExists(ctx, database, "impl_workspaces")
	if err != nil || !exists {
		return err
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "project_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "checkout_role", definition: "TEXT NOT NULL DEFAULT '' CHECK (checkout_role IN ('', 'main', 'stage'))"},
		{name: "cleanup_proof_kind", definition: "TEXT NOT NULL DEFAULT 'unknown' CHECK (cleanup_proof_kind IN ('ancestor', 'patch_equivalent', 'cached', 'unknown'))"},
		{name: "cleanup_proof_source_ref", definition: "TEXT"},
		{name: "cleanup_proof_target_commit", definition: "TEXT"},
		{name: "cleanup_proof_at", definition: "DATETIME"},
		{name: "cleanup_risk_reason", definition: "TEXT"},
	} {
		if err := ensureColumn(ctx, database, "impl_workspaces", column.name, column.definition); err != nil {
			return err
		}
	}
	if err := ensureImplWorkspaceCompositePrimaryKey(ctx, database); err != nil {
		return err
	}
	for _, indexSQL := range []string{
		`CREATE INDEX IF NOT EXISTS idx_impl_workspaces_project_status_updated ON impl_workspaces (project_id, status, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_impl_workspaces_plan_dir_rel ON impl_workspaces (plan_dir_rel) WHERE plan_dir_rel IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_impl_workspaces_checkout_path ON impl_workspaces (checkout_path)`,
	} {
		if err := ensureIndex(ctx, database, indexSQL); err != nil {
			return err
		}
	}
	return nil
}

func ensureImplWorkspaceCompositePrimaryKey(ctx context.Context, database *sql.DB) error {
	composite, err := implWorkspacesHasCompositePrimaryKey(ctx, database)
	if err != nil || composite {
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
		`DROP INDEX IF EXISTS idx_impl_workspaces_status_updated`,
		`DROP INDEX IF EXISTS idx_impl_workspaces_project_status_updated`,
		`DROP INDEX IF EXISTS idx_impl_workspaces_plan_dir_rel`,
		`DROP INDEX IF EXISTS idx_impl_workspaces_checkout_path`,
		`CREATE TABLE impl_workspaces_new (
			project_id TEXT NOT NULL DEFAULT '',
			workspace_slug TEXT NOT NULL,
			checkout_role TEXT NOT NULL DEFAULT '' CHECK (checkout_role IN ('', 'main', 'stage')),
			checkout_path TEXT NOT NULL,
			display_name TEXT NOT NULL,
			host TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			plan_dir_rel TEXT REFERENCES plan_workspaces (plan_dir_rel),
			plan_dir TEXT,
			status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cleaned_up', 'merged')),
			branch TEXT,
			commit_hash TEXT,
			trunk_branch TEXT,
			top_branch TEXT,
			bottom_branch TEXT,
			bottom_parent_branch TEXT,
			base_branch TEXT,
			ahead_count INTEGER NOT NULL DEFAULT 0,
			behind_count INTEGER NOT NULL DEFAULT 0,
			merged_at DATETIME,
			cleaned_up_at DATETIME,
			merge_evidence TEXT,
			cleanup_proof_kind TEXT NOT NULL DEFAULT 'unknown' CHECK (cleanup_proof_kind IN ('ancestor', 'patch_equivalent', 'cached', 'unknown')),
			cleanup_proof_source_ref TEXT,
			cleanup_proof_target_commit TEXT,
			cleanup_proof_at DATETIME,
			cleanup_risk_reason TEXT,
			env_last_repaired_at DATETIME,
			env_last_error TEXT,
			git_detail TEXT,
			discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_id, workspace_slug)
		)`,
		`INSERT INTO impl_workspaces_new (
			project_id, workspace_slug, checkout_role, checkout_path, display_name, host, url,
			plan_dir_rel, plan_dir, status, branch, commit_hash, trunk_branch, top_branch,
			bottom_branch, bottom_parent_branch, base_branch, ahead_count, behind_count,
			merged_at, cleaned_up_at, merge_evidence, cleanup_proof_kind,
			cleanup_proof_source_ref, cleanup_proof_target_commit, cleanup_proof_at,
			cleanup_risk_reason, env_last_repaired_at, env_last_error, git_detail,
			discovered_at, last_discovered_at, updated_at
		)
		SELECT COALESCE(project_id, ''), workspace_slug, COALESCE(checkout_role, ''), checkout_path,
			display_name, host, url, plan_dir_rel, plan_dir, status, branch, commit_hash,
			trunk_branch, top_branch, bottom_branch, bottom_parent_branch, base_branch,
			ahead_count, behind_count, merged_at, cleaned_up_at, merge_evidence,
			COALESCE(NULLIF(cleanup_proof_kind, ''), 'unknown'), cleanup_proof_source_ref,
			cleanup_proof_target_commit, cleanup_proof_at, cleanup_risk_reason,
			env_last_repaired_at, env_last_error, git_detail, discovered_at,
			last_discovered_at, updated_at
		FROM impl_workspaces`,
		`DROP TABLE impl_workspaces`,
		`ALTER TABLE impl_workspaces_new RENAME TO impl_workspaces`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func implWorkspacesHasCompositePrimaryKey(ctx context.Context, database *sql.DB) (bool, error) {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info(impl_workspaces)")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	pk := map[int]string{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt any
		var pkIndex int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pkIndex); err != nil {
			return false, err
		}
		if pkIndex > 0 {
			pk[pkIndex] = name
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return len(pk) == 2 && strings.EqualFold(pk[1], "project_id") && strings.EqualFold(pk[2], "workspace_slug"), nil
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

func ensureLayoutPreferencesViewportClass(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "layout_preferences")
	if err != nil || !exists {
		return err
	}
	hasViewportClass, err := tableColumnExists(ctx, database, "layout_preferences", "viewport_class")
	if err != nil || hasViewportClass {
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE layout_preferences_new (
			user_email TEXT NOT NULL,
			page TEXT NOT NULL CHECK (page IN ('agent-chat', 'thoughts')),
			view TEXT NOT NULL CHECK (view IN ('focus', 'split')),
			viewport_class TEXT NOT NULL DEFAULT 'desktop-full' CHECK (viewport_class IN ('mobile', 'desktop-half', 'desktop-full')),
			config_json TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_email, page, view, viewport_class)
		)`,
		`INSERT INTO layout_preferences_new (user_email, page, view, viewport_class, config_json, created_at, updated_at)
		 SELECT user_email, page, view, 'desktop-full', config_json, created_at, updated_at FROM layout_preferences`,
		`DROP TABLE layout_preferences`,
		`ALTER TABLE layout_preferences_new RENAME TO layout_preferences`,
		`CREATE INDEX IF NOT EXISTS idx_layout_preferences_user ON layout_preferences (user_email, page, view, viewport_class)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
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
		{name: "project_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "workspace_slug", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "impl_workspace_path", definition: "TEXT"},
		{name: "impl_workspace_url", definition: "TEXT"},
		{name: "impl_workspace_discovered_at", definition: "DATETIME"},
		{name: "impl_workspace_state", definition: "TEXT NOT NULL DEFAULT 'none' CHECK (impl_workspace_state IN ('none', 'active', 'merged', 'missing'))"},
		{name: "impl_workspace_merged_at", definition: "DATETIME"},
		{name: "impl_workspace_missing_at", definition: "DATETIME"},
		{name: "qrspi_lifecycle", definition: "TEXT NOT NULL DEFAULT 'question' CHECK (qrspi_lifecycle IN ('question', 'research', 'design', 'outline', 'review_outline', 'plan', 'review_plan', 'workspace', 'implement', 'review_implementation', 'verify', 'merged', 'closed'))"},
		{name: "qrspi_lifecycle_updated_at", definition: "DATETIME"},
		{name: "qrspi_closed_reason", definition: "TEXT NOT NULL DEFAULT ''"},
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
	for _, indexSQL := range []string{
		"CREATE INDEX IF NOT EXISTS idx_plan_workspaces_lifecycle_activity ON plan_workspaces (qrspi_lifecycle, artifact_updated_at DESC, plan_dir_rel) WHERE archived_at IS NULL",
		"CREATE INDEX IF NOT EXISTS idx_plan_workspaces_project_active_activity ON plan_workspaces (project_id, artifact_updated_at DESC, plan_dir_rel) WHERE archived_at IS NULL",
		"CREATE INDEX IF NOT EXISTS idx_plan_workspaces_project_lifecycle_activity ON plan_workspaces (project_id, qrspi_lifecycle, artifact_updated_at DESC, plan_dir_rel) WHERE archived_at IS NULL",
	} {
		if err := ensureIndex(ctx, database, indexSQL); err != nil {
			return err
		}
	}
	return nil
}

func ensureAgentThreadWorkspaces(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "agent_threads")
	if err != nil || !exists {
		return err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS agent_thread_workspaces (
			thread_id TEXT NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			is_primary INTEGER NOT NULL DEFAULT 0,
			role TEXT NOT NULL DEFAULT 'related' CHECK (role IN ('primary', 'related')),
			adopted_from TEXT NOT NULL DEFAULT '',
			adopted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (thread_id, workspace_id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_thread_workspaces_primary
			ON agent_thread_workspaces(thread_id)
			WHERE is_primary = 1`,
		`CREATE INDEX IF NOT EXISTS idx_agent_thread_workspaces_workspace
			ON agent_thread_workspaces(workspace_id, thread_id)`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	hasWorkspaceID, err := tableColumnExists(ctx, database, "agent_threads", "workspace_id")
	if err != nil || !hasWorkspaceID {
		return err
	}
	if _, err := database.ExecContext(ctx, `DROP INDEX IF EXISTS idx_agent_threads_workspace_updated`); err != nil {
		return err
	}
	return dropColumnIfExists(ctx, database, "agent_threads", "workspace_id")
}

func ensureAgentThreadProjectColumnsIfTableExists(ctx context.Context, database *sql.DB) error {
	exists, err := tableExists(ctx, database, "agent_threads")
	if err != nil || !exists {
		return err
	}
	if err := ensureColumn(ctx, database, "agent_threads", "project_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return ensureIndex(ctx, database, "CREATE INDEX IF NOT EXISTS idx_agent_threads_project_user_updated ON agent_threads (project_id, user_email, updated_at DESC) WHERE archived_at IS NULL")
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
