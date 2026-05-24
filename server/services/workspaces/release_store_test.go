package workspaces

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/pkg/release"
)

func TestSQLReleaseQueueStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewSQLReleaseQueueStore(openReleaseQueueTestQueries(t))

	created, err := store.CreateReleaseQueueItem(ctx, CreateReleaseQueueItemParams{
		ID: "item-1", DefinitionID: release.DefinitionID("default"), DefinitionVersion: "v1",
		WorkflowID: runtime.WorkflowID("release.promote"), WorkflowVersion: "v1", FlowID: release.FlowID("promote"),
		SourceSlug: "feature-1", TargetLane: "stage", ExpectedSourceCommit: "abc", ExpectedTargetCommit: "def", ActorEmail: "agent@example.test",
	})
	if err != nil {
		t.Fatalf("CreateReleaseQueueItem: %v", err)
	}
	if created.Status != ReleaseQueueStatusPending || created.WorkflowID != "release.promote" || created.FlowID != "promote" {
		t.Fatalf("created = %+v", created)
	}

	claimed, ok, err := store.ClaimNextPendingReleaseQueueItem(ctx)
	if err != nil || !ok {
		t.Fatalf("ClaimNextPendingReleaseQueueItem = (%+v,%v,%v), want item", claimed, ok, err)
	}
	if claimed.ID != "item-1" || claimed.Status != ReleaseQueueStatusRunning || claimed.StartedAt == nil {
		t.Fatalf("claimed = %+v", claimed)
	}

	if err := store.MarkReleaseQueueItemRunning(ctx, "item-1", runtime.NodeID("preflight")); err != nil {
		t.Fatalf("MarkReleaseQueueItemRunning: %v", err)
	}
	if err := store.AppendReleaseQueueEvent(ctx, AppendReleaseQueueEventParams{ItemID: "item-1", Level: "info", NodeID: "preflight", Message: "preflight ok"}); err != nil {
		t.Fatalf("AppendReleaseQueueEvent: %v", err)
	}
	if err := store.MarkReleaseQueueItemTerminal(ctx, "item-1", ReleaseQueueStatusFailed, "boom"); err != nil {
		t.Fatalf("MarkReleaseQueueItemTerminal: %v", err)
	}

	active, err := store.ListActiveReleaseQueueItems(ctx)
	if err != nil {
		t.Fatalf("ListActiveReleaseQueueItems: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active len = %d, want 0", len(active))
	}
	history, err := store.ListRecentReleaseQueueItems(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentReleaseQueueItems: %v", err)
	}
	if len(history) != 1 || history[0].Status != ReleaseQueueStatusFailed || history[0].ErrorMessage != "boom" || history[0].FinishedAt == nil {
		t.Fatalf("history = %+v", history)
	}
	events, err := store.ListReleaseQueueEvents(ctx, "item-1", 10)
	if err != nil {
		t.Fatalf("ListReleaseQueueEvents: %v", err)
	}
	if len(events) != 1 || events[0].NodeID != "preflight" || events[0].Message != "preflight ok" {
		t.Fatalf("events = %+v", events)
	}
}

func openReleaseQueueTestQueries(t *testing.T) *db.Queries {
	t.Helper()
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	schemaPath := filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaPath, err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	return db.New(dbConn)
}
