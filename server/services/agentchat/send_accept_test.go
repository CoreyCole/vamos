package agentchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
)

type blockingTemporal struct {
	started chan struct{}
	release chan struct{}
	calls   int
	mu      sync.Mutex
}

func (b *blockingTemporal) StartWorkflow(
	ctx context.Context,
	_ string,
	_ any,
	_ any,
) (string, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return "workflow_blocked", nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(5 * time.Second):
		return "workflow_blocked_timeout", nil
	}
}

func (b *blockingTemporal) SignalWithStartWorkflow(
	ctx context.Context,
	_ string,
	_ string,
	_ any,
	_ any,
	_ any,
) (string, error) {
	return b.StartWorkflow(ctx, "", nil, nil)
}

func setupWorkspaceResumeFixture(
	t *testing.T,
) (*Service, *db.Queries, *blockingTemporal) {
	t.Helper()
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	if _, err := queries.CreateWorkspace(t.Context(), db.CreateWorkspaceParams{
		ID:           "workspace_1",
		UserEmail:    "owner@example.com",
		Title:        "Plan workspace",
		RootDocPath:  "thoughts/plan",
		WorkflowType: "agent-chat",
		Source:       "web",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpsertThreadWorkspaceAssociation(
		t.Context(),
		db.UpsertThreadWorkspaceAssociationParams{
			ThreadID:    "thread_1",
			WorkspaceID: "workspace_1",
			IsPrimary:   1,
			Role:        "primary",
			AdoptedFrom: "test",
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := queries.AttachThreadToWorkspace(
		t.Context(),
		db.AttachThreadToWorkspaceParams{
			ID:          "thread_1",
			WorkspaceID: nullString("workspace_1"),
		},
	); err != nil {
		t.Fatal(err)
	}
	blocker := &blockingTemporal{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	service.temporal = blocker
	service.liveThreads = make(map[string]*liveThreadState)
	return service, queries, blocker
}

func TestResumeWorkspaceThreadAcceptPatchesBeforeTemporal(t *testing.T) {
	service, _, blocker := setupWorkspaceResumeFixture(t)
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"hello accept now"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/workspace_1/thread/thread_1/resume",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("workspace_id", "thread_id")
	ctx.SetParamValues("workspace_1", "thread_1")
	ctx.Set("user_email", "owner@example.com")

	done := make(chan error, 1)
	go func() {
		done <- handler.ResumeWorkspaceThread(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResumeWorkspaceThread error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accept handler blocked on Temporal StartWorkflow")
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-live-transcript"`) {
		t.Fatalf("missing live transcript morph: %s", body)
	}
	if !strings.Contains(body, `id="agent-chat-working"`) ||
		!strings.Contains(body, `data-testid="agent-chat-working"`) {
		t.Fatalf("missing working row in live transcript: %s", body)
	}
	if !strings.Contains(body, "hello accept now") {
		t.Fatalf("missing optimistic user prompt in live transcript: %s", body)
	}
	if !strings.Contains(body, `"chatDraft":""`) {
		t.Fatalf("missing chatDraft clear: %s", body)
	}
	if strings.Contains(body, "datastar-execute-script") ||
		strings.Contains(body, "composer?.reset()") {
		t.Fatalf("must not ExecuteScript composer clear: %s", body)
	}
	if strings.Contains(body, `id="agent-chat-scroll-region"`) {
		t.Fatalf("must not remorph Host scroll region: %s", body)
	}
	if strings.Contains(body, `id="agent-chat-messages"`) {
		t.Fatalf("must not morph messages chrome: %s", body)
	}
	if strings.Contains(body, "agent-chat-turn-working") {
		t.Fatalf("forbidden working id: %s", body)
	}

	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Temporal StartWorkflow was not started asynchronously")
	}
	close(blocker.release)
}

func TestResumeEmbeddedWorkspaceThreadAcceptPatchesLiveOnly(t *testing.T) {
	service, _, blocker := setupWorkspaceResumeFixture(t)
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"embedded accept"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/thoughts/chat/thread/thread_1/resume?workbench_v2=1",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues("thread_1")
	ctx.Set("user_email", "owner@example.com")

	done := make(chan error, 1)
	go func() {
		done <- handler.ResumeEmbeddedThread(ctx)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResumeEmbeddedThread error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("embedded accept blocked on Temporal")
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-working"`) ||
		!strings.Contains(body, "embedded accept") ||
		!strings.Contains(body, `"chatDraft":""`) {
		t.Fatalf("embedded accept missing patches: %s", body)
	}
	if strings.Contains(body, "datastar-execute-script") ||
		strings.Contains(body, "composer?.reset()") {
		t.Fatalf("must not ExecuteScript composer clear: %s", body)
	}
	if strings.Contains(body, `id="agent-chat-scroll-region"`) {
		t.Fatalf("must not remorph Host scroll region: %s", body)
	}
	if strings.Contains(body, "doc-right-chat-panel") {
		t.Fatalf("v2 must not morph right rail chrome: %s", body)
	}
	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("async Temporal not started")
	}
	close(blocker.release)
}

func TestLiveTranscriptRegionSSROmitsWorkingWithoutFlag(t *testing.T) {
	var buf strings.Builder
	err := LiveTranscriptRegion(
		"thread_1",
		TranscriptPaneState{},
		"",
	).Render(t.Context(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `id="agent-chat-working"`) {
		t.Fatalf("SSR cold GET must not include working row: %s", html)
	}
	if !strings.Contains(html, "Waiting for the first completed turn") {
		t.Fatalf("SSR empty live missing waiting copy: %s", html)
	}
}

func setupFreeformResumeFixture(t *testing.T) (*Service, *db.Queries, *blockingTemporal) {
	t.Helper()
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	blocker := &blockingTemporal{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	service.temporal = blocker
	service.liveThreads = make(map[string]*liveThreadState)
	return service, queries, blocker
}

func TestResumeEmbeddedFreeformThreadAcceptPatchesBeforeTemporal(t *testing.T) {
	service, _, blocker := setupFreeformResumeFixture(t)
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"freeform accept now"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/thoughts/chat/thread/thread_1/resume?workbench_v2=1",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues("thread_1")
	ctx.Set("user_email", "owner@example.com")

	done := make(chan error, 1)
	go func() {
		done <- handler.ResumeEmbeddedThread(ctx)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResumeEmbeddedThread error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("freeform accept blocked on Temporal SignalWithStart")
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-live-transcript"`) {
		t.Fatalf("missing live transcript morph: %s", body)
	}
	if !strings.Contains(body, `id="agent-chat-working"`) ||
		!strings.Contains(body, `data-testid="agent-chat-working"`) {
		t.Fatalf("missing working row in live transcript: %s", body)
	}
	if !strings.Contains(body, "freeform accept now") {
		t.Fatalf("missing optimistic user prompt in live transcript: %s", body)
	}
	if !strings.Contains(body, `id="live-message-000"`) &&
		!strings.Contains(body, "live-message-000") {
		t.Fatalf("missing live-message-000 key matching reducer: %s", body)
	}
	if !strings.Contains(body, `"chatDraft":""`) {
		t.Fatalf("missing chatDraft clear: %s", body)
	}
	if strings.Contains(body, "datastar-execute-script") ||
		strings.Contains(body, "composer?.reset()") {
		t.Fatalf("must not ExecuteScript composer clear: %s", body)
	}
	if strings.Contains(body, `id="agent-chat-scroll-region"`) {
		t.Fatalf("must not remorph Host scroll region: %s", body)
	}
	if strings.Contains(body, "doc-right-chat-panel") {
		t.Fatalf("v2 must not morph right rail chrome: %s", body)
	}

	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Temporal SignalWithStart was not started asynchronously")
	}
	close(blocker.release)
}

func TestLiveTranscriptShowWorkingDerivedFromRunAndLive(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)

	state, err := service.BuildLiveTranscriptState(
		t.Context(),
		"owner@example.com",
		"thread_1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if state.ShowWorking {
		t.Fatal("expected ShowWorking false with no in-flight run")
	}

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_pending_1",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "seeded user",
		WorkflowID:  "wf_1",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(
		db.AgentThread{ID: "thread_1"},
		run,
	); err != nil {
		t.Fatal(err)
	}

	state, err = service.BuildLiveTranscriptState(
		t.Context(),
		"owner@example.com",
		"thread_1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !state.ShowWorking {
		t.Fatal("expected ShowWorking true with running run and user-only live")
	}
	if len(state.Live.Items) == 0 {
		t.Fatal("expected seeded user live item")
	}

	// Assistant live item drops working (Architect lock) — call helper directly
	// so we do not need a markdown renderer for bubble HTML.
	liveWithAssistant := LiveTranscriptView{
		Items: append(
			append([]TranscriptMessage{}, state.Live.Items...),
			TranscriptMessage{
				Role:    "assistant",
				Content: "hello",
				Variant: "bubble",
			},
		),
	}
	if service.liveTranscriptShowWorking("thread_1", liveWithAssistant) {
		t.Fatal("expected ShowWorking false once assistant live item exists")
	}

	if err := queries.CompleteAgentRun(t.Context(), db.CompleteAgentRunParams{
		ID: run.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if service.liveTranscriptShowWorking("thread_1", state.Live) {
		t.Fatal("expected ShowWorking false when latest run is complete")
	}
}

func TestSeedPendingUserPromptNotifiesThreadWithoutWorkspace(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.liveThreads = make(map[string]*liveThreadState)
	service.liveFlush = nil
	service.notifier = NewNotifier()

	ch := service.notifier.Subscribe("thread_1")
	defer service.notifier.Unsubscribe("thread_1", ch)

	run, err := queries.CreateAgentRun(t.Context(), db.CreateAgentRunParams{
		ID:          "run_no_ws",
		ThreadID:    "thread_1",
		Trigger:     "resume",
		Status:      "running",
		PromptText:  "docs desk prompt",
		WorkflowID:  "wf_no_ws",
		RootDocPath: "thoughts/plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.seedPendingUserPrompt(
		db.AgentThread{ID: "thread_1"},
		run,
	); err != nil {
		t.Fatal(err)
	}
	live, _ := service.buildLiveTranscript("thread_1")
	if len(live.Items) == 0 {
		t.Fatal("seed must apply without WorkspaceID")
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected live transcript notify on threadID")
	}
}
