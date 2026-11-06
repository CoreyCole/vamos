package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// Open opens a SQLite database with WAL mode
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return database, nil
}

// Initialize creates the schema if needed
func Initialize(database *sql.DB, schemaPath string) error {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}

	if _, err := database.Exec(string(schema)); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	return nil
}
