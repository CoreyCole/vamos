CREATE TABLE IF NOT EXISTS users (
username TEXT PRIMARY KEY,
timezone TEXT NOT NULL DEFAULT 'Local',
created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
CHECK (length (trim (username)) BETWEEN 3 AND 32)
) ;

CREATE TABLE IF NOT EXISTS daily_games (
username TEXT NOT NULL REFERENCES users (username),
puzzle_date TEXT NOT NULL,
answer TEXT NOT NULL,
word_list_version TEXT NOT NULL,
status TEXT NOT NULL CHECK (status IN ('active', 'won', 'lost')),
created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
completed_at TEXT,
PRIMARY KEY (username, puzzle_date)
) ;

CREATE TABLE IF NOT EXISTS guesses (
id INTEGER PRIMARY KEY AUTOINCREMENT,
username TEXT NOT NULL,
puzzle_date TEXT NOT NULL,
row_index INTEGER NOT NULL,
guess TEXT NOT NULL,
result_json TEXT NOT NULL,
created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
UNIQUE (username, puzzle_date, row_index),
UNIQUE (username, puzzle_date, guess),
FOREIGN KEY (username, puzzle_date)
REFERENCES daily_games (username, puzzle_date)
) ;

CREATE INDEX IF NOT EXISTS guesses_game_idx
ON guesses (username, puzzle_date, row_index) ;
