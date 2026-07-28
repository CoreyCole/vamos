package agentchat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendHermesTranscriptIsIdempotentAndRedactsToolArguments(t *testing.T) {
	plan := t.TempDir()
	e := HermesTranscriptEvent{
		ID:       "event-1",
		Type:     "tool",
		ThreadID: "thread-1",
		Tool:     &HermesToolCard{Name: "shell", Status: "done"},
	}
	if err := AppendHermesTranscript(plan, e); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(plan, e); err != nil {
		t.Fatal(err)
	}
	events, err := readHermesTranscript(plan, "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Tool.Name != "shell" {
		t.Fatalf("events=%+v", events)
	}
	data, err := os.ReadFile(filepath.Join(plan, ".vamos/sessions/hermes/thread-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "arguments") {
		t.Fatal("raw tool arguments persisted")
	}
}

func TestHermesTranscriptPathRejectsTraversal(t *testing.T) {
	if _, err := HermesTranscriptPath(t.TempDir(), "../secret"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
