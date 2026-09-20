package agentchat

import (
	"strings"
	"testing"
)

func TestFormatMessageFrontmatterRoundTrip(t *testing.T) {
	fields := []messageFrontmatterField{
		{Key: " process_cwd ", Value: " /tmp/workspace "},
		{Key: "plan_dir", Value: "owner/plans/alpha"},
		{Key: "impl_dir", Value: "   "},
	}
	body := "Please implement the groups.\n"
	got := formatMessageFrontmatter(fields, body)
	want := "process_cwd: /tmp/workspace\nplan_dir: owner/plans/alpha\n---\nPlease implement the groups.\n"
	if got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
	parsed, parsedBody, ok := parseMessageFrontmatter(got)
	if !ok {
		t.Fatal("expected frontmatter")
	}
	if parsedBody != body {
		t.Fatalf("body = %q", parsedBody)
	}
	if len(parsed) != 2 || parsed[0].Key != "process_cwd" || parsed[0].Value != "/tmp/workspace" ||
		parsed[1].Key != "plan_dir" ||
		parsed[1].Value != "owner/plans/alpha" {
		t.Fatalf("fields = %#v", parsed)
	}
}

func TestFormatMessageFrontmatterEmptyFields(t *testing.T) {
	body := "plain prompt"
	got := formatMessageFrontmatter(nil, body)
	if got != body {
		t.Fatalf("got %q", got)
	}
	if _, _, ok := parseMessageFrontmatter(got); ok {
		t.Fatal("empty fields must fail-closed")
	}
}

func TestParseMessageFrontmatter(t *testing.T) {
	content := "process_cwd: /home/ruby/cn/chestnut-flake/vamos\nplan_dir: CoreyCole/plans/docs--vamos\n---\nPlease implement the groups.\n"
	fields, body, ok := parseMessageFrontmatter(content)
	if !ok {
		t.Fatal("expected frontmatter")
	}
	if body != "Please implement the groups.\n" {
		t.Fatalf("body = %q", body)
	}
	if len(fields) != 2 || fields[0].Key != "process_cwd" || fields[1].Key != "plan_dir" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestParseMessageFrontmatterFailClosed(t *testing.T) {
	cases := []string{
		"process_cwd: /tmp\nno closer",
		"hello world\n---\nbody",
		"---\n---\nbody",
		"not yaml at all",
	}
	for _, content := range cases {
		if _, _, ok := parseMessageFrontmatter(content); ok {
			t.Fatalf("expected fail-closed for %q", content)
		}
	}
}

func TestUserBubbleFrontmatterPillsNeverFlashYAML(t *testing.T) {
	msg := TranscriptMessage{
		DOMID:   "u1",
		Role:    "user",
		Variant: "bubble",
		Content: "process_cwd: /tmp/workspace\nplan_dir: owner/plans/alpha\n---\nShip the pills.",
	}
	var b strings.Builder
	if err := TranscriptMessageWithFork(
		"t1",
		msg,
		"",
	).Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if !strings.Contains(html, `id="msg-u1-frontmatter"`) {
		t.Fatalf("missing pills: %s", html)
	}
	if !strings.Contains(html, "process_cwd") || !strings.Contains(html, "plan_dir") {
		t.Fatalf("missing keys: %s", html)
	}
	bodyStart := strings.Index(html, `id="msg-u1-frontmatter"`)
	bodyEnd := strings.Index(html, `id="msg-u1-menu"`)
	if bodyStart < 0 || bodyEnd <= bodyStart {
		t.Fatalf("could not slice bubble body: %s", html)
	}
	bubble := html[bodyStart:bodyEnd]
	if strings.Contains(bubble, "process_cwd: /tmp") || strings.Contains(bubble, "---") {
		t.Fatalf("raw yaml flashed: %s", bubble)
	}
	if !strings.Contains(html, "Ship the pills.") {
		t.Fatalf("missing body: %s", html)
	}
}
