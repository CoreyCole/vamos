package agentchat

import (
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestThreadSummaryRowMorphIDAndCopy(t *testing.T) {
	t.Parallel()

	msg := TranscriptMessage{
		DOMID:   "parent-1",
		EntryID: "parent-1",
		Role:    "assistant",
		Content: "hello",
		ThreadSummary: &ThreadReplySummary{
			ParentEntryID: "parent-1",
			ReplyCount:    2,
			LastReplyAt:   time.Now().Add(-2 * time.Minute),
			Authors: []ThreadReplyAuthor{
				{Initial: "C", Name: "Corey", AvatarBg: "bg-indigo-600"},
				{Initial: "L", Name: "Lead", AvatarBg: "bg-emerald-600"},
			},
		},
	}
	html := testhelpers.RenderToDocument(t, TranscriptMessageWithFork("t1", msg, "")).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="msg-parent-1-thread-summary"`,
		`2 replies`,
		`Last reply`,
		`id="agent-chat-scroll-region"`,
	} {
		if want == `id="agent-chat-scroll-region"` {
			if strings.Contains(out, want) {
				t.Fatalf("summary row must not remorph Host; html=%s", out)
			}
			continue
		}
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	summaryAt := strings.Index(out, `id="msg-parent-1-thread-summary"`)
	chipAt := strings.Index(out, `id="bot-dm-chip-`)
	if chipAt >= 0 && summaryAt > chipAt {
		t.Fatalf(
			"thread summary must sit above BotDMChip; summary=%d chip=%d",
			summaryAt,
			chipAt,
		)
	}
}

func TestMessageThreadHostDoesNotReplaceScrollRegion(t *testing.T) {
	t.Parallel()

	view := MessageThreadView{
		ThreadID: "t1",
		Open:     true,
		Parent: TranscriptMessage{
			DOMID:   "parent-1",
			EntryID: "parent-1",
			Role:    "assistant",
			Content: "parent body",
		},
		Replies: []MessageThreadReply{
			{
				ID:     "r1",
				Body:   "First reply from Corey.",
				Author: ThreadReplyAuthor{Initial: "C", Name: "Corey"},
			},
			{
				ID:     "r2",
				Body:   "Second reply from Lead.",
				Author: ThreadReplyAuthor{Initial: "L", Name: "Lead"},
			},
		},
		ComposerDisabled: true,
	}
	html := testhelpers.RenderToDocument(t, MessageThreadHost(view)).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="agent-chat-message-thread"`,
		`First reply from Corey.`,
		`Second reply from Lead.`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `id="agent-chat-scroll-region"`) {
		t.Fatal("message thread host must not include Pattern A Host")
	}
	if strings.Contains(out, `id="agent-chat-message-thread-reply-form"`) {
		t.Fatal("pairwise/view-only must omit reply composer")
	}
	if strings.Contains(out, `data-composer-shell`) {
		t.Fatal("pairwise/view-only must omit shared composer shell")
	}
	if strings.Contains(out, `w-[360px]`) || strings.Contains(out, `border-l`) {
		t.Fatal("desktop thread focus must not be a right split panel")
	}
}

func TestMessageThreadReplyUsesSharedComposer(t *testing.T) {
	t.Parallel()

	view := MessageThreadView{
		ThreadID: "t1",
		Open:     true,
		Parent: TranscriptMessage{
			DOMID:   "parent-1",
			EntryID: "parent-1",
			Role:    "assistant",
			Content: "parent body",
		},
	}
	html := testhelpers.RenderToDocument(t, MessageThreadHost(view)).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="agent-chat-message-thread-reply-form"`,
		`data-composer-shell`,
		`name="parent_entry_id"`,
		`name="body"`,
		`placeholder="Reply…"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `id="agent-chat-composer-form"`) {
		t.Fatal("thread reply must not steal the room composer form id")
	}
	if strings.Contains(out, `w-[360px]`) || strings.Contains(out, `border-l`) {
		t.Fatal("desktop thread focus must not be a right split panel")
	}
}

func TestSharedThreadChatMessageThreadFixtureDOM(t *testing.T) {
	t.Parallel()

	r, err := markdown.NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	s := &Service{renderer: r}
	stable := s.messageThreadFixtureMessages()
	view := s.messageThreadFixtureView("fixture-thread", false)
	html := testhelpers.RenderToDocument(t, SharedThreadChat(EmbeddedFreeformPanelArgs{
		ThreadID:      "fixture-thread",
		HasThread:     true,
		Transcript:    applyStableTranscriptWindow(TranscriptPaneState{Stable: stable}),
		MessageThread: view,
	})).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	for _, want := range []string{
		`id="agent-chat-scroll-region"`,
		`id="msg-` + messageThreadParentFixtureDOMID + `-thread-summary"`,
		`2 replies`,
		`id="agent-chat-message-thread"`,
		`First reply from Corey.`,
		`Second reply from Lead.`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `id="agent-chat-thread-family"`) {
		t.Fatal("family bar must stay parked")
	}
	if strings.Contains(out, `w-[360px]`) {
		t.Fatal("open thread must fill the chat column, not a 360px split")
	}
	if !strings.Contains(out, `hidden`) {
		t.Fatal("open thread must hide the main transcript column")
	}
}
