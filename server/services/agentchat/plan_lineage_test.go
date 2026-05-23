package agentchat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPlanNodesFindsImplementationReviewFollowups(t *testing.T) {
	root := makePlanDir(t)
	child := filepath.Join(
		root,
		"reviews",
		"2026-05-13_12-00-00_parent_implementation-review",
	)
	writePlanMarkers(t, child)
	planningReview := filepath.Join(
		root,
		"reviews",
		"2026-05-13_11-00-00_parent_plan-review",
	)
	if err := os.MkdirAll(planningReview, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planningReview, "review.md"),
		[]byte("# Review\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	node, err := DiscoverPlanNodes(root)
	if err != nil {
		t.Fatalf("DiscoverPlanNodes() error = %v", err)
	}
	if node.Kind != PlanNodeParent {
		t.Fatalf("node.Kind = %q, want parent", node.Kind)
	}
	if len(node.Children) != 1 {
		t.Fatalf("len(node.Children) = %d, want 1", len(node.Children))
	}
	childNode := node.Children[0]
	if childNode.Kind != PlanNodeImplementationReviewFollowup {
		t.Fatalf("child.Kind = %q, want implementation review followup", childNode.Kind)
	}
	wantRel := "reviews/2026-05-13_12-00-00_parent_implementation-review"
	if childNode.RootRelPath != wantRel {
		t.Fatalf("child.RootRelPath = %q, want %q", childNode.RootRelPath, wantRel)
	}
}

func TestIsImplementationReviewPlanDirRequiresMarkers(t *testing.T) {
	root := makePlanDir(t)
	child := filepath.Join(
		root,
		"reviews",
		"2026-05-13_12-00-00_parent_implementation-review",
	)
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsImplementationReviewPlanDir(child) {
		t.Fatal("IsImplementationReviewPlanDir() = true without plan markers")
	}
	writePlanMarkers(t, child)
	if !IsImplementationReviewPlanDir(child) {
		t.Fatal("IsImplementationReviewPlanDir() = false with plan markers")
	}
}

func TestDefaultArtifactPathPrefersParentRootArtifacts(t *testing.T) {
	files := []string{
		"reviews/2026-05-13_12-00-00_parent_implementation-review/design.md",
		"reviews/2026-05-13_12-00-00_parent_implementation-review/plan.md",
		"design.md",
		"outline.md",
	}
	if got := defaultArtifactPath(files); got != "design.md" {
		t.Fatalf("defaultArtifactPath() = %q, want parent design.md", got)
	}

	files = []string{
		"reviews/2026-05-13_12-00-00_parent_implementation-review/design.md",
		"outline.md",
		"plan.md",
	}
	if got := defaultArtifactPath(files); got != "outline.md" {
		t.Fatalf("defaultArtifactPath() = %q, want parent outline.md", got)
	}
}

func TestFocusedRootDocPathUsesSelectedImplementationReviewPlan(t *testing.T) {
	root := makePlanDir(t)
	child := filepath.Join(
		root,
		"reviews",
		"2026-05-13_12-00-00_parent_implementation-review",
	)
	writePlanMarkers(t, child)
	selected := filepath.Join(child, "design.md")

	if got := focusedRootDocPath(root, selected); got != child {
		t.Fatalf("focusedRootDocPath() = %q, want %q", got, child)
	}
	if got := focusedRootDocPath(root, filepath.Join(root, "design.md")); got != root {
		t.Fatalf("focusedRootDocPath(parent design) = %q, want %q", got, root)
	}
}

func makePlanDir(t *testing.T) string {
	t.Helper()
	root := filepath.Join(
		t.TempDir(),
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-05-12_15-17-18_parent",
	)
	writePlanMarkers(t, root)
	return root
}

func writePlanMarkers(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "design.md", "outline.md", "plan.md"} {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte("# "+name+"\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
}
