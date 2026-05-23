-- name: GetUserChatSelection :one
SELECT *
FROM user_chat_selections
WHERE user_email = sqlc.arg('user_email');

-- name: UpsertUserChatSelection :one
INSERT INTO user_chat_selections (
    user_email,
    workspace_id,
    thread_id,
    run_id
)
VALUES (
    sqlc.arg('user_email'),
    sqlc.arg('workspace_id'),
    sqlc.narg('thread_id'),
    sqlc.narg('run_id')
)
ON CONFLICT (user_email) DO UPDATE SET
workspace_id = excluded.workspace_id,
thread_id = excluded.thread_id,
run_id = excluded.run_id,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;
