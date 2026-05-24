package workspaces

import (
	"context"
	"errors"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/pkg/release"
)

func TestEvaluateReleaseActionsUsesConfiguredCheckoutSlugs(t *testing.T) {
	reg := testReleaseRegistry(t)
	lanes := map[release.LaneID]Workspace{
		"stage": {Slug: "integration", Commit: "stage-1"},
		"main":  {Slug: "trunk", Commit: "main-1", IsMain: true},
	}

	actions := EvaluateReleaseActions(reg, lanes, Workspace{Slug: "feature-a", Commit: "feature-1"})
	if len(actions) != 1 {
		t.Fatalf("actions len = %d, want 1: %+v", len(actions), actions)
	}
	if actions[0].FlowID != "promote_to_stage" || actions[0].TargetLane != "stage" || actions[0].ExpectedTargetCommit != "stage-1" || actions[0].Disabled {
		t.Fatalf("feature action = %+v", actions[0])
	}

	stageActions := EvaluateReleaseActions(reg, lanes, Workspace{Slug: "integration", Commit: "stage-1"})
	if len(stageActions) != 1 || stageActions[0].FlowID != "release_to_main" || stageActions[0].TargetLane != "main" {
		t.Fatalf("stage actions = %+v", stageActions)
	}
}

func TestReleaseProjectionMovesFeatureActionsToRows(t *testing.T) {
	reg := testReleaseRegistry(t)
	projector := &ReleaseProjector{Registry: reg}
	views := lifecycleSnapshotsToImplViews([]WorkspaceLifecycleSnapshot{
		snapshotFromState(Workspace{Slug: "feature-a", Commit: "feature-1"}, WorkspaceLifecycleState{}),
		snapshotFromState(Workspace{Slug: "integration", Commit: "stage-1"}, WorkspaceLifecycleState{}),
		snapshotFromState(Workspace{Slug: "trunk", Commit: "main-1", IsMain: true}, WorkspaceLifecycleState{}),
	})

	panel, rowActions, err := projector.BuildWorkspaceProjection(context.Background(), views)
	if err != nil {
		t.Fatalf("BuildWorkspaceProjection: %v", err)
	}
	for _, lane := range panel.Lanes {
		for _, action := range lane.Actions {
			if action.FlowID == "promote_to_stage" && action.SourceSlug == "feature-a" {
				t.Fatalf("feature action leaked into panel lane actions: %+v", action)
			}
		}
	}
	if len(rowActions["feature-a"]) != 1 {
		t.Fatalf("feature row actions = %+v", rowActions)
	}
	if got := rowActions["feature-a"][0]; got.FlowID != "promote_to_stage" || got.TargetLane != "stage" {
		t.Fatalf("feature row action = %+v", got)
	}
}

func TestResolveReleaseActionFindsRowAction(t *testing.T) {
	reg := testReleaseRegistry(t)
	panel := ReleasePanelModel{Enabled: true}
	views := []ImplWorkspaceView{{
		Row: db.ImplWorkspace{WorkspaceSlug: "feature-a"},
		ReleaseActions: []ReleaseActionView{{
			DefinitionID:         "default",
			DefinitionVersion:    "v1",
			FlowID:               "promote_to_stage",
			SourceSlug:           "feature-a",
			ExpectedSourceCommit: "feature-1",
			ExpectedTargetCommit: "stage-1",
		}},
	}}

	_, action, ok := resolveReleaseAction(panel, views, reg, releaseEnqueueRequest{
		DefinitionID:      "default",
		DefinitionVersion: "v1",
		FlowID:            "promote_to_stage",
		SourceSlug:        "feature-a",
	})
	if !ok {
		t.Fatal("resolveReleaseAction did not find row action")
	}
	if action.SourceSlug != "feature-a" || action.FlowID != "promote_to_stage" {
		t.Fatalf("action = %+v", action)
	}
}

func TestBuildPanelDisablesActionsWhenQueueActive(t *testing.T) {
	reg := testReleaseRegistry(t)
	store := fakeReleaseQueueStore{active: []ReleaseQueueItem{{ID: "item-1", Status: ReleaseQueueStatusRunning}}}
	projector := &ReleaseProjector{Registry: reg, Store: store}
	panel, err := projector.BuildPanel(context.Background(), []ImplWorkspaceView{
		{Runtime: snapshotFromState(Workspace{Slug: "integration", Commit: "stage-1"}, WorkspaceLifecycleState{})},
		{Runtime: snapshotFromState(Workspace{Slug: "trunk", Commit: "main-1", IsMain: true}, WorkspaceLifecycleState{})},
	})
	if err != nil {
		t.Fatalf("BuildPanel: %v", err)
	}
	var found bool
	for _, lane := range panel.Lanes {
		for _, action := range lane.Actions {
			found = true
			if !action.Disabled || action.DisabledReason != "release queue already has active work" {
				t.Fatalf("action = %+v", action)
			}
		}
	}
	if !found {
		t.Fatalf("expected lane action in panel: %+v", panel)
	}
	if len(panel.Queue.Active) != 1 {
		t.Fatalf("active queue = %+v", panel.Queue.Active)
	}
}

func TestInspectReleasePreconditionsCoversDirtyAndCommitMismatch(t *testing.T) {
	inspector := &fakeGitInspector{
		head:   map[string]string{"/feature": "feature-new", "/stage": "stage-1"},
		clean:  map[string]bool{"/stage": false},
		detail: map[string]string{"/stage": " M file.go"},
		behind: 1,
	}
	def := testReleaseRegistry(t).Definitions()[0]
	flow := def.Flows["promote_to_stage"]

	result := InspectReleasePreconditions(context.Background(), inspector, def, flow,
		Workspace{Slug: "feature-a", CheckoutPath: "/feature", Commit: "feature-old"},
		Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage-1"},
	)
	if result.OK {
		t.Fatalf("result.OK = true, want false: %+v", result)
	}
	if result.DisabledReason != "target checkout is dirty: M file.go" {
		t.Fatalf("DisabledReason = %q", result.DisabledReason)
	}
	var sawMismatch bool
	for _, check := range result.Checks {
		if check.ID == "expected_source_commit" && !check.OK {
			sawMismatch = true
		}
	}
	if !sawMismatch {
		t.Fatalf("checks missing source mismatch: %+v", result.Checks)
	}
}

func TestInspectReleasePreconditionsAllowsAbbreviatedProjectedCommits(t *testing.T) {
	inspector := &fakeGitInspector{
		head:   map[string]string{"/feature": "feature-1234567890", "/stage": "stage-1234567890"},
		clean:  map[string]bool{"/stage": true},
		behind: 1,
	}
	def := testReleaseRegistry(t).Definitions()[0]
	flow := def.Flows["promote_to_stage"]

	result := InspectReleasePreconditions(context.Background(), inspector, def, flow,
		Workspace{Slug: "feature-a", CheckoutPath: "/feature", Commit: "feature"},
		Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage"},
	)
	if !result.OK {
		t.Fatalf("result.OK = false: %+v", result)
	}
}

func TestInspectReleasePreconditionsAllowsUnknownSourceCommitInTargetCheckout(t *testing.T) {
	inspector := &fakeGitInspector{
		head:             map[string]string{"/feature": "feature-1", "/stage": "stage-1"},
		clean:            map[string]bool{"/stage": true},
		aheadBehindError: true,
	}
	def := testReleaseRegistry(t).Definitions()[0]
	flow := def.Flows["promote_to_stage"]

	result := InspectReleasePreconditions(context.Background(), inspector, def, flow,
		Workspace{Slug: "feature-a", CheckoutPath: "/feature", Commit: "feature-1"},
		Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage-1"},
	)
	if !result.OK {
		t.Fatalf("result.OK = false: %+v", result)
	}
}

func TestInspectReleasePreconditionsRequiresStageAheadOfMain(t *testing.T) {
	inspector := &fakeGitInspector{
		head:   map[string]string{"/stage": "stage-1", "/main": "main-1"},
		clean:  map[string]bool{"/main": true},
		behind: 0,
	}
	def := testReleaseRegistry(t).Definitions()[0]
	flow := def.Flows["release_to_main"]

	result := InspectReleasePreconditions(context.Background(), inspector, def, flow,
		Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage-1"},
		Workspace{Slug: "trunk", CheckoutPath: "/main", Commit: "main-1"},
	)
	if result.OK || result.DisabledReason != "source is not ahead of target" {
		t.Fatalf("result = %+v", result)
	}
}

func testReleaseRegistry(t *testing.T) *release.Registry {
	t.Helper()
	workflowRegistry := runtime.NewRegistry()
	promote := runtime.New[struct{}]("release.promote_to_stage").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight"}).
		Service("merge", runtime.ServiceSpec{Type: "release.merge"}).
		Edge("preflight", "merge").
		Done("done").
		Edge("merge", "done").
		MustBuild()
	stageToMain := runtime.New[struct{}]("release.stage_to_main").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight"}).
		Service("push", runtime.ServiceSpec{Type: "release.push"}).
		Edge("preflight", "push").
		Done("done").
		Edge("push", "done").
		MustBuild()
	if err := workflowRegistry.Register(promote); err != nil {
		t.Fatalf("register promote workflow: %v", err)
	}
	if err := workflowRegistry.Register(stageToMain); err != nil {
		t.Fatalf("register main workflow: %v", err)
	}
	def := release.NewDefinition("default").
		Lane("stage", release.CheckoutSlug("integration"), release.Label("Stage")).
		Lane("main", release.CheckoutSlug("trunk"), release.Label("Main"), release.Protected()).
		Flow("promote_to_stage", "release.promote_to_stage", release.FlowLabel("Promote to stage"), release.FromFeature(), release.ToLane("stage"), release.NoPush()).
		Flow("release_to_main", "release.stage_to_main", release.FlowLabel("Release to main"), release.FromLane("stage"), release.ToLane("main"), release.PushAfterVerifyPolicy()).
		MustBuild(workflowRegistry)
	reg := release.NewRegistry(workflowRegistry)
	if err := reg.Register(def); err != nil {
		t.Fatalf("register release def: %v", err)
	}
	return reg
}

type fakeGitInspector struct {
	head             map[string]string
	clean            map[string]bool
	detail           map[string]string
	behind           int
	aheadBehindError bool
}

var errFakeGit = errors.New("fake git error")

func (f *fakeGitInspector) Head(_ context.Context, checkout string) (string, error) {
	return f.head[checkout], nil
}

func (f *fakeGitInspector) IsClean(_ context.Context, checkout string) (bool, string, error) {
	return f.clean[checkout], f.detail[checkout], nil
}

func (f *fakeGitInspector) IsAncestor(_ context.Context, checkout string, ancestor string, descendant string) (bool, error) {
	return true, nil
}

func (f *fakeGitInspector) AheadBehind(_ context.Context, checkout string, left string, right string) (int, int, error) {
	if f.aheadBehindError {
		return 0, 0, errFakeGit
	}
	return 0, f.behind, nil
}

type fakeReleaseQueueStore struct{ active []ReleaseQueueItem }

func (f fakeReleaseQueueStore) CreateReleaseQueueItem(context.Context, CreateReleaseQueueItemParams) (ReleaseQueueItem, error) {
	return ReleaseQueueItem{}, nil
}
func (f fakeReleaseQueueStore) GetReleaseQueueItem(context.Context, string) (ReleaseQueueItem, error) {
	return ReleaseQueueItem{}, nil
}
func (f fakeReleaseQueueStore) ListActiveReleaseQueueItems(context.Context) ([]ReleaseQueueItem, error) {
	return f.active, nil
}
func (f fakeReleaseQueueStore) ListRecentReleaseQueueItems(context.Context, int) ([]ReleaseQueueItem, error) {
	return nil, nil
}
func (f fakeReleaseQueueStore) ClaimNextPendingReleaseQueueItem(context.Context) (ReleaseQueueItem, bool, error) {
	return ReleaseQueueItem{}, false, nil
}
func (f fakeReleaseQueueStore) MarkReleaseQueueItemRunning(context.Context, string, runtime.NodeID) error {
	return nil
}
func (f fakeReleaseQueueStore) MarkReleaseQueueItemTerminal(context.Context, string, ReleaseQueueStatus, string) error {
	return nil
}
func (f fakeReleaseQueueStore) AppendReleaseQueueEvent(context.Context, AppendReleaseQueueEventParams) error {
	return nil
}
func (f fakeReleaseQueueStore) ListReleaseQueueEvents(context.Context, string, int) ([]ReleaseQueueEvent, error) {
	return nil, nil
}
