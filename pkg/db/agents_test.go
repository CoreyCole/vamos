package db

import (
	"context"
	"database/sql"
	"testing"
)

func TestBotHomeThreadKeyedBySlug(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	for _, slug := range []string{"alpha", "beta"} {
		thread, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
			ID:        "thread-" + slug,
			UserEmail: "shared",
			Title:     slug,
			Cwd:       "/tmp/" + slug,
			LineageID: "lin-" + slug,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
			AgentSlug: sql.NullString{String: slug, Valid: true},
			Cwd:       thread.Cwd,
			Title:     slug,
			ID:        thread.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	homeA, err := q.GetBotHomeThreadBySlug(
		ctx,
		sql.NullString{String: "alpha", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	homeB, err := q.GetBotHomeThreadBySlug(
		ctx,
		sql.NullString{String: "beta", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if homeA.ID == homeB.ID {
		t.Fatalf("two slugs shared thread %s", homeA.ID)
	}
	if homeA.RoomKind != "bot_home" || homeB.RoomKind != "bot_home" {
		t.Fatalf("room_kind a=%q b=%q", homeA.RoomKind, homeB.RoomKind)
	}

	dup, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-alpha-dup",
		UserEmail: "shared",
		Title:     "alpha-dup",
		Cwd:       "/tmp/alpha-dup",
		LineageID: "lin-alpha-dup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "alpha", Valid: true},
		Cwd:       dup.Cwd,
		Title:     "alpha-dup",
		ID:        dup.ID,
	}); err == nil {
		t.Fatal("duplicate active bot_home agent_slug accepted")
	}
}
