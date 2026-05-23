-- name: GetLayoutPreference :one
SELECT * FROM layout_preferences
WHERE user_email = ? AND page = ? AND view = ?;

-- name: UpsertLayoutPreference :one
INSERT INTO layout_preferences (user_email, page, view, config_json)
VALUES (?, ?, ?, ?)
ON CONFLICT (user_email, page, view) DO UPDATE SET
config_json = excluded.config_json,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: DeleteLayoutPreference :exec
DELETE FROM layout_preferences
WHERE user_email = ? AND page = ? AND view = ? ;
