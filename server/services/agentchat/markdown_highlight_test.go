package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

// Proves SharedThreadChat assistant bubbles keep fenced-code chroma highlight
// via renderMarkdown → MarkdownBytesToHTML (same path as leftover agentchat).
func TestRenderMarkdown_FencedCodeSyntaxHighlightForChat(t *testing.T) {
	t.Parallel()

	r, err := markdown.NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	md := []byte("```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n")
	html, err := renderMarkdown(r, md)
	if err != nil {
		t.Fatalf("renderMarkdown() error = %v", err)
	}
	if !strings.Contains(html, "chroma") {
		t.Fatalf("chat renderMarkdown missing chroma highlight; html = %s", html)
	}
	if !strings.Contains(html, "Println") {
		t.Fatalf("chat renderMarkdown dropped source text; html = %s", html)
	}

	// Transcript surface: ChatMessage embeds HTMLContent in markdown-viewer.
	doc := testhelpers.RenderToDocument(t, ChatMessage(ChatMessageArgs{
		ID:          "msg-highlight",
		Role:        "assistant",
		Content:     string(md),
		HTMLContent: html,
	})).Doc
	out, err := doc.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, "markdown-viewer") {
		t.Fatalf("ChatMessage missing markdown-viewer; out = %s", out)
	}
	if !strings.Contains(out, "chroma") {
		t.Fatalf("ChatMessage HTMLContent missing chroma; out = %s", out)
	}
}
