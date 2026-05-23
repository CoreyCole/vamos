-- name: CreateChatMessage :one
INSERT INTO chat_messages (id, thread_id, role, content)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetChatMessages :many
SELECT * FROM chat_messages
WHERE thread_id = ?
ORDER BY created_at ASC;

-- name: GetChatMessageCount :one
SELECT COUNT(*) FROM chat_messages WHERE thread_id = ?;
