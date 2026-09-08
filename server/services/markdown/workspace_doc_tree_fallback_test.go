package markdown

import (
	"path/filepath"
	"testing"
)

func TestInferWorkspaceRootNearestAgents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outer := filepath.Join(root, "creative-mode-agent", "plans", "demo")
	inner := filepath.Join(outer, "reviews", "child")
	mustMkdirAll(t, inner)
	mustWriteFile(t, filepath.Join(outer, "AGENTS.md"), []byte("# Outer"))
	mustWriteFile(t, filepath.Join(inner, "AGENTS.md"), []byte("# Inner"))
	mustWriteFile(t, filepath.Join(inner, "plan.md"), []byte("# Plan"))

	got, ok := InferWorkspaceRoot(
		root,
		"thoughts/creative-mode-agent/plans/demo/reviews/child/plan.md",
	)
	if !ok {
		t.Fatal("InferWorkspaceRoot ok=false")
	}
	want := "creative-mode-agent/plans/demo/reviews/child"
	if got != want {
		t.Fatalf("InferWorkspaceRoot=%q, want %q", got, want)
	}
}

func TestNormalizeWorkspaceDocPathStripsThoughtsPrefix(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"thoughts/CoreyCole/x.md", "/thoughts/CoreyCole/x.md", "CoreyCole/x.md"} {
		if got := NormalizeWorkspaceDocPath(input); got != "CoreyCole/x.md" {
			t.Fatalf("NormalizeWorkspaceDocPath(%q)=%q", input, got)
		}
	}
}
