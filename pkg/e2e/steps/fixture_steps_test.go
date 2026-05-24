package steps

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	e2e "github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

func TestLoadFixtureSetsContextState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := &e2e.Context{Config: e2e.Config{Workspace: e2e.WorkspaceIdentity{
		Slug:         "feature-a",
		CheckoutPath: t.TempDir(),
		DBPath:       dbPath,
	}}}
	state := LoadFixture(t, ctx, "thoughts-workbench.basic")
	if got, want := state.Name, "thoughts-workbench.basic"; got != want {
		t.Fatalf("state.Name=%q want %q", got, want)
	}
	stored, ok := ctx.Fixture.(fixtures.State)
	if !ok {
		t.Fatalf("ctx.Fixture type=%T want fixtures.State", ctx.Fixture)
	}
	if got, want := stored.Data["workspace_slug"], "feature-a"; got != want {
		t.Fatalf("workspace_slug=%v want %q", got, want)
	}
}
