package vamos

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	_ "modernc.org/sqlite"
)

func TestBuildAuthURLUsesVamosTokenEnv(t *testing.T) {
	t.Setenv("VAMOS_E2E_AUTH_TOKEN", "secret-token")
	got, err := BuildAuthURL(duiruntime.Config{BaseURL: "http://example.test/"}, "/thoughts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "http://example.test/internal/playwright-auth?") {
		t.Fatalf("BuildAuthURL()=%q", got)
	}
	if !strings.Contains(got, "redirect=%2Fthoughts") || !strings.Contains(got, "token=secret-token") {
		t.Fatalf("BuildAuthURL()=%q, want redirect and env token", got)
	}
}

func TestWorkspacePreflightRejectsMainAndCanonicalDB(t *testing.T) {
	checkout := newWorkspaceCheckout(t, "feature-a")
	valid, err := ReadWorkspaceEnv(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if err := valid.Preflight(context.Background(), checkout); err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}

	cases := []struct {
		name string
		edit func(WorkspaceEnv) WorkspaceEnv
		want string
	}{
		{name: "main slug", edit: func(w WorkspaceEnv) WorkspaceEnv { w.Slug = "main"; return w }, want: "registered non-main workspace"},
		{name: "canonical data db", edit: func(w WorkspaceEnv) WorkspaceEnv { w.DBPath = filepath.Join(checkout, "data", "vamos.db"); return w }, want: "refusing canonical DB path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.edit(valid).Preflight(context.Background(), checkout)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Preflight() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAppRegistersAuthAndPreflight(t *testing.T) {
	app := App()
	if app.Name != "vamos" || app.Authenticate == nil || app.Preflight == nil {
		t.Fatalf("App()=%#v, want named app with auth and preflight hooks", app)
	}
}

func newWorkspaceCheckout(t *testing.T, slug string) string {
	t.Helper()
	checkout := t.TempDir()
	mustMkdir(t, filepath.Join(checkout, ".vamos", "run"))
	mustMkdir(t, filepath.Join(checkout, ".vamos", "state"))
	if err := os.WriteFile(filepath.Join(checkout, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "VAMOS_WORKSPACE_SLUG='" + slug + "'\n" +
		"VAMOS_WORKSPACE_CHECKOUT='" + checkout + "'\n" +
		"VAMOS_WORKSPACE_MANAGER_URL='https://main.workspaces.creative-mode.ai'\n"
	if err := os.WriteFile(filepath.Join(checkout, ".vamos", "run", "workspace.env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(checkout, ".vamos", "state", "agents.db"))
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
