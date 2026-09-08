package agentchat

import (
	"context"
	"net/url"
	"strings"

	"github.com/a-h/templ"
)

const (
	chromaHighlightFixtureDOMID = "ai470-chroma-highlight-fixture"
	groupQuoteFixtureDOMID      = "ai470-group-quote-fixture"
	groupPeerFixtureDOMID       = "ai470-group-peer-fixture"
)

// chromaHighlightFixtureMarkdown is a visible assistant bubble for UX chroma VA.
const chromaHighlightFixtureMarkdown = "Syntax-highlight smoke (AI-470):\n\n```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"chroma\")\n}\n```\n"

const (
	groupPeerFixtureMarkdown = "Feel ask: Grok Bot–like bubbles — left agents with colored avatar+name, right user with no header."
	groupLeadFixtureMarkdown = "Greenlit. Prefer HTML in `RoomChatPane`; reuse agentchat only if clean."
)

// RenderSharedThreadChatWithChromaFixture is SharedThreadChat plus a fenced ```go
// assistant bubble so converge VA can smoke chroma in-workbench.
func (s *Service) RenderSharedThreadChatWithChromaFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "dm-bot")
}

// RenderSharedThreadChatWithGroupBubbleFixture seeds multi-author bubbles + NestedQuoteBlock.
func (s *Service) RenderSharedThreadChatWithGroupBubbleFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "group")
}

func (s *Service) renderSharedThreadChat(
	ctx context.Context,
	threadID, userEmail string,
	fixtureMode string,
) (templ.Component, error) {
	thread, err := s.queries.GetSharedAgentThread(ctx, strings.TrimSpace(threadID))
	if err != nil {
		return nil, err
	}
	draft, err := s.GetThreadDraft(ctx, userEmail, thread.ID)
	if err != nil {
		return nil, err
	}
	stable, err := s.buildStableTranscript(ctx, thread)
	if err != nil {
		return nil, err
	}
	switch fixtureMode {
	case "dm-bot":
		stable = append(stable, s.chromaHighlightFixtureMessage())
	case "group":
		stable = append(stable, s.groupBubbleFixtureMessages()...)
		stable = append(stable, s.chromaHighlightFixtureMessage())
	}
	live, cursor := s.buildLiveTranscript(thread.ID)
	args := EmbeddedFreeformPanelArgs{
		ThreadID:    thread.ID,
		HasThread:   true,
		Cwd:         thread.Cwd,
		Placeholder: leftoverComposerPlaceholder(fixtureMode),
		Transcript: TranscriptPaneState{
			Stable: stable,
			Live:   live,
			Cursor: cursor,
			Policy: s.defaultTranscriptRenderPolicy(),
		},
		ComposerAction: "@post('" + thoughtsThreadChatAction(
			thread.ID,
			"resume",
		) + "?workbench_v2=1', {contentType: 'form'})",
		StreamURL: thoughtsThreadChatAction(
			thread.ID,
			"stream",
		) + "?since=0&workbench_v2=1",
		InitialDraft: draft,
		DraftSaveAction: "@post('/agent-chat/thread/" + url.PathEscape(
			thread.ID,
		) + "/draft', {filterSignals: {include: /^chatDraft$/}})",
	}
	return SharedThreadChat(args), nil
}

func leftoverComposerPlaceholder(fixtureMode string) string {
	switch fixtureMode {
	case "group":
		return "Message Vamos dev"
	default:
		return "Message Bot"
	}
}

func (s *Service) chromaHighlightFixtureMessage() TranscriptMessage {
	msg := s.newBubbleTranscriptMessage(
		chromaHighlightFixtureDOMID,
		chromaHighlightFixtureDOMID,
		"assistant",
		chromaHighlightFixtureMarkdown,
		false,
	)
	return withDefaultAssistantBubbleChrome(msg)
}

func (s *Service) groupBubbleFixtureMessages() []TranscriptMessage {
	peer := s.newBubbleTranscriptMessage(
		groupPeerFixtureDOMID,
		groupPeerFixtureDOMID,
		"assistant",
		groupPeerFixtureMarkdown,
		false,
	)
	peer.AuthorInitial = "F"
	peer.AuthorName = "Chestnut FE Engineer"
	peer.AvatarBg = "bg-amber-700"
	peer.NameColor = "text-amber-300"

	lead := s.newBubbleTranscriptMessage(
		groupQuoteFixtureDOMID,
		groupQuoteFixtureDOMID,
		"assistant",
		groupLeadFixtureMarkdown,
		false,
	)
	lead.AuthorInitial = "L"
	lead.AuthorName = "Chestnut Lead Engineer"
	lead.AvatarBg = "bg-emerald-600"
	lead.NameColor = "text-emerald-300"
	lead.QuoteInitial = "I"
	lead.QuoteName = "Chestnut Infra Engineer"
	lead.QuoteAvatarBg = "bg-teal-600"
	lead.QuoteNameColor = "text-teal-300"
	lead.QuoteText = "Host slug ai470-leftover-converge is up; rebuild after tip."

	user := s.newBubbleTranscriptMessage(
		"ai470-group-user-fixture",
		"ai470-group-user-fixture",
		"user",
		"Also bump chip vertical gap a touch if cheap — gap-3 → gap-4.",
		false,
	)
	return []TranscriptMessage{peer, lead, user}
}
