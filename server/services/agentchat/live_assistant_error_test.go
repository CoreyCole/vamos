package agentchat

import (
	"encoding/json"
	"strings"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
)

func TestLiveAssistantErrorWithoutContentRendersErrorCard(t *testing.T) {
	t.Parallel()
	service, _ := newThreadDraftService(t)
	item := conversation.LiveTurnItem{
		Key:  "err1",
		Kind: conversation.LiveTurnAssistantMessage,
		Role: "assistant",
		MessageJSON: json.RawMessage(
			`{"role":"assistant","content":[],"stopReason":"error","errorMessage":"{\"detail\":\"The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.\"}"}`,
		),
		IsFinal: true,
	}
	items, err := service.liveTurnTranscriptItems(
		"thread_1",
		item,
		service.defaultTranscriptRenderPolicy(),
		nil,
		map[string]TranscriptMessage{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 error card", len(items))
	}
	got := items[0]
	if got.Variant != "detail" || !got.IsError {
		t.Fatalf("variant=%q isError=%v, want error detail", got.Variant, got.IsError)
	}
	if !strings.Contains(got.Content, "gpt-5.4") {
		t.Fatalf("error body missing model detail: %q", got.Content)
	}
}

func TestLiveAssistantEmptyContentWithoutErrorIsOmitted(t *testing.T) {
	t.Parallel()
	service, _ := newThreadDraftService(t)
	item := conversation.LiveTurnItem{
		Key:         "empty",
		Kind:        conversation.LiveTurnAssistantMessage,
		Role:        "assistant",
		MessageJSON: json.RawMessage(`{"role":"assistant","content":[]}`),
		IsFinal:     true,
	}
	items, err := service.liveTurnTranscriptItems(
		"thread_1",
		item,
		service.defaultTranscriptRenderPolicy(),
		nil,
		map[string]TranscriptMessage{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("empty assistant without error should stay omitted, got %#v", items)
	}
}
