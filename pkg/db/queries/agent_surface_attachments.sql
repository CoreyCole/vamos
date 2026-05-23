-- name: CreateAgentSurfaceAttachment :one
INSERT INTO agent_surface_attachments (
    id,
    chat_session_id,
    run_id,
    surface_kind,
    surface_id,
    user_email,
    permission_mode,
    owner_lease_expires_at,
    last_heartbeat_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('chat_session_id'),
    sqlc.narg('run_id'),
    sqlc.arg('surface_kind'),
    sqlc.narg('surface_id'),
    sqlc.narg('user_email'),
    sqlc.arg('permission_mode'),
    sqlc.narg('owner_lease_expires_at'),
    sqlc.narg('last_heartbeat_at')
)
RETURNING * ;

-- name: ListAgentSurfaceAttachmentsBySession :many
SELECT *
FROM agent_surface_attachments
WHERE chat_session_id = sqlc.arg ('chat_session_id')
ORDER BY connected_at ASC ;
