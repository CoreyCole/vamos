package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildThreadArtifactHeaderActionsOmitsThoughts(t *testing.T) {
	t.Parallel()

	chatHref := "/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	viewHref := "/thoughts/owner/plans/alpha/design.md"
	html := renderHeaderActions(t, "owner/plans/alpha/design.md", chatHref, viewHref)
	if strings.Contains(html, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot:\n%s", html)
	}
	if !strings.Contains(html, "<span>Chat about this plan</span>") {
		t.Fatalf("missing Chat about this plan:\n%s", html)
	}
	if !strings.Contains(html, ">alpha</span>") {
		t.Fatalf("missing plan name subtitle:\n%s", html)
	}
	if strings.Contains(html, "<span>Chat</span>") {
		t.Fatalf("old Chat label still present:\n%s", html)
	}
	if !strings.Contains(html, `href="`+chatHref+`"`) {
		t.Fatalf("missing plan-lead href:\n%s", html)
	}
	if !strings.Contains(html, "<span>View Document</span>") ||
		!strings.Contains(html, `href="`+viewHref+`"`) {
		t.Fatalf("View Document should live in the 3-dot:\n%s", html)
	}
	if strings.Contains(html, ">Document</p>") ||
		strings.Contains(html, "border-t border-border") {
		t.Fatalf("kebab should not have a Document section or divider:\n%s", html)
	}
}

func TestBuildThreadArtifactHeaderActionsOmitsChatWithoutHref(t *testing.T) {
	t.Parallel()

	html := renderHeaderActions(t, "owner/plans/alpha/design.md", "", "")
	if strings.Contains(html, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot:\n%s", html)
	}
	if strings.Contains(html, "Chat about this plan") {
		t.Fatalf("Chat about this plan should be hidden on plan chat:\n%s", html)
	}
}

func renderHeaderActions(
	t *testing.T,
	docPath, chatHref, viewDocumentHref string,
) string {
	t.Helper()
	comp := BuildThreadArtifactHeaderActions(nil, docPath, chatHref, viewDocumentHref)
	if comp == nil {
		t.Fatal("header actions is nil")
	}
	var body bytes.Buffer
	if err := comp.Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	return body.String()
}
