package workspaces

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

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

func TestReleaseProjectionGatesPromoteToStageOnWorkflowReadiness(t *testing.T) {
	reg := testReleaseRegistry(t)
	projector := &ReleaseProjector{Registry: reg}
	baseViews := []ImplWorkspaceView{
		{Runtime: snapshotFromState(Workspace{Slug: "feature-a", Commit: "feature-1"}, WorkspaceLifecycleState{})},
		{Runtime: snapshotFromState(Workspace{Slug: "integration", Commit: "stage-1"}, WorkspaceLifecycleState{})},
		{Runtime: snapshotFromState(Workspace{Slug: "trunk", Commit: "main-1", IsMain: true}, WorkspaceLifecycleState{})},
	}

	_, missingActions, err := projector.BuildWorkspaceProjection(context.Background(), baseViews)
	if err != nil {
		t.Fatalf("BuildWorkspaceProjection(missing): %v", err)
	}
	missing := missingActions["feature-a"][0]
	if !missing.Disabled || missing.DisabledReason != "QRSPI human review is not ready" {
		t.Fatalf("missing summary action = %+v", missing)
	}

	readyViews := applyOptionsToImplWorkspaceViews(baseViews, WithWorkspaceWorkflowSummaries(map[string]WorkspaceWorkflowSummary{
		"feature-a": {Stage: "human-review", Status: "waiting_human", WaitingHuman: true},
	}))
	_, readyActions, err := projector.BuildWorkspaceProjection(context.Background(), readyViews)
	if err != nil {
		t.Fatalf("BuildWorkspaceProjection(ready): %v", err)
	}
	ready := readyActions["feature-a"][0]
	if ready.Disabled {
		t.Fatalf("human-review-ready action disabled: %+v", ready)
	}
}

func TestWorkspaceHumanReviewReadyAcceptsDoneAndApprovedOutcomes(t *testing.T) {
	cases := []WorkspaceWorkflowSummary{
		{Status: "done"},
		{Outcome: "human-approved"},
		{Outcome: "ready-for-promotion"},
		{Stage: "verify", WaitingHuman: true},
	}
	for _, tc := range cases {
		if !workspaceHumanReviewReady(tc) {
			t.Fatalf("workspaceHumanReviewReady(%+v) = false", tc)
		}
	}
	if workspaceHumanReviewReady(WorkspaceWorkflowSummary{Stage: "implement", Status: "running"}) {
		t.Fatal("implementing workflow unexpectedly ready")
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

func TestReleaseProjectionCheapModeDoesNotRunGit(t *testing.T) {
	reg := testReleaseRegistry(t)
	inspector := &fakeGitInspector{
		head:   map[string]string{"/stage": "stage-2", "/main": "main-1"},
		clean:  map[string]bool{"/main": false},
		detail: map[string]string{"/main": " M file.go"},
	}
	projector := &ReleaseProjector{Registry: reg, Git: inspector}
	views := lifecycleSnapshotsToImplViews([]WorkspaceLifecycleSnapshot{
		snapshotFromState(Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage-1"}, WorkspaceLifecycleState{}),
		snapshotFromState(Workspace{Slug: "trunk", CheckoutPath: "/main", Commit: "main-1", IsMain: true}, WorkspaceLifecycleState{}),
	})

	panel, _, err := projector.BuildWorkspaceProjection(context.Background(), views)
	if err != nil {
		t.Fatalf("BuildWorkspaceProjection: %v", err)
	}
	if inspector.calls != 0 {
		t.Fatalf("git inspector calls = %d, want 0", inspector.calls)
	}
	var found bool
	for _, lane := range panel.Lanes {
		for _, action := range lane.Actions {
			if action.FlowID == "release_to_main" {
				found = true
				if action.Disabled {
					t.Fatalf("cheap projection disabled action via git preflight: %+v", action)
				}
			}
		}
	}
	if !found {
		t.Fatalf("release_to_main action missing: %+v", panel)
	}
}

func TestReleaseProjectionPreflightModeRunsGit(t *testing.T) {
	reg := testReleaseRegistry(t)
	inspector := &fakeGitInspector{
		head:   map[string]string{"/stage": "stage-1", "/main": "main-1"},
		clean:  map[string]bool{"/main": false},
		detail: map[string]string{"/main": " M file.go"},
		behind: 1,
	}
	projector := &ReleaseProjector{Registry: reg, Git: inspector}
	views := lifecycleSnapshotsToImplViews([]WorkspaceLifecycleSnapshot{
		snapshotFromState(Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: "stage-1"}, WorkspaceLifecycleState{}),
		snapshotFromState(Workspace{Slug: "trunk", CheckoutPath: "/main", Commit: "main-1", IsMain: true}, WorkspaceLifecycleState{}),
	})

	panel, _, err := projector.BuildWorkspaceProjection(context.Background(), views, ReleaseProjectionOptions{Mode: ReleaseProjectionPreflight})
	if err != nil {
		t.Fatalf("BuildWorkspaceProjection: %v", err)
	}
	if inspector.calls == 0 {
		t.Fatal("git inspector was not called in preflight mode")
	}
	var found bool
	for _, lane := range panel.Lanes {
		for _, action := range lane.Actions {
			if action.FlowID == "release_to_main" {
				found = true
				if !action.Disabled || action.DisabledReason != "target checkout is dirty: M file.go" {
					t.Fatalf("preflight action = %+v", action)
				}
			}
		}
	}
	if !found {
		t.Fatalf("release_to_main action missing: %+v", panel)
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

func TestHandleEnqueueReleaseRunsPreflightAndBlocksDirtyTarget(t *testing.T) {
	t.Parallel()

	reg := testReleaseRegistry(t)
	store := &recordingReleaseQueueStore{}
	starter := &recordingReleaseStarter{}
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: releaseTestSnapshots()},
		"https://main.test",
		"trunk",
		WithReleaseQueue(&ReleaseProjector{Registry: reg, Store: store, Git: &fakeGitInspector{
			head:   map[string]string{"/stage": "stage-1", "/main": "main-1"},
			clean:  map[string]bool{"/main": false},
			detail: map[string]string{"/main": " M file.go"},
			behind: 1,
		}}, store, starter),
	)

	err := handler.HandleEnqueueRelease(releaseEnqueueContext("integration", "stage-1", "main-1"))
	if err == nil {
		t.Fatal("HandleEnqueueRelease succeeded, want conflict")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusConflict {
		t.Fatalf("error = %#v, want HTTP 409", err)
	}
	if store.createCalls != 0 || starter.calls != 0 {
		t.Fatalf("queue create/start calls = %d/%d, want 0/0", store.createCalls, starter.calls)
	}
}

func TestHandleEnqueueReleaseBlocksChangedPreflightCommits(t *testing.T) {
	t.Parallel()

	reg := testReleaseRegistry(t)
	store := &recordingReleaseQueueStore{}
	starter := &recordingReleaseStarter{}
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: releaseTestSnapshots()},
		"https://main.test",
		"trunk",
		WithReleaseQueue(&ReleaseProjector{Registry: reg, Store: store, Git: &fakeGitInspector{
			head:   map[string]string{"/stage": "stage-2", "/main": "main-1"},
			clean:  map[string]bool{"/main": true},
			behind: 1,
		}}, store, starter),
	)

	err := handler.HandleEnqueueRelease(releaseEnqueueContext("integration", "stage-1", "main-1"))
	if err == nil {
		t.Fatal("HandleEnqueueRelease succeeded, want conflict")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusConflict {
		t.Fatalf("error = %#v, want HTTP 409", err)
	}
	if store.createCalls != 0 || starter.calls != 0 {
		t.Fatalf("queue create/start calls = %d/%d, want 0/0", store.createCalls, starter.calls)
	}
}

func TestHandleEnqueueReleaseStoresPreflightCommits(t *testing.T) {
	t.Parallel()

	reg := testReleaseRegistry(t)
	store := &recordingReleaseQueueStore{}
	starter := &recordingReleaseStarter{}
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: releaseTestSnapshots("stage-long", "main-long")},
		"https://main.test",
		"trunk",
		WithReleaseQueue(&ReleaseProjector{Registry: reg, Store: store, Git: &fakeGitInspector{
			head:   map[string]string{"/stage": "stage-long", "/main": "main-long"},
			clean:  map[string]bool{"/main": true},
			behind: 1,
		}}, store, starter),
	)

	c := releaseEnqueueContext("integration", "stage", "main")
	if err := handler.HandleEnqueueRelease(c); err != nil {
		t.Fatalf("HandleEnqueueRelease: %v", err)
	}
	if store.createCalls != 1 || starter.calls != 1 {
		t.Fatalf("queue create/start calls = %d/%d, want 1/1", store.createCalls, starter.calls)
	}
	if store.created.ExpectedSourceCommit != "stage-long" || store.created.ExpectedTargetCommit != "main-long" {
		t.Fatalf("created commits = %q/%q", store.created.ExpectedSourceCommit, store.created.ExpectedTargetCommit)
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
	calls            int
}

var errFakeGit = errors.New("fake git error")

func (f *fakeGitInspector) Head(_ context.Context, checkout string) (string, error) {
	f.calls++
	return f.head[checkout], nil
}

func (f *fakeGitInspector) IsClean(_ context.Context, checkout string) (bool, string, error) {
	f.calls++
	return f.clean[checkout], f.detail[checkout], nil
}

func (f *fakeGitInspector) IsAncestor(_ context.Context, checkout string, ancestor string, descendant string) (bool, error) {
	f.calls++
	return true, nil
}

func (f *fakeGitInspector) AheadBehind(_ context.Context, checkout string, left string, right string) (int, int, error) {
	f.calls++
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

func releaseTestSnapshots(commits ...string) []WorkspaceLifecycleSnapshot {
	stageCommit := "stage-1"
	mainCommit := "main-1"
	if len(commits) > 0 {
		stageCommit = commits[0]
	}
	if len(commits) > 1 {
		mainCommit = commits[1]
	}
	return []WorkspaceLifecycleSnapshot{
		snapshotFromState(Workspace{Slug: "integration", CheckoutPath: "/stage", Commit: stageCommit}, WorkspaceLifecycleState{}),
		snapshotFromState(Workspace{Slug: "trunk", CheckoutPath: "/main", Commit: mainCommit, IsMain: true}, WorkspaceLifecycleState{}),
	}
}

func releaseEnqueueContext(sourceSlug, sourceCommit, targetCommit string) echo.Context {
	form := url.Values{}
	form.Set("definition_id", "default")
	form.Set("definition_version", "v1")
	form.Set("flow_id", "release_to_main")
	form.Set("source_slug", sourceSlug)
	form.Set("expected_source_commit", sourceCommit)
	form.Set("expected_target_commit", targetCommit)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/release/enqueue", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec)
}

type recordingReleaseQueueStore struct {
	created     CreateReleaseQueueItemParams
	createCalls int
}

func (s *recordingReleaseQueueStore) CreateReleaseQueueItem(_ context.Context, arg CreateReleaseQueueItemParams) (ReleaseQueueItem, error) {
	s.createCalls++
	s.created = arg
	return ReleaseQueueItem{ID: arg.ID}, nil
}
func (s *recordingReleaseQueueStore) GetReleaseQueueItem(context.Context, string) (ReleaseQueueItem, error) {
	return ReleaseQueueItem{}, nil
}
func (s *recordingReleaseQueueStore) ListActiveReleaseQueueItems(context.Context) ([]ReleaseQueueItem, error) {
	return nil, nil
}
func (s *recordingReleaseQueueStore) ListRecentReleaseQueueItems(context.Context, int) ([]ReleaseQueueItem, error) {
	return nil, nil
}
func (s *recordingReleaseQueueStore) ClaimNextPendingReleaseQueueItem(context.Context) (ReleaseQueueItem, bool, error) {
	return ReleaseQueueItem{}, false, nil
}
func (s *recordingReleaseQueueStore) MarkReleaseQueueItemRunning(context.Context, string, runtime.NodeID) error {
	return nil
}
func (s *recordingReleaseQueueStore) MarkReleaseQueueItemTerminal(context.Context, string, ReleaseQueueStatus, string) error {
	return nil
}
func (s *recordingReleaseQueueStore) AppendReleaseQueueEvent(context.Context, AppendReleaseQueueEventParams) error {
	return nil
}
func (s *recordingReleaseQueueStore) ListReleaseQueueEvents(context.Context, string, int) ([]ReleaseQueueEvent, error) {
	return nil, nil
}

type recordingReleaseStarter struct{ calls int }

func (s *recordingReleaseStarter) EnqueueRelease(context.Context, string) error {
	s.calls++
	return nil
}
