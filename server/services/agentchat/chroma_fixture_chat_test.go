package agentchat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

// Mirrors RenderSharedThreadChatWithPairwiseFixture seed + ComposerDisabled
// without a DB: pairwise DOM anchors must land as #msg-ai470-pairwise-* and
// the composer must stay absent (view-only affordance).
func TestRenderSharedThreadChatWithPairwiseFixtureDOMAnchors(t *testing.T) {
	t.Parallel()

	r, err := markdown.NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	s := &Service{renderer: r}
	stable := s.pairwiseFixtureMessages()
	stable = append(stable, s.chromaHighlightFixtureMessage())
	html := testhelpers.RenderToDocument(t, SharedThreadChat(EmbeddedFreeformPanelArgs{
		ThreadID:         "pair-fixture-thread",
		HasThread:        true,
		ComposerDisabled: true,
		Placeholder:      "should not render",
		Transcript: applyStableTranscriptWindow(TranscriptPaneState{
			Stable: stable,
			Policy: s.defaultTranscriptRenderPolicy(),
		}),
	})).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="msg-` + pairwisePeerFixtureDOMID + `"`,
		`id="msg-` + pairwiseSelfFixtureDOMID + `"`,
		`id="msg-` + chromaHighlightFixtureDOMID + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pairwise fixture missing %s; html = %s", want, out)
		}
	}
	if strings.Contains(out, `id="agent-chat-composer-form"`) {
		t.Fatalf("pairwise fixture has composer; html = %s", out)
	}
	if strings.Contains(out, "should not render") {
		t.Fatalf("pairwise fixture leaked placeholder; html = %s", out)
	}
}

// History fixture must exceed the stable window so MessagesPane paints
// #agent-chat-scroll-sentinel-above with history?before= on PatchAboveExpr.
func TestRenderSharedThreadChatWithHistoryFixtureSentinelAbove(t *testing.T) {
	t.Parallel()

	r, err := markdown.NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	s := &Service{renderer: r}
	stable := s.historyFixtureMessages()
	if len(stable) <= stableTranscriptInitialLimit {
		t.Fatalf("history fixture len=%d want >%d", len(stable), stableTranscriptInitialLimit)
	}
	state := applyStableTranscriptWindow(TranscriptPaneState{
		Stable: stable,
		Policy: s.defaultTranscriptRenderPolicy(),
	})
	if !state.HasMoreOlder {
		t.Fatal("expected HasMoreOlder after history fixture window")
	}
	wantBefore := fmt.Sprintf("%s%d", historyFixtureDOMIDPrefix, historyFixtureExtraMessages)
	if state.OlderBefore != wantBefore {
		t.Fatalf("OlderBefore=%q want %q", state.OlderBefore, wantBefore)
	}
	lastID := fmt.Sprintf("%s%d", historyFixtureDOMIDPrefix, len(stable)-1)
	foundLast := false
	for _, msg := range state.Stable {
		if msg.DOMID == lastID {
			foundLast = true
			break
		}
	}
	if !foundLast {
		t.Fatalf("windowed page missing newest %q", lastID)
	}

	html := renderComponentString(t, MessagesPane("history-fixture-thread", state, true, "", true))
	for _, want := range []string{
		`id="agent-chat-scroll-sentinel-above"`,
		`/agent-chat/thread/history-fixture-thread/history?before=` + wantBefore,
		`id="msg-` + lastID + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("history fixture missing %q in:\n%s", want, html)
		}
	}
	firstID := historyFixtureDOMIDPrefix + "0"
	if strings.Contains(html, `id="msg-`+firstID+`"`) {
		t.Fatalf("history fixture first-paint still has oldest %s", firstID)
	}
}
