package runtime

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const workspaceDBBusyTimeoutMS = 5_000

func ReadWorkspaceEnv(checkout string) (WorkspaceIdentity, error) {
	path := filepath.Join(checkout, ".vamos", "run", "workspace.env")
	f, err := os.Open(path)
	if err != nil {
		return WorkspaceIdentity{}, err
	}
	defer f.Close()

	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			vals[strings.TrimSpace(key)] = unshellValue(strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return WorkspaceIdentity{}, err
	}

	dbPath := vals["VAMOS_DATABASE_PATH"]
	if dbPath == "" {
		dbPath = filepath.Join(checkout, ".vamos", "state", "agents.db")
	}
	return WorkspaceIdentity{
		Slug:         vals["VAMOS_WORKSPACE_SLUG"],
		CheckoutPath: vals["VAMOS_WORKSPACE_CHECKOUT"],
		DBPath:       dbPath,
		ManagerURL:   vals["VAMOS_WORKSPACE_MANAGER_URL"],
	}, nil
}

func unshellValue(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		inner := strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		return strings.ReplaceAll(inner, "'\\''", "'")
	}
	return strings.Trim(value, "\"")
}

func PreflightWorkspace(ctx context.Context, cfg Config) error {
	ws := cfg.Workspace
	if ws.Slug == "" || ws.Slug == "main" {
		return fmt.Errorf(
			"direct fixtures require registered non-main workspace slug, got %q",
			ws.Slug,
		)
	}
	if filepath.Clean(ws.CheckoutPath) != filepath.Clean(cfg.RepoRoot) {
		return fmt.Errorf(
			"workspace checkout mismatch: env=%s pwd=%s",
			ws.CheckoutPath,
			cfg.RepoRoot,
		)
	}
	if ws.DBPath == "" {
		return fmt.Errorf("workspace DB path is empty")
	}
	cleanDB := filepath.Clean(ws.DBPath)
	rel, err := filepath.Rel(cfg.RepoRoot, cleanDB)
	if err == nil && strings.HasPrefix(rel, "..") {
		return fmt.Errorf(
			"workspace DB path %s is outside checkout %s",
			ws.DBPath,
			cfg.RepoRoot,
		)
	}
	if strings.HasPrefix(rel, "data"+string(filepath.Separator)) ||
		strings.Contains(cleanDB, "/vamos/data/") ||
		strings.Contains(cleanDB, "/.local/state/vamos/") {
		return fmt.Errorf("refusing canonical DB path %s", ws.DBPath)
	}
	if _, err := os.Stat(cleanDB); err != nil {
		return fmt.Errorf("workspace DB stat %s: %w", ws.DBPath, err)
	}
	db, err := OpenWorkspaceDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}

func OpenWorkspaceDB(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.Workspace.DBPath == "" {
		return nil, fmt.Errorf("workspace DB path is empty")
	}
	dsn := cfg.Workspace.DBPath + "?_pragma=" + url.QueryEscape(
		fmt.Sprintf("busy_timeout(%d)", workspaceDBBusyTimeoutMS),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
