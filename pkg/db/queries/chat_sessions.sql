-- name: CreateChatSession :one
INSERT INTO chat_sessions (
    id,
    workspace_id,
    created_by_user_email,
    parent_session_id,
    forked_from_seq,
    branch_id,
    workflow_id,
    workflow_node_id,
    workflow_attempt,
    topology_kind
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('workspace_id'),
    sqlc.arg('created_by_user_email'),
    sqlc.narg('parent_session_id'),
    sqlc.narg('forked_from_seq'),
    sqlc.arg('branch_id'),
    sqlc.narg('workflow_id'),
    sqlc.narg('workflow_node_id'),
    sqlc.arg('workflow_attempt'),
    sqlc.arg('topology_kind')
)
RETURNING * ;

-- name: GetChatSession :one
SELECT *
FROM chat_sessions
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: ListChatSessionsByWorkspace :many
SELECT *
FROM chat_sessions
WHERE workspace_id = sqlc.arg ('workspace_id')
AND archived_at IS NULL
ORDER BY updated_at DESC ;

-- name: UpdateChatSessionProjectionSeq :exec
UPDATE chat_sessions
SET current_projection_seq = sqlc.arg ('current_projection_seq'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;
