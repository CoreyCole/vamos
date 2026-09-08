package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildThreadArtifactHeaderActionsHidesThoughtsOnThoughts(t *testing.T) {
	t.Parallel()

	chatHref := "/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	html := renderHeaderActions(t, "owner/plans/alpha/design.md", chatHref, false)
	if strings.Contains(html, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot on thoughts:\n%s", html)
	}
	if !strings.Contains(html, "<span>chat about this plan</span>") {
		t.Fatalf("missing chat about this plan:\n%s", html)
	}
	if strings.Contains(html, "<span>Chat</span>") {
		t.Fatalf("old Chat label still present:\n%s", html)
	}
	if !strings.Contains(html, `href="`+chatHref+`"`) {
		t.Fatalf("missing plan-lead href:\n%s", html)
	}
}

func TestBuildThreadArtifactHeaderActionsShowsThoughtsOffThoughts(t *testing.T) {
	t.Parallel()

	html := renderHeaderActions(t, "owner/plans/alpha/design.md", "", true)
	if !strings.Contains(html, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts missing off thoughts:\n%s", html)
	}
	if strings.Contains(html, "chat about this plan") {
		t.Fatalf("chat about this plan should be hidden on plan chat:\n%s", html)
	}
}

func renderHeaderActions(
	t *testing.T,
	docPath, chatHref string,
	showThoughts bool,
) string {
	t.Helper()
	comp := BuildThreadArtifactHeaderActions(nil, docPath, chatHref, showThoughts)
	if comp == nil {
		t.Fatal("header actions is nil")
	}
	var body bytes.Buffer
	if err := comp.Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	return body.String()
}
