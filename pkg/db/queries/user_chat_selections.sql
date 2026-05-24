-- name: GetUserChatSelection :one
SELECT *
FROM user_chat_selections
WHERE
    user_email = sqlc.arg('user_email')
    AND scope = sqlc.arg('scope')
    AND scope_id = sqlc.arg('scope_id');

-- name: GetLatestUserChatSelectionByScope :one
SELECT *
FROM user_chat_selections
WHERE
    user_email = sqlc.arg('user_email')
    AND scope = sqlc.arg('scope')
ORDER BY updated_at DESC
LIMIT 1;

-- name: UpsertUserChatSelection :one
INSERT INTO user_chat_selections (
    user_email,
    scope,
    scope_id,
    workspace_id,
    thread_id,
    run_id
)
VALUES (
    sqlc.arg('user_email'),
    sqlc.arg('scope'),
    sqlc.arg('scope_id'),
    sqlc.arg('workspace_id'),
    sqlc.narg('thread_id'),
    sqlc.narg('run_id')
)
ON CONFLICT (user_email, scope, scope_id) DO UPDATE SET
workspace_id = excluded.workspace_id,
thread_id = excluded.thread_id,
run_id = excluded.run_id,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;
