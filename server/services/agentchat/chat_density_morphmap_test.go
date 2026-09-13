package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/markdown"
)

func TestAssistantTranscriptDensityIDs(t *testing.T) {
	r, err := markdown.NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	s := &Service{detailCollapseLineLimit: 8, renderer: r}
	content := []any{
		map[string]any{"type": "thinking", "thinking": "plan the change"},
		map[string]any{
			"type": "toolCall",
			"id":   "call_bash_1",
			"name": "bash",
			"arguments": map[string]any{
				"command": "ls",
			},
		},
		map[string]any{
			"type": "toolCall",
			"id":   "call_sub_1",
			"name": "subagent",
			"arguments": map[string]any{
				"agent": "worker",
				"task":  "inspect",
			},
		},
		map[string]any{"type": "text", "text": "Here is the markdown result."},
	}
	items := s.assistantTranscriptItems("entry1", "entry1", content, false, map[string]TranscriptMessage{})
	var reasoning, tools, bubbles []TranscriptMessage
	for _, item := range items {
		switch item.Variant {
		case "detail":
			if item.Title == "thinking" {
				reasoning = append(reasoning, item)
			} else {
				tools = append(tools, item)
			}
		default:
			bubbles = append(bubbles, item)
		}
	}
	if len(reasoning) != 1 || reasoning[0].DOMID != "entry1-reasoning" {
		t.Fatalf("reasoning DOMID = %#v", reasoning)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %#v", tools)
	}
	if tools[0].DOMID != "entry1-tool-0" || tools[1].DOMID != "entry1-tool-1" {
		t.Fatalf("tool DOMIDs = %q %q", tools[0].DOMID, tools[1].DOMID)
	}
	if len(bubbles) != 1 || bubbles[0].Variant != "bubble" {
		t.Fatalf("bubble = %#v", bubbles)
	}
	if bubbles[0].HTMLContent == "" {
		t.Fatal("markdown bubble HTMLContent empty")
	}
}

func TestTranscriptDetailCardDensityMorphMap(t *testing.T) {
	reasoning := TranscriptMessage{
		DOMID:                 "entry1-reasoning",
		EntryID:               "entry1",
		Variant:               "detail",
		Title:                 "thinking",
		HeaderSummary:         "[14] plan the change",
		Content:               "> plan the change",
		HTMLContent:           "<blockquote><p>plan the change</p></blockquote>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}
	tool := TranscriptMessage{
		DOMID:                 "entry1-tool-0",
		EntryID:               "entry1",
		Variant:               "detail",
		Title:                 "bash",
		ToolCallID:            "call_bash_1",
		HeaderCode:            "ls",
		Content:               "```json\n{\n  \"command\": \"ls\"\n}\n```",
		HTMLContent:           "<pre>ls</pre>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}
	subagent := TranscriptMessage{
		DOMID:                 "entry1-tool-1",
		EntryID:               "entry1",
		Variant:               "detail",
		Title:                 "subagent",
		ToolCallID:            "call_sub_1",
		HeaderSummary:         "agent: worker · task: inspect",
		Content:               "```json\n{}\n```",
		HTMLContent:           "<pre>{}</pre>",
		Collapsible:           true,
		HideBodyWhenCollapsed: true,
	}

	for _, msg := range []TranscriptMessage{reasoning, tool, subagent} {
		var b strings.Builder
		if err := TranscriptDetailCard(msg).Render(t.Context(), &b); err != nil {
			t.Fatal(err)
		}
		html := b.String()
		id := `id="msg-` + msg.DOMID + `"`
		if !strings.Contains(html, "<details") {
			t.Fatalf("%s missing <details>: %s", msg.DOMID, html)
		}
		if !strings.Contains(html, id) {
			t.Fatalf("%s missing %s", msg.DOMID, id)
		}
		if !strings.Contains(html, "min-h-11") {
			t.Fatalf("%s missing soft mobile hit target", msg.DOMID)
		}
		if strings.Contains(html, "data-on:click") {
			t.Fatalf("%s still uses signal toggle (should be native details)", msg.DOMID)
		}
		kind := chatDensityKind(msg)
		if !strings.Contains(html, `data-chat-density="`+kind+`"`) {
			t.Fatalf("%s missing data-chat-density=%s", msg.DOMID, kind)
		}
	}

	// markdown + chroma stay open: bubble path must NOT wrap in details
	bubble := TranscriptMessage{
		DOMID:       "entry1",
		Role:        "assistant",
		Variant:     "bubble",
		Content:     "hello",
		HTMLContent: `<pre class="chroma"><span class="k">func</span></pre>`,
		AuthorName:  "Bot",
	}
	var bb strings.Builder
	if err := TranscriptMessageWithFork("t1", bubble, "").Render(t.Context(), &bb); err != nil {
		t.Fatal(err)
	}
	out := bb.String()
	if strings.Contains(out, "<details") {
		t.Fatalf("bubble wrapped in details: %s", out)
	}
	if !strings.Contains(out, `id="msg-entry1"`) {
		t.Fatalf("bubble missing stable id: %s", out)
	}
	if !strings.Contains(out, "markdown-viewer") || !strings.Contains(out, "chroma") {
		t.Fatalf("bubble lost markdown/chroma: %s", out)
	}
}
