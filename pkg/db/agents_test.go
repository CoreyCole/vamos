package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
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
	}); err != nil {
		t.Fatalf("second live bot_home for same slug: %v", err)
	}
}

func TestListAgentThreadsByAgentSlug(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	older, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-alpha-old",
		UserEmail: "shared",
		Title:     "alpha-old",
		Cwd:       "/tmp/alpha-old",
		LineageID: "lin-alpha-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "alpha", Valid: true},
		Cwd:       older.Cwd,
		Title:     "alpha-old",
		ID:        older.ID,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)

	newer, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-alpha-new",
		UserEmail: "shared",
		Title:     "alpha-new",
		Cwd:       "/tmp/alpha-new",
		LineageID: "lin-alpha-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "alpha", Valid: true},
		Cwd:       newer.Cwd,
		Title:     "alpha-new",
		ID:        newer.ID,
	}); err != nil {
		t.Fatal(err)
	}

	other, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-beta",
		UserEmail: "shared",
		Title:     "beta",
		Cwd:       "/tmp/beta",
		LineageID: "lin-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "beta", Valid: true},
		Cwd:       other.Cwd,
		Title:     "beta",
		ID:        other.ID,
	}); err != nil {
		t.Fatal(err)
	}

	listed, err := q.ListAgentThreadsByAgentSlug(
		ctx,
		sql.NullString{String: "alpha", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("got %d alpha threads, want 2", len(listed))
	}
	if listed[0].ID != newer.ID || listed[1].ID != older.ID {
		t.Fatalf(
			"order = %s,%s want %s,%s",
			listed[0].ID,
			listed[1].ID,
			newer.ID,
			older.ID,
		)
	}
}

func TestListAgentThreadsFreeform(t *testing.T) {
	ctx := context.Background()
	_, q := openWorkspaceDocsTestDB(t)

	free, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-free",
		UserEmail: "shared",
		Title:     "free",
		Cwd:       "/tmp/free",
		LineageID: "lin-free",
	})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := q.CreateAgentThread(ctx, CreateAgentThreadParams{
		ID:        "thread-bot",
		UserEmail: "shared",
		Title:     "bot",
		Cwd:       "/tmp/bot",
		LineageID: "lin-bot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "alpha", Valid: true},
		Cwd:       bot.Cwd,
		Title:     "bot",
		ID:        bot.ID,
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := q.ListAgentThreadsFreeform(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != free.ID {
		t.Fatalf("freeform list = %+v", listed)
	}
}
