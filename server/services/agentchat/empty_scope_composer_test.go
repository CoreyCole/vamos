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
		"plan",
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
	for _, unwanted := range []string{`aria-label="Add"`, `title="Add"`, "Freeform chat"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("empty-scope composer still has %q: %s", unwanted, html)
		}
	}
	if !strings.Contains(html, ">plan<") {
		t.Fatalf("plan N=0 Mode must be plan: %s", html)
	}
}


func TestEmptyScopeComposerDocsDeskModeIsDocs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := EmptyScopeComposer(
		"@post('/rooms/plan/docs--vamos/threads', {contentType: 'form'})",
		"thoughts/docs/vamos/index.html",
		"docs",
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "Freeform chat") {
		t.Fatalf("docs desk Mode must not be Freeform chat: %s", html)
	}
	if !strings.Contains(html, ">docs<") {
		t.Fatalf("docs desk Mode must be docs: %s", html)
	}
}

func TestEmptyScopeComposerAgentModeIsAgent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := EmptyScopeComposer(
		"@post('/rooms/dm/research/threads', {contentType: 'form'})",
		"",
		"agent",
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "Freeform chat") {
		t.Fatalf("bot DM Mode must not be Freeform chat: %s", html)
	}
	if !strings.Contains(html, ">agent<") {
		t.Fatalf("bot DM Mode must be agent: %s", html)
	}
}

func TestEmptyScopeModeLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action, doc, want string
	}{
		{
			"@post('/rooms/plan/real-plan/threads', {contentType: 'form'})",
			"thoughts/creative-mode-agent/plans/real-plan/AGENTS.md",
			"plan",
		},
		{
			"@post('/rooms/plan/docs--vamos/threads', {contentType: 'form'})",
			"thoughts/docs/vamos/index.html",
			"docs",
		},
		{
			"@post('/rooms/dm/research/threads', {contentType: 'form'})",
			"",
			"agent",
		},
		{
			"@post('/rooms/freeform/threads', {contentType: 'form'})",
			"",
			"freeform",
		},
	}
	for _, tc := range cases {
		if got := emptyScopeModeLabel(tc.action, tc.doc); got != tc.want {
			t.Fatalf("mode(%q, %q) = %q, want %q", tc.action, tc.doc, got, tc.want)
		}
	}
}
