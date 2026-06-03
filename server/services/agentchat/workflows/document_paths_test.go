package workflows

import "testing"

func TestDocumentPathFromRootNormalizesAbsoluteThoughtsRoot(t *testing.T) {
	got, err := documentPathFromRoot(
		"/home/ruby/cn/chestnut-flake/cn-agents/thoughts/creative-mode-agent/plans/demo",
		"questions/demo.md",
	)
	if err != nil {
		t.Fatalf("documentPathFromRoot() error = %v", err)
	}
	want := "thoughts/creative-mode-agent/plans/demo/questions/demo.md"
	if got != want {
		t.Fatalf("documentPathFromRoot() = %q, want %q", got, want)
	}
}

func TestDocumentPathFromRootKeepsThoughtsRelativeArtifactPath(t *testing.T) {
	got, err := documentPathFromRoot(
		"/home/ruby/cn/chestnut-flake/cn-agents/thoughts/creative-mode-agent/plans/demo",
		"thoughts/creative-mode-agent/plans/demo/questions/demo.md",
	)
	if err != nil {
		t.Fatalf("documentPathFromRoot() error = %v", err)
	}
	want := "thoughts/creative-mode-agent/plans/demo/questions/demo.md"
	if got != want {
		t.Fatalf("documentPathFromRoot() = %q, want %q", got, want)
	}
}
