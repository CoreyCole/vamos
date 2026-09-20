package agentchat

import (
	"strings"
	"testing"
)

func TestGroupTranscriptActivitySingletonHasNoGroup(t *testing.T) {
	msgs := []TranscriptMessage{
		{DOMID: "u1", Variant: "bubble", Role: "user", Content: "hi"},
		{DOMID: "e1-reasoning", Variant: "detail", Title: "thinking"},
		{DOMID: "a1", Variant: "bubble", Role: "assistant", Content: "done"},
	}
	segs := groupTranscriptActivity(msgs)
	if len(segs) != 3 {
		t.Fatalf("segs = %d", len(segs))
	}
	if segs[1].Grouped {
		t.Fatal("singleton detail should not be grouped")
	}
}

func TestGroupTranscriptActivityCollapsesConsecutiveDetails(t *testing.T) {
	msgs := []TranscriptMessage{
		{DOMID: "u1", Variant: "bubble", Role: "user"},
		{DOMID: "e1-reasoning", Variant: "detail", Title: "thinking"},
		{DOMID: "e1-tool-0", Variant: "detail", Title: "bash"},
		{DOMID: "e1-tool-1", Variant: "detail", Title: "read"},
		{DOMID: "e1-tool-2", Variant: "detail", Title: "read"},
		{DOMID: "a1", Variant: "bubble", Role: "assistant"},
	}
	segs := groupTranscriptActivity(msgs)
	if len(segs) != 3 {
		t.Fatalf("segs = %d %#v", len(segs), segs)
	}
	if !segs[1].Grouped || len(segs[1].Messages) != 4 {
		t.Fatalf("group = %#v", segs[1])
	}
	sum := activityGroupSummary(segs[1].Messages)
	if sum != "THINKING · BASH · READ×2" {
		t.Fatalf("summary = %q", sum)
	}
	if activityGroupID(segs[1].Messages) != "e1-reasoning" {
		t.Fatalf("id = %q", activityGroupID(segs[1].Messages))
	}
}

func TestTranscriptActivityGroupMorphMap(t *testing.T) {
	msgs := []TranscriptMessage{
		{
			DOMID:                 "e1-reasoning",
			Variant:               "detail",
			Title:                 "thinking",
			Collapsible:           true,
			HideBodyWhenCollapsed: true,
			HeaderSummary:         "plan",
		},
		{
			DOMID:                 "e1-tool-0",
			Variant:               "detail",
			Title:                 "bash",
			ToolCallID:            "call_1",
			Collapsible:           true,
			HideBodyWhenCollapsed: true,
			HeaderCode:            "ls",
		},
	}
	var b strings.Builder
	if err := TranscriptActivityGroup(
		"t1",
		msgs,
		"",
	).Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if !strings.Contains(html, `id="msg-e1-reasoning-activity-group"`) {
		t.Fatalf("missing group id: %s", html)
	}
	if strings.Contains(html, `<details open`) {
		t.Fatalf("group should default collapsed: %s", html)
	}
	if !strings.Contains(html, "THINKING · BASH") {
		t.Fatalf("missing summary: %s", html)
	}
	if !strings.Contains(html, `id="msg-e1-reasoning"`) ||
		!strings.Contains(html, `data-chat-density="reasoning"`) {
		t.Fatalf("inner density ids missing: %s", html)
	}
	if !strings.Contains(html, `id="msg-e1-tool-0"`) ||
		!strings.Contains(html, `data-chat-density="tool"`) {
		t.Fatalf("inner tool density missing: %s", html)
	}
	if strings.Contains(html, "agent-chat-scroll-region") {
		t.Fatal("must not remorph Pattern A Host")
	}
}

func TestTranscriptMessageListSingletonSkipsGroupChrome(t *testing.T) {
	msgs := []TranscriptMessage{
		{
			DOMID:                 "e1-reasoning",
			Variant:               "detail",
			Title:                 "thinking",
			Collapsible:           true,
			HideBodyWhenCollapsed: true,
		},
	}
	var b strings.Builder
	if err := TranscriptMessageList("t1", msgs, "").Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if strings.Contains(html, "activity-group") {
		t.Fatalf("singleton should not wrap: %s", html)
	}
	if !strings.Contains(html, `id="msg-e1-reasoning"`) {
		t.Fatalf("missing detail id: %s", html)
	}
}
