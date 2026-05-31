package vamos

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	_ "modernc.org/sqlite"
)

const workspaceDBBusyTimeoutMS = 5_000

type WorkspaceEnv struct {
	Slug         string
	CheckoutPath string
	DBPath       string
	ManagerURL   string
}

func ReadWorkspaceEnv(checkout string) (WorkspaceEnv, error) {
	path := filepath.Join(checkout, ".vamos", "run", "workspace.env")
	f, err := os.Open(path)
	if err != nil {
		return WorkspaceEnv{}, err
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
		return WorkspaceEnv{}, err
	}

	dbPath := vals["VAMOS_DATABASE_PATH"]
	if dbPath == "" {
		dbPath = filepath.Join(checkout, ".vamos", "state", "agents.db")
	}
	return WorkspaceEnv{
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

func Preflight(ctx context.Context, cfg duiruntime.Config) error {
	ws, err := ReadWorkspaceEnv(cfg.RepoRoot)
	if err != nil {
		return err
	}
	return ws.Preflight(ctx, cfg.RepoRoot)
}

func (w WorkspaceEnv) Preflight(ctx context.Context, repoRoot string) error {
	if w.Slug == "" || w.Slug == "main" {
		return fmt.Errorf(
			"direct fixtures require registered non-main workspace slug, got %q",
			w.Slug,
		)
	}
	if filepath.Clean(w.CheckoutPath) != filepath.Clean(repoRoot) {
		return fmt.Errorf(
			"workspace checkout mismatch: env=%s pwd=%s",
			w.CheckoutPath,
			repoRoot,
		)
	}
	if w.DBPath == "" {
		return fmt.Errorf("workspace DB path is empty")
	}
	cleanDB := filepath.Clean(w.DBPath)
	rel, err := filepath.Rel(repoRoot, cleanDB)
	if err == nil && strings.HasPrefix(rel, "..") {
		return fmt.Errorf(
			"workspace DB path %s is outside checkout %s",
			w.DBPath,
			repoRoot,
		)
	}
	if strings.HasPrefix(rel, "data"+string(filepath.Separator)) ||
		strings.Contains(cleanDB, "/vamos/data/") ||
		strings.Contains(cleanDB, "/.local/state/vamos/") {
		return fmt.Errorf("refusing canonical DB path %s", w.DBPath)
	}
	if _, err := os.Stat(cleanDB); err != nil {
		return fmt.Errorf("workspace DB stat %s: %w", w.DBPath, err)
	}
	db, err := openWorkspaceDBPath(ctx, w.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PingContext(ctx)
}

func OpenWorkspaceDB(ctx context.Context, cfg duiruntime.Config) (*sql.DB, error) {
	ws, err := ReadWorkspaceEnv(cfg.RepoRoot)
	if err != nil {
		return nil, err
	}
	if ws.DBPath == "" {
		return nil, fmt.Errorf("workspace DB path is empty")
	}
	return openWorkspaceDBPath(ctx, ws.DBPath)
}

func openWorkspaceDBPath(ctx context.Context, dbPath string) (*sql.DB, error) {
	dsn := dbPath + "?_pragma=" + url.QueryEscape(
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
