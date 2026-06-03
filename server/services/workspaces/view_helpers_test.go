package workspaces

import (
	"database/sql"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

func dbImplWorkspace(slug, checkoutPath, status string) db.ImplWorkspace {
	return db.ImplWorkspace{WorkspaceSlug: slug, CheckoutPath: checkoutPath, Status: status}
}

func TestWorkspaceCardTitleFormatsTimestampedSlug(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "2026-05-11-06-13-26-workspace-subdomains-https-caddy-review"}
	if got, want := workspaceCardTitle(
		ws,
	), "Workspace Subdomains HTTPS Caddy Review"; got != want {
		t.Fatalf("workspaceCardTitle() = %q, want %q", got, want)
	}
}

func TestWorkspaceCardTimestampFormatsSlugTimestamp(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "2026-05-11-06-13-26-workspace-subdomains-https-caddy-review"}
	if got, want := workspaceCardTimestamp(ws), "May 11, 2026 · 6:13 AM"; got != want {
		t.Fatalf("workspaceCardTimestamp() = %q, want %q", got, want)
	}
}

func TestWorkspaceCardTitleFormatsMain(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "main", IsMain: true}
	if got, want := workspaceCardTitle(ws), "Main"; got != want {
		t.Fatalf("workspaceCardTitle() = %q, want %q", got, want)
	}
	if got := workspaceCardTimestamp(ws); got != "" {
		t.Fatalf("workspaceCardTimestamp() = %q, want empty", got)
	}
}

func TestWorkspaceActionIndicatorIsScopedBySlugAndAction(t *testing.T) {
	t.Parallel()

	start := workspaceActionIndicator("feature", "start")
	if start == "" || start[0] != '_' {
		t.Fatalf("workspaceActionIndicator() = %q, want private signal name", start)
	}
	if got := workspaceActionIndicatorSignal("feature", "start"); got != "$"+start {
		t.Fatalf("workspaceActionIndicatorSignal() = %q", got)
	}
	if got := workspaceActionIndicator("other", "start"); got == start {
		t.Fatalf("different workspace reused indicator %q", got)
	}
	if got := workspaceActionIndicator("feature", "stop"); got == start {
		t.Fatalf("different action reused indicator %q", got)
	}
}

func TestWorkspaceActionHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		observed          WorkspaceObservedState
		wantStart         bool
		wantStop          bool
		wantRestart       bool
		wantTransitioning bool
	}{
		{name: "starting", observed: WorkspaceObservedStarting, wantTransitioning: true},
		{name: "stopping", observed: WorkspaceObservedStopping, wantTransitioning: true},
		{name: "stopped", observed: WorkspaceObservedStopped, wantStart: true},
		{name: "failed", observed: WorkspaceObservedFailed, wantStart: true},
		{name: "crashed", observed: WorkspaceObservedCrashed, wantStart: true},
		{
			name:        "running",
			observed:    WorkspaceObservedRunning,
			wantStop:    true,
			wantRestart: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snap := WorkspaceLifecycleSnapshot{
				Workspace:     Workspace{Slug: "feature"},
				ObservedState: tc.observed,
			}
			if got := workspaceCanStart(snap); got != tc.wantStart {
				t.Fatalf("workspaceCanStart() = %v, want %v", got, tc.wantStart)
			}
			if got := workspaceCanStop(snap); got != tc.wantStop {
				t.Fatalf("workspaceCanStop() = %v, want %v", got, tc.wantStop)
			}
			if got := workspaceCanRestart(snap); got != tc.wantRestart {
				t.Fatalf("workspaceCanRestart() = %v, want %v", got, tc.wantRestart)
			}
			if got := workspaceTransitioning(snap); got != tc.wantTransitioning {
				t.Fatalf(
					"workspaceTransitioning() = %v, want %v",
					got,
					tc.wantTransitioning,
				)
			}
		})
	}
}

func TestWorkspaceActionHelpersDisallowMainWorkspace(t *testing.T) {
	t.Parallel()

	snap := WorkspaceLifecycleSnapshot{
		Workspace:     Workspace{Slug: "main", IsMain: true},
		ObservedState: WorkspaceObservedRunning,
	}
	if workspaceCanStart(snap) || workspaceCanStop(snap) || workspaceCanRestart(snap) {
		t.Fatalf("main workspace should not allow lifecycle actions")
	}
}

func TestWorkspaceTransitionLabelFallsBackToWorkspaceStatus(t *testing.T) {
	t.Parallel()

	snap := WorkspaceLifecycleSnapshot{Workspace: Workspace{Status: StatusStopped}}
	if got := workspaceTransitionLabel(snap); got != "stopped" {
		t.Fatalf("workspaceTransitionLabel() = %q, want stopped", got)
	}
}

func TestIsHistoricalImplWorkspaceViewUsesFilesystemBackedRuntime(t *testing.T) {
	t.Parallel()

	activeRuntime := ImplWorkspaceView{
		Row: dbImplWorkspace("active", "/repo/active", string(ImplWorkspaceStatusActive)),
		Runtime: snapshotFromState(
			Workspace{Slug: "active", CheckoutPath: "/repo/active"},
			WorkspaceLifecycleState{},
		),
		HasRuntime: true,
	}
	if isHistoricalImplWorkspaceView(activeRuntime, nil) {
		t.Fatalf("active runtime-backed workspace should be current")
	}

	missingRuntime := ImplWorkspaceView{
		Row: dbImplWorkspace("missing", "/repo/missing", string(ImplWorkspaceStatusActive)),
	}
	if !isHistoricalImplWorkspaceView(missingRuntime, nil) {
		t.Fatalf("active row without runtime should be historical")
	}

	noCheckout := ImplWorkspaceView{Row: dbImplWorkspace("missing-path", "", string(ImplWorkspaceStatusActive))}
	if !isHistoricalImplWorkspaceView(noCheckout, nil) {
		t.Fatalf("active row without checkout should be historical")
	}

	merged := ImplWorkspaceView{Row: dbImplWorkspace("merged", "/repo/merged", string(ImplWorkspaceStatusMerged))}
	if !isHistoricalImplWorkspaceView(merged, nil) {
		t.Fatalf("merged row should be historical")
	}
}

func TestWorkspaceCleanupReadiness(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		view      ImplWorkspaceView
		wantGroup CleanupGroup
		wantSafe  bool
	}{
		{
			name:      "active ahead needs attention",
			view:      ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusActive), AheadCount: 3}},
			wantGroup: CleanupGroupNeedsAttention,
		},
		{
			name: "merged ancestor safe",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{
				Status:                string(ImplWorkspaceStatusMerged),
				CleanupProofKind:      string(MergeProofAncestor),
				CleanupProofSourceRef: sql.NullString{String: "origin/main", Valid: true},
			}},
			wantGroup: CleanupGroupSafeToCleanup,
			wantSafe:  true,
		},
		{
			name:      "merged patch equivalent safe",
			view:      ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofPatchEquivalent)}},
			wantGroup: CleanupGroupSafeToCleanup,
			wantSafe:  true,
		},
		{
			name:      "merged cached safe",
			view:      ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofCached)}},
			wantGroup: CleanupGroupSafeToCleanup,
			wantSafe:  true,
		},
		{
			name:      "merged unknown needs attention",
			view:      ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofUnknown)}},
			wantGroup: CleanupGroupNeedsAttention,
		},
		{
			name:      "cleaned up history",
			view:      ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusCleanedUp)}},
			wantGroup: CleanupGroupCleanedUp,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := workspaceCleanupReadiness(tc.view)
			if got.Group != tc.wantGroup || got.Safe != tc.wantSafe {
				t.Fatalf("workspaceCleanupReadiness() = %+v, want group %q safe %v", got, tc.wantGroup, tc.wantSafe)
			}
		})
	}
}

func TestWorkspaceReleaseSummaryShowsCleanupReadiness(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		view ImplWorkspaceView
		want string
	}{
		{
			name: "merged safe",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofAncestor)}},
			want: "Merged · safe to clean up",
		},
		{
			name: "merged unknown",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofUnknown)}},
			want: "Merged status lacks strong cleanup proof",
		},
		{
			name: "cleaned up",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusCleanedUp)}},
			want: "Cleaned up",
		},
		{
			name: "active ahead",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusActive), AheadCount: 3}},
			want: "Unmerged · 3 ahead",
		},
		{
			name: "active no release action",
			view: ImplWorkspaceView{Row: db.ImplWorkspace{Status: string(ImplWorkspaceStatusActive)}},
			want: "No release action",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := workspaceReleaseSummary(tc.view); got != tc.want {
				t.Fatalf("workspaceReleaseSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsHistoricalImplWorkspaceViewKeepsMainAndProtectedCurrent(t *testing.T) {
	t.Parallel()

	main := ImplWorkspaceView{Row: dbImplWorkspace("main", "", string(ImplWorkspaceStatusCleanedUp)), IsMain: true}
	if isHistoricalImplWorkspaceView(main, nil) {
		t.Fatalf("main should stay current")
	}

	protected := map[string]ReleaseLaneWorkspace{
		"stage": {Slug: "stage", Protected: true},
	}
	stage := ImplWorkspaceView{Row: dbImplWorkspace("stage", "", string(ImplWorkspaceStatusCleanedUp))}
	if isHistoricalImplWorkspaceView(stage, protected) {
		t.Fatalf("protected release lane should stay current")
	}
}

func TestApplyWorkspacesFilterSearchGroupSortAndHistory(t *testing.T) {
	t.Parallel()

	oldTime := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	views := []ImplWorkspaceView{
		{
			Row: db.ImplWorkspace{
				ProjectID:     "vamos",
				WorkspaceSlug: "merged-weak",
				DisplayName:   "Merged Weak",
				CheckoutPath:  "/repo/merged-weak",
				Status:        string(ImplWorkspaceStatusMerged),
				UpdatedAt:     newTime,
			},
		},
		{
			Row: db.ImplWorkspace{
				ProjectID:     "vamos",
				WorkspaceSlug: "zeta-active",
				DisplayName:   "Zeta Active",
				CheckoutPath:  "/repo/zeta-active",
				Status:        string(ImplWorkspaceStatusActive),
				Branch:        sql.NullString{String: "feature/search-branch", Valid: true},
				CommitHash:    sql.NullString{String: "abcdef123456", Valid: true},
				UpdatedAt:     oldTime,
			},
			Runtime:    snapshotFromState(Workspace{Slug: "zeta-active", CheckoutPath: "/repo/zeta-active", Status: StatusRunning}, WorkspaceLifecycleState{}),
			HasRuntime: true,
			Workflow:   WorkspaceWorkflowSummary{WorkflowType: "qrspi", Stage: "implement", Status: "running"},
			Plan: PlanWorkspaceView{
				PlanDirRel: "thoughts/creative-mode-agent/plans/search-plan",
				Projects:   []PlanWorkspaceProjectView{{ProjectID: "datastarui", Role: "related", Label: "datastarui"}},
				Bindings:   []PlanWorkspaceImplBindingView{{ProjectID: "vamos", Role: "primary", WorkspaceSlug: "zeta-active", CheckoutPath: "/repo/zeta-active", Status: "active"}},
			},
		},
		{
			Row: db.ImplWorkspace{
				ProjectID:     "vamos",
				WorkspaceSlug: "alpha-active",
				DisplayName:   "Alpha Active",
				CheckoutPath:  "/repo/alpha-active",
				Status:        string(ImplWorkspaceStatusActive),
				UpdatedAt:     newTime,
			},
			Runtime:    snapshotFromState(Workspace{Slug: "alpha-active", CheckoutPath: "/repo/alpha-active", Status: StatusRunning}, WorkspaceLifecycleState{}),
			HasRuntime: true,
		},
		{
			Row: db.ImplWorkspace{
				ProjectID:     "vamos",
				WorkspaceSlug: "main",
				DisplayName:   "Main",
				Status:        string(ImplWorkspaceStatusActive),
				UpdatedAt:     oldTime,
			},
			IsMain: true,
		},
	}

	active := applyWorkspacesFilter(views, WorkspacesFilter{}, map[string]ReleaseLaneWorkspace{"main": {Slug: "main", Protected: true}})
	gotSlugs := workspaceViewSlugs(active)
	wantSlugs := []string{"main", "alpha-active", "zeta-active"}
	if !equalStrings(gotSlugs, wantSlugs) {
		t.Fatalf("active/default slugs = %#v, want %#v", gotSlugs, wantSlugs)
	}
	if len(views[1].Plan.Projects) != 1 {
		t.Fatalf("filter mutated original views: %#v", views[1].Plan.Projects)
	}

	searched := applyWorkspacesFilter(views, WorkspacesFilter{Query: "DATASTARUI"}, nil)
	if got := workspaceViewSlugs(searched); !equalStrings(got, []string{"zeta-active"}) {
		t.Fatalf("search by plan project = %#v", got)
	}

	byName := applyWorkspacesFilter(views, WorkspacesFilter{Sort: WorkspacesSortNameAsc}, nil)
	if got := workspaceViewSlugs(byName); !equalStrings(got, []string{"main", "alpha-active", "zeta-active"}) {
		t.Fatalf("name sort slugs = %#v", got)
	}

	history := applyWorkspacesFilter(views, WorkspacesFilter{History: WorkspacesHistoryAll}, nil)
	if got := workspaceViewSlugs(history); !equalStrings(got, []string{"main", "alpha-active", "merged-weak", "zeta-active"}) {
		t.Fatalf("history slugs = %#v", got)
	}
}

func TestGroupFilterImplWorkspaceViews(t *testing.T) {
	t.Parallel()

	views := []ImplWorkspaceView{
		{Row: db.ImplWorkspace{WorkspaceSlug: "active", CheckoutPath: "/repo/active", Status: string(ImplWorkspaceStatusActive)}, Runtime: snapshotFromState(Workspace{Slug: "active", CheckoutPath: "/repo/active"}, WorkspaceLifecycleState{}), HasRuntime: true},
		{Row: db.ImplWorkspace{WorkspaceSlug: "safe", Status: string(ImplWorkspaceStatusMerged), CleanupProofKind: string(MergeProofAncestor)}},
		{Row: db.ImplWorkspace{WorkspaceSlug: "cleaned", Status: string(ImplWorkspaceStatusCleanedUp)}},
	}

	all := groupFilterImplWorkspaceViews(views, WorkspacesFilter{History: WorkspacesHistoryAll}, nil)
	if len(all.NeedsAttention) != 1 || len(all.SafeToCleanup) != 1 || len(all.CleanedUp) != 1 {
		t.Fatalf("all groups = %#v", all)
	}
	needs := groupFilterImplWorkspaceViews(views, WorkspacesFilter{History: WorkspacesHistoryAll, Group: WorkspacesGroupNeedsAttention}, nil)
	if len(needs.NeedsAttention) != 1 || len(needs.SafeToCleanup) != 0 || len(needs.CleanedUp) != 0 {
		t.Fatalf("needs group = %#v", needs)
	}
	safe := groupFilterImplWorkspaceViews(views, WorkspacesFilter{History: WorkspacesHistoryAll, Group: WorkspacesGroupSafeToCleanup}, nil)
	if len(safe.SafeToCleanup) != 1 || len(safe.NeedsAttention) != 0 || len(safe.CleanedUp) != 0 {
		t.Fatalf("safe group = %#v", safe)
	}
	cleanedDefault := groupFilterImplWorkspaceViews(views, WorkspacesFilter{Group: WorkspacesGroupCleanedUp}, nil)
	if len(cleanedDefault.CleanedUp) != 0 {
		t.Fatalf("cleaned default group = %#v", cleanedDefault)
	}
}

func workspaceViewSlugs(views []ImplWorkspaceView) []string {
	out := make([]string, 0, len(views))
	for _, view := range views {
		out = append(out, workspaceViewSlug(view))
	}
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
