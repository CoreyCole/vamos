package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestBotDMChipPopoverCopyAndPairwiseHrefs(t *testing.T) {
	t.Parallel()

	chip := groupBotDMChipFixture("turn-1")
	html := testhelpers.RenderToDocument(t, BotDMChipPopover(*chip)).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, "3 messages with 2 bots") {
		t.Fatalf("missing chip copy; html = %s", out)
	}
	if !strings.Contains(out, `id="bot-dm-chip-turn-1"`) {
		t.Fatalf("missing stable chip id; html = %s", out)
	}
	for _, want := range []string{
		"Infra Engineer",
		"infra",
		"Research",
		"research",
		`href="/rooms/a2a/infra/lead"`,
		`href="/rooms/a2a/lead/research"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q; html = %s", want, out)
		}
	}
	if strings.Contains(out, "/rooms/agent_dm/") {
		t.Fatalf("used leftover agent_dm URL; html = %s", out)
	}
}

func TestPairwiseSharedThreadChatHasNoComposer(t *testing.T) {
	t.Parallel()

	html := testhelpers.RenderToDocument(t, SharedThreadChat(EmbeddedFreeformPanelArgs{
		ThreadID:         "pair-thread",
		HasThread:        true,
		ComposerDisabled: true,
		Placeholder:      "should not render",
	})).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if strings.Contains(out, `id="agent-chat-composer-form"`) {
		t.Fatalf("pairwise markup has composer; html = %s", out)
	}
	if strings.Contains(out, "should not render") {
		t.Fatalf("pairwise markup leaked placeholder; html = %s", out)
	}
}

func TestPairwiseA2AHrefCanonicalOrder(t *testing.T) {
	t.Parallel()
	if got, want := pairwiseA2AHref(
		"research",
		"lead",
	), "/rooms/a2a/lead/research"; got != want {
		t.Fatalf("pairwiseA2AHref = %q, want %q", got, want)
	}
}

func TestLiveBotDMChipCountsOriginCallAndDestReply(t *testing.T) {
	t.Parallel()

	origin := "turn-origin-1"
	speaker := "lead"
	inboundAndReply := []TranscriptMessage{
		{Variant: "bubble", Role: "user", Content: "hello from lead"},
		{Variant: "bubble", Role: "assistant", Content: "reply from infra"},
	}
	first := deriveLiveBotDMChip(origin, speaker, []pairwiseWindowCount{{
		DestSlug:        "infra",
		DestName:        "Infra Engineer",
		VisibleMessages: countVisiblePairwiseBubbles(inboundAndReply),
	}})
	if first == nil {
		t.Fatal("expected chip")
	}
	if first.MessageCount != 2 {
		t.Fatalf("N = %d, want 2 (inbound + dest reply)", first.MessageCount)
	}
	if len(first.Bots) != 1 {
		t.Fatalf("M = %d, want 1", len(first.Bots))
	}
	if first.Bots[0].Count != 2 {
		t.Fatalf("peer count = %d, want 2", first.Bots[0].Count)
	}

	afterMore := append(
		append([]TranscriptMessage{}, inboundAndReply...),
		TranscriptMessage{
			Variant: "bubble", Role: "assistant", Content: "still talking",
		},
	)
	second := deriveLiveBotDMChip(origin, speaker, []pairwiseWindowCount{{
		DestSlug:        "infra",
		DestName:        "Infra Engineer",
		VisibleMessages: countVisiblePairwiseBubbles(afterMore),
	}})
	if second == nil {
		t.Fatal("expected updated chip")
	}
	if second.MessageCount != 3 {
		t.Fatalf(
			"live N = %d, want 3 (do not freeze at tool return)",
			second.MessageCount,
		)
	}
	if botDMChipID(*first) != botDMChipID(*second) {
		t.Fatalf(
			"chip id changed across SSE update: %q vs %q",
			botDMChipID(*first),
			botDMChipID(*second),
		)
	}
	if botDMChipID(*first) != "bot-dm-chip-turn-origin-1" {
		t.Fatalf("chip id = %q", botDMChipID(*first))
	}

	items := []TranscriptMessage{
		{
			DOMID:      "live-" + origin,
			EntryID:    origin,
			Variant:    "bubble",
			Role:       "assistant",
			Content:    "I'll ping infra",
			AuthorName: speaker,
		},
		{
			EntryID:    origin,
			Variant:    "detail",
			Title:      "message_room",
			HeaderCode: "infra",
		},
	}
	items = attachBotDMChipToOriginTurn(items, second)
	if items[0].BotDMChip == nil {
		t.Fatal("chip not attached to origin assistant bubble")
	}
	html := testhelpers.RenderToDocument(t, BotDMChipPopover(*items[0].BotDMChip)).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, "3 messages with 1 bots") {
		t.Fatalf("missing live copy; html = %s", out)
	}
	if !strings.Contains(out, `id="bot-dm-chip-turn-origin-1"`) {
		t.Fatalf("missing stable chip id; html = %s", out)
	}
}

func TestCompactToolCallPresentationMessageRoomShowsArgs(t *testing.T) {
	t.Parallel()
	s := &Service{}
	title, headerCode, _, hide := s.compactToolCallPresentation(
		"message_room",
		map[string]any{"to": "infra"},
	)
	if title != "message_room" {
		t.Fatalf("title = %q", title)
	}
	if headerCode != "infra" {
		t.Fatalf("headerCode = %q, want infra (args not silent)", headerCode)
	}
	if !hide {
		t.Fatal("expected collapsed chrome with visible header args")
	}
}

func TestAttachDerivedBotDMChipsReadsJSONLAndSurvivesRotate(t *testing.T) {
	t.Parallel()
	thoughtsRoot := t.TempDir()
	id := RoomIdentity{Kind: RoomKindPairwise, PairA: "lead", PairB: "infra"}
	currentAbs, err := EnsureRoomCurrentJSONL(thoughtsRoot, id)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"message","message":{"role":"user","content":"ping"}}`,
		`{"type":"message","message":{"role":"assistant","content":"pong"}}`,
		`{"type":"message","message":{"role":"assistant","content":"more"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(currentAbs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	home := db.AgentThread{ID: "home-lead", RoomKind: RoomKindBotHome}
	svc := &Service{thoughtsRoot: thoughtsRoot}
	items := []TranscriptMessage{
		{
			EntryID:    "turn-origin-1",
			Variant:    "bubble",
			Role:       "assistant",
			AuthorName: "lead",
			Content:    "I'll ping infra",
		},
		{
			EntryID:    "turn-origin-1",
			Variant:    "detail",
			Title:      "message_room",
			HeaderCode: "infra",
		},
	}
	got := svc.attachDerivedBotDMChips(home, items)
	if got[0].BotDMChip == nil {
		t.Fatal("chip not attached from JSONL")
	}
	if got[0].BotDMChip.MessageCount != 3 {
		t.Fatalf("N = %d, want 3", got[0].BotDMChip.MessageCount)
	}

	rotated, err := RotateRoomCurrentJSONL(thoughtsRoot, id, RoomHandoffSpec{
		Timestamp: "2026-09-10_08-00-00",
		Body:      "idle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.HistoryRel == "" {
		t.Fatal("expected history file")
	}
	after := svc.attachDerivedBotDMChips(home, items)
	if after[0].BotDMChip == nil {
		t.Fatal("chip missing after rotate")
	}
	if after[0].BotDMChip.MessageCount != 3 {
		t.Fatalf(
			"post-rotate N = %d, want 3 (do not recount empty current)",
			after[0].BotDMChip.MessageCount,
		)
	}
}

func TestPairwiseLiveEventNotifiesOriginHomeThread(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(
		filepath.Join(t.TempDir(), "chip-notify.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	home, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-home-lead",
		UserEmail: "owner@example.com",
		Title:     "Lead home",
		Cwd:       "thoughts/agents/lead",
		LineageID: "lin-home",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, db.BindAgentThreadBotHomeParams{
		AgentSlug: sql.NullString{String: "lead", Valid: true},
		Cwd:       home.Cwd,
		Title:     home.Title,
		ID:        home.ID,
	}); err != nil {
		t.Fatal(err)
	}
	pair, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-pair",
		UserEmail: "owner@example.com",
		Title:     "a2a",
		Cwd:       "thoughts/a2a/infra__lead",
		LineageID: "lin-pair",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadPairwise(ctx, db.BindAgentThreadPairwiseParams{
		PairAgentSlugA: sql.NullString{String: "infra", Valid: true},
		PairAgentSlugB: sql.NullString{String: "lead", Valid: true},
		Cwd:            pair.Cwd,
		Title:          pair.Title,
		ID:             pair.ID,
	}); err != nil {
		t.Fatal(err)
	}

	notifier := NewNotifier()
	originCh := notifier.Subscribe(home.ID)
	defer notifier.Unsubscribe(home.ID, originCh)
	svc := &Service{queries: q, notifier: notifier}
	svc.notifyPairwiseOriginTranscripts(ctx, "", pair.ID)

	select {
	case signal := <-originCh:
		if signal.Scope != PatchLiveTranscript {
			t.Fatalf("scope = %q", signal.Scope)
		}
	default:
		t.Fatal("pairwise live event did not dirty origin home thread")
	}
}
