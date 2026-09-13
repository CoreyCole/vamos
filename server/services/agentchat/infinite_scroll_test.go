package agentchat

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestWindowStableTranscriptKeepsNewestPage(t *testing.T) {
	t.Parallel()
	msgs := make([]TranscriptMessage, 0, stableTranscriptInitialLimit+5)
	for i := 0; i < stableTranscriptInitialLimit+5; i++ {
		msgs = append(msgs, TranscriptMessage{DOMID: fmt.Sprintf("m%d", i)})
	}
	page, hasMore, before := windowStableTranscript(msgs)
	if !hasMore {
		t.Fatal("expected hasMore")
	}
	if len(page) != stableTranscriptInitialLimit {
		t.Fatalf("len(page)=%d", len(page))
	}
	if before != page[0].DOMID {
		t.Fatalf("before=%q page0=%q", before, page[0].DOMID)
	}
	if page[0].DOMID != "m5" {
		t.Fatalf("expected oldest kept m5, got %q", page[0].DOMID)
	}
}

func TestOlderStablePagePrependWindow(t *testing.T) {
	t.Parallel()
	msgs := []TranscriptMessage{
		{DOMID: "a"}, {DOMID: "b"}, {DOMID: "c"}, {DOMID: "d"}, {DOMID: "e"},
	}
	page, next, hasMore := olderStablePage(msgs, "d", 2)
	if got := joinDOMIDs(page); got != "b,c" {
		t.Fatalf("page=%q", got)
	}
	if !hasMore || next != "b" {
		t.Fatalf("next=%q hasMore=%v", next, hasMore)
	}
	page, next, hasMore = olderStablePage(msgs, "b", 2)
	if got := joinDOMIDs(page); got != "a" {
		t.Fatalf("final page=%q", got)
	}
	if hasMore || next != "" {
		t.Fatalf("expected exhausted next=%q hasMore=%v", next, hasMore)
	}
}

func TestMessagesPanePatternAMorphMap(t *testing.T) {
	t.Parallel()
	state := TranscriptPaneState{
		Stable: []TranscriptMessage{
			{DOMID: "old", Role: "user", Content: "hi"},
		},
		HasMoreOlder: true,
		OlderBefore:  "old",
		Live:         LiveTranscriptView{},
	}
	html := renderComponentString(t, MessagesPane("thread-1", state, true, "", true))
	for _, want := range []string{
		`id="agent-chat-scroll-region"`,
		`id="agent-chat-stable-transcript"`,
		`id="agent-chat-live-transcript"`,
		`id="chat-latest"`,
		`id="agent-chat-scroll-sentinel-above"`,
		`data-on:intersect=`,
		`/agent-chat/thread/thread-1/history?before=old`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in:\n%s", want, html)
		}
	}
	itemsAt := strings.Index(html, `id="agent-chat-stable-transcript"`)
	liveAt := strings.Index(html, `id="agent-chat-live-transcript"`)
	latestAt := strings.Index(html, `id="chat-latest"`)
	if !(itemsAt >= 0 && liveAt > itemsAt && latestAt > liveAt) {
		t.Fatalf("order items=%d live=%d latest=%d", itemsAt, liveAt, latestAt)
	}
}

func joinDOMIDs(msgs []TranscriptMessage) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		parts = append(parts, m.DOMID)
	}
	return strings.Join(parts, ",")
}

func renderComponentString(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}
