package verifycmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const maxInvariantDetailRows = 5

type DBWorkspacesVerifyConfig struct {
	DatabasePath string
	Format       VerifyOutputFormat
}

type VerifyOutputFormat string

const (
	VerifyOutputText VerifyOutputFormat = "text"
	VerifyOutputJSON VerifyOutputFormat = "json"
)

type DBWorkspacesVerifyReport struct {
	Status string                    `json:"status"`
	Checks []DBWorkspacesVerifyCheck `json:"checks"`
}

type DBWorkspacesVerifyCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
	Count   int    `json:"count,omitempty"`
}

type DBInvariantFailure struct {
	Check string
	Rows  []map[string]string
}

func (e DBInvariantFailure) Error() string {
	if len(e.Rows) == 0 {
		return e.Check
	}
	return fmt.Sprintf("%s failed with %d row(s)", e.Check, len(e.Rows))
}

type invariantSpec struct {
	name  string
	query string
}

var dbWorkspaceInvariantSpecs = []invariantSpec{
	{
		name: "duplicate_impl_checkout_path",
		query: `
select checkout_path, count(*) as count
from impl_workspaces
group by checkout_path
having count(*) > 1`,
	},
	{
		name: "orphan_impl_plan_dir_rel",
		query: `
select project_id, workspace_slug, plan_dir_rel
from impl_workspaces
where plan_dir_rel is not null
  and not exists (
    select 1 from plan_workspaces p
    where p.plan_dir_rel = impl_workspaces.plan_dir_rel
  )`,
	},
	{
		name: "active_binding_missing_plan",
		query: `
select b.plan_dir_rel, b.project_id
from plan_workspace_impl_bindings b
where b.archived_at is null
  and not exists (
    select 1 from plan_workspaces p
    where p.plan_dir_rel = b.plan_dir_rel
  )`,
	},
	{
		name: "active_binding_missing_impl",
		query: `
select b.plan_dir_rel, b.project_id, b.impl_project_id, b.impl_workspace_slug
from plan_workspace_impl_bindings b
where b.archived_at is null
  and b.status = 'active'
  and b.impl_workspace_slug is not null
  and not exists (
    select 1 from impl_workspaces i
    where i.project_id = b.impl_project_id
      and i.workspace_slug = b.impl_workspace_slug
  )`,
	},
	{
		name: "duplicate_active_primary_plan_project",
		query: `
select plan_dir_rel, count(*) as count
from plan_workspace_projects
where archived_at is null and role = 'primary'
group by plan_dir_rel
having count(*) > 1`,
	},
	{
		name: "protected_workspace_terminal_status",
		query: `
select project_id, workspace_slug, checkout_role, status
from impl_workspaces
where (workspace_slug in ('main', 'stage') or checkout_role in ('main', 'stage'))
  and status in ('merged', 'cleaned_up')`,
	},
}

func MainDBWorkspaces(args []string) error {
	cfg, err := LoadDBWorkspacesVerifyConfig(args)
	if err != nil {
		return err
	}
	report, runErr := RunDBWorkspacesVerify(context.Background(), cfg)
	if err := renderDBWorkspacesVerifyReport(os.Stdout, report, cfg.Format); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	if report.Status != statusPassed {
		return errors.New("workspace DB verification failed")
	}
	return nil
}

func LoadDBWorkspacesVerifyConfig(args []string) (DBWorkspacesVerifyConfig, error) {
	cfg := DBWorkspacesVerifyConfig{Format: VerifyOutputText}
	fs := flag.NewFlagSet("vamos ctl verify db-workspaces", flag.ContinueOnError)
	fs.StringVar(&cfg.DatabasePath, "database-path", "", "path to Vamos SQLite agents.db")
	format := string(cfg.Format)
	fs.StringVar(&format, "format", format, "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	cfg.DatabasePath = strings.TrimSpace(cfg.DatabasePath)
	cfg.Format = VerifyOutputFormat(strings.TrimSpace(format))
	if cfg.DatabasePath == "" {
		return cfg, errors.New("--database-path is required")
	}
	switch cfg.Format {
	case VerifyOutputText, VerifyOutputJSON:
	default:
		return cfg, fmt.Errorf("unsupported --format %q", cfg.Format)
	}
	return cfg, nil
}

func RunDBWorkspacesVerify(ctx context.Context, cfg DBWorkspacesVerifyConfig) (DBWorkspacesVerifyReport, error) {
	report := DBWorkspacesVerifyReport{Status: statusPassed}
	database, err := openWorkspaceVerifyDB(ctx, cfg.DatabasePath)
	if err != nil {
		report.Status = statusFailed
		report.Checks = []DBWorkspacesVerifyCheck{{
			Name:    "open_database",
			Status:  statusFailed,
			Details: err.Error(),
		}}
		return report, err
	}
	defer func() { _ = database.Close() }()

	checks := []DBWorkspacesVerifyCheck{
		runForeignKeysEnabledCheck(ctx, database),
		runForeignKeyCheck(ctx, database),
	}
	for _, spec := range dbWorkspaceInvariantSpecs {
		checks = append(checks, runScalarInvariant(ctx, database, spec))
	}
	report.Checks = checks
	for _, check := range checks {
		if check.Status != statusPassed {
			report.Status = statusFailed
			return report, DBInvariantFailure{Check: check.Name}
		}
	}
	return report, nil
}

func openWorkspaceVerifyDB(ctx context.Context, path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", sqliteVerifyDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite DB: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite DB: %w", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return database, nil
}

func sqliteVerifyDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%s_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path, separator)
}

func runForeignKeysEnabledCheck(ctx context.Context, database *sql.DB) DBWorkspacesVerifyCheck {
	var enabled int
	if err := database.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return DBWorkspacesVerifyCheck{Name: "foreign_keys_enabled", Status: statusFailed, Details: err.Error()}
	}
	if enabled != 1 {
		return DBWorkspacesVerifyCheck{Name: "foreign_keys_enabled", Status: statusFailed, Details: "PRAGMA foreign_keys is not enabled"}
	}
	return DBWorkspacesVerifyCheck{Name: "foreign_keys_enabled", Status: statusPassed}
}

func runForeignKeyCheck(ctx context.Context, database *sql.DB) DBWorkspacesVerifyCheck {
	rows, err := collectRows(ctx, database, "PRAGMA foreign_key_check")
	if err != nil {
		return DBWorkspacesVerifyCheck{Name: "foreign_key_check", Status: statusFailed, Details: err.Error()}
	}
	if len(rows) > 0 {
		return DBWorkspacesVerifyCheck{Name: "foreign_key_check", Status: statusFailed, Count: len(rows), Details: formatInvariantRows(rows)}
	}
	return DBWorkspacesVerifyCheck{Name: "foreign_key_check", Status: statusPassed}
}

func runScalarInvariant(ctx context.Context, database *sql.DB, spec invariantSpec) DBWorkspacesVerifyCheck {
	rows, err := collectRows(ctx, database, spec.query)
	if err != nil {
		return DBWorkspacesVerifyCheck{Name: spec.name, Status: statusFailed, Details: err.Error()}
	}
	if len(rows) > 0 {
		return DBWorkspacesVerifyCheck{Name: spec.name, Status: statusFailed, Count: len(rows), Details: formatInvariantRows(rows)}
	}
	return DBWorkspacesVerifyCheck{Name: spec.name, Status: statusPassed}
}

func collectRows(ctx context.Context, database *sql.DB, query string) ([]map[string]string, error) {
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]string{}
	for rows.Next() {
		raw := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := map[string]string{}
		for i, column := range columns {
			if raw[i].Valid {
				row[column] = raw[i].String
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func renderDBWorkspacesVerifyReport(w io.Writer, report DBWorkspacesVerifyReport, format VerifyOutputFormat) error {
	switch format {
	case VerifyOutputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case VerifyOutputText:
		fmt.Fprintf(w, "workspace DB verification: %s\n", report.Status)
		for _, check := range report.Checks {
			if check.Count > 0 {
				fmt.Fprintf(w, "- %s: %s (%d row(s))", check.Name, check.Status, check.Count)
			} else {
				fmt.Fprintf(w, "- %s: %s", check.Name, check.Status)
			}
			if check.Details != "" {
				fmt.Fprintf(w, " — %s", check.Details)
			}
			fmt.Fprintln(w)
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func formatInvariantRows(rows []map[string]string) string {
	if len(rows) == 0 {
		return ""
	}
	limit := len(rows)
	if limit > maxInvariantDetailRows {
		limit = maxInvariantDetailRows
	}
	parts := make([]string, 0, limit+1)
	for _, row := range rows[:limit] {
		keys := make([]string, 0, len(row))
		for key := range row {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, fmt.Sprintf("%s=%s", key, row[key]))
		}
		parts = append(parts, strings.Join(values, ", "))
	}
	if len(rows) > limit {
		parts = append(parts, fmt.Sprintf("... %d more row(s)", len(rows)-limit))
	}
	return strings.Join(parts, "; ")
}
