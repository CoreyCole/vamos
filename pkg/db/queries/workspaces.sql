-- name: CreateWorkspace :one
INSERT INTO workspaces (
    id,
    user_email,
    title,
    root_doc_path,
    cwd,
    workflow_type,
    workflow_state_json,
    source,
    selected_thread_id,
    selected_doc_path
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('user_email'),
    sqlc.arg('title'),
    sqlc.arg('root_doc_path'),
    sqlc.narg('cwd'),
    sqlc.arg('workflow_type'),
    sqlc.narg('workflow_state_json'),
    sqlc.arg('source'),
    sqlc.narg('selected_thread_id'),
    sqlc.narg('selected_doc_path')
)
RETURNING * ;

-- name: GetWorkspaceForUser :one
SELECT *
FROM workspaces
WHERE id = sqlc.arg ('id')
AND user_email = sqlc.arg ('user_email')
AND archived_at IS NULL ;

-- name: GetWorkspace :one
SELECT *
FROM workspaces
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: ListWorkspaces :many
SELECT *
FROM workspaces
WHERE archived_at IS NULL
ORDER BY updated_at DESC
LIMIT sqlc.arg ('limit') ;

-- name: ListWorkspacesForUser :many
SELECT *
FROM workspaces
WHERE user_email = sqlc.arg ('user_email')
AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT sqlc.arg ('limit') ;

-- name: FindWorkspaceByRootDocPathForUser :one
SELECT *
FROM workspaces
WHERE user_email = sqlc.arg ('user_email')
AND root_doc_path = sqlc.arg ('root_doc_path')
AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT 1 ;

-- name: FindWorkspaceByRootDocPath :one
SELECT *
FROM workspaces
WHERE root_doc_path = sqlc.arg ('root_doc_path')
AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT 1 ;

-- name: UpdateWorkspaceSelectedThread :exec
UPDATE workspaces
SET selected_thread_id = sqlc.narg ('selected_thread_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateWorkspaceSelectedDoc :exec
UPDATE workspaces
SET selected_doc_path = sqlc.narg ('selected_doc_path'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateWorkspaceWorkflowState :exec
UPDATE workspaces
SET workflow_type = sqlc.arg ('workflow_type'),
workflow_state_json = sqlc.narg ('workflow_state_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateWorkspaceCurrentSession :exec
UPDATE workspaces
SET current_session_id = sqlc.narg ('current_session_id'),
current_branch_id = sqlc.narg ('current_branch_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;
