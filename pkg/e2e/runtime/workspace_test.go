package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReadWorkspaceEnvAndPreflightWorkspace(t *testing.T) {
	checkout := newWorkspaceCheckout(t, "feature-a")
	cfg, err := LoadConfigFromEnv(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Workspace.Slug, "feature-a"; got != want {
		t.Fatalf("Workspace.Slug=%q want %q", got, want)
	}
	if err := PreflightWorkspace(context.Background(), cfg); err != nil {
		t.Fatalf("PreflightWorkspace() error = %v", err)
	}
}

func TestPreflightWorkspaceRejectsUnsafeWorkspaces(t *testing.T) {
	checkout := newWorkspaceCheckout(t, "feature-a")
	valid := Config{
		RepoRoot: checkout,
		Workspace: WorkspaceIdentity{
			Slug:         "feature-a",
			CheckoutPath: checkout,
			DBPath:       filepath.Join(checkout, ".vamos", "state", "agents.db"),
		},
	}
	cases := []struct {
		name string
		edit func(Config) Config
		want string
	}{
		{
			name: "empty slug",
			edit: func(c Config) Config { c.Workspace.Slug = ""; return c },
			want: "registered non-main workspace",
		},
		{
			name: "main slug",
			edit: func(c Config) Config { c.Workspace.Slug = "main"; return c },
			want: "registered non-main workspace",
		},
		{
			name: "checkout mismatch",
			edit: func(c Config) Config { c.Workspace.CheckoutPath = filepath.Join(checkout, "other"); return c },
			want: "workspace checkout mismatch",
		},
		{
			name: "canonical data db",
			edit: func(c Config) Config { c.Workspace.DBPath = filepath.Join(checkout, "data", "vamos.db"); return c },
			want: "refusing canonical DB path",
		},
		{name: "home state db", edit: func(c Config) Config {
			c.Workspace.DBPath = filepath.Join(
				checkout,
				".local",
				"state",
				"vamos",
				"vamos.db",
			)
			return c
		}, want: "refusing canonical DB path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := PreflightWorkspace(context.Background(), tc.edit(valid))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf(
					"PreflightWorkspace() error = %v, want substring %q",
					err,
					tc.want,
				)
			}
		})
	}
}

func newWorkspaceCheckout(t *testing.T, slug string) string {
	t.Helper()
	checkout := t.TempDir()
	mustMkdir(t, filepath.Join(checkout, ".vamos", "run"))
	mustMkdir(t, filepath.Join(checkout, ".vamos", "state"))
	if err := os.WriteFile(
		filepath.Join(checkout, "go.mod"),
		[]byte("module test\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	env := "VAMOS_WORKSPACE_SLUG='" + slug + "'\n" +
		"VAMOS_WORKSPACE_CHECKOUT='" + checkout + "'\n" +
		"VAMOS_WORKSPACE_MANAGER_URL='https://main.workspaces.creative-mode.ai'\n"
	if err := os.WriteFile(
		filepath.Join(checkout, ".vamos", "run", "workspace.env"),
		[]byte(env),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(
		"sqlite",
		filepath.Join(checkout, ".vamos", "state", "agents.db"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return checkout
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
