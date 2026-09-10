package db

import (
	"context"
	"database/sql"
	"testing"
)

func TestCreateAgentSlugUniqueAmongActive(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	if _, err := q.CreateAgent(ctx, CreateAgentParams{
		ID:   "agent-1",
		Slug: "nova",
		Name: "Nova",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAgent(ctx, CreateAgentParams{
		ID:   "agent-2",
		Slug: "nova",
		Name: "Nova 2",
	}); err == nil {
		t.Fatal("duplicate active slug accepted")
	}
	got, err := q.GetAgentBySlug(ctx, "nova")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "agent-1" {
		t.Fatalf("GetAgentBySlug = %#v", got)
	}
}

func TestBotHomeThreadKeyedByAgentID(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	a, err := q.CreateAgent(
		ctx,
		CreateAgentParams{ID: "a1", Slug: "alpha", Name: "Alpha"},
	)
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.CreateAgent(ctx, CreateAgentParams{ID: "a2", Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []Agent{a, b} {
		thread, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
			ID:        "thread-" + agent.Slug,
			UserEmail: "shared",
			Title:     agent.Name,
			Cwd:       "/tmp/" + agent.Slug,
			LineageID: "lin-" + agent.Slug,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
			AgentID: sql.NullString{String: agent.ID, Valid: true},
			Cwd:     thread.Cwd,
			Title:   agent.Name,
			ID:      thread.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	homeA, err := q.GetBotHomeThreadByAgentID(
		ctx,
		sql.NullString{String: a.ID, Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	homeB, err := q.GetBotHomeThreadByAgentID(
		ctx,
		sql.NullString{String: b.ID, Valid: true},
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
}
