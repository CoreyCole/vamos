-- name: CreateChatThread :one
INSERT INTO chat_threads (id, user_email, title)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetChatThread :one
SELECT * FROM chat_threads WHERE id = ? AND archived_at IS NULL;

-- name: ListChatThreads :many
SELECT * FROM chat_threads
WHERE user_email = ? AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT ?;

-- name: UpdateChatThread :exec
UPDATE chat_threads SET
    title = COALESCE(sqlc.narg('title'), title),
    archived_at = COALESCE(sqlc.narg('archived_at'), archived_at),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');
