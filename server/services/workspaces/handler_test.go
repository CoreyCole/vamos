package workspaces

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestBuildNavItems(t *testing.T) {
	items := []Workspace{
		{
			Slug:        "main",
			DisplayName: "main",
			URL:         "https://main.cn-agents.test/",
			Status:      StatusRunning,
		},
		{
			Slug:        "feature",
			DisplayName: "feature branch",
			URL:         "https://feature.cn-agents.test/",
			Status:      StatusStopped,
		},
	}

	got := BuildNavItems(
		items,
		"main",
		"https://main.cn-agents.test/",
		"/agent-chat?thread=1",
	)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].URL != "https://main.cn-agents.test/workspaces/switch/main?redirect=%2Fagent-chat%3Fthread%3D1" ||
		!got[0].Current {
		t.Fatalf("running nav item = %+v", got[0])
	}
	if got[1].URL != "https://main.cn-agents.test/workspaces" ||
		got[1].Status != "stopped" {
		t.Fatalf("stopped nav item = %+v", got[1])
	}
}

func TestBuildNavItemsTreatsMainAndCurrentWorkspaceAsReachable(t *testing.T) {
	t.Parallel()

	items := []Workspace{
		{
			Slug:        "main",
			DisplayName: "main",
			URL:         "https://main.cn-agents.test/",
			Status:      StatusStopped,
		},
		{
			Slug:        "child",
			DisplayName: "child",
			URL:         "https://child.cn-agents.test/",
			Status:      StatusStopped,
		},
	}

	got := BuildNavItems(items, "child", "https://main.cn-agents.test", "/thoughts/")
	if got[0].Status != "running" ||
		got[0].URL != "https://main.cn-agents.test/workspaces/switch/main?redirect=%2Fthoughts%2F" {
		t.Fatalf("main nav item = %+v", got[0])
	}
	if got[1].Status != "running" ||
		got[1].URL != "https://main.cn-agents.test/workspaces/switch/child?redirect=%2Fthoughts%2F" ||
		!got[1].Current {
		t.Fatalf("current child nav item = %+v", got[1])
	}
}

func TestHandleSwitchWorkspaceRedirectsToTargetHandoff(t *testing.T) {
	manager := &ManagerService{
		workspaces: map[string]Workspace{
			"feature": {
				Slug:   "feature",
				URL:    "https://feature.cn-agents.test/",
				Status: StatusRunning,
			},
		},
	}
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithDevAuth(nil, signer),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/workspaces/switch/feature?redirect=/thoughts/",
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("feature")
	c.Set("user_email", "user@example.com")
	if err := handler.HandleSwitchWorkspace(c); err != nil {
		t.Fatalf("HandleSwitchWorkspace() error = %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(
		location,
		"https://feature.cn-agents.test/internal/dev-auth/handoff?token=",
	) {
		t.Fatalf("Location = %q", location)
	}
	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	claims, err := verifier.Verify(u.Query().Get("token"), "feature")
	if err != nil {
		t.Fatalf("Verify(location token) error = %v", err)
	}
	if claims.Email != "user@example.com" || claims.RedirectPath != "/thoughts/" {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestHandleSwitchWorkspaceRedirectsManagerWorkspacePageToChildRoot(t *testing.T) {
	manager := &fakeLifecycleManager{
		workspaces: []Workspace{{
			Slug:   "feature",
			URL:    "https://feature.cn-agents.test/",
			Status: StatusRunning,
		}},
	}
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithDevAuth(nil, signer),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/workspaces/switch/feature?redirect=/workspaces",
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("feature")
	c.Set("user_email", "user@example.com")
	if err := handler.HandleSwitchWorkspace(c); err != nil {
		t.Fatalf("HandleSwitchWorkspace() error = %v", err)
	}
	location := rec.Header().Get("Location")
	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	claims, err := verifier.Verify(u.Query().Get("token"), "feature")
	if err != nil {
		t.Fatalf("Verify(location token) error = %v", err)
	}
	if claims.RedirectPath != "/" {
		t.Fatalf("redirect path = %q, want child root", claims.RedirectPath)
	}
}

func TestHandleStartReturnsAcceptedLifecycleSnapshot(t *testing.T) {
	manager := &fakeLifecycleManager{
		snapshots: []WorkspaceLifecycleSnapshot{
			{
				Workspace: Workspace{
					Slug:        "feature",
					DisplayName: "feature",
					Status:      StatusStopped,
				},
				DesiredState:  WorkspaceDesiredStopped,
				ObservedState: WorkspaceObservedStopped,
			},
		},
	}
	handler := NewHandler(manager, "https://main.cn-agents.test", "main")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/feature/start", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("feature")

	if err := handler.HandleStart(c); err != nil {
		t.Fatalf("HandleStart() error = %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if len(manager.requests) != 1 ||
		manager.requests[0].Kind != WorkspaceTransitionStart {
		t.Fatalf("requests = %#v", manager.requests)
	}
	if strings.Contains(rec.Header().Get("Location"), "/workspaces") {
		t.Fatalf("unexpected redirect location %q", rec.Header().Get("Location"))
	}
}

func TestHandleWorkspacesStreamInitialRenderContainsWholeList(t *testing.T) {
	manager := &fakeLifecycleManager{
		snapshots: []WorkspaceLifecycleSnapshot{
			{
				Workspace: Workspace{
					Slug:        "feature",
					DisplayName: "feature",
					Status:      StatusRunning,
				},
				DesiredState:  WorkspaceDesiredRunning,
				ObservedState: WorkspaceObservedRunning,
			},
			{
				Workspace: Workspace{
					Slug:        "stopped",
					DisplayName: "stopped",
					Status:      StatusStopped,
				},
				DesiredState:  WorkspaceDesiredStopped,
				ObservedState: WorkspaceObservedStopped,
			},
		},
	}
	notifier := NewLifecycleNotifier()
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithLifecycleNotifier(notifier),
	)
	e := echo.New()
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/workspaces/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() {
		done <- handler.HandleWorkspacesStream(e.NewContext(req, rec))
	}()
	deadline := time.After(time.Second)
	for !strings.Contains(rec.Body.String(), "workspaces-list") {
		select {
		case err := <-done:
			t.Fatalf("HandleWorkspacesStream() returned before initial render: %v", err)
		case <-deadline:
			t.Fatal("stream initial render timed out")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("HandleWorkspacesStream() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"workspaces-list", "feature", "stopped"} {
		if !strings.Contains(html, want) {
			t.Fatalf("stream missing %q: %s", want, html)
		}
	}
}

func TestHandleWorkspacesStreamNotificationRendersUpdatedList(t *testing.T) {
	manager := &fakeLifecycleManager{
		snapshots: []WorkspaceLifecycleSnapshot{
			{
				Workspace: Workspace{
					Slug:        "feature",
					DisplayName: "feature",
					Status:      StatusStarting,
				},
				DesiredState:  WorkspaceDesiredRunning,
				ObservedState: WorkspaceObservedStarting,
			},
		},
	}
	notifier := NewLifecycleNotifier()
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithLifecycleNotifier(notifier),
	)
	e := echo.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/workspaces/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() {
		done <- handler.HandleWorkspacesStream(e.NewContext(req, rec))
	}()
	waitForBodyContains(t, rec, done, "starting")
	manager.snapshots[0].ObservedState = WorkspaceObservedRunning
	manager.snapshots[0].Workspace.Status = StatusRunning
	notifier.Notify("feature")
	waitForBodyContains(t, rec, done, "running")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("HandleWorkspacesStream() error = %v", err)
	}
}

func TestLifecycleNotifierSubscribeCleanup(t *testing.T) {
	notifier := NewLifecycleNotifier()
	ch, unsubscribe := notifier.Subscribe()
	if got := notifier.SubscriberCount(); got != 1 {
		t.Fatalf("SubscriberCount = %d, want 1", got)
	}
	notifier.Notify("feature")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified")
	}
	unsubscribe()
	if got := notifier.SubscriberCount(); got != 0 {
		t.Fatalf("SubscriberCount after unsubscribe = %d, want 0", got)
	}
}

func TestWorkspacesPageRendersWorkspaceCardsAndForms(t *testing.T) {
	items := []Workspace{
		{
			Slug:         "main",
			DisplayName:  "main",
			CheckoutPath: "/repo/cn-agents",
			URL:          "https://main.cn-agents.test/",
			Status:       StatusRunning,
			Port:         4200,
			Branch:       "main",
			Commit:       "abc123",
			LogPath:      "/state/main/agents-server.log",
			IsMain:       true,
		},
		{
			Slug:         "feature",
			DisplayName:  "feature branch",
			CheckoutPath: "/repo/cn-agents-feature",
			Status:       StatusStopped,
		},
		{
			Slug:         "crashed",
			DisplayName:  "crashed branch",
			CheckoutPath: "/repo/cn-agents-crashed",
			Status:       StatusCrashed,
			Error:        "exit status 1",
		},
	}

	snapshots := make([]WorkspaceLifecycleSnapshot, 0, len(items))
	for _, item := range items {
		snapshots = append(
			snapshots,
			snapshotFromState(item, WorkspaceLifecycleState{Error: item.Error}),
		)
	}

	var body bytes.Buffer
	if err := WorkspacesPage(
		lifecycleSnapshotsToImplViews(snapshots),
		"https://main.cn-agents.test",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		"Workspaces",
		"Local implementation checkouts discovered under the configured workspace parent.",
		"Implementation workspace",
		"Local checkout",
		"main",
		"/workspaces/switch/main",
		"127.0.0.1:4200",
		`method="post"`,
		`data-init="@get('/workspaces/stream')"`,
		`id="workspaces-list"`,
		"Feature Branch",
		`action="/workspaces/feature/start"`,
		"Crashed Branch",
		"exit status 1",
		`action="/workspaces/crashed/start"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("WorkspacesPage missing %q: %s", want, html)
		}
	}
	for _, absent := range []string{"Plan workspaces", "No impl workspace yet"} {
		if strings.Contains(html, absent) {
			t.Fatalf("WorkspacesPage unexpectedly contained %q: %s", absent, html)
		}
	}
}

func TestWorkspacesPageRendersTransitionSafeActionControls(t *testing.T) {
	cases := []struct {
		name       string
		observed   WorkspaceObservedState
		want       []string
		wantAbsent []string
	}{
		{
			name:     "starting",
			observed: WorkspaceObservedStarting,
			want:     []string{"Starting...", "disabled"},
			wantAbsent: []string{
				`action="/workspaces/feature/start"`,
				`action="/workspaces/feature/stop"`,
				`action="/workspaces/feature/restart"`,
			},
		},
		{
			name:     "stopping",
			observed: WorkspaceObservedStopping,
			want:     []string{"Stopping...", "disabled"},
			wantAbsent: []string{
				`action="/workspaces/feature/start"`,
				`action="/workspaces/feature/stop"`,
				`action="/workspaces/feature/restart"`,
			},
		},
		{
			name:     "failed",
			observed: WorkspaceObservedFailed,
			want: []string{
				`action="/workspaces/feature/start"`,
				`data-indicator="_workspaceAction`,
			},
			wantAbsent: []string{
				`action="/workspaces/feature/stop"`,
				`action="/workspaces/feature/restart"`,
			},
		},
		{
			name:     "crashed",
			observed: WorkspaceObservedCrashed,
			want: []string{
				`action="/workspaces/feature/start"`,
				`data-indicator="_workspaceAction`,
			},
			wantAbsent: []string{
				`action="/workspaces/feature/stop"`,
				`action="/workspaces/feature/restart"`,
			},
		},
		{
			name:     "running",
			observed: WorkspaceObservedRunning,
			want: []string{
				`action="/workspaces/feature/stop"`,
				`action="/workspaces/feature/restart"`,
				`data-attr-disabled="$_workspaceAction`,
			},
			wantAbsent: []string{`action="/workspaces/feature/start"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := WorkspaceLifecycleSnapshot{
				Workspace: Workspace{
					Slug:         "feature",
					DisplayName:  "feature branch",
					CheckoutPath: "/repo/cn-agents-feature",
					Status:       statusFromObserved(tc.observed),
					URL:          "https://feature.cn-agents.test/",
				},
				ObservedState: tc.observed,
			}
			var body bytes.Buffer
			if err := WorkspacesPage(
				lifecycleSnapshotsToImplViews([]WorkspaceLifecycleSnapshot{snap}),
				"https://main.cn-agents.test",
			).Render(t.Context(), &body); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			html := body.String()
			for _, want := range tc.want {
				if !strings.Contains(html, want) {
					t.Fatalf("WorkspacesPage missing %q: %s", want, html)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(html, absent) {
					t.Fatalf("WorkspacesPage unexpectedly contained %q: %s", absent, html)
				}
			}
		})
	}
}

func TestWorkspacesPageRendersImplRowWithPlanBinding(t *testing.T) {
	views := BuildImplWorkspaceViews([]db.ImplWorkspace{
		{
			WorkspaceSlug: "feature",
			CheckoutPath:  "/repo/cn-agents-feature",
			DisplayName:   "Feature Workspace",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/feature",
				Valid:  true,
			},
			PlanDir: sql.NullString{
				String: "/repo/thoughts/creative-mode-agent/plans/feature",
				Valid:  true,
			},
			UpdatedAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
		},
	}, nil, WorkspaceLifecycleSnapshot{})

	var body bytes.Buffer
	if err := WorkspacesPage(
		views,
		"https://main.cn-agents.test",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"Feature Workspace", "Plan artifact", "creative-mode-agent/plans/feature", "Local checkout", "Updated", `action="/workspaces/feature/start"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("WorkspacesPage missing %q: %s", want, html)
		}
	}
	for _, absent := range []string{"Plan workspaces", "No impl workspace yet"} {
		if strings.Contains(html, absent) {
			t.Fatalf("WorkspacesPage unexpectedly contained %q: %s", absent, html)
		}
	}
}

func TestWorkspacesPageRendersNestedReviewWorkspaces(t *testing.T) {
	views := BuildImplWorkspaceViews([]db.ImplWorkspace{
		{
			WorkspaceSlug: "parent",
			CheckoutPath:  "/repo/cn-agents-parent",
			DisplayName:   "Parent Workspace",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/workspace-discovery-sync",
				Valid:  true,
			},
		},
		{
			WorkspaceSlug: "review-child",
			CheckoutPath:  "/repo/cn-agents-review-child",
			DisplayName:   "Review Child Workspace",
			Status:        string(ImplWorkspaceStatusActive),
			PlanDirRel: sql.NullString{
				String: "creative-mode-agent/plans/workspace-discovery-sync/reviews/implementation-review",
				Valid:  true,
			},
		},
	}, nil, WorkspaceLifecycleSnapshot{})

	var body bytes.Buffer
	if err := WorkspacesPage(
		views,
		"https://main.cn-agents.test",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	parentIdx := strings.Index(html, "Parent Workspace")
	childIdx := strings.Index(html, "Review Child Workspace")
	if parentIdx < 0 || childIdx < 0 || parentIdx > childIdx {
		t.Fatalf(
			"nested workspace order wrong: parent=%d child=%d html=%s",
			parentIdx,
			childIdx,
			html,
		)
	}
	for _, want := range []string{
		`data-workspace-children="parent"`,
		`data-workspace-tree-item="review-child"`,
		`action="/workspaces/review-child/start"`,
		"creative-mode-agent/plans/workspace-discovery-sync/reviews/implementation-review",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("WorkspacesPage missing %q: %s", want, html)
		}
	}
}

func TestFilterHistoricalImplWorkspaceViewsPromotesActiveChildren(t *testing.T) {
	views := []ImplWorkspaceView{
		{
			Row: db.ImplWorkspace{
				WorkspaceSlug: "merged-parent",
				DisplayName:   "Merged Parent",
				Status:        string(ImplWorkspaceStatusMerged),
			},
			Children: []ImplWorkspaceView{
				{
					Row: db.ImplWorkspace{
						WorkspaceSlug: "active-child",
						DisplayName:   "Active Child",
						Status:        string(ImplWorkspaceStatusActive),
					},
				},
				{
					Row: db.ImplWorkspace{
						WorkspaceSlug: "cleaned-child",
						DisplayName:   "Cleaned Child",
						Status:        string(ImplWorkspaceStatusCleanedUp),
					},
				},
			},
		},
		{
			Row: db.ImplWorkspace{
				WorkspaceSlug: "active-parent",
				DisplayName:   "Active Parent",
				Status:        string(ImplWorkspaceStatusActive),
			},
			Children: []ImplWorkspaceView{{
				Row: db.ImplWorkspace{
					WorkspaceSlug: "merged-child",
					DisplayName:   "Merged Child",
					Status:        string(ImplWorkspaceStatusMerged),
				},
			}},
		},
	}

	filtered := filterHistoricalImplWorkspaceViews(views, false)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2: %#v", len(filtered), filtered)
	}
	if filtered[0].Row.WorkspaceSlug != "active-child" {
		t.Fatalf(
			"first filtered slug = %q, want promoted active child",
			filtered[0].Row.WorkspaceSlug,
		)
	}
	if filtered[1].Row.WorkspaceSlug != "active-parent" || len(filtered[1].Children) != 0 {
		t.Fatalf("active parent not preserved with historical child hidden: %#v", filtered[1])
	}

	shown := filterHistoricalImplWorkspaceViews(views, true)
	if len(shown) != len(views) || shown[0].Row.WorkspaceSlug != "merged-parent" || len(shown[0].Children) != 2 {
		t.Fatalf("show historical changed tree: %#v", shown)
	}
	if len(views[0].Children) != 2 || len(views[1].Children) != 1 {
		t.Fatalf("filter mutated input children: %#v", views)
	}
}

func TestWorkspacesPageSuppressesLifecycleActionsForHistoricalRows(t *testing.T) {
	views := BuildImplWorkspaceViews([]db.ImplWorkspace{
		{
			WorkspaceSlug: "merged",
			CheckoutPath:  "/repo/cn-agents-merged",
			DisplayName:   "Merged Workspace",
			Status:        string(ImplWorkspaceStatusMerged),
		},
		{
			WorkspaceSlug: "cleaned",
			CheckoutPath:  "/repo/cn-agents-cleaned",
			DisplayName:   "Cleaned Workspace",
			Status:        string(ImplWorkspaceStatusCleanedUp),
		},
	}, nil, WorkspaceLifecycleSnapshot{})

	var body bytes.Buffer
	if err := WorkspacesPage(
		views,
		"https://main.cn-agents.test",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"Historical workspace", "Merged", "Cleaned up"} {
		if !strings.Contains(html, want) {
			t.Fatalf("WorkspacesPage missing %q: %s", want, html)
		}
	}
	for _, absent := range []string{
		`action="/workspaces/merged/start"`,
		`action="/workspaces/merged/stop"`,
		`action="/workspaces/merged/restart"`,
		`action="/workspaces/cleaned/start"`,
		`action="/workspaces/cleaned/stop"`,
		`action="/workspaces/cleaned/restart"`,
	} {
		if strings.Contains(html, absent) {
			t.Fatalf("WorkspacesPage unexpectedly contained %q: %s", absent, html)
		}
	}
}

func TestHandleWorkspacesPageFollowsImplWorkspaceOrder(t *testing.T) {
	manager := &fakeLifecycleManager{
		snapshots: []WorkspaceLifecycleSnapshot{
			snapshotFromState(
				Workspace{Slug: "old", DisplayName: "old runtime", Status: StatusStopped},
				WorkspaceLifecycleState{},
			),
			snapshotFromState(
				Workspace{Slug: "new", DisplayName: "new runtime", Status: StatusStopped},
				WorkspaceLifecycleState{},
			),
		},
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{
			{
				WorkspaceSlug: "new",
				DisplayName:   "New Workspace",
				CheckoutPath:  "/repo/new",
				Status:        string(ImplWorkspaceStatusActive),
			},
			{
				WorkspaceSlug: "old",
				DisplayName:   "Old Workspace",
				CheckoutPath:  "/repo/old",
				Status:        string(ImplWorkspaceStatusActive),
			},
		}}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	html := rec.Body.String()
	newIdx := strings.Index(html, "New Workspace")
	oldIdx := strings.Index(html, "Old Workspace")
	if newIdx < 0 || oldIdx < 0 || newIdx > oldIdx {
		t.Fatalf(
			"impl workspace order not preserved: new=%d old=%d html=%s",
			newIdx,
			oldIdx,
			html,
		)
	}
}

func TestHandleRefreshWorkspacesRunsSyncBeforeManagerRefresh(t *testing.T) {
	managerRefreshed := make(chan struct{}, 1)
	manager := &fakeLifecycleManager{
		beforeRefresh: func() {
			managerRefreshed <- struct{}{}
		},
	}
	syncCalled := make(chan struct{}, 1)
	releaseSync := make(chan struct{})
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithWorkspaceSyncRefresh(func(context.Context) error {
			syncCalled <- struct{}{}
			<-releaseSync
			return nil
		}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/refresh", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	select {
	case <-syncCalled:
	case <-time.After(time.Second):
		t.Fatal("workspace sync did not start")
	}
	select {
	case <-managerRefreshed:
		t.Fatal("manager refresh ran before workspace sync completed")
	default:
	}
	close(releaseSync)
	select {
	case <-managerRefreshed:
	case <-time.After(time.Second):
		t.Fatal("manager refresh did not run after workspace sync")
	}
}

func waitForBodyContains(
	t *testing.T,
	rec *httptest.ResponseRecorder,
	done <-chan error,
	want string,
) {
	t.Helper()
	deadline := time.After(time.Second)
	for !strings.Contains(rec.Body.String(), want) {
		select {
		case err := <-done:
			t.Fatalf("stream returned before body contained %q: %v", want, err)
		case <-deadline:
			t.Fatalf(
				"timed out waiting for stream body to contain %q: %s",
				want,
				rec.Body.String(),
			)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type fakeImplWorkspaceLister struct {
	rows []db.ImplWorkspace
	err  error
}

func (f fakeImplWorkspaceLister) ListImplWorkspaces(
	context.Context,
) ([]db.ImplWorkspace, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]db.ImplWorkspace(nil), f.rows...), nil
}

type fakeLifecycleManager struct {
	workspaces    []Workspace
	snapshots     []WorkspaceLifecycleSnapshot
	requests      []WorkspaceLifecycleRequest
	beforeRefresh func()
}

func (f *fakeLifecycleManager) Refresh(context.Context) error {
	if f.beforeRefresh != nil {
		f.beforeRefresh()
	}
	return nil
}

func (f *fakeLifecycleManager) List() []Workspace {
	if len(f.workspaces) != 0 {
		return append([]Workspace(nil), f.workspaces...)
	}
	return snapshotsToWorkspaces(f.snapshots)
}

func (f *fakeLifecycleManager) Lookup(slug string) (Workspace, bool) {
	for _, ws := range f.List() {
		if ws.Slug == slug {
			return ws, true
		}
	}
	return Workspace{}, false
}

func (f *fakeLifecycleManager) LookupHost(
	string,
) (Workspace, bool) {
	return Workspace{}, false
}

func (f *fakeLifecycleManager) Start(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

func (f *fakeLifecycleManager) Stop(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

func (f *fakeLifecycleManager) Restart(context.Context, string) (Workspace, error) {
	return Workspace{}, nil
}

func (f *fakeLifecycleManager) RequestLifecycle(
	_ context.Context,
	req WorkspaceLifecycleRequest,
) (WorkspaceLifecycleSnapshot, error) {
	f.requests = append(f.requests, req)
	snap := f.snapshots[0]
	snap.DesiredState = req.DesiredState
	snap.ObservedState = WorkspaceObservedStarting
	return snap, nil
}

func (f *fakeLifecycleManager) ListLifecycle(
	context.Context,
) ([]WorkspaceLifecycleSnapshot, error) {
	return append([]WorkspaceLifecycleSnapshot(nil), f.snapshots...), nil
}

func (f *fakeLifecycleManager) CompleteTransition(
	context.Context,
	string,
	string,
	WorkspaceTransitionResult,
) error {
	return nil
}
