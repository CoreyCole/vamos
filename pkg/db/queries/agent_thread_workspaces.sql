-- name: ThreadHasWorkspaceAssociation :one
SELECT EXISTS(
    SELECT 1
    FROM agent_thread_workspaces
    WHERE
        thread_id = sqlc.arg('thread_id')
        AND workspace_id = sqlc.arg('workspace_id')
);

-- name: ListThreadWorkspaceAssociations :many
SELECT
    atw.*, w.id AS workspace_id, w.user_email, w.title, w.root_doc_path, w.cwd,
    w.workflow_type, w.workflow_state_json, w.source, w.selected_thread_id,
    w.selected_doc_path, w.current_session_id, w.current_branch_id,
    w.created_at, w.updated_at, w.archived_at
FROM agent_thread_workspaces atw
JOIN workspaces w ON w.id = atw.workspace_id
WHERE
    atw.thread_id = sqlc.arg('thread_id')
    AND w.archived_at IS NULL
ORDER BY atw.is_primary DESC, atw.adopted_at ASC, atw.created_at ASC;

-- name: GetPrimaryWorkspaceForThread :one
SELECT w.*
FROM agent_thread_workspaces atw
JOIN workspaces w ON w.id = atw.workspace_id
JOIN agent_threads t ON t.id = atw.thread_id
WHERE
    atw.thread_id = sqlc.arg('thread_id')
    AND atw.is_primary = 1
    AND sqlc.arg('user_email') = sqlc.arg('user_email')
    AND t.archived_at IS NULL
    AND w.archived_at IS NULL;

-- name: ListThreadsByPrimaryWorkspace :many
SELECT
    t.id,
    t.user_email,
    t.title,
    t.cwd,
    t.lineage_id,
    t.head_entry_id,
    t.parent_thread_id,
    t.forked_from_entry_id,
    t.created_at,
    t.updated_at,
    t.archived_at
FROM agent_threads t
JOIN agent_thread_workspaces atw ON atw.thread_id = t.id
WHERE
    atw.workspace_id = sqlc.arg('workspace_id')
    AND atw.is_primary = 1
    AND t.archived_at IS NULL
ORDER BY t.updated_at DESC;

-- name: AttachThreadToWorkspace :exec
INSERT INTO agent_thread_workspaces (
    thread_id, workspace_id, is_primary, role, adopted_from, adopted_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.narg('workspace_id'),
    1,
    'primary',
    'legacy_attach',
    CURRENT_TIMESTAMP
)
ON CONFLICT (thread_id, workspace_id) DO UPDATE SET
is_primary = 1,
role = 'primary',
adopted_at = CURRENT_TIMESTAMP ;

-- name: DemoteThreadPrimaryWorkspaces :exec
UPDATE agent_thread_workspaces
SET
is_primary = 0,
role = 'related'
WHERE thread_id = sqlc.arg ('thread_id') ;

-- name: UpsertThreadWorkspaceAssociation :exec
INSERT INTO agent_thread_workspaces (
thread_id, workspace_id, is_primary, role, adopted_from, adopted_at
) VALUES (
sqlc.arg ('thread_id'), sqlc.arg ('workspace_id'), sqlc.arg ('is_primary'),
sqlc.arg ('role'), sqlc.arg ('adopted_from'), CURRENT_TIMESTAMP
)
ON CONFLICT (thread_id, workspace_id) DO UPDATE SET
is_primary = excluded.is_primary,
role = excluded.role,
adopted_from = COALESCE (NULLIF (excluded.adopted_from,
''),
agent_thread_workspaces.adopted_from),
adopted_at = CURRENT_TIMESTAMP ;
