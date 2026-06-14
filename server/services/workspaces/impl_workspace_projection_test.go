package workspaces

import (
	"database/sql"
	"strings"
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

func TestBuildImplWorkspaceViewsKeepsActiveRowVisibleWithoutRuntime(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "2026-05-24-01-04-46-page-reload-ux",
		CheckoutPath:  "/tmp/vamos-2026-05-24_01-04-46_page-reload-ux",
		DisplayName:   "Page reload UX",
		Status:        string(ImplWorkspaceStatusActive),
	}}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want active row visible: %+v", len(got), got)
	}
	view := got[0]
	if view.HasRuntime {
		t.Fatalf("HasRuntime = true, want DB-only projection")
	}
	if view.Runtime.Workspace.Status != StatusStopped ||
		view.Runtime.Workspace.Slug != rows[0].WorkspaceSlug ||
		view.Runtime.Workspace.CheckoutPath != rows[0].CheckoutPath {
		t.Fatalf("synthesized runtime = %+v", view.Runtime.Workspace)
	}
}

func TestBuildImplWorkspaceViewsKeepsActiveRowVisibleWithCrashedRuntime(t *testing.T) {
	rows := []db.ImplWorkspace{{
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/cn-agents-feature",
		DisplayName:   "Feature row",
		Status:        string(ImplWorkspaceStatusActive),
	}}
	runtime := []WorkspaceLifecycleSnapshot{snapshotFromState(Workspace{
		Slug:         "feature",
		DisplayName:  "Feature runtime",
		CheckoutPath: "/repo/cn-agents-feature",
		Status:       StatusCrashed,
		Error:        "exit status 1",
	}, WorkspaceLifecycleState{Error: "exit status 1"})}

	got := BuildImplWorkspaceViews(rows, runtime, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want active row visible: %+v", len(got), got)
	}
	view := got[0]
	if !view.HasRuntime || view.Runtime.Workspace.Status != StatusCrashed ||
		view.Runtime.Error != "exit status 1" {
		t.Fatalf("view = %+v, want crashed runtime attached to visible active row", view)
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

func TestBuildImplWorkspaceViewsKeepsSamePlanDifferentProjectsAsSiblings(t *testing.T) {
	rows := []db.ImplWorkspace{
		{
			ProjectID:     "vamos",
			WorkspaceSlug: "vamos-parent",
			CheckoutPath:  "/repo/vamos-parent",
			DisplayName:   "Vamos parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-06-01_multi-project",
				Valid:  true,
			},
		},
		{
			ProjectID:     "datastarui",
			WorkspaceSlug: "datastarui-parent",
			CheckoutPath:  "/repo/datastarui-parent",
			DisplayName:   "DatastarUI parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-06-01_multi-project",
				Valid:  true,
			},
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want two sibling roots: %+v", len(got), got)
	}
	if got[0].Row.WorkspaceSlug != "vamos-parent" || got[1].Row.WorkspaceSlug != "datastarui-parent" {
		t.Fatalf("got = %+v, want both parent rows preserved in input order", got)
	}
}

func TestBuildImplWorkspaceViewsNestsReviewPlanRowsByProject(t *testing.T) {
	rows := []db.ImplWorkspace{
		{
			ProjectID:     "vamos",
			WorkspaceSlug: "vamos-parent",
			CheckoutPath:  "/repo/vamos-parent",
			DisplayName:   "Vamos parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-06-01_multi-project",
				Valid:  true,
			},
		},
		{
			ProjectID:     "datastarui",
			WorkspaceSlug: "datastarui-parent",
			CheckoutPath:  "/repo/datastarui-parent",
			DisplayName:   "DatastarUI parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-06-01_multi-project",
				Valid:  true,
			},
		},
		{
			ProjectID:     "datastarui",
			WorkspaceSlug: "datastarui-review",
			CheckoutPath:  "/repo/datastarui-review",
			DisplayName:   "DatastarUI review",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/2026-06-01_multi-project/reviews/2026-06-01_multi-project_implementation-review",
				Valid:  true,
			},
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want two roots: %+v", len(got), got)
	}
	if len(got[0].Children) != 0 {
		t.Fatalf("vamos children = %+v, want none", got[0].Children)
	}
	if len(got[1].Children) != 1 || got[1].Children[0].Row.WorkspaceSlug != "datastarui-review" {
		t.Fatalf("datastarui children = %+v, want review child", got[1].Children)
	}
}

func TestBuildImplWorkspaceViewsKeepsAmbiguousReviewPlanRowsTopLevel(t *testing.T) {
	rows := []db.ImplWorkspace{
		{
			ProjectID:     "vamos",
			WorkspaceSlug: "vamos-parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel:    sql.NullString{String: "creative-mode-agent/plans/demo", Valid: true},
		},
		{
			ProjectID:     "datastarui",
			WorkspaceSlug: "datastarui-parent",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel:    sql.NullString{String: "creative-mode-agent/plans/demo", Valid: true},
		},
		{
			WorkspaceSlug: "review-child",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel:    sql.NullString{String: "creative-mode-agent/plans/demo/reviews/review", Valid: true},
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 3 || got[2].Row.WorkspaceSlug != "review-child" || len(got[2].Children) != 0 {
		t.Fatalf("got = %+v, want ambiguous review child as top-level row", got)
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
			WorkspaceSlug:    "merged",
			DisplayName:      "Merged",
			CheckoutPath:     "/repo/merged",
			Status:           string(ImplWorkspaceStatusMerged),
			CleanupProofKind: string(MergeProofAncestor),
		},
	}

	got := BuildImplWorkspaceViews(rows, nil, WorkspaceLifecycleSnapshot{})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Row.Status != string(ImplWorkspaceStatusCleanedUp) ||
		got[0].Cleanup.Group != CleanupGroupCleanedUp ||
		got[1].Row.Status != string(ImplWorkspaceStatusMerged) ||
		got[1].Cleanup.Group != CleanupGroupSafeToCleanup {
		t.Fatalf("got = %+v", got)
	}
}

func TestDetectLifecycleRuntimeConflictsMergedRunning(t *testing.T) {
	row := db.ImplWorkspace{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/feature",
		Status:        string(ImplWorkspaceStatusMerged),
	}
	runtime := snapshotFromState(Workspace{
		ProjectID:    row.ProjectID,
		Slug:         row.WorkspaceSlug,
		CheckoutPath: row.CheckoutPath,
		Status:       StatusRunning,
	}, WorkspaceLifecycleState{DesiredState: WorkspaceDesiredRunning})

	got := DetectLifecycleRuntimeConflicts(row, runtime, true)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1: %+v", len(got), got)
	}
	if got[0].Source != WorkspaceDiagnosticSourceLocalRun || got[0].Severity != WorkspaceDiagnosticWarning {
		t.Fatalf("diagnostic = %+v, want local runtime warning", got[0])
	}
	if got[0].ProjectID != row.ProjectID || got[0].WorkspaceSlug != row.WorkspaceSlug || got[0].CheckoutPath != row.CheckoutPath {
		t.Fatalf("diagnostic target = %+v, want row target", got[0])
	}
	if !strings.Contains(got[0].Message, "stale") || !strings.Contains(got[0].Detail, "manager DB lifecycle=merged") {
		t.Fatalf("diagnostic copy = %+v, want stale manager lifecycle detail", got[0])
	}
}

func TestDetectLifecycleRuntimeConflictsCleanedCrashed(t *testing.T) {
	row := db.ImplWorkspace{
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/feature",
		Status:        string(ImplWorkspaceStatusCleanedUp),
	}
	runtime := snapshotFromState(Workspace{
		Slug:         row.WorkspaceSlug,
		CheckoutPath: row.CheckoutPath,
		Status:       StatusCrashed,
	}, WorkspaceLifecycleState{})

	got := BuildWorkspaceLifecycleDiagnostic(row, runtime, true, WorkspaceSyncDiagnostic{})
	if got.Lifecycle != ImplWorkspaceStatusCleanedUp || got.RuntimeStatus != string(StatusCrashed) {
		t.Fatalf("diagnostic = %+v, want cleaned lifecycle and crashed runtime", got)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "local_runtime_conflicts_with_manager_lifecycle" {
		t.Fatalf("diagnostics = %+v, want conflict warning", got.Diagnostics)
	}
	if !strings.Contains(got.CleanupMessage, "human approval") {
		t.Fatalf("cleanup message = %q, want human approval warning", got.CleanupMessage)
	}
}

func TestBuildWorkspaceLifecycleDiagnosticDoesNotOverrideLifecycle(t *testing.T) {
	row := db.ImplWorkspace{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/feature",
		Status:        string(ImplWorkspaceStatusMerged),
	}
	runtime := snapshotFromState(Workspace{
		ProjectID:    row.ProjectID,
		Slug:         row.WorkspaceSlug,
		CheckoutPath: row.CheckoutPath,
		Status:       StatusRunning,
	}, WorkspaceLifecycleState{})

	got := BuildWorkspaceLifecycleDiagnostic(row, runtime, true, WorkspaceSyncDiagnostic{})
	if got.Lifecycle != ImplWorkspaceStatusMerged || got.LifecycleSource != WorkspaceDiagnosticSourceManagerDB {
		t.Fatalf("lifecycle = %q source %q, want merged from manager DB", got.Lifecycle, got.LifecycleSource)
	}
	if got.RuntimeStatus != string(StatusRunning) || got.RuntimeSource != WorkspaceDiagnosticSourceLocalRun {
		t.Fatalf("runtime = %q source %q, want running local diagnostics", got.RuntimeStatus, got.RuntimeSource)
	}
}

func TestBuildWorkspaceLifecycleDiagnosticFiltersSyncWarningsByTarget(t *testing.T) {
	row := db.ImplWorkspace{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/feature",
		Status:        string(ImplWorkspaceStatusActive),
	}
	sync := WorkspaceSyncDiagnostic{Warnings: []WorkspaceDiagnostic{
		{Source: WorkspaceDiagnosticSourceSync, Severity: WorkspaceDiagnosticWarning, Code: "match_slug", ProjectID: row.ProjectID, WorkspaceSlug: row.WorkspaceSlug},
		{Source: WorkspaceDiagnosticSourceSync, Severity: WorkspaceDiagnosticWarning, Code: "match_path", CheckoutPath: row.CheckoutPath},
		{Source: WorkspaceDiagnosticSourceSync, Severity: WorkspaceDiagnosticWarning, Code: "other", ProjectID: row.ProjectID, WorkspaceSlug: "other", CheckoutPath: "/repo/other"},
	}}

	got := BuildWorkspaceLifecycleDiagnostic(row, WorkspaceLifecycleSnapshot{}, false, sync)
	if len(got.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v, want two matching warnings", got.Diagnostics)
	}
	codes := map[string]bool{}
	for _, diagnostic := range got.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if !codes["match_slug"] || !codes["match_path"] || codes["other"] {
		t.Fatalf("codes = %+v, want only matching warnings", codes)
	}
}

func TestBuildImplWorkspaceViewsAttachesDiagnostics(t *testing.T) {
	rows := []db.ImplWorkspace{{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "feature",
		CheckoutPath:  "/repo/feature",
		Status:        string(ImplWorkspaceStatusMerged),
	}}
	runtime := []WorkspaceLifecycleSnapshot{snapshotFromState(Workspace{
		ProjectID:    rows[0].ProjectID,
		Slug:         rows[0].WorkspaceSlug,
		CheckoutPath: rows[0].CheckoutPath,
		Status:       StatusRunning,
	}, WorkspaceLifecycleState{})}

	got := BuildImplWorkspaceViews(rows, runtime, WorkspaceLifecycleSnapshot{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Diagnostics.Lifecycle != ImplWorkspaceStatusMerged || len(got[0].Diagnostics.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want merged lifecycle conflict", got[0].Diagnostics)
	}
}

func TestGroupImplWorkspaceViewsByCleanupReadiness(t *testing.T) {
	t.Parallel()

	views := BuildImplWorkspaceViews([]db.ImplWorkspace{
		{WorkspaceSlug: "active", Status: string(ImplWorkspaceStatusActive), AheadCount: 2},
		{WorkspaceSlug: "merged", Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofAncestor)},
		{WorkspaceSlug: "cleaned", Status: string(ImplWorkspaceStatusCleanedUp)},
	}, nil, WorkspaceLifecycleSnapshot{})

	groups := groupImplWorkspaceViews(views, nil, false)
	if len(groups.NeedsAttention) != 1 || workspaceViewSlug(groups.NeedsAttention[0]) != "active" {
		t.Fatalf("needs attention = %+v", groups.NeedsAttention)
	}
	if len(groups.SafeToCleanup) != 1 || workspaceViewSlug(groups.SafeToCleanup[0]) != "merged" {
		t.Fatalf("safe to cleanup = %+v", groups.SafeToCleanup)
	}
	if len(groups.CleanedUp) != 0 {
		t.Fatalf("cleaned hidden = %+v, want empty", groups.CleanedUp)
	}

	groups = groupImplWorkspaceViews(views, nil, true)
	if len(groups.CleanedUp) != 1 || workspaceViewSlug(groups.CleanedUp[0]) != "cleaned" {
		t.Fatalf("cleaned shown = %+v", groups.CleanedUp)
	}
}

func TestGroupImplWorkspaceViewsPromotesActiveChildren(t *testing.T) {
	t.Parallel()

	views := []ImplWorkspaceView{{
		Row:     db.ImplWorkspace{WorkspaceSlug: "parent", Status: string(ImplWorkspaceStatusCleanedUp)},
		Cleanup: CleanupReadiness{Group: CleanupGroupCleanedUp},
		Children: []ImplWorkspaceView{{
			Row:     db.ImplWorkspace{WorkspaceSlug: "child", Status: string(ImplWorkspaceStatusActive)},
			Cleanup: CleanupReadiness{Group: CleanupGroupNeedsAttention},
		}},
	}}

	groups := groupImplWorkspaceViews(views, nil, false)
	if len(groups.NeedsAttention) != 1 || workspaceViewSlug(groups.NeedsAttention[0]) != "child" {
		t.Fatalf("needs attention = %+v, want active child promoted", groups.NeedsAttention)
	}
}

func TestGroupImplWorkspaceViewsKeepsProtectedLaneVisible(t *testing.T) {
	t.Parallel()

	views := []ImplWorkspaceView{{
		Row:     db.ImplWorkspace{WorkspaceSlug: "stage", Status: string(ImplWorkspaceStatusCleanedUp)},
		Cleanup: CleanupReadiness{Group: CleanupGroupCleanedUp},
	}}
	protected := map[string]ReleaseLaneWorkspace{"stage": {Slug: "stage", Protected: true}}

	groups := groupImplWorkspaceViews(views, protected, false)
	if len(groups.NeedsAttention) != 1 || workspaceViewSlug(groups.NeedsAttention[0]) != "stage" {
		t.Fatalf("needs attention = %+v, want protected lane visible", groups.NeedsAttention)
	}
	if action := workspaceCleanupAction(groups.NeedsAttention[0]); !action.Disabled {
		t.Fatalf("cleanup action = %+v, want protected-like cleaned row uncleanable", action)
	}
}
