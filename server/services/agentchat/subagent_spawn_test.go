package agentchat

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func TestSpawnSubagentInheritsParentEmailAndRoomKind(t *testing.T) {
	t.Parallel()
	service, queries := newThreadDraftService(t)
	ctx := t.Context()

	parent, err := queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "parent-plan",
		UserEmail: "owner@example.com",
		Title:     "Plan room",
		Cwd:       "thoughts/owner/plans/alpha",
		LineageID: "lin-parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.BindAgentThreadPlan(ctx, db.BindAgentThreadPlanParams{
		AgentID: sql.NullString{},
		Cwd:     parent.Cwd,
		Title:   parent.Title,
		ID:      parent.ID,
	}); err != nil {
		t.Fatal(err)
	}

	child, err := service.SpawnSubagent(ctx, SpawnSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: parent.ID,
		Title:          "worker-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !child.ParentThreadID.Valid || child.ParentThreadID.String != parent.ID {
		t.Fatalf("parent_thread_id = %#v, want %q", child.ParentThreadID, parent.ID)
	}
	if child.UserEmail != "owner@example.com" {
		t.Fatalf("user_email = %q", child.UserEmail)
	}
	if child.RoomKind != RoomKindPlan {
		t.Fatalf("room_kind = %q, want plan (not subagent)", child.RoomKind)
	}
	if child.RoomKind == "subagent" {
		t.Fatal("room_kind=subagent must not be written")
	}

	// Non-owner opens via inherited shared room_kind.
	got, err := queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        child.ID,
		UserEmail: "viewer@example.com",
	})
	if err != nil {
		t.Fatalf("Open /threads/{child} 404 for inherited room: %v", err)
	}
	if got.ID != child.ID {
		t.Fatalf("opened %q", got.ID)
	}
}

func TestAttachSubagentUpsertsSessionArtifactAndProjectedThread(t *testing.T) {
	t.Parallel()
	service, queries := newThreadDraftService(t)
	ctx := t.Context()

	parent, err := queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "parent-bot",
		UserEmail: "owner@example.com",
		Title:     "Nova",
		Cwd:       "thoughts/agents/nova",
		LineageID: "lin-bot",
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := queries.CreateAgent(ctx, db.CreateAgentParams{
		ID: "agent-nova", Slug: "nova", Name: "Nova",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.BindAgentThreadBotHome(ctx, db.BindAgentThreadBotHomeParams{
		AgentID: sql.NullString{String: agent.ID, Valid: true},
		Cwd:     parent.Cwd,
		Title:   parent.Title,
		ID:      parent.ID,
	}); err != nil {
		t.Fatal(err)
	}

	child, err := service.SpawnSubagent(ctx, SpawnSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: parent.ID,
		Title:          "attach-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.RoomKind != RoomKindBotHome {
		t.Fatalf("inherited room_kind = %q, want bot_home", child.RoomKind)
	}

	artifact := "thoughts/agents/nova/sessions/handoffs/2026-09-12_worker.jsonl"
	session, err := service.AttachSubagentSession(ctx, AttachSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: parent.ID,
		ChildThreadID:  child.ID,
		ArtifactPath:   artifact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !session.ArtifactPath.Valid || session.ArtifactPath.String != artifact {
		t.Fatalf("artifact_path = %#v", session.ArtifactPath)
	}
	if !session.ProjectedThreadID.Valid || session.ProjectedThreadID.String != child.ID {
		t.Fatalf("projected_thread_id = %#v", session.ProjectedThreadID)
	}

	again, err := service.AttachSubagentSession(ctx, AttachSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: parent.ID,
		ChildThreadID:  child.ID,
		ArtifactPath:   artifact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != session.ID {
		t.Fatalf("upsert changed id %q -> %q", session.ID, again.ID)
	}
	if again.ProjectedThreadID.String != child.ID {
		t.Fatalf("projected_thread_id after upsert = %#v", again.ProjectedThreadID)
	}
}

func TestBuildLiveTranscriptStateIncludesSubagentCards(t *testing.T) {
	t.Parallel()
	service, queries := newThreadDraftService(t)
	ctx := t.Context()
	createDraftThread(t, queries, "parent-live")

	child, err := service.SpawnSubagent(ctx, SpawnSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: "parent-live",
		Title:          "card-child",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := service.BuildLiveTranscriptState(ctx, "owner@example.com", "parent-live")
	if err != nil {
		t.Fatal(err)
	}
	wantID := subagentCardDOMID(child.ID)
	found := false
	for _, item := range state.Live.Items {
		if item.DOMID == wantID {
			found = true
			if item.Variant != subagentCardVariant {
				t.Fatalf("variant = %q", item.Variant)
			}
			if item.HeaderHref != "/threads/"+child.ID {
				t.Fatalf("Open href = %q", item.HeaderHref)
			}
			if item.HeaderCode != "Open" {
				t.Fatalf("Open label = %q", item.HeaderCode)
			}
			if item.BotDMChip != nil {
				t.Fatal("BotDMChip must stay unused for child cards")
			}
			break
		}
	}
	if !found {
		t.Fatalf("live state missing %s in %#v", wantID, state.Live.Items)
	}

	var buf strings.Builder
	if err := LiveTranscriptRegion("parent-live", state, "").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, wantID) {
		t.Fatalf("live HTML missing card id: %s", html)
	}
	if !strings.Contains(html, "/threads/"+child.ID) {
		t.Fatalf("live HTML missing Open href: %s", html)
	}
	if strings.Contains(html, "bot-dm-chip") {
		t.Fatalf("BotDMChip leaked into subagent card render: %s", html)
	}
}

func TestSpawnDoesNotWriteRoomKindSubagent(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "no-subagent-kind.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &Service{db: database.DB(), queries: database.Queries}
	createDraftThread(t, database.Queries, "parent-plain")

	child, err := service.SpawnSubagent(t.Context(), SpawnSubagentInput{
		UserEmail:      "owner@example.com",
		ParentThreadID: "parent-plain",
		Title:          "plain-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.RoomKind == "subagent" {
		t.Fatal("room_kind=subagent written")
	}
	// Attempting to set subagent via SetAgentThreadRoomKind must fail CHECK.
	err = database.Queries.SetAgentThreadRoomKind(t.Context(), db.SetAgentThreadRoomKindParams{
		RoomKind: "subagent",
		ID:       child.ID,
	})
	if err == nil {
		t.Fatal("expected CHECK to reject room_kind=subagent")
	}
}
