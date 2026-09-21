package agentchat

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEmptyScopeComposerUsesAgentChatComposer(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := EmptyScopeComposer(
		"@post('/rooms/plan/real-plan/threads', {contentType: 'form'})",
		"thoughts/creative-mode-agent/plans/real-plan/AGENTS.md",
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`id="agent-chat-composer"`,
		`data-composer-shell`,
		`id="agent-chat-composer-metadata-content"`,
		`aria-label="Send message"`,
		`evt.key === 'Enter' && !evt.shiftKey`,
		`el.form.requestSubmit()`,
		`name="attached_paths[]"`,
		`AGENTS.md`,
		`name="artifact"`,
		`aria-label="Info"`,
		`title="Info"`,
		`Attached files`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("empty-scope composer missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{`aria-label="Add"`, `title="Add"`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("empty-scope composer still has %q: %s", unwanted, html)
		}
	}
}
