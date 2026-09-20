package agentchat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThoughtsPlanKey(t *testing.T) {
	t.Parallel()
	got := thoughtsPlanKey(
		"/abs/thoughts/user/plans/plan-a/design.md",
	)
	if got != "thoughts/user/plans/plan-a" {
		t.Fatalf("thoughtsPlanKey() = %q", got)
	}
	if thoughtsPlanKey(
		"thoughts/user/plans/plan-a/outline.md",
	) != "thoughts/user/plans/plan-a" {
		t.Fatalf("relative thoughtsPlanKey mismatch")
	}
}

func TestThoughtsAgentsDeskKey(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	desk := filepath.Join(root, "docs", "vamos")
	if err := os.MkdirAll(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(desk, "AGENTS.md"),
		[]byte("#\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	s := &Service{thoughtsRoot: root}
	got := s.thoughtsAgentsDeskKey("thoughts/docs/vamos/index.html")
	if got != "thoughts/docs/vamos" {
		t.Fatalf("thoughtsAgentsDeskKey = %q", got)
	}
	if s.thoughtsAgentsDeskKey("thoughts/owner/notes.md") != "" {
		t.Fatal("expected empty desk key without AGENTS.md")
	}
}
