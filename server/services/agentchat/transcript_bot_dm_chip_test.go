package agentchat

import (
	"strings"
	"testing"

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
