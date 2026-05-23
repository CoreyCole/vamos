package workspace

import (
	"context"
	"testing"
)

func TestLiveFlushLoopCoalescesDirtyKeys(t *testing.T) {
	var workspaces []string
	loop := NewLiveFlushLoop(LiveFlushPolicy{}, func(workspaceID string) {
		workspaces = append(workspaces, workspaceID)
	})
	loop.MarkDirty("workspace-1", "thread-1")
	loop.MarkDirty("workspace-1", "thread-1")
	loop.MarkDirty("workspace-1", "thread-1")

	if got := loop.FlushOnce(context.Background()); got != 1 {
		t.Fatalf("FlushOnce() = %d, want 1", got)
	}
	if len(workspaces) != 1 || workspaces[0] != "workspace-1" {
		t.Fatalf("workspaces = %#v, want workspace-1 once", workspaces)
	}
	if got := loop.FlushOnce(context.Background()); got != 0 {
		t.Fatalf("second FlushOnce() = %d, want 0", got)
	}
}
