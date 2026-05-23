-- name: CreateSession :one
INSERT INTO sessions (id, user_email, expires_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ? AND expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: UpdateSessionAccess :exec
UPDATE sessions
SET last_accessed_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP;

-- name: GetSessionByEmail :one
SELECT * FROM sessions
WHERE user_email = ? AND expires_at > CURRENT_TIMESTAMP
ORDER BY created_at DESC
LIMIT 1;
