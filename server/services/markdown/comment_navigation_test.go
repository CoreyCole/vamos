package markdown

import (
	"strings"
	"testing"
)

func TestCommentTargetScriptEscapesIDs(t *testing.T) {
	script := CommentTargetScript("c'1", "sec\\two")
	for _, want := range []string{
		"workbench-section-nav",
		`detail: { hash: 'sec\\two', updateURL: false }`,
		`document.getElementById('comment-thread-c\'1')`,
		"scrollIntoView({block: 'center'})",
		"ring-2",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q: %s", want, script)
		}
	}
}

func TestCommentTargetScriptSectionOnlyDispatchesWorkbenchSectionNav(t *testing.T) {
	script := CommentTargetScript("", "#intro")
	for _, want := range []string{
		"workbench-section-nav",
		`detail: { hash: 'intro', updateURL: false }`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("section-only script missing %q: %s", want, script)
		}
	}
	for _, unwanted := range []string{
		"scrollIntoView({block: 'center'})",
		"comment-thread-",
		"ring-2",
	} {
		if strings.Contains(script, unwanted) {
			t.Fatalf("section-only script should not contain %q: %s", unwanted, script)
		}
	}
}

func TestCommentTargetScriptEmpty(t *testing.T) {
	if got := CommentTargetScript("", ""); got != "" {
		t.Fatalf("empty script = %q, want empty", got)
	}
}
