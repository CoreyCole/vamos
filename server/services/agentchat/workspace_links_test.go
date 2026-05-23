package agentchat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func TestWorkspaceSlugForPlanPreservesTimestampedPlanIdentity(t *testing.T) {
	t.Parallel()

	slug, err := WorkspaceSlugForPlan(
		"cn-agents",
		"thoughts/creative-mode-agent/plans/2026-05-12_15-17-18_fast-iteration-workspace-ux",
	)
	if err != nil {
		t.Fatalf("WorkspaceSlugForPlan: %v", err)
	}
	want := "2026-05-12-15-17-18-fast-iteration-workspace-ux"
	if slug != want {
		t.Fatalf("slug = %q, want %q", slug, want)
	}
}

func TestWorkspaceActionsForStatusAreHumanControlled(t *testing.T) {
	t.Parallel()

	running := workspaceActionsForStatus(workspaces.StatusRunning)
	if !hasWorkspaceAction(running, WorkspaceActionStop) ||
		!hasWorkspaceAction(running, WorkspaceActionMerge) ||
		!hasWorkspaceAction(running, WorkspaceActionRefresh) ||
		hasWorkspaceAction(running, WorkspaceActionStart) {
		t.Fatalf("running actions = %#v", running)
	}

	failed := workspaceActionsForStatus(workspaces.StatusFailed)
	if !hasWorkspaceAction(failed, WorkspaceActionRetry) ||
		!hasWorkspaceAction(failed, WorkspaceActionMerge) ||
		hasWorkspaceAction(failed, WorkspaceActionDelete) {
		t.Fatalf("failed actions = %#v", failed)
	}
}

func TestVerifyPlanWorkspaceLinksAcceptsParentAndFollowup(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "2026-05-12_15-17-18_fast-iteration-workspace-ux")
	writePlanMarkersForTest(t, root)
	child := filepath.Join(
		root,
		"reviews",
		"2026-05-13_10-00-00_fast-iteration-workspace-ux_implementation-review",
	)
	writePlanMarkersForTest(t, child)
	planningReview := filepath.Join(
		root,
		"reviews",
		"2026-05-13_11-00-00_fast-iteration-workspace-ux_plan-review",
	)
	if err := os.MkdirAll(planningReview, 0o755); err != nil {
		t.Fatalf("MkdirAll planning review: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(planningReview, "review.md"),
		[]byte("review"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile review: %v", err)
	}

	if err := VerifyPlanWorkspaceLinks(context.Background(), root); err != nil {
		t.Fatalf("VerifyPlanWorkspaceLinks: %v", err)
	}
	lineage, err := DiscoverPlanNodes(root)
	if err != nil {
		t.Fatalf("DiscoverPlanNodes: %v", err)
	}
	if len(lineage.Children) != 1 {
		t.Fatalf(
			"children = %d, want only implementation-review follow-up",
			len(lineage.Children),
		)
	}
	parentSlug, err := WorkspaceSlugForPlan("cn-agents", lineage.RootRelPath)
	if err != nil {
		t.Fatalf("parent slug: %v", err)
	}
	childSlug, err := WorkspaceSlugForPlan("cn-agents", lineage.Children[0].RootRelPath)
	if err != nil {
		t.Fatalf("child slug: %v", err)
	}
	if parentSlug == childSlug {
		t.Fatalf("parent and child slugs should differ: %q", parentSlug)
	}
}

func TestLinkPlanNodeMissingWorkspaceKeepsAdvisoryState(t *testing.T) {
	t.Parallel()

	svc := &Service{projectName: "cn-agents"}
	node := PlanNode{
		RootRelPath: "reviews/2026-05-13_12-00-00_fast-iteration-workspace-ux_implementation-review",
	}
	svc.linkPlanNode(&node, map[string]workspaces.Workspace{})

	if node.Workspace == nil || node.Workspace.Slug == "" {
		t.Fatalf("workspace link missing: %#v", node.Workspace)
	}
	if node.Workspace.Status != string(workspaces.StatusStopped) {
		t.Fatalf("status = %q, want stopped", node.Workspace.Status)
	}
	if node.Stack == nil || node.Stack.Available ||
		!strings.Contains(node.Stack.Detail, "not found") {
		t.Fatalf("stack = %#v, want unavailable not-found detail", node.Stack)
	}
}

func writePlanMarkersForTest(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	for _, name := range []string{"AGENTS.md", "design.md", "outline.md", "plan.md"} {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte(name),
			0o644,
		); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
}
