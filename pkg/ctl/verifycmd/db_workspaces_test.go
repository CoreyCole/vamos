package verifycmd

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoadDBWorkspacesVerifyConfigRequiresDatabasePath(t *testing.T) {
	t.Parallel()

	_, err := LoadDBWorkspacesVerifyConfig(nil)
	if err == nil {
		t.Fatal("LoadDBWorkspacesVerifyConfig() error = nil, want required database path error")
	}
	if !strings.Contains(err.Error(), "--database-path is required") {
		t.Fatalf("LoadDBWorkspacesVerifyConfig() error = %v, want database path error", err)
	}
}

func TestLoadDBWorkspacesVerifyConfigParsesJSONFormat(t *testing.T) {
	t.Parallel()

	cfg, err := LoadDBWorkspacesVerifyConfig([]string{"--database-path", "agents.db", "--format", "json"})
	if err != nil {
		t.Fatalf("LoadDBWorkspacesVerifyConfig() error = %v", err)
	}
	if cfg.DatabasePath != "agents.db" {
		t.Fatalf("DatabasePath = %q, want agents.db", cfg.DatabasePath)
	}
	if cfg.Format != VerifyOutputJSON {
		t.Fatalf("Format = %q, want json", cfg.Format)
	}
}

func TestLoadDBWorkspacesVerifyConfigRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := LoadDBWorkspacesVerifyConfig([]string{"--database-path", "agents.db", "--format", "yaml"})
	if err == nil {
		t.Fatal("LoadDBWorkspacesVerifyConfig() error = nil, want unsupported format error")
	}
}

func TestRunDBWorkspacesVerifyFailsOpenDatabaseWithReport(t *testing.T) {
	t.Parallel()

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{
		DatabasePath: filepath.Join(t.TempDir(), "missing", "agents.db"),
		Format:       VerifyOutputText,
	})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want open failure")
	}
	assertDBVerifyCheckFailed(t, report, "open_database", false)
}

func TestRunDBWorkspacesVerifyPassesCanonicalCleanDB(t *testing.T) {
	t.Parallel()

	path := openCanonicalWorkspaceVerifyFixture(t, `
INSERT INTO plan_workspaces (plan_dir_rel, project_id, plan_dir, label, artifact_updated_at)
VALUES ('thoughts/test/plans/example', 'vamos', '/thoughts/test/plans/example', 'Example', CURRENT_TIMESTAMP);
INSERT INTO plan_workspace_projects (plan_dir_rel, project_id, role)
VALUES ('thoughts/test/plans/example', 'vamos', 'primary');
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status, plan_dir_rel)
VALUES ('vamos', 'feature', '', '/repo/vamos-feature', 'Feature', 'active', 'thoughts/test/plans/example');
INSERT INTO plan_workspace_impl_bindings (plan_dir_rel, project_id, workspace_slug, checkout_path, status, binding_source, impl_project_id, impl_workspace_slug)
VALUES ('thoughts/test/plans/example', 'vamos', 'feature', '/repo/vamos-feature', 'active', 'binding_file', 'vamos', 'feature');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err != nil {
		t.Fatalf("RunDBWorkspacesVerify() error = %v; report = %#v", err, report)
	}
	if report.Status != statusPassed {
		t.Fatalf("report.Status = %q, want %q", report.Status, statusPassed)
	}
}

func TestRunDBWorkspacesVerifyPassesNullImplProjectIDForDefaultProject(t *testing.T) {
	t.Parallel()

	path := openCanonicalWorkspaceVerifyFixture(t, `
INSERT INTO plan_workspaces (plan_dir_rel, project_id, plan_dir, label, artifact_updated_at)
VALUES ('thoughts/test/plans/example', 'vamos', '/thoughts/test/plans/example', 'Example', CURRENT_TIMESTAMP);
INSERT INTO plan_workspace_projects (plan_dir_rel, project_id, role)
VALUES ('thoughts/test/plans/example', 'vamos', 'primary');
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status, plan_dir_rel)
VALUES ('', 'feature', '', '/repo/vamos-feature', 'Feature', 'active', 'thoughts/test/plans/example');
INSERT INTO plan_workspace_impl_bindings (plan_dir_rel, project_id, workspace_slug, checkout_path, status, binding_source, impl_project_id, impl_workspace_slug)
VALUES ('thoughts/test/plans/example', 'vamos', 'feature', '/repo/vamos-feature', 'active', 'binding_file', NULL, 'feature');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err != nil {
		t.Fatalf("RunDBWorkspacesVerify() error = %v; report = %#v", err, report)
	}
	if report.Status != statusPassed {
		t.Fatalf("report.Status = %q, want %q", report.Status, statusPassed)
	}
}

func TestRunDBWorkspacesVerifyFailsForeignKeyCheck(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, fkWorkspaceVerifySchema+`
PRAGMA foreign_keys = OFF;
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status, plan_dir_rel)
VALUES ('vamos', 'feature', '', '/repo/vamos-feature', 'Feature', 'active', 'thoughts/missing/plan');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "foreign_key_check", true)
}

func TestRunDBWorkspacesVerifyFailsDuplicateCheckoutPath(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status)
VALUES ('vamos', 'stage', 'stage', '/repo/vamos', 'Stage', 'active'),
       ('vamos', 'local', '', '/repo/vamos', 'Local', 'active');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "duplicate_impl_checkout_path", true)
}

func TestRunDBWorkspacesVerifyFailsOrphanImplPlanRef(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status, plan_dir_rel)
VALUES ('vamos', 'feature', '', '/repo/vamos-feature', 'Feature', 'active', 'thoughts/missing/plan');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "orphan_impl_plan_dir_rel", true)
}

func TestRunDBWorkspacesVerifyFailsMissingBindingPlan(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO plan_workspace_impl_bindings (plan_dir_rel, project_id, status)
VALUES ('thoughts/missing/plan', 'vamos', 'planned');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "active_binding_missing_plan", true)
}

func TestRunDBWorkspacesVerifyFailsMissingBindingImpl(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO plan_workspaces (plan_dir_rel, project_id, plan_dir, label, artifact_updated_at)
VALUES ('thoughts/test/plans/example', 'vamos', '/thoughts/test/plans/example', 'Example', CURRENT_TIMESTAMP);
INSERT INTO plan_workspace_impl_bindings (plan_dir_rel, project_id, status, impl_project_id, impl_workspace_slug)
VALUES ('thoughts/test/plans/example', 'vamos', 'active', 'vamos', 'missing');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "active_binding_missing_impl", true)
}

func TestRunDBWorkspacesVerifyFailsDuplicatePrimaryPlanProject(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO plan_workspaces (plan_dir_rel, project_id, plan_dir, label, artifact_updated_at)
VALUES ('thoughts/test/plans/example', 'vamos', '/thoughts/test/plans/example', 'Example', CURRENT_TIMESTAMP);
INSERT INTO plan_workspace_projects (plan_dir_rel, project_id, role)
VALUES ('thoughts/test/plans/example', 'vamos', 'primary'),
       ('thoughts/test/plans/example', 'datastarui', 'primary');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "duplicate_active_primary_plan_project", true)
}

func TestRunDBWorkspacesVerifyFailsProtectedTerminalStatus(t *testing.T) {
	t.Parallel()

	path := openWorkspaceVerifyFixture(t, legacyWorkspaceVerifySchema+`
INSERT INTO impl_workspaces (project_id, workspace_slug, checkout_role, checkout_path, display_name, status)
VALUES ('vamos', 'stage', 'stage', '/repo/vamos', 'Stage', 'merged');`)

	report, err := RunDBWorkspacesVerify(t.Context(), DBWorkspacesVerifyConfig{DatabasePath: path, Format: VerifyOutputText})
	if err == nil {
		t.Fatal("RunDBWorkspacesVerify() error = nil, want invariant failure")
	}
	assertDBVerifyCheckFailed(t, report, "protected_workspace_terminal_status", true)
}

func TestRenderDBWorkspacesVerifyReportJSON(t *testing.T) {
	t.Parallel()

	report := DBWorkspacesVerifyReport{Status: statusPassed, Checks: []DBWorkspacesVerifyCheck{{Name: "foreign_keys_enabled", Status: statusPassed}}}
	var out bytes.Buffer
	if err := renderDBWorkspacesVerifyReport(&out, report, VerifyOutputJSON); err != nil {
		t.Fatalf("renderDBWorkspacesVerifyReport() error = %v", err)
	}
	var decoded DBWorkspacesVerifyReport
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(rendered) error = %v; output = %s", err, out.String())
	}
	if decoded.Status != statusPassed || len(decoded.Checks) != 1 {
		t.Fatalf("decoded report = %#v, want passed report with one check", decoded)
	}
}

func TestRenderDBWorkspacesVerifyReportText(t *testing.T) {
	t.Parallel()

	report := DBWorkspacesVerifyReport{Status: statusFailed, Checks: []DBWorkspacesVerifyCheck{{Name: "duplicate_impl_checkout_path", Status: statusFailed, Count: 1, Details: "checkout_path=/repo/vamos"}}}
	var out bytes.Buffer
	if err := renderDBWorkspacesVerifyReport(&out, report, VerifyOutputText); err != nil {
		t.Fatalf("renderDBWorkspacesVerifyReport() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"workspace DB verification: failed", "duplicate_impl_checkout_path", "checkout_path=/repo/vamos"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered text = %q, want substring %q", text, want)
		}
	}
}

func openCanonicalWorkspaceVerifyFixture(t *testing.T, seed string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agents.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.ExecContext(t.Context(), "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	schemaPath := filepath.Join("..", "..", "db", "migrations", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaPath, err)
	}
	if _, err := database.ExecContext(t.Context(), string(schema)); err != nil {
		t.Fatalf("exec canonical schema: %v", err)
	}
	if _, err := database.ExecContext(t.Context(), seed); err != nil {
		t.Fatalf("exec seed: %v", err)
	}
	return path
}

func openWorkspaceVerifyFixture(t *testing.T, schemaAndSeed string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agents.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.ExecContext(t.Context(), schemaAndSeed); err != nil {
		t.Fatalf("exec fixture: %v", err)
	}
	return path
}

func assertDBVerifyCheckFailed(t *testing.T, report DBWorkspacesVerifyReport, name string, wantCount bool) {
	t.Helper()
	if report.Status != statusFailed {
		t.Fatalf("report.Status = %q, want %q; report = %#v", report.Status, statusFailed, report)
	}
	for _, check := range report.Checks {
		if check.Name != name {
			continue
		}
		if check.Status != statusFailed {
			t.Fatalf("check %s status = %q, want %q; report = %#v", name, check.Status, statusFailed, report)
		}
		if wantCount && check.Count == 0 {
			t.Fatalf("check %s Count = 0, want > 0; check = %#v", name, check)
		}
		return
	}
	t.Fatalf("missing failed check %s; report = %#v", name, report)
}

const fkWorkspaceVerifySchema = `
CREATE TABLE plan_workspaces (
  plan_dir_rel TEXT PRIMARY KEY,
  project_id TEXT NOT NULL DEFAULT '',
  plan_dir TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL DEFAULT '',
  artifact_updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  archived_at DATETIME
);
CREATE TABLE impl_workspaces (
  project_id TEXT NOT NULL DEFAULT '',
  workspace_slug TEXT NOT NULL,
  checkout_role TEXT NOT NULL DEFAULT '',
  checkout_path TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  plan_dir_rel TEXT REFERENCES plan_workspaces(plan_dir_rel),
  status TEXT NOT NULL DEFAULT 'active',
  archived_at DATETIME,
  PRIMARY KEY (project_id, workspace_slug)
);
CREATE TABLE plan_workspace_projects (
  plan_dir_rel TEXT NOT NULL REFERENCES plan_workspaces(plan_dir_rel),
  project_id TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'related',
  archived_at DATETIME,
  PRIMARY KEY (plan_dir_rel, project_id)
);
CREATE TABLE plan_workspace_impl_bindings (
  plan_dir_rel TEXT NOT NULL REFERENCES plan_workspaces(plan_dir_rel),
  project_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'planned',
  impl_project_id TEXT,
  impl_workspace_slug TEXT,
  archived_at DATETIME,
  PRIMARY KEY (plan_dir_rel, project_id),
  FOREIGN KEY (impl_project_id, impl_workspace_slug) REFERENCES impl_workspaces(project_id, workspace_slug)
);
`

const legacyWorkspaceVerifySchema = `
CREATE TABLE plan_workspaces (
  plan_dir_rel TEXT PRIMARY KEY,
  project_id TEXT NOT NULL DEFAULT '',
  plan_dir TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL DEFAULT '',
  artifact_updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  archived_at DATETIME
);
CREATE TABLE impl_workspaces (
  project_id TEXT NOT NULL DEFAULT '',
  workspace_slug TEXT NOT NULL,
  checkout_role TEXT NOT NULL DEFAULT '',
  checkout_path TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  plan_dir_rel TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  archived_at DATETIME
);
CREATE TABLE plan_workspace_projects (
  plan_dir_rel TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'related',
  archived_at DATETIME
);
CREATE TABLE plan_workspace_impl_bindings (
  plan_dir_rel TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'planned',
  impl_project_id TEXT,
  impl_workspace_slug TEXT,
  archived_at DATETIME
);
`
