package agentchat

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func seedTestRoster(t *testing.T, bots ...roster.Bot) *roster.Store {
	t.Helper()
	store := &roster.Store{Path: filepath.Join(t.TempDir(), "agents.yml")}
	for _, bot := range bots {
		if _, err := store.Create(bot); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestResolveMessageRoomSlugToPairwise(t *testing.T) {
	t.Parallel()
	thoughtsRoot := t.TempDir()
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "msg-room.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	store := seedTestRoster(t,
		roster.Bot{Slug: "alpha", Name: "Alpha"},
		roster.Bot{Slug: "beta", Name: "Beta"},
	)
	svc := &Service{queries: q, thoughtsRoot: thoughtsRoot, roster: store}

	got, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "beta",
		FromAgentID: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RoomKind != RoomKindPairwise {
		t.Fatalf("kind = %q", got.RoomKind)
	}
	if got.SpeakerSlug != "beta" {
		t.Fatalf("speaker = %q, want dest bot", got.SpeakerSlug)
	}
	thread, err := q.GetAgentThread(ctx, got.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.RoomKind != RoomKindPairwise {
		t.Fatalf("thread kind = %q", thread.RoomKind)
	}

	again, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "beta",
		FromAgentID: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ThreadID != got.ThreadID {
		t.Fatalf("ensure pairwise reused %q vs %q", again.ThreadID, got.ThreadID)
	}

	reply, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:             "alpha",
		FromAgentID:    "beta",
		OriginThreadID: got.ThreadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ThreadID != got.ThreadID {
		t.Fatalf("same-pair reply thread = %q", reply.ThreadID)
	}
	if reply.SpeakerSlug != "alpha" {
		t.Fatalf("reply speaker = %q", reply.SpeakerSlug)
	}
}

func TestResolveMessageRoomPlanDirToPlanThread(t *testing.T) {
	t.Parallel()
	_, thoughtsRoot, _, planRel, svc, database := setupPlanDirRelTest(t)
	ctx := t.Context()
	store := seedTestRoster(t,
		roster.Bot{Slug: "lead", Name: "Lead"},
		roster.Bot{Slug: "courier", Name: "Courier"},
	)
	svc.roster = store
	if err := database.Queries.SetPlanWorkspaceLeadAgent(
		ctx,
		db.SetPlanWorkspaceLeadAgentParams{
			LeadAgentSlug: sql.NullString{String: "lead", Valid: true},
			PlanDirRel:    planRel,
		},
	); err != nil {
		t.Fatal(err)
	}
	_ = thoughtsRoot
	got, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "thoughts/owner/plans/alpha",
		FromAgentID: "courier",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RoomKind != RoomKindPlan {
		t.Fatalf("kind = %q", got.RoomKind)
	}
	if got.SpeakerSlug != "lead" {
		t.Fatalf("speaker = %q, want lead", got.SpeakerSlug)
	}
	thread, err := database.Queries.GetAgentThread(ctx, got.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.RoomKind != RoomKindPlan {
		t.Fatalf("thread kind = %q", thread.RoomKind)
	}
}

func TestResolveMessageRoomRejectsBotHomeAndUUID(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "msg-rej.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	store := seedTestRoster(t, roster.Bot{Slug: "alpha", Name: "Alpha"})
	if err := os.WriteFile(store.Path, []byte(
		"bots:\n"+
			"- slug: alpha\n  name: Alpha\n"+
			"- slug: gone\n  name: Gone\n  archived: true\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{queries: q, thoughtsRoot: t.TempDir(), roster: store}

	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "/rooms/dm/beta",
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomBotHome) {
		t.Fatalf("bot-home url: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "thoughts/agents/beta",
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomBotHome) {
		t.Fatalf("agents path: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          uuid.NewString(),
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomThreadUUID) {
		t.Fatalf("uuid: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "alpha",
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomSelf) {
		t.Fatalf("self: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "missing",
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomUnknownSlug) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "gone",
		FromAgentID: "alpha",
	}); !errors.Is(err, ErrMessageRoomArchivedSlug) {
		t.Fatalf("archived: %v", err)
	}
}
