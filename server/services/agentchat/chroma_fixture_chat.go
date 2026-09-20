package agentchat

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/a-h/templ"
)

const (
	chromaHighlightFixtureDOMID  = "ai470-chroma-highlight-fixture"
	groupQuoteFixtureDOMID       = "ai470-group-quote-fixture"
	groupPeerFixtureDOMID        = "ai470-group-peer-fixture"
	densityFixtureReasoningDOMID = "ai470-density-fixture-reasoning"
	densityFixtureTool0DOMID     = "ai470-density-fixture-tool-0"
	densityFixtureTool1DOMID     = "ai470-density-fixture-tool-1"
	pairwisePeerFixtureDOMID     = "ai470-pairwise-peer-fixture"
	pairwiseSelfFixtureDOMID     = "ai470-pairwise-self-fixture"
	historyFixtureDOMIDPrefix    = "ai470-history-fixture-"
	historyFixtureExtraMessages  = 5 // stableTranscriptInitialLimit+N so HasMoreOlder=true
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
	case "pairwise":
		// Distinctive A2A peer/self bubbles so UX/E2E can assert #msg-ai470-pairwise-*.
		stable = append(stable, s.pairwiseFixtureMessages()...)
		stable = append(stable, s.chromaHighlightFixtureMessage())
	case "density":
		stable = append(stable, s.densityFixtureMessages()...)
		stable = append(stable, s.chromaHighlightFixtureMessage())
	case "history":
		// Long wipe so applyStableTranscriptWindow yields SentinelAbove.
		stable = append(stable, s.historyFixtureMessages()...)
	}
	live, cursor := s.buildLiveTranscript(thread.ID)
	args := EmbeddedFreeformPanelArgs{
		ThreadID:  thread.ID,
		HasThread: true,
		Cwd:       thread.Cwd,
		Placeholder: sharedThreadComposerPlaceholder(EmbeddedFreeformPanelArgs{
			HasThread:   true,
			Placeholder: composerPlaceholderForTitle(thread.Title),
		}),
		Transcript: applyStableTranscriptWindow(TranscriptPaneState{
			Stable:      stable,
			Live:        live,
			Cursor:      cursor,
			Policy:      s.defaultTranscriptRenderPolicy(),
			ShowWorking: s.liveTranscriptShowWorking(thread.ID, live),
		}),
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
		// Plan rooms stay sendable without a roster-bound AgentID — plan-lead is
		// derived from plan_dir_rel (BE contract). Pairwise remains view-only.
		ComposerDisabled: fixtureMode == "pairwise" ||
			thread.RoomKind == RoomKindPairwise,
	}
	if family, ferr := s.BuildChatThreadFamily(ctx, thread.ID); ferr == nil {
		args.ThreadFamily = family
	}
	return SharedThreadChat(args), nil
}

// RenderSharedThreadChatWithPairwiseFixture is view-only pairwise chrome (no composer).
func (s *Service) RenderSharedThreadChatWithPairwiseFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "pairwise")
}

// RenderSharedThreadChatWithDensityFixture seeds reasoning/tool/subagent details
// plus a chroma markdown bubble so UX can re-VA chat density in dogfood.
func (s *Service) RenderSharedThreadChatWithDensityFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "density")
}

// RenderSharedThreadChatWithHistoryFixture seeds >stableTranscriptInitialLimit
// messages so InfiniteScroll paints #agent-chat-scroll-sentinel-above.
func (s *Service) RenderSharedThreadChatWithHistoryFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "history")
}

func groupBotDMChipFixture(originTurnID string) *BotDMChip {
	return &BotDMChip{
		OriginTurnID: originTurnID,
		SpeakerSlug:  "lead",
		MessageCount: 3,
		Bots: []BotDMChipPeer{
			{
				Name:          "Infra Engineer",
				Slug:          "infra",
				AuthorInitial: "I",
				AvatarBg:      "bg-teal-600",
				Count:         2,
			},
			{
				Name:          "Research",
				Slug:          "research",
				AuthorInitial: "R",
				AvatarBg:      "bg-violet-600",
				Count:         1,
			},
		},
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
	lead.BotDMChip = groupBotDMChipFixture(groupQuoteFixtureDOMID)

	user := s.newBubbleTranscriptMessage(
		"ai470-group-user-fixture",
		"ai470-group-user-fixture",
		"user",
		"Also bump chip vertical gap a touch if cheap — gap-3 → gap-4.",
		false,
	)
	return []TranscriptMessage{peer, lead, user}
}

func (s *Service) densityFixtureMessages() []TranscriptMessage {
	reasoning := TranscriptMessage{
		DOMID:                 densityFixtureReasoningDOMID,
		EntryID:               densityFixtureReasoningDOMID,
		Variant:               "detail",
		Title:                 "thinking",
		HeaderSummary:         "[14] plan the change",
		Content:               "> plan the change",
		HTMLContent:           "<blockquote><p>plan the change</p></blockquote>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}
	tool := TranscriptMessage{
		DOMID:                 densityFixtureTool0DOMID,
		EntryID:               densityFixtureTool0DOMID,
		Variant:               "detail",
		Title:                 "bash",
		ToolCallID:            "call_density_bash_1",
		HeaderCode:            "ls",
		Content:               "```json\n{\n  \"command\": \"ls\"\n}\n```",
		HTMLContent:           "<pre>ls</pre>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}
	subagent := TranscriptMessage{
		DOMID:                 densityFixtureTool1DOMID,
		EntryID:               densityFixtureTool1DOMID,
		Variant:               "detail",
		Title:                 "subagent",
		ToolCallID:            "call_density_sub_1",
		HeaderSummary:         "agent: worker · task: inspect",
		Content:               "```json\n{}\n```",
		HTMLContent:           "<pre>{}</pre>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}
	return []TranscriptMessage{reasoning, tool, subagent}
}

func (s *Service) pairwiseFixtureMessages() []TranscriptMessage {
	peer := s.newBubbleTranscriptMessage(
		pairwisePeerFixtureDOMID,
		pairwisePeerFixtureDOMID,
		"assistant",
		"Pairwise peer fixture (AI-470) — left agent in the A2A pair.",
		false,
	)
	peer.AuthorInitial = "P"
	peer.AuthorName = "Pair Peer"
	peer.AvatarBg = "bg-sky-600"
	peer.NameColor = "text-sky-300"

	self := s.newBubbleTranscriptMessage(
		pairwiseSelfFixtureDOMID,
		pairwiseSelfFixtureDOMID,
		"assistant",
		"Pairwise self fixture (AI-470) — pair partner, view-only composer.",
		false,
	)
	self.AuthorInitial = "S"
	self.AuthorName = "Pair Self"
	self.AvatarBg = "bg-rose-600"
	self.NameColor = "text-rose-300"
	return []TranscriptMessage{peer, self}
}

// historyFixtureMessages seeds stableTranscriptInitialLimit+N bubbles with stable
// DOMIDs so windowStableTranscript sets HasMoreOlder and OlderBefore.
func (s *Service) historyFixtureMessages() []TranscriptMessage {
	n := stableTranscriptInitialLimit + historyFixtureExtraMessages
	out := make([]TranscriptMessage, 0, n)
	for i := 0; i < n; i++ {
		domID := fmt.Sprintf("%s%d", historyFixtureDOMIDPrefix, i)
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgText := fmt.Sprintf(
			"History fixture message %d (AI-470 long transcript for InfiniteScroll).",
			i,
		)
		out = append(
			out,
			s.newBubbleTranscriptMessage(domID, domID, role, msgText, false),
		)
	}
	return out
}
