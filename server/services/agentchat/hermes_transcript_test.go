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

func TestAppendHermesTranscriptRejectsSessionDirectorySymlinkOutsidePlan(t *testing.T) {
	plan := t.TempDir()
	outside := t.TempDir()
	hermesDir := filepath.Join(plan, ".vamos", "sessions", "hermes")
	if err := os.MkdirAll(filepath.Dir(hermesDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, hermesDir); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(plan, HermesTranscriptEvent{
		ID: "event-1", Type: "final", ThreadID: "thread-1", Content: "nope",
	}); err == nil || !strings.Contains(err.Error(), "escapes plan directory") {
		t.Fatalf("AppendHermesTranscript() error = %v, want containment rejection", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "thread-1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("outside transcript = %v, want no file", err)
	}
}

func TestHermesTranscriptPathRejectsTraversal(t *testing.T) {
	if _, err := HermesTranscriptPath(t.TempDir(), "../secret"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
