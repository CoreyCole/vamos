package agentchat

import (
	"context"
	"net/url"
	"strings"

	"github.com/a-h/templ"
)

const chromaHighlightFixtureDOMID = "ai470-chroma-highlight-fixture"

// chromaHighlightFixtureMarkdown is a visible assistant bubble for UX chroma VA.
const chromaHighlightFixtureMarkdown = "Syntax-highlight smoke (AI-470):\n\n```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"chroma\")\n}\n```\n"

// RenderSharedThreadChatWithChromaFixture is SharedThreadChat plus a fenced ```go
// assistant bubble so converge VA can smoke chroma in-workbench.
func (s *Service) RenderSharedThreadChatWithChromaFixture(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, true)
}

func (s *Service) renderSharedThreadChat(
	ctx context.Context,
	threadID, userEmail string,
	withChromaFixture bool,
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
	if withChromaFixture {
		stable = append(stable, s.newBubbleTranscriptMessage(
			chromaHighlightFixtureDOMID,
			chromaHighlightFixtureDOMID,
			"assistant",
			chromaHighlightFixtureMarkdown,
			false,
		))
	}
	live, cursor := s.buildLiveTranscript(thread.ID)
	args := EmbeddedFreeformPanelArgs{
		ThreadID:  thread.ID,
		HasThread: true,
		Cwd:       thread.Cwd,
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
