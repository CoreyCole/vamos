package db

import (
	"context"
	"testing"
)

func TestWorkspaceErrorEventsUpsertDedupesAndFilters(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	first, err := q.UpsertWorkspaceErrorEvent(ctx, UpsertWorkspaceErrorEventParams{
		WorkspaceSlug: "feature",
		Source:        "switch",
		Severity:      "warn",
		Message:       "workspace unavailable during switch",
		Detail:        "status=crashed",
		DedupeKey:     "feature-switch-crashed",
		PayloadJson:   "{}",
	})
	if err != nil {
		t.Fatalf("UpsertWorkspaceErrorEvent(first) error = %v", err)
	}
	if first.WorkspaceSlug != "feature" || first.OccurrenceCount != 1 {
		t.Fatalf("first = %+v", first)
	}

	second, err := q.UpsertWorkspaceErrorEvent(ctx, UpsertWorkspaceErrorEventParams{
		WorkspaceSlug: "feature",
		Source:        "switch",
		Severity:      "warn",
		Message:       "workspace unavailable during switch",
		Detail:        "status=crashed again",
		DedupeKey:     "feature-switch-crashed",
		PayloadJson:   `{"attempt":2}`,
	})
	if err != nil {
		t.Fatalf("UpsertWorkspaceErrorEvent(second) error = %v", err)
	}
	if second.ID != first.ID || second.OccurrenceCount != 2 {
		t.Fatalf("second = %+v, first = %+v", second, first)
	}

	if _, err := q.UpsertWorkspaceErrorEvent(ctx, UpsertWorkspaceErrorEventParams{
		WorkspaceSlug: "other",
		Source:        "manager",
		Severity:      "error",
		Message:       "workspace manager reported failure",
		Detail:        "boom",
		DedupeKey:     "other-manager-boom",
		PayloadJson:   "{}",
	}); err != nil {
		t.Fatalf("UpsertWorkspaceErrorEvent(other) error = %v", err)
	}

	recent, err := q.ListRecentWorkspaceErrorEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentWorkspaceErrorEvents() error = %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent len = %d, want 2", len(recent))
	}

	filtered, err := q.ListRecentWorkspaceErrorEventsForWorkspace(ctx, ListRecentWorkspaceErrorEventsForWorkspaceParams{
		WorkspaceSlug: "feature",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("ListRecentWorkspaceErrorEventsForWorkspace() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].WorkspaceSlug != "feature" || filtered[0].OccurrenceCount != 2 {
		t.Fatalf("filtered = %+v", filtered)
	}
}
