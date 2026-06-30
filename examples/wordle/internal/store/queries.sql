-- name: UpsertUser :exec
INSERT INTO users (username, timezone, last_seen_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (username) DO UPDATE SET
timezone = excluded.timezone,
last_seen_at = CURRENT_TIMESTAMP ;

-- name: GetDailyGame :one
SELECT username,
puzzle_date,
answer,
word_list_version,
status,
created_at,
completed_at
FROM daily_games
WHERE username = ? AND puzzle_date = ? ;

-- name: CreateDailyGame :one
INSERT INTO daily_games (username,
puzzle_date,
answer,
word_list_version,
status)
VALUES (?, ?, ?, ?, 'active')
RETURNING username,
puzzle_date,
answer,
word_list_version,
status,
created_at,
completed_at ;

-- name: ListGuesses :many
SELECT id, username, puzzle_date, row_index, guess, result_json, created_at
FROM guesses
WHERE username = ? AND puzzle_date = ?
ORDER BY row_index ASC ;

-- name: CountGuesses :one
SELECT count (*)
FROM guesses
WHERE username = ? AND puzzle_date = ? ;

-- name: InsertGuess :one
INSERT INTO guesses (username, puzzle_date, row_index, guess, result_json)
VALUES (?, ?, ?, ?, ?)
RETURNING id, username, puzzle_date, row_index, guess, result_json, created_at ;

-- name: UpdateGameStatus :exec
UPDATE daily_games
SET status = ?,
completed_at = CASE WHEN ? = 'active' THEN NULL ELSE CURRENT_TIMESTAMP END
WHERE username = ? AND puzzle_date = ? ;
