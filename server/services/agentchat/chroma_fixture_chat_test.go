package agentchat

import (
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
