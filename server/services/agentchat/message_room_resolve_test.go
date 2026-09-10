package agentchat

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func TestResolveMessageRoomSlugToPairwise(t *testing.T) {
	t.Parallel()
	thoughtsRoot := t.TempDir()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "msg-room.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	alpha, err := q.CreateAgent(ctx, db.CreateAgentParams{
		ID: "id-alpha", Slug: "alpha", Name: "Alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := q.CreateAgent(ctx, db.CreateAgentParams{
		ID: "id-beta", Slug: "beta", Name: "Beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{queries: q, thoughtsRoot: thoughtsRoot}

	got, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "beta",
		FromAgentID: alpha.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RoomKind != RoomKindPairwise {
		t.Fatalf("kind = %q", got.RoomKind)
	}
	if got.SpeakerAgentID != beta.ID {
		t.Fatalf("speaker = %q, want dest bot", got.SpeakerAgentID)
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
		FromAgentID: alpha.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ThreadID != got.ThreadID {
		t.Fatalf("ensure pairwise reused %q vs %q", again.ThreadID, got.ThreadID)
	}

	reply, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:             "alpha",
		FromAgentID:    beta.ID,
		OriginThreadID: got.ThreadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ThreadID != got.ThreadID {
		t.Fatalf("same-pair reply thread = %q", reply.ThreadID)
	}
	if reply.SpeakerAgentID != alpha.ID {
		t.Fatalf("reply speaker = %q", reply.SpeakerAgentID)
	}
}

func TestResolveMessageRoomPlanDirToPlanThread(t *testing.T) {
	t.Parallel()
	_, thoughtsRoot, _, planRel, svc, database := setupPlanDirRelTest(t)
	ctx := t.Context()
	lead, err := database.Queries.CreateAgent(ctx, db.CreateAgentParams{
		ID: "lead-1", Slug: "lead", Name: "Lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Queries.SetPlanWorkspaceLeadAgent(
		ctx,
		db.SetPlanWorkspaceLeadAgentParams{
			LeadAgentID: sql.NullString{String: lead.ID, Valid: true},
			PlanDirRel:  planRel,
		},
	); err != nil {
		t.Fatal(err)
	}
	from, err := database.Queries.CreateAgent(ctx, db.CreateAgentParams{
		ID: "from-1", Slug: "courier", Name: "Courier",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = thoughtsRoot
	got, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "thoughts/owner/plans/alpha",
		FromAgentID: from.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RoomKind != RoomKindPlan {
		t.Fatalf("kind = %q", got.RoomKind)
	}
	if got.SpeakerAgentID != lead.ID {
		t.Fatalf("speaker = %q, want lead", got.SpeakerAgentID)
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
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "msg-rej.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	alpha, err := q.CreateAgent(ctx, db.CreateAgentParams{
		ID: "id-a", Slug: "alpha", Name: "Alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{queries: q, thoughtsRoot: t.TempDir()}

	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "/rooms/dm/beta",
		FromAgentID: alpha.ID,
	}); !errors.Is(err, ErrMessageRoomBotHome) {
		t.Fatalf("bot-home url: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "thoughts/agents/beta",
		FromAgentID: alpha.ID,
	}); !errors.Is(err, ErrMessageRoomBotHome) {
		t.Fatalf("agents path: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          uuid.NewString(),
		FromAgentID: alpha.ID,
	}); !errors.Is(err, ErrMessageRoomThreadUUID) {
		t.Fatalf("uuid: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "alpha",
		FromAgentID: alpha.ID,
	}); !errors.Is(err, ErrMessageRoomSelf) {
		t.Fatalf("self: %v", err)
	}
	if _, err := svc.ResolveMessageRoom(ctx, MessageRoomResolveInput{
		To:          "missing",
		FromAgentID: alpha.ID,
	}); !errors.Is(err, ErrMessageRoomUnknownSlug) {
		t.Fatalf("unknown: %v", err)
	}
}
