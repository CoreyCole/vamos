package workspaces

import (
	"database/sql"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestBuildImplWorkspaceViewsPinsMain(t *testing.T) {
	main := snapshotFromState(Workspace{
		Slug:         mainWorkspaceSlug,
		DisplayName:  "main",
		CheckoutPath: "/repo/cn-agents",
		Status:       StatusRunning,
		IsMain:       true,
	}, WorkspaceLifecycleState{})
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/cn-agents-feature",
		DisplayName:   "Feature",
		Status:        string(ImplWorkspaceStatusActive),
	}}

	got := BuildImplWorkspaceViews(rows, nil, main)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if !got[0].IsMain || got[0].Row.WorkspaceSlug != mainWorkspaceSlug {
		t.Fatalf("first view = %+v, want pinned main", got[0])
	}
	if got[1].Row.WorkspaceSlug != "feature" {
		t.Fatalf("second view = %+v, want feature", got[1])
	}
}

func TestOrderReleaseLaneViewsFirstPinsMainThenStage(t *testing.T) {
	t.Parallel()

	views := []ImplWorkspaceView{
		{Row: db.ImplWorkspace{WorkspaceSlug: "feature", Status: string(ImplWorkspaceStatusActive)}},
		{Row: db.ImplWorkspace{WorkspaceSlug: "stage-custom", Status: string(ImplWorkspaceStatusActive)}},
		{Row: db.ImplWorkspace{WorkspaceSlug: "trunk-custom", Status: string(ImplWorkspaceStatusActive)}},
	}
	lanes := []ReleaseLaneWorkspace{
		{Role: ReleaseLaneRoleStage, Slug: "stage-custom"},
		{Role: ReleaseLaneRoleMain, Slug: "trunk-custom"},
	}

	got := orderReleaseLaneViewsFirst(views, lanes)
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if workspaceViewSlug(got[0]) != "trunk-custom" || workspaceViewSlug(got[1]) != "stage-custom" || workspaceViewSlug(got[2]) != "feature" {
		t.Fatalf("order = [%q %q %q], want main stage feature", workspaceViewSlug(got[0]), workspaceViewSlug(got[1]), workspaceViewSlug(got[2]))
	}
}

func TestBuildImplWorkspaceViewsMatchesRuntimeBySlug(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/stale-path",
		DisplayName:   "Feature row",
		Status:        string(ImplWorkspaceStatusActive),
	}}
	runtime := []WorkspaceLifecycleSnapshot{snapshotFromState(Workspace{
		Slug:         "feature",
		DisplayName:  "Feature runtime",
		CheckoutPath: "/repo/runtime-path",
		Status:       StatusRunning,
	}, WorkspaceLifecycleState{})}

	got := BuildImplWorkspaceViews(rows, runtime, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 || !got[0].HasRuntime {
		t.Fatalf("got = %+v, want runtime match", got)
	}
	if got[0].Runtime.Workspace.CheckoutPath != "/repo/runtime-path" {
		t.Fatalf("runtime checkout = %q", got[0].Runtime.Workspace.CheckoutPath)
	}
}

func TestBuildImplWorkspaceViewsMatchesRuntimeByPath(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "feature-row",
		CheckoutPath:  "/repo/cn-agents-feature/",
		DisplayName:   "Feature row",
		Status:        string(ImplWorkspaceStatusActive),
	}}
	runtime := []WorkspaceLifecycleSnapshot{snapshotFromState(Workspace{
		Slug:         "feature-runtime",
		DisplayName:  "Feature runtime",
		CheckoutPath: "/repo/cn-agents-feature",
		Status:       StatusRunning,
	}, WorkspaceLifecycleState{})}

	got := BuildImplWorkspaceViews(rows, runtime, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 || !got[0].HasRuntime {
		t.Fatalf("got = %+v, want runtime match", got)
	}
	if got[0].Runtime.Workspace.Slug != "feature-runtime" {
		t.Fatalf("runtime slug = %q", got[0].Runtime.Workspace.Slug)
	}
}

func TestBuildImplWorkspaceViewsSynthesizesStoppedRuntimeFromImplRow(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/cn-agents-feature",
		DisplayName:   "Feature",
		Status:        string(ImplWorkspaceStatusActive),
		Branch:        sql.NullString{String: "feature-branch", Valid: true},
		CommitHash:    sql.NullString{String: "abc123", Valid: true},
		TopBranch:     sql.NullString{String: "feature-top", Valid: true},
	}}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d", len(got))
	}
	view := got[0]
	if view.HasRuntime {
		t.Fatalf("HasRuntime = true, want synthesized non-runtime")
	}
	if view.Runtime.Workspace.Status != StatusStopped ||
		view.Runtime.Workspace.Branch != "feature-branch" ||
		view.Runtime.Workspace.Commit != "abc123" {
		t.Fatalf("synthesized runtime = %+v", view.Runtime.Workspace)
	}
	if view.Runtime.Workspace.Stack.TopBranch != "feature-top" {
		t.Fatalf("stack = %+v", view.Runtime.Workspace.Stack)
	}
}

func TestBuildImplWorkspaceViewsNestsReviewPlanRows(t *testing.T) {
	rows := []db.ImplWorkspace{
		{
			WorkspaceSlug: "parent",
			CheckoutPath:  "/repo/cn-agents-parent",
			DisplayName:   "Parent workspace",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-05-20_20-18-45_workspace-discovery-sync",
				Valid:  true,
			},
		},
		{
			WorkspaceSlug: "review-child",
			CheckoutPath:  "/repo/cn-agents-review-child",
			DisplayName:   "Review child workspace",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-05-20_20-18-45_workspace-discovery-sync/reviews/2026-05-21_00-12-21_workspace-discovery-sync_implementation-review",
				Valid:  true,
			},
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want parent root only: %+v", len(got), got)
	}
	if got[0].Row.WorkspaceSlug != "parent" {
		t.Fatalf("root = %+v, want parent", got[0])
	}
	if len(got[0].Children) != 1 ||
		got[0].Children[0].Row.WorkspaceSlug != "review-child" {
		t.Fatalf("children = %+v, want review child", got[0].Children)
	}
}

func TestBuildImplWorkspaceViewsKeepsOrphanReviewPlanRowsTopLevel(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "review-child",
		CheckoutPath:  "/repo/cn-agents-review-child",
		DisplayName:   "Review child workspace",
		Status:        string(ImplWorkspaceStatusActive),
		PlanDirRel: sql.NullString{
			String: "creative-mode-agent/plans/missing-parent/reviews/review-dir",
			Valid:  true,
		},
	}}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 || got[0].Row.WorkspaceSlug != "review-child" ||
		len(got[0].Children) != 0 {
		t.Fatalf("got = %+v, want orphan child as top-level row", got)
	}
}

func TestImplViewsToNavWorkspacesKeepsNestedRowsFlat(t *testing.T) {
	views := []ImplWorkspaceView{{
		Row: db.ImplWorkspace{
			WorkspaceSlug: "parent",
			DisplayName:   "Parent",
			CheckoutPath:  "/repo/parent",
			Status:        string(ImplWorkspaceStatusActive),
		},
		Children: []ImplWorkspaceView{{
			Row: db.ImplWorkspace{
				WorkspaceSlug: "child",
				DisplayName:   "Child",
				CheckoutPath:  "/repo/child",
				Status:        string(ImplWorkspaceStatusActive),
			},
		}},
	}}

	got := ImplViewsToNavWorkspaces(views)
	if len(got) != 2 || got[0].Slug != "parent" || got[1].Slug != "child" {
		t.Fatalf("got = %+v, want flat parent and child nav rows", got)
	}
}

func TestImplViewsToNavWorkspacesDoesNotIncludePlanOnlyRows(t *testing.T) {
	views := []ImplWorkspaceView{
		{
			Row: db.ImplWorkspace{
				WorkspaceSlug: "active-row",
				DisplayName:   "Active row",
				CheckoutPath:  "/repo/active-row",
				Status:        string(ImplWorkspaceStatusActive),
			},
		},
		{
			Row: db.ImplWorkspace{
				WorkspaceSlug: "cleaned-up-row",
				DisplayName:   "Cleaned up row",
				CheckoutPath:  "/repo/cleaned-up-row",
				Status:        string(ImplWorkspaceStatusCleanedUp),
			},
		},
	}

	got := ImplViewsToNavWorkspaces(views)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %+v", len(got), got)
	}
	if got[0].Slug != "active-row" {
		t.Fatalf("nav row = %+v", got[0])
	}
}

func TestBuildImplWorkspaceViewsPreservesCleanedUpMergedRowsAsHistory(t *testing.T) {
	rows := []db.ImplWorkspace{
		{
			WorkspaceSlug: "cleaned",
			DisplayName:   "Cleaned",
			CheckoutPath:  "/repo/cleaned",
			Status:        string(ImplWorkspaceStatusCleanedUp),
		},
		{
			WorkspaceSlug: "merged",
			DisplayName:   "Merged",
			CheckoutPath:  "/repo/merged",
			Status:        string(ImplWorkspaceStatusMerged),
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Row.Status != string(ImplWorkspaceStatusCleanedUp) ||
		got[1].Row.Status != string(ImplWorkspaceStatusMerged) {
		t.Fatalf("got = %+v", got)
	}
}
