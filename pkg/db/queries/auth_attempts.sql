-- name: LogAuthAttempt :one
INSERT INTO auth_attempts (email, success, error_message)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetRecentAuthAttempts :many
SELECT * FROM auth_attempts
WHERE email = ?
ORDER BY attempted_at DESC
LIMIT ?;
