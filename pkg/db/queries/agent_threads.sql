-- name: CreateAgentThread :one
INSERT INTO agent_threads (
    id,
    user_email,
    workspace_id,
    title,
    cwd,
    lineage_id,
    head_entry_id,
    parent_thread_id,
    forked_from_entry_id
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('user_email'),
    sqlc.narg('workspace_id'),
    sqlc.arg('title'),
    sqlc.arg('cwd'),
    sqlc.arg('lineage_id'),
    sqlc.narg('head_entry_id'),
    sqlc.narg('parent_thread_id'),
    sqlc.narg('forked_from_entry_id')
)
RETURNING * ;

-- name: GetAgentThread :one
SELECT *
FROM agent_threads
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: GetAgentThreadForUser :one
SELECT *
FROM agent_threads
WHERE id = sqlc.arg ('id')
AND user_email = sqlc.arg ('user_email')
AND archived_at IS NULL ;

-- name: ListAgentThreads :many
SELECT *
FROM agent_threads
WHERE user_email = sqlc.arg ('user_email')
AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT sqlc.arg ('limit') ;

-- name: UpdateAgentThreadHead :exec
UPDATE agent_threads
SET head_entry_id = sqlc.narg ('head_entry_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentThreadTitle :exec
UPDATE agent_threads
SET title = sqlc.arg ('title'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentThreadCwd :exec
UPDATE agent_threads
SET cwd = sqlc.arg ('cwd'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: ListAgentThreadsByWorkspace :many
SELECT *
FROM agent_threads
WHERE workspace_id = sqlc.arg ('workspace_id')
AND archived_at IS NULL
ORDER BY updated_at DESC ;

-- name: ListAgentThreadsForUserWithWorkspace :many
SELECT t.*, w.root_doc_path AS workspace_root_doc_path
FROM agent_threads t
LEFT JOIN workspaces w
ON w.id = t.workspace_id
AND w.user_email = t.user_email
AND w.archived_at IS NULL
WHERE t.user_email = sqlc.arg ('user_email')
AND t.archived_at IS NULL
ORDER BY t.updated_at DESC ;

-- name: AttachThreadToWorkspace :exec
UPDATE agent_threads
SET workspace_id = sqlc.arg ('workspace_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: GetAgentThreadForWorkspaceUser :one
SELECT t.*
FROM agent_threads AS t
JOIN workspaces AS w
ON w.id = t.workspace_id
WHERE t.id = sqlc.arg ('thread_id')
AND t.workspace_id = sqlc.arg ('workspace_id')
AND w.user_email = sqlc.arg ('user_email')
AND t.archived_at IS NULL
AND w.archived_at IS NULL ;
