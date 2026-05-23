-- name: GetUserPreferences :one
SELECT * FROM user_preferences
WHERE user_email = ?
LIMIT 1;

-- name: UpsertUserPreferences :one
INSERT INTO user_preferences (user_email, theme, syntax_theme)
VALUES (?, ?, ?)
ON CONFLICT(user_email) DO UPDATE SET
    theme = excluded.theme,
    syntax_theme = excluded.syntax_theme,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: UpdateTheme :one
INSERT INTO user_preferences (user_email, theme, syntax_theme)
VALUES (?, ?, 'github-dark')
ON CONFLICT(user_email) DO UPDATE SET
    theme = excluded.theme,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: UpdateSyntaxTheme :one
INSERT INTO user_preferences (user_email, theme, syntax_theme)
VALUES (?, 'dark', ?)
ON CONFLICT(user_email) DO UPDATE SET
    syntax_theme = excluded.syntax_theme,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;
