package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildThreadArtifactHeaderActionsOmitsThoughts(t *testing.T) {
	t.Parallel()

	chatHref := "/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	html := renderHeaderActions(t, "owner/plans/alpha/design.md")
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
	if strings.Contains(html, "View Document") {
		t.Fatalf("View Document must not be in the 3-dot:\n%s", html)
	}
	if strings.Contains(html, ">Document</p>") ||
		strings.Contains(html, "border-t border-border") {
		t.Fatalf("kebab should not have a Document section or divider:\n%s", html)
	}
}

func TestBuildThreadArtifactHeaderActionsAlwaysChatsOnPlanDocs(t *testing.T) {
	t.Parallel()

	html := renderHeaderActions(t, "owner/plans/alpha/design.md")
	if !strings.Contains(html, "<span>Chat about this plan</span>") {
		t.Fatalf("Chat about this plan required on threads and thoughts:\n%s", html)
	}
	if strings.Contains(html, "View Document") {
		t.Fatalf("View Document must not be in the 3-dot:\n%s", html)
	}
}

func renderHeaderActions(
	t *testing.T,
	docPath string,
) string {
	t.Helper()
	comp := BuildThreadArtifactHeaderActions(nil, docPath)
	if comp == nil {
		t.Fatal("header actions is nil")
	}
	var body bytes.Buffer
	if err := comp.Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	return body.String()
}

func TestBuildChatHeaderOverflowShareOnlyWithoutDoc(t *testing.T) {
	t.Parallel()
	html := renderChatHeaderOverflow(t, nil, "", false)
	for _, want := range []string{
		`data-testid="workbench-overflow-actions"`,
		"Share artifact",
		"Share chat",
		"Share",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "Copy path") || strings.Contains(html, "Artifact") {
		t.Fatalf("empty doc must not paint Artifact group:\n%s", html)
	}
	if n := strings.Count(html, `data-testid="workbench-overflow-actions"`); n != 1 {
		t.Fatalf("want one overflow root, got %d", n)
	}
}

func TestBuildChatHeaderOverflowFoldsArtifactSkipsPlanChat(t *testing.T) {
	t.Parallel()
	page := &PageArgs{
		FilePath: "owner/plans/alpha/design.md",
		ViewerArgs: ViewerArgs{
			RawMarkdown: "# Design",
			CommentMode: CommentModeDocumentOnly,
		},
	}
	html := renderChatHeaderOverflow(t, page, page.FilePath, false)
	for _, want := range []string{
		"Share artifact",
		"Share chat",
		"Copy path",
		"Copy document",
		"Comment",
		"Artifact",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "Chat about this plan") {
		t.Fatalf("plan room must skip Chat about this plan:\n%s", html)
	}
}

func TestBuildChatHeaderOverflowIncludesPlanChatOutsidePlanRoom(t *testing.T) {
	t.Parallel()
	html := renderChatHeaderOverflow(t, nil, "owner/plans/alpha/design.md", true)
	if !strings.Contains(html, "Chat about this plan") {
		t.Fatalf("non-plan room should keep Chat about this plan:\n%s", html)
	}
}

func renderChatHeaderOverflow(
	t *testing.T,
	pageArgs *PageArgs,
	docPath string,
	includePlanChat bool,
) string {
	t.Helper()
	comp := BuildChatHeaderOverflow(pageArgs, docPath, includePlanChat)
	if comp == nil {
		t.Fatal("chat header overflow is nil")
	}
	var body bytes.Buffer
	if err := comp.Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	return body.String()
}
