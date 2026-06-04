package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/db"
)

// Service wraps the database connection
type Service struct {
	db      *sql.DB
	Queries *db.Queries
}

// NewService initializes the database connection and runs migrations
func NewService(dbPath string) (*Service, error) {
	// Ensure the directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Open database connection
	database, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx := context.Background()

	// Test connection
	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := enableSQLiteForeignKeys(ctx, database); err != nil {
		return nil, fmt.Errorf("failed to enable sqlite foreign keys: %w", err)
	}

	// Enable WAL mode for concurrent reads during writes
	if _, err := database.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	if err := reconcileRunningRunIndexPreflight(
		ctx,
		database,
	); err != nil {
		return nil, fmt.Errorf("failed to reconcile active agent runs: %w", err)
	}

	if err := prepareSchemaCompatibilityMigrations(ctx, database); err != nil {
		return nil, fmt.Errorf(
			"failed to prepare schema compatibility migrations: %w",
			err,
		)
	}

	// Run migrations (read schema.sql and execute)
	schemaSQL, err := readSchemaSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}

	if _, err := database.ExecContext(ctx, string(schemaSQL)); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	if err := runRuntimeMigrations(ctx, database); err != nil {
		return nil, fmt.Errorf("failed to run runtime migrations: %w", err)
	}

	queries := db.New(database)

	return &Service{
		db:      database,
		Queries: queries,
	}, nil
}

const sqliteBusyTimeoutMS = 5000

func sqliteDSN(dbPath string) string {
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	return fmt.Sprintf(
		"%s%s_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)",
		dbPath,
		separator,
		sqliteBusyTimeoutMS,
	)
}

func readSchemaSQL() ([]byte, error) {
	candidates := []string{
		filepath.Join("pkg", "db", "migrations", "schema.sql"),
		filepath.Join("..", "vamos", "pkg", "db", "migrations", "schema.sql"),
		filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql"),
		filepath.Join("..", "..", "..", "..", "pkg", "db", "migrations", "schema.sql"),
		filepath.Join("..", "..", "pkg", "db", "migrations", "schema.sql"),
	}
	var lastErr error
	for _, candidate := range candidates {
		schemaSQL, err := os.ReadFile(candidate)
		if err == nil {
			return schemaSQL, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Close closes the database connection
func (s *Service) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection
func (s *Service) DB() *sql.DB {
	return s.db
}
