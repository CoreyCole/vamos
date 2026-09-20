package agentchat

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestFamilyThreadHrefRoomsVsSpawn(t *testing.T) {
	t.Parallel()

	homeBot := db.AgentThread{
		ID:        "home-bot",
		RoomKind:  RoomKindBotHome,
		AgentSlug: sql.NullString{String: "nova", Valid: true},
	}
	if got, want := familyThreadHref(homeBot), "/rooms/dm/nova"; got != want {
		t.Fatalf("bot home href = %q, want %q", got, want)
	}

	homePair := db.AgentThread{
		ID:             "home-pair",
		RoomKind:       RoomKindPairwise,
		PairAgentSlugA: sql.NullString{String: "research", Valid: true},
		PairAgentSlugB: sql.NullString{String: "lead", Valid: true},
	}
	if got, want := familyThreadHref(homePair), "/rooms/a2a/lead/research"; got != want {
		t.Fatalf("pairwise home href = %q, want %q", got, want)
	}

	homePlan := db.AgentThread{
		ID:         "home-plan",
		RoomKind:   RoomKindPlan,
		PlanDirRel: sql.NullString{String: "thoughts/owner/plans/alpha", Valid: true},
	}
	got := familyThreadHref(homePlan)
	if !strings.HasPrefix(got, "/rooms/plan/alpha") {
		t.Fatalf("plan home href = %q, want /rooms/plan/alpha...", got)
	}
	if !strings.Contains(got, "artifact=") {
		t.Fatalf("plan home href missing artifact: %q", got)
	}

	child := db.AgentThread{
		ID:             "child-1",
		RoomKind:       RoomKindBotHome,
		ParentThreadID: sql.NullString{String: homeBot.ID, Valid: true},
		AgentSlug:      sql.NullString{String: "nova", Valid: true},
	}
	if got, want := familyThreadHref(child), "/threads/child-1"; got != want {
		t.Fatalf("spawn child href = %q, want %q", got, want)
	}
	if strings.Contains(got, "subagent") {
		t.Fatal("href must not invent room_kind=subagent")
	}
}

func TestSharedThreadChatFamilyBarMorphMapAndComposerGate(t *testing.T) {
	t.Parallel()

	family := ChatThreadFamily{
		CurrentID:    "child-1",
		CurrentTitle: "Worker",
		Rows: []ChatThreadFamilyRow{
			{ID: "home-bot", Title: "Nova", Href: "/rooms/dm/nova", IsHome: true},
			{
				ID:       "child-1",
				Title:    "Worker",
				Href:     "/threads/child-1",
				IsActive: true,
				Status:   "ready",
			},
		},
	}
	html := testhelpers.RenderToDocument(t, SharedThreadChat(EmbeddedFreeformPanelArgs{
		ThreadID:         "child-1",
		HasThread:        true,
		ComposerDisabled: true,
		ThreadFamily:     family,
	})).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="agent-chat-scroll-region"`,
		`id="agent-chat-stable-transcript"`,
		`id="agent-chat-message-thread"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		`id="agent-chat-thread-family"`,
		`data-testid="agent-chat-thread-family"`,
		`>Threads</span>`,
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("parked family bar leaked %q in:\n%s", unwanted, out)
		}
	}
	if strings.Contains(out, `id="agent-chat-composer-form"`) {
		t.Fatal("pairwise/composer-disabled chat must omit composer")
	}
}
