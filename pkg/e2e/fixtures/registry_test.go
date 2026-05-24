package fixtures

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDefaultRegistryResolvesBasicFixture(t *testing.T) {
	registry := DefaultRegistry()
	builder, err := registry.Resolve("thoughts-workbench.basic")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	db, err := sql.Open("sqlite", t.TempDir()+"/agents.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state, err := builder(context.Background(), db, Input{Workspace: WorkspaceIdentity{
		Slug:         "feature-a",
		CheckoutPath: t.TempDir(),
		DBPath:       t.TempDir() + "/agents.db",
	}})
	if err != nil {
		t.Fatalf("builder() error = %v", err)
	}
	if got, want := state.Name, "thoughts-workbench.basic"; got != want {
		t.Fatalf("state.Name=%q want %q", got, want)
	}
	if got, want := state.Data["workspace_slug"], "feature-a"; got != want {
		t.Fatalf("workspace_slug=%v want %q", got, want)
	}
}

func TestBuildThoughtsWorkbenchBasicRequiresWorkspace(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/agents.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = BuildThoughtsWorkbenchBasic(
		context.Background(),
		db,
		Input{Workspace: WorkspaceIdentity{Slug: "main"}},
	)
	if err == nil {
		t.Fatal("expected main workspace rejection")
	}
}
