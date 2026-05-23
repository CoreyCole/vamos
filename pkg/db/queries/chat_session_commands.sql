-- name: CreateChatCommand :one
INSERT INTO chat_session_commands (
    id,
    session_id,
    idempotency_key,
    command_type,
    status,
    actor_email,
    payload_json,
    outcome_json
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('session_id'),
    sqlc.arg('idempotency_key'),
    sqlc.arg('command_type'),
    sqlc.arg('status'),
    sqlc.arg('actor_email'),
    sqlc.arg('payload_json'),
    sqlc.narg('outcome_json')
)
RETURNING * ;

-- name: GetChatCommandByIdempotencyKey :one
SELECT *
FROM chat_session_commands
WHERE session_id = sqlc.arg ('session_id')
AND idempotency_key = sqlc.arg ('idempotency_key') ;

-- name: UpdateChatCommandStatus :one
UPDATE chat_session_commands
SET status = sqlc.arg ('status'),
outcome_json = sqlc.narg ('outcome_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
RETURNING * ;
