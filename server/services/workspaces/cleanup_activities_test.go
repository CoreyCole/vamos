package workspaces

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

type fakeImplWorkspaceCleanupMarker struct {
	slugs []string
	err   error
}

func (f *fakeImplWorkspaceCleanupMarker) MarkImplWorkspaceCleanedUp(ctx context.Context, arg db.MarkImplWorkspaceCleanedUpParams) (int64, error) {
	f.slugs = append(f.slugs, arg.WorkspaceSlug)
	return 1, f.err
}

func TestCleanupActivityRemovesCheckoutMarksRowAndPreservesPlanDir(t *testing.T) {
	m, checkout := newTestManager(t)
	planDir := filepath.Join(filepath.Dir(checkout), "thoughts", "creative-mode-agent", "plans", "demo")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := &fakeImplWorkspaceCleanupMarker{}
	activities := &CleanupActivities{Manager: m, Store: marker}

	err := activities.CleanupWorkspace(context.Background(), WorkspaceCleanupWorkflowInput{
		Slug:         "foo",
		TransitionID: "cleanup-1",
		Disposition:  WorkspaceCleanupDispositionUnmerged,
		Confirmed:    true,
	})
	if err != nil {
		t.Fatalf("CleanupWorkspace: %v", err)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Fatalf("checkout stat err=%v want not exist", err)
	}
	if _, err := os.Stat(planDir); err != nil {
		t.Fatalf("plan dir should be preserved: %v", err)
	}
	if len(marker.slugs) != 1 || marker.slugs[0] != "foo" {
		t.Fatalf("marked slugs=%v want [foo]", marker.slugs)
	}
}

func TestCleanupActivityRetryAfterRemovalIsDuplicateSuccess(t *testing.T) {
	m, checkout := newTestManager(t)
	activities := &CleanupActivities{Manager: m, Store: &fakeImplWorkspaceCleanupMarker{}}
	input := WorkspaceCleanupWorkflowInput{Slug: "foo", TransitionID: "cleanup-1", Disposition: WorkspaceCleanupDispositionMerged}

	if err := activities.CleanupWorkspace(context.Background(), input); err != nil {
		t.Fatalf("first CleanupWorkspace: %v", err)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Fatalf("checkout stat err=%v want not exist", err)
	}
	if err := activities.CleanupWorkspace(context.Background(), input); err != nil {
		t.Fatalf("retry CleanupWorkspace: %v", err)
	}
}

func TestCleanupActivityRejectsUnmergedWithoutConfirmation(t *testing.T) {
	m, checkout := newTestManager(t)
	activities := &CleanupActivities{Manager: m}
	err := activities.CleanupWorkspace(context.Background(), WorkspaceCleanupWorkflowInput{
		Slug:        "foo",
		Disposition: WorkspaceCleanupDispositionUnmerged,
	})
	if err == nil {
		t.Fatal("CleanupWorkspace err=nil want confirmation error")
	}
	if _, statErr := os.Stat(checkout); statErr != nil {
		t.Fatalf("checkout should remain: %v", statErr)
	}
}

func TestWorkspaceCleanupActionRejectsConfiguredCheckout(t *testing.T) {
	action := workspaceCleanupAction(ImplWorkspaceView{
		Runtime: snapshotFromState(Workspace{Slug: "work", CheckoutPath: "/repo/work", IsConfigured: true}, WorkspaceLifecycleState{}),
	})
	if !action.Disabled || action.DisabledReason != "configured checkout cannot be cleaned up" {
		t.Fatalf("cleanup action = %+v, want configured checkout disabled", action)
	}
}

func TestWorkspaceCleanupActionRequiresStrongProofForMergedRows(t *testing.T) {
	safe := workspaceCleanupAction(ImplWorkspaceView{Row: db.ImplWorkspace{
		WorkspaceSlug:     "safe",
		Status:            string(ImplWorkspaceStatusMerged),
		CleanupProofKind:  string(MergeProofAncestor),
		CleanupRiskReason: nullableString("ignored after strong proof"),
	}})
	if safe.Disabled || safe.Label != "Clean up" || safe.Disposition != WorkspaceCleanupDispositionMerged {
		t.Fatalf("safe action = %+v, want enabled merged cleanup", safe)
	}

	unknown := workspaceCleanupAction(ImplWorkspaceView{Row: db.ImplWorkspace{
		WorkspaceSlug:     "unknown",
		Status:            string(ImplWorkspaceStatusMerged),
		CleanupProofKind:  string(MergeProofUnknown),
		CleanupRiskReason: nullableString("fetch failed"),
	}})
	if !unknown.Disabled || unknown.DisabledReason != "fetch failed" {
		t.Fatalf("unknown action = %+v, want disabled with risk reason", unknown)
	}
}

func TestCleanupActivityRejectsConfiguredCheckout(t *testing.T) {
	m, checkout := newTestManager(t)
	m.discovery.ConfiguredCheckouts = map[string]ConfiguredCheckout{
		"stage": {RootPath: checkout, DisplayName: "Stage"},
	}
	activities := &CleanupActivities{Manager: m}
	err := activities.CleanupWorkspace(context.Background(), WorkspaceCleanupWorkflowInput{
		Slug:        "foo",
		Disposition: WorkspaceCleanupDispositionMerged,
	})
	if err == nil {
		t.Fatal("CleanupWorkspace err=nil want configured checkout error")
	}
	if _, statErr := os.Stat(checkout); statErr != nil {
		t.Fatalf("checkout should remain: %v", statErr)
	}
}
