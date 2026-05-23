package markdown

import (
	"path/filepath"
	"testing"
)

func TestBuildWorkspaceDocTreeFromRootUsesFilesystemFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	planRoot := filepath.Join(root, "creative-mode-agent", "plans", "demo")
	mustMkdirAll(t, planRoot)
	mustWriteFile(t, filepath.Join(planRoot, "AGENTS.md"), []byte("# Plan"))
	mustWriteFile(t, filepath.Join(planRoot, "outline.md"), []byte("# Outline"))
	mustWriteFile(t, filepath.Join(planRoot, "plan.md"), []byte("# Implementation"))
	mustWriteFile(t, filepath.Join(planRoot, "notes.txt"), []byte("include"))
	mustWriteFile(t, filepath.Join(planRoot, "image.png"), []byte("skip"))
	mustMkdirAll(t, filepath.Join(planRoot, "context"))
	mustWriteFile(t, filepath.Join(planRoot, "context", "design.md"), []byte("# Design"))

	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}
	nodes, err := service.BuildWorkspaceDocTreeFromRoot(
		"creative-mode-agent/plans/demo",
		"creative-mode-agent/plans/demo/plan.md",
	)
	if err != nil {
		t.Fatalf("BuildWorkspaceDocTreeFromRoot error = %v", err)
	}

	labels := make(map[string]bool)
	var active bool
	for _, node := range nodes {
		labels[node.Label] = true
		if node.IsActive && node.Path == "creative-mode-agent/plans/demo/plan.md" {
			active = true
		}
		for _, child := range node.Children {
			labels[child.Label] = true
		}
	}
	for _, want := range []string{"AGENTS.md", "outline.md", "plan.md", "notes.txt", "context"} {
		if !labels[want] {
			t.Fatalf("missing label %q in nodes %#v", want, nodes)
		}
	}
	if labels["image.png"] {
		t.Fatalf("binary file should not be included: %#v", nodes)
	}
	if !active {
		t.Fatalf("current plan.md node was not marked active: %#v", nodes)
	}
}

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
