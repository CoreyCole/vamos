package workspaces

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/pkg/release"
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

func TestBuildNavItemsTreatsProtectedReleaseLanesAsReachable(t *testing.T) {
	t.Parallel()

	items := []Workspace{
		{
			Slug:        "production-like-stage",
			DisplayName: "Stage",
			URL:         "https://stage.cn-agents.test/",
			Status:      StatusStopped,
		},
	}

	got := BuildNavItems(
		items,
		"main",
		"https://main.cn-agents.test",
		"/workspaces",
		"production-like-stage",
	)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Status != "running" || got[0].URL != "https://main.cn-agents.test/workspaces/switch/production-like-stage?redirect=%2Fworkspaces" {
		t.Fatalf("protected stage nav item = %+v", got[0])
	}
}

func TestHandleSwitchWorkspaceAllowsProtectedStoppedReleaseLaneWithURL(t *testing.T) {
	t.Parallel()

	_, releases, err := BuildDefaultReleaseRegistry("stage", "main")
	if err != nil {
		t.Fatalf("BuildDefaultReleaseRegistry() error = %v", err)
	}
	manager := &fakeLifecycleManager{
		workspaces: []Workspace{{
			Slug:   "stage",
			URL:    "https://stage.cn-agents.test/",
			Status: StatusStopped,
		}},
	}
	signer, _ := newTestHandoffSigner(t)
	h := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithDevAuth(&fakeSessionCreator{}, signer),
		WithReleaseProjector(&ReleaseProjector{Registry: releases}),
	)
	req := httptest.NewRequest(http.MethodGet, "/workspaces/switch/stage?redirect=/workspaces", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/workspaces/switch/:slug")
	c.SetParamNames("slug")
	c.SetParamValues("stage")
	c.Set("user_email", "dev@example.com")

	if err := h.HandleSwitchWorkspace(c); err != nil {
		t.Fatalf("HandleSwitchWorkspace() error = %v", err)
	}
	if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want redirect", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "https://stage.cn-agents.test/internal/dev-auth/handoff?token=") {
		t.Fatalf("Location = %q, want stage handoff", location)
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

func TestHandleSwitchWorkspaceUnavailableRecordsAndRedirectsToErrors(t *testing.T) {
	store := &fakeWorkspaceErrorEventStore{}
	manager := &fakeLifecycleManager{workspaces: []Workspace{{
		Slug:   "feature",
		Status: StatusCrashed,
		Error:  "boom",
	}}}
	signer, _ := newTestHandoffSigner(t)
	h := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithDevAuth(nil, signer),
		WithWorkspaceErrorStore(store),
	)

	req := httptest.NewRequest(http.MethodGet, "/workspaces/switch/feature?redirect=/thoughts/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("feature")
	c.Set("user_email", "user@example.com")

	if err := h.HandleSwitchWorkspace(c); err != nil {
		t.Fatalf("HandleSwitchWorkspace() error = %v", err)
	}
	if got := rec.Header().Get("Location"); got != "/workspaces/errors?workspace=feature" {
		t.Fatalf("Location = %q, want errors page", got)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}
	event := store.events[0]
	if event.WorkspaceSlug != "feature" || event.Source != WorkspaceErrorSourceSwitch || event.Severity != WorkspaceErrorSeverityWarn {
		t.Fatalf("event = %+v", event)
	}
	if event.DedupeKey == "" {
		t.Fatal("empty dedupe key")
	}
}

func TestWorkspaceErrorRecorderRecordValidatesAndDerivesDedupeKey(t *testing.T) {
	store := &fakeWorkspaceErrorEventStore{}
	recorder := &WorkspaceErrorRecorder{Store: store}
	if _, err := recorder.Record(context.Background(), WorkspaceErrorRecordRequest{WorkspaceSlug: "feature", Source: WorkspaceErrorSourceSwitch}); err == nil {
		t.Fatal("Record() error = nil, want missing message error")
	}
	event, err := recorder.Record(context.Background(), WorkspaceErrorRecordRequest{
		WorkspaceSlug: " feature ",
		Source:        WorkspaceErrorSourceSwitch,
		Message:       " failed switch ",
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if event.DedupeKey == "" || event.Severity != string(WorkspaceErrorSeverityError) {
		t.Fatalf("event = %+v", event)
	}
}

func TestWorkspaceErrorDedupeKeyStableAndNonEmpty(t *testing.T) {
	first := workspaceErrorDedupeKey(" Feature ", "switch", "panic: boom")
	second := workspaceErrorDedupeKey("feature", "switch", "panic:   boom")
	if first == "" {
		t.Fatal("empty key")
	}
	if first != second {
		t.Fatalf("keys differ: %q != %q", first, second)
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

func TestHandleWorkspaceErrorsPageRendersEmptyState(t *testing.T) {
	manager := &fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{{Workspace: Workspace{Slug: "feature", Status: StatusRunning}}}}
	handler := NewHandler(manager, "https://main.cn-agents.test", "main")
	req := httptest.NewRequest(http.MethodGet, "/workspaces/errors", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.HandleWorkspaceErrorsPage(c); err != nil {
		t.Fatalf("HandleWorkspaceErrorsPage() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"Workspace errors", `id="workspace-error-queue"`, "No recorded workspace errors yet.", `data-init="@get(&#39;/workspaces/errors/stream&#39;)"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("errors page missing %q: %s", want, html)
		}
	}
}

func TestHandleWorkspaceErrorsPageRendersFilteredEvents(t *testing.T) {
	store := &fakeWorkspaceErrorEventStore{listed: []WorkspaceErrorEvent{
		{ID: 1, WorkspaceSlug: "feature", Source: "switch", Severity: "warn", Message: "workspace unavailable during switch", Detail: "boom", OccurrenceCount: 2, FirstSeenAt: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC), LastSeenAt: time.Date(2026, 5, 24, 12, 5, 0, 0, time.UTC)},
		{ID: 2, WorkspaceSlug: "other", Source: "log", Severity: "error", Message: "other panic", OccurrenceCount: 1},
	}}
	manager := &fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{{Workspace: Workspace{Slug: "feature", Status: StatusRunning, URL: "https://feature.cn-agents.test/"}, ObservedState: WorkspaceObservedRunning}}}
	signer, _ := newTestHandoffSigner(t)
	handler := NewHandler(manager, "https://main.cn-agents.test", "main", WithDevAuth(nil, signer), WithWorkspaceErrorStore(store))
	req := httptest.NewRequest(http.MethodGet, "/workspaces/errors?workspace=feature", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.HandleWorkspaceErrorsPage(c); err != nil {
		t.Fatalf("HandleWorkspaceErrorsPage() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"Filtered: feature", "workspace unavailable during switch", "feature · switch · warn", "boom", "x2", `data-init="@get(&#39;/workspaces/errors/stream?workspace=feature&#39;)"`, "/workspaces/switch/feature"} {
		if !strings.Contains(html, want) {
			t.Fatalf("filtered errors page missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "other panic") {
		t.Fatalf("filtered errors page contained other workspace event: %s", html)
	}
}

func TestHandleWorkspaceErrorsStreamInitialRender(t *testing.T) {
	store := &fakeWorkspaceErrorEventStore{listed: []WorkspaceErrorEvent{{ID: 1, WorkspaceSlug: "feature", Source: "switch", Severity: "warn", Message: "stream event", OccurrenceCount: 1}}}
	manager := &fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{{Workspace: Workspace{Slug: "feature", Status: StatusRunning}}}}
	notifier := NewLifecycleNotifier()
	handler := NewHandler(manager, "https://main.cn-agents.test", "main", WithLifecycleNotifier(notifier), WithWorkspaceErrorStore(store))
	e := echo.New()
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/workspaces/errors/stream?workspace=feature", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() { done <- handler.HandleWorkspaceErrorsStream(e.NewContext(req, rec)) }()
	waitForBodyContains(t, rec, done, "stream event")
	waitForBodyContains(t, rec, done, "workspace-error-queue")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("HandleWorkspaceErrorsStream() error = %v", err)
	}
}

func TestHandleWorkspaceErrorsStreamNotificationRendersUpdatedEvents(t *testing.T) {
	store := &fakeWorkspaceErrorEventStore{listed: []WorkspaceErrorEvent{{ID: 1, WorkspaceSlug: "feature", Source: "switch", Severity: "warn", Message: "old event", OccurrenceCount: 1}}}
	manager := &fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{{Workspace: Workspace{Slug: "feature", Status: StatusRunning}}}}
	notifier := NewLifecycleNotifier()
	handler := NewHandler(manager, "https://main.cn-agents.test", "main", WithLifecycleNotifier(notifier), WithWorkspaceErrorStore(store))
	e := echo.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/workspaces/errors/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() { done <- handler.HandleWorkspaceErrorsStream(e.NewContext(req, rec)) }()
	waitForBodyContains(t, rec, done, "old event")
	store.listed = append(store.listed, WorkspaceErrorEvent{ID: 2, WorkspaceSlug: "feature", Source: "log", Severity: "error", Message: "new event", OccurrenceCount: 1})
	notifier.Notify("feature")
	waitForBodyContains(t, rec, done, "new event")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("HandleWorkspaceErrorsStream() error = %v", err)
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

func TestWorkspacesPageRendersWorkspaceTableAndDialogs(t *testing.T) {
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
		false,
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
		"<table",
		`data-workspace-row="main"`,
		`id="workspace-dialog-main"`,
		"QRSPI",
		"Runtime",
		"Branch",
		"Commit",
		"Release",
		"/workspaces/switch/main",
		"127.0.0.1:4200",
		`method="post"`,
		`data-init="@get('/workspaces/stream')"`,
		`id="workspaces-list"`,
		"Feature Branch",
		`data-workspace-row="feature"`,
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

func TestReleasePanelCollapsesLanesAndMergeQueue(t *testing.T) {
	panel := ReleasePanelModel{
		Enabled: true,
		Lanes: []ReleaseLaneView{
			{
				ID:    release.LaneID("stage"),
				Label: "Stage",
				Workspace: Workspace{
					Slug:   "stage",
					URL:    "https://stage.workspaces.test/",
					Status: StatusRunning,
					Commit: "abcdef1234567890",
				},
				Actions: []ReleaseActionView{{
					DefinitionID:      release.DefinitionID("default"),
					DefinitionVersion: "v1",
					FlowID:            release.FlowID("stage_to_main"),
					Label:             "stage → main",
					SourceSlug:        "stage",
					TargetLane:        release.LaneID("main"),
				}},
			},
			{
				ID:    release.LaneID("main"),
				Label: "Main",
				Workspace: Workspace{
					Slug:   "main",
					Status: StatusStopped,
					Commit: "1234567890abcdef",
				},
			},
		},
		Queue: ReleaseQueueView{
			Active: []ReleaseQueueItem{{ID: "active-1", Status: ReleaseQueueStatusRunning}},
		},
		History: []ReleaseQueueItem{{ID: "history-1", Status: ReleaseQueueStatusSucceeded}},
	}

	var body bytes.Buffer
	if err := ReleasePanel(panel).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		"<details",
		"Release lanes",
		"Merge queue · 1 active · 0 pending · 1 history",
		"Stage",
		"stage",
		"abcdef1",
		"Open",
		`action="/workspaces/stage/stop"`,
		"Main",
		"1234567",
		`action="/workspaces/main/start"`,
		"stage → main",
		"active-1",
		"history-1",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ReleasePanel missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<details open") || strings.Contains(html, " open=\"") {
		t.Fatalf("ReleasePanel details should be collapsed by default: %s", html)
	}
}

func TestReleasePanelDisabledStateUsesCompactCard(t *testing.T) {
	var body bytes.Buffer
	if err := ReleasePanel(ReleasePanelModel{}).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	if !strings.Contains(html, "Release queue not configured.") {
		t.Fatalf("disabled ReleasePanel missing message: %s", html)
	}
	if strings.Contains(html, "Release lanes") || strings.Contains(html, "Merge queue") {
		t.Fatalf("disabled ReleasePanel rendered enabled disclosures: %s", html)
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
				false,
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
		false,
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
		false,
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
		`data-workspace-row="parent"`,
		`data-workspace-row="review-child"`,
		`id="workspace-dialog-review-child"`,
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
						CheckoutPath:  "/repo/active-child",
						Status:        string(ImplWorkspaceStatusActive),
					},
					Runtime:    snapshotFromState(Workspace{Slug: "active-child", CheckoutPath: "/repo/active-child"}, WorkspaceLifecycleState{}),
					HasRuntime: true,
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
				CheckoutPath:  "/repo/active-parent",
				Status:        string(ImplWorkspaceStatusActive),
			},
			Runtime:    snapshotFromState(Workspace{Slug: "active-parent", CheckoutPath: "/repo/active-parent"}, WorkspaceLifecycleState{}),
			HasRuntime: true,
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

func TestWorkspacesPageRendersHistoricalToggleAndPreservesMode(t *testing.T) {
	views := BuildImplWorkspaceViews([]db.ImplWorkspace{{
		WorkspaceSlug: "active",
		CheckoutPath:  "/repo/active",
		DisplayName:   "Active Workspace",
		Status:        string(ImplWorkspaceStatusActive),
	}}, nil, WorkspaceLifecycleSnapshot{})

	var body bytes.Buffer
	if err := WorkspacesPage(views, "https://main.cn-agents.test", false).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render(default) error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"Show historical", `data-init="@get('/workspaces/stream')"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("default page missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "Hide historical") {
		t.Fatalf("default page unexpectedly contained Hide historical: %s", html)
	}
	idx := strings.Index(html, `action="/workspaces/refresh"`)
	if idx < 0 {
		t.Fatalf("missing refresh form: %s", html)
	}
	next := html[idx:]
	end := strings.Index(next, "</form>")
	if end < 0 {
		t.Fatalf("refresh form not closed: %s", next)
	}
	if strings.Contains(next[:end], `name="show_historical" value="true"`) {
		t.Fatalf("default refresh form unexpectedly preserved historical mode: %s", next[:end])
	}

	body.Reset()
	if err := WorkspacesPage(views, "https://main.cn-agents.test", true).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render(historical) error = %v", err)
	}
	html = body.String()
	for _, want := range []string{"Hide historical", `name="show_historical" value="true"`, `data-init="@get('/workspaces/stream?show_historical=true')"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("historical page missing %q: %s", want, html)
		}
	}
}

func TestWorkspacesPagePreservesHistoricalModeInLifecycleActionForms(t *testing.T) {
	snap := WorkspaceLifecycleSnapshot{
		Workspace: Workspace{
			Slug:         "feature",
			DisplayName:  "feature branch",
			CheckoutPath: "/repo/feature",
			URL:          "https://feature.test",
			Status:       StatusRunning,
		},
		ObservedState: WorkspaceObservedRunning,
	}
	var body bytes.Buffer
	if err := WorkspacesPage(
		lifecycleSnapshotsToImplViews([]WorkspaceLifecycleSnapshot{snap}),
		"https://main.cn-agents.test",
		true,
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, action := range []string{"restart", "stop"} {
		idx := strings.Index(html, `action="/workspaces/feature/`+action+`"`)
		if idx < 0 {
			t.Fatalf("missing %s form: %s", action, html)
		}
		next := html[idx:]
		end := strings.Index(next, "</form>")
		if end < 0 || !strings.Contains(next[:end], `name="show_historical" value="true"`) {
			t.Fatalf("%s form did not preserve show_historical: %s", action, next)
		}
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
		false,
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

func TestHandleWorkspacesPageHidesHistoricalRowsByDefault(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{
			snapshotFromState(Workspace{Slug: "active", CheckoutPath: "/repo/active", Status: StatusRunning}, WorkspaceLifecycleState{}),
		}},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{
			{
				WorkspaceSlug: "active",
				CheckoutPath:  "/repo/active",
				DisplayName:   "Active Workspace",
				Status:        string(ImplWorkspaceStatusActive),
			},
			{
				WorkspaceSlug: "merged",
				CheckoutPath:  "/repo/merged",
				DisplayName:   "Merged Workspace",
				Status:        string(ImplWorkspaceStatusMerged),
			},
			{
				WorkspaceSlug: "cleaned",
				CheckoutPath:  "/repo/cleaned",
				DisplayName:   "Cleaned Workspace",
				Status:        string(ImplWorkspaceStatusCleanedUp),
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
	for _, want := range []string{"Active Workspace", "Show historical"} {
		if !strings.Contains(html, want) {
			t.Fatalf("default page missing %q: %s", want, html)
		}
	}
	for _, absent := range []string{"Merged Workspace", "Cleaned Workspace", "Historical workspace"} {
		if strings.Contains(html, absent) {
			t.Fatalf("default page unexpectedly contained %q: %s", absent, html)
		}
	}
}

func TestHandleWorkspacesPageShowsHistoricalRowsWhenRequested(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{
			{
				WorkspaceSlug: "active",
				CheckoutPath:  "/repo/active",
				DisplayName:   "Active Workspace",
				Status:        string(ImplWorkspaceStatusActive),
			},
			{
				WorkspaceSlug: "merged",
				CheckoutPath:  "/repo/merged",
				DisplayName:   "Merged Workspace",
				Status:        string(ImplWorkspaceStatusMerged),
			},
			{
				WorkspaceSlug: "cleaned",
				CheckoutPath:  "/repo/cleaned",
				DisplayName:   "Cleaned Workspace",
				Status:        string(ImplWorkspaceStatusCleanedUp),
			},
		}}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces?show_historical=true", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{
		"Active Workspace",
		"Merged Workspace",
		"Cleaned Workspace",
		"Historical workspace",
		"Merged",
		"Cleaned up",
		"Hide historical",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("historical page missing %q: %s", want, html)
		}
	}
	for _, absent := range []string{
		`action="/workspaces/merged/start"`,
		`action="/workspaces/cleaned/start"`,
		`action="/workspaces/merged/restart"`,
		`action="/workspaces/cleaned/stop"`,
	} {
		if strings.Contains(html, absent) {
			t.Fatalf("historical page unexpectedly contained action %q: %s", absent, html)
		}
	}
}

func TestHandleWorkspacesPagePromotesActiveChildUnderHiddenHistoricalParent(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{
			snapshotFromState(Workspace{Slug: "child", CheckoutPath: "/repo/child", Status: StatusRunning}, WorkspaceLifecycleState{}),
		}},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{
			{
				WorkspaceSlug: "parent",
				CheckoutPath:  "/repo/parent",
				DisplayName:   "Merged Parent",
				Status:        string(ImplWorkspaceStatusMerged),
				PlanDirRel: sql.NullString{
					String: "creative-mode-agent/plans/parent",
					Valid:  true,
				},
			},
			{
				WorkspaceSlug: "child",
				CheckoutPath:  "/repo/child",
				DisplayName:   "Active Review Child",
				Status:        string(ImplWorkspaceStatusActive),
				PlanDirRel: sql.NullString{
					String: "creative-mode-agent/plans/parent/reviews/implementation-review",
					Valid:  true,
				},
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
	if !strings.Contains(html, "Active Review Child") {
		t.Fatalf("active child hidden with historical parent: %s", html)
	}
	if strings.Contains(html, "Merged Parent") || strings.Contains(html, `data-workspace-children="parent"`) {
		t.Fatalf("historical parent still rendered in default mode: %s", html)
	}
}

func TestHandleWorkspacesPageHidesMissingActiveRowsByDefault(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
			WorkspaceSlug: "missing",
			CheckoutPath:  "/repo/missing",
			DisplayName:   "Missing Workspace",
			Status:        string(ImplWorkspaceStatusActive),
		}}}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	if strings.Contains(rec.Body.String(), "Missing Workspace") {
		t.Fatalf("default page rendered missing active row: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/workspaces?show_historical=true", nil)
	rec = httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage(historical) error = %v", err)
	}
	if !strings.Contains(rec.Body.String(), "Missing Workspace") {
		t.Fatalf("historical page hid missing active row: %s", rec.Body.String())
	}
}

func TestHandleCleanupWorkspaceAlreadyCleanedIsIdempotent(t *testing.T) {
	starter := &fakeCleanupStarter{}
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
			WorkspaceSlug: "cleaned",
			CheckoutPath:  "/repo/cleaned",
			DisplayName:   "Cleaned Workspace",
			Status:        string(ImplWorkspaceStatusCleanedUp),
		}}}),
		WithWorkspaceCleanupStarter(starter),
	)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/workspaces/cleanup", strings.NewReader("slug=cleaned"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	if err := handler.HandleCleanupWorkspace(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleCleanupWorkspace(datastar) error = %v", err)
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want ok/accepted", rec.Code)
	}
	if len(starter.inputs) != 0 {
		t.Fatalf("idempotent cleanup started workflow: %#v", starter.inputs)
	}
	if strings.Contains(rec.Body.String(), "Cleaned Workspace") {
		t.Fatalf("default idempotent patch rendered cleaned row: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/workspaces/cleanup", strings.NewReader("slug=cleaned"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	if err := handler.HandleCleanupWorkspace(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleCleanupWorkspace(redirect) error = %v", err)
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/workspaces" {
		t.Fatalf("redirect = status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestHandleCleanupWorkspaceDatastarOmitsCleanedSlugFromDefaultPatch(t *testing.T) {
	starter := &fakeCleanupStarter{}
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{
			snapshotFromState(Workspace{Slug: "active", CheckoutPath: "/repo/active", Status: StatusRunning}, WorkspaceLifecycleState{}),
		}},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
			WorkspaceSlug: "active",
			CheckoutPath:  "/repo/active",
			DisplayName:   "Active Workspace",
			Status:        string(ImplWorkspaceStatusActive),
		}}}),
		WithWorkspaceCleanupStarter(starter),
	)
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		"/workspaces/cleanup",
		strings.NewReader("slug=active&confirmed=true"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	if err := handler.HandleCleanupWorkspace(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleCleanupWorkspace() error = %v", err)
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want ok/accepted", rec.Code)
	}
	if len(starter.inputs) != 1 || starter.inputs[0].Slug != "active" {
		t.Fatalf("cleanup inputs = %#v", starter.inputs)
	}
	if strings.Contains(rec.Body.String(), "Active Workspace") {
		t.Fatalf("default cleanup patch rendered cleaned slug: %s", rec.Body.String())
	}
}

func TestHandleCleanupWorkspaceRejectsProtectedReleaseLane(t *testing.T) {
	_, reg, err := BuildDefaultReleaseRegistry("stage", "main")
	if err != nil {
		t.Fatalf("BuildDefaultReleaseRegistry: %v", err)
	}
	handler := NewHandler(
		&fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{
			snapshotFromState(Workspace{Slug: "main", CheckoutPath: "/repo/main", Status: StatusRunning, IsMain: true}, WorkspaceLifecycleState{}),
		}},
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
			WorkspaceSlug: "main",
			CheckoutPath:  "/repo/main",
			DisplayName:   "Main",
			Status:        string(ImplWorkspaceStatusActive),
		}}}),
		WithReleaseProjector(&ReleaseProjector{Registry: reg}),
		WithWorkspaceCleanupStarter(&fakeCleanupStarter{}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/cleanup", strings.NewReader("slug=main"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	err = handler.HandleCleanupWorkspace(e.NewContext(req, rec))
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("HandleCleanupWorkspace() err = %#v, want bad request", err)
	}
}

func TestHandleRefreshWorkspacesRedirectPreservesHistoricalMode(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) { return WorkspaceSyncRefreshResult{}, nil }),
	)
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		"/workspaces/refresh",
		strings.NewReader("show_historical=true"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	if got := rec.Header().Get("Location"); got != "/workspaces?show_historical=true" {
		t.Fatalf("Location = %q, want historical redirect", got)
	}
}

func TestLifecycleActionDatastarPreservesHistoricalMode(t *testing.T) {
	manager := &fakeLifecycleManager{snapshots: []WorkspaceLifecycleSnapshot{{
		Workspace: Workspace{
			Slug:         "active",
			DisplayName:  "Active Workspace",
			CheckoutPath: "/repo/active",
			Status:       StatusRunning,
		},
		ObservedState: WorkspaceObservedRunning,
	}}}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{
			{
				WorkspaceSlug: "active",
				CheckoutPath:  "/repo/active",
				DisplayName:   "Active Workspace",
				Status:        string(ImplWorkspaceStatusActive),
			},
			{
				WorkspaceSlug: "merged",
				CheckoutPath:  "/repo/merged",
				DisplayName:   "Merged Workspace",
				Status:        string(ImplWorkspaceStatusMerged),
			},
		}}),
	)
	e := echo.New()

	for _, tc := range []struct {
		name       string
		body       string
		wantMerged bool
	}{
		{name: "default", body: "", wantMerged: false},
		{name: "historical", body: "show_historical=true", wantMerged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/workspaces/active/restart",
				strings.NewReader(tc.body),
			)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Datastar-Request", "true")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("slug")
			c.SetParamValues("active")
			if err := handler.HandleRestart(c); err != nil {
				t.Fatalf("HandleRestart() error = %v", err)
			}
			html := rec.Body.String()
			if got := strings.Contains(html, "Merged Workspace"); got != tc.wantMerged {
				t.Fatalf("merged visibility = %v, want %v: %s", got, tc.wantMerged, html)
			}
		})
	}
}

func TestHandleRefreshWorkspacesRecordsResultAndNotifies(t *testing.T) {
	notifier := NewLifecycleNotifier()
	notifications, unsubscribe := notifier.Subscribe()
	defer unsubscribe()
	managerRefreshes := 0
	manager := &fakeLifecycleManager{
		beforeRefresh: func() {
			managerRefreshes++
		},
	}
	wantResult := WorkspaceSyncRefreshResult{
		ImplUpserted:    1,
		ImplRepairedEnv: 1,
		Changed:         true,
	}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithLifecycleNotifier(notifier),
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) {
			return wantResult, nil
		}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/refresh", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	waitForNotifications(t, notifications, 1)
	waitForRefreshDone(t, handler)
	waitForNotifications(t, notifications, 1)
	state := handler.refreshState()
	if state.InFlight {
		t.Fatal("refresh still in flight")
	}
	if state.LastResult != wantResult {
		t.Fatalf("LastResult = %+v, want %+v", state.LastResult, wantResult)
	}
	if state.LastError != "" {
		t.Fatalf("LastError = %q, want empty", state.LastError)
	}
	if state.CompletedAt.IsZero() {
		t.Fatal("CompletedAt is zero")
	}
	if managerRefreshes != 1 {
		t.Fatalf("manager refreshes = %d, want 1", managerRefreshes)
	}
}

func TestHandleRefreshWorkspacesPatchesAfterManagerRefreshAndTerminalAdoption(t *testing.T) {
	managerRefreshes := 0
	manager := &fakeLifecycleManager{beforeRefresh: func() { managerRefreshes++ }}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithImplWorkspaces(fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
			WorkspaceSlug: "adopted-workspace",
			CheckoutPath:  "/repo/adopted-workspace",
			DisplayName:   "Adopted Workspace",
			Status:        string(ImplWorkspaceStatusActive),
		}}}),
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) {
			return WorkspaceSyncRefreshResult{ImplUpserted: 1, ImplRepairedEnv: 1, Changed: true}, nil
		}),
		WithWorkspaceSyncCompletion(func(ctx context.Context, result WorkspaceSyncRefreshResult, err error) WorkspaceSyncRefreshResult {
			if err != nil {
				return result
			}
			_ = manager.Refresh(ctx)
			result.ImportedPiSessions = 1
			result.AdoptedQRSPIWorkspaces = 1
			result.Changed = true
			return result
		}),
	)
	e := echo.New()
	refreshReq := httptest.NewRequest(http.MethodPost, "/workspaces/refresh", nil)
	refreshReq.Header.Set("Datastar-Request", "true")
	refreshRec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(refreshReq, refreshRec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	if !strings.Contains(refreshRec.Body.String(), "Refreshing workspace registry") {
		t.Fatalf("Datastar start patch missing refreshing state: %s", refreshRec.Body.String())
	}
	waitForRefreshDone(t, handler)
	if managerRefreshes != 1 {
		t.Fatalf("manager refreshes = %d, want 1", managerRefreshes)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	pageRec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(pageReq, pageRec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	html := pageRec.Body.String()
	for _, want := range []string{"Adopted Workspace", "env repaired 1", "terminal imported 1", "QRSPI adopted 1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q after refresh completion: %s", want, html)
		}
	}
}

func TestHandleRefreshWorkspacesDatastarShowsInFlightStatus(t *testing.T) {
	releaseSync := make(chan struct{})
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) {
			<-releaseSync
			return WorkspaceSyncRefreshResult{}, nil
		}),
	)
	defer close(releaseSync)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/refresh", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"Refreshing...", "Refreshing workspace registry"} {
		if !strings.Contains(html, want) {
			t.Fatalf("Datastar refresh response missing %q: %s", want, html)
		}
	}
}

func TestHandleWorkspacesPageRendersRefreshResultSummary(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
	)
	handler.recordWorkspaceSyncRefresh(WorkspaceSyncRefreshResult{
		ImplUpserted:           1,
		ImportedPiSessions:     2,
		AdoptedQRSPIWorkspaces: 1,
		Changed:                true,
	}, nil, time.Date(2026, 5, 24, 12, 34, 56, 0, time.UTC))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"Last refresh:", "workspaces 1", "terminal imported 2", "QRSPI adopted 1", "12:34:56"} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing refresh summary %q: %s", want, html)
		}
	}
}

func TestHandleWorkspacesPageRendersRefreshError(t *testing.T) {
	handler := NewHandler(
		&fakeLifecycleManager{},
		"https://main.cn-agents.test",
		"main",
	)
	handler.recordWorkspaceSyncRefresh(WorkspaceSyncRefreshResult{}, errors.New("boom"), time.Now())
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	html := rec.Body.String()
	if !strings.Contains(html, "Last refresh failed: boom") {
		t.Fatalf("page missing refresh error: %s", html)
	}
}

func TestHandleRefreshWorkspacesRecordsRefreshError(t *testing.T) {
	notifier := NewLifecycleNotifier()
	notifications, unsubscribe := notifier.Subscribe()
	defer unsubscribe()
	managerRefreshes := 0
	handler := NewHandler(
		&fakeLifecycleManager{beforeRefresh: func() { managerRefreshes++ }},
		"https://main.cn-agents.test",
		"main",
		WithLifecycleNotifier(notifier),
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) {
			return WorkspaceSyncRefreshResult{}, errors.New("boom")
		}),
	)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/workspaces/refresh", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleRefreshWorkspaces(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleRefreshWorkspaces() error = %v", err)
	}
	waitForNotifications(t, notifications, 1)
	waitForRefreshDone(t, handler)
	waitForNotifications(t, notifications, 1)
	state := handler.refreshState()
	if !strings.Contains(state.LastError, "boom") {
		t.Fatalf("LastError = %q, want boom", state.LastError)
	}
	if state.CompletedAt.IsZero() {
		t.Fatal("CompletedAt is zero")
	}
	if managerRefreshes != 0 {
		t.Fatalf("manager refreshes = %d, want 0", managerRefreshes)
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
		WithWorkspaceSyncRefresh(func(context.Context) (WorkspaceSyncRefreshResult, error) {
			syncCalled <- struct{}{}
			<-releaseSync
			return WorkspaceSyncRefreshResult{}, nil
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

func waitForRefreshDone(t *testing.T, handler *Handler) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if !handler.refreshState().InFlight {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for refresh to finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func waitForNotifications(t *testing.T, notifications <-chan struct{}, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for i := 0; i < count; i++ {
		select {
		case <-notifications:
		case <-deadline:
			t.Fatalf("timed out waiting for notification %d", i+1)
		}
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

type fakeCleanupStarter struct {
	inputs []WorkspaceCleanupWorkflowInput
	err    error
}

func (f *fakeCleanupStarter) StartCleanup(_ context.Context, input WorkspaceCleanupWorkflowInput) error {
	if f.err != nil {
		return f.err
	}
	f.inputs = append(f.inputs, input)
	return nil
}

type fakeWorkspaceErrorEventStore struct {
	events []UpsertWorkspaceErrorEventParams
	listed []WorkspaceErrorEvent
}

func (f *fakeWorkspaceErrorEventStore) UpsertWorkspaceErrorEvent(_ context.Context, arg UpsertWorkspaceErrorEventParams) (WorkspaceErrorEvent, error) {
	f.events = append(f.events, arg)
	event := WorkspaceErrorEvent{
		ID:              int64(len(f.events)),
		WorkspaceSlug:   arg.WorkspaceSlug,
		Source:          string(arg.Source),
		Severity:        string(arg.Severity),
		Message:         arg.Message,
		Detail:          arg.Detail,
		DedupeKey:       arg.DedupeKey,
		OccurrenceCount: 1,
	}
	f.listed = append(f.listed, event)
	return event, nil
}

func (f *fakeWorkspaceErrorEventStore) ListRecentWorkspaceErrorEvents(_ context.Context, _ int64) ([]WorkspaceErrorEvent, error) {
	return append([]WorkspaceErrorEvent(nil), f.listed...), nil
}

func (f *fakeWorkspaceErrorEventStore) ListRecentWorkspaceErrorEventsForWorkspace(_ context.Context, arg ListRecentWorkspaceErrorEventsForWorkspaceParams) ([]WorkspaceErrorEvent, error) {
	out := make([]WorkspaceErrorEvent, 0, len(f.listed))
	for _, event := range f.listed {
		if event.WorkspaceSlug == arg.WorkspaceSlug {
			out = append(out, event)
		}
	}
	return out, nil
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
