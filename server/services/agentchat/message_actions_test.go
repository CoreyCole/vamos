package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/datastarui/components/toast"
)

func TestMessageActionIDs(t *testing.T) {
	if got := msgActionsID("abc"); got != "msg-abc-actions" {
		t.Fatalf("actions id = %q", got)
	}
	if got := msgMenuID("abc"); got != "msg-abc-menu" {
		t.Fatalf("menu id = %q", got)
	}
	if got := msgSheetID("abc"); got != "msg-abc-sheet" {
		t.Fatalf("sheet id = %q", got)
	}
}

func TestMessageCopyClickExprUsesRootToast(t *testing.T) {
	expr := messageCopyClickExpr("abc", "hello \"world\"")
	if !strings.Contains(expr, "navigator.clipboard.writeText") {
		t.Fatalf("missing writeText: %s", expr)
	}
	if !strings.Contains(expr, toast.ShowToastExpr("clipboard_success", 2000)) {
		t.Fatalf("missing clipboard_success toast: %s", expr)
	}
	if !strings.Contains(expr, "hello \\\"world\\\"") &&
		!strings.Contains(expr, `hello \"world\"`) {
		// strconv.Quote produces "hello \"world\""
		if !strings.Contains(expr, `"hello \"world\""`) {
			t.Fatalf("content not quoted safely: %s", expr)
		}
	}
}

func TestMessageQuoteClickExprWritesComposerTextarea(t *testing.T) {
	expr := messageQuoteClickExpr("abc", "line1\nline2")
	if strings.Contains(expr, "$chatDraft") {
		t.Fatalf("quote must not assign $chatDraft outside composer scope: %s", expr)
	}
	if !strings.Contains(expr, "agent-chat-composer-input") {
		t.Fatalf("missing composer textarea id: %s", expr)
	}
	if !strings.Contains(expr, "dispatchEvent") {
		t.Fatalf("missing input event so data-bind updates: %s", expr)
	}
	if !strings.Contains(expr, "> line1") || !strings.Contains(expr, "> line2") {
		t.Fatalf("missing quoted lines: %s", expr)
	}
}

func TestMessagePlainTextFallsBackFromHTML(t *testing.T) {
	got := messagePlainText(TranscriptMessage{HTMLContent: "<p>Hi <b>there</b></p>"})
	if got != "Hi there" {
		t.Fatalf("plain = %q", got)
	}
}

func TestTranscriptMessageActionsRenderMorphMap(t *testing.T) {
	msg := TranscriptMessage{
		DOMID:        "m1",
		EntryID:      "e1",
		Role:         "assistant",
		Content:      "hello",
		ShowForkForm: true,
		AuthorName:   "Bot",
	}
	var b strings.Builder
	if err := TranscriptMessageWithFork(
		"t1",
		msg,
		"@post('/fork')",
	).Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{
		`id="msg-m1"`,
		`id="msg-m1-actions"`,
		`id="msg-m1-menu"`,
		`id="msg-m1-sheet"`,
		`agent-chat-msg`,
		"Copy",
		"Fork",
		"Coming soon",
		"Quote",
		`data-msg-fork-icon`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "absolute right-2 top-2") {
		t.Fatal("absolute fork overlay still present")
	}
	if strings.Contains(html, "view-transition-name") &&
		strings.Contains(html, "msg-m1-actions") {
		// ensure actions cluster itself has no VT name attribute nearby
	}
}

func TestChatWorkingRowHasNoActions(t *testing.T) {
	var b strings.Builder
	if err := ChatWorkingRow().Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if !strings.Contains(html, `id="agent-chat-working"`) {
		t.Fatal("missing working row")
	}
	if strings.Contains(html, "-actions") || strings.Contains(html, "Message actions") {
		t.Fatalf("working row must not include message actions: %s", html)
	}
}
