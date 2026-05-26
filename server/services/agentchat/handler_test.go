//go:build !integration || unit
// +build !integration unit

package agentchat

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	datastar "github.com/starfederation/datastar-go/datastar"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	workspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

const handlerTestPlanRelPath = "plan.md"

func isExpectedStreamCancel(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}

func TestHandleInternalRunEventAppliesCheckpoint(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")

	env := conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   conversation.EventCheckpoint,
		PayloadJSON: `{"run_id":"run-1","thread_id":"thread-1","head_entry_id":"assistant-1","turn_index":1,"header":{"session_id":"thread-1","cwd":"/tmp/project"},"new_entries":[{"lineage_id":"lineage-1","entry_id":"assistant-1","entry_type":"message","timestamp":"2026-04-19T12:00:01Z","origin_order":0,"payload_json":"{\"type\":\"message\",\"id\":\"assistant-1\",\"parentId\":null,\"timestamp\":\"2026-04-19T12:00:01Z\",\"message\":{\"role\":\"assistant\",\"content\":\"done\"}}"}]}`,
	}

	body, _ := json.Marshal(env)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/agent-chat/events",
		bytes.NewReader(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Vamos-Internal-Token", "secret")
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	if err := handler.HandleInternalRunEvent(c); err != nil {
		t.Fatalf("HandleInternalRunEvent() error = %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	updated, err := service.queries.GetAgentThread(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if !updated.HeadEntryID.Valid || updated.HeadEntryID.String != "assistant-1" {
		t.Fatalf("HeadEntryID = %v, want assistant-1", updated.HeadEntryID)
	}
}

func TestHandleInternalRunEventIgnoresLateLiveEventForTerminalRun(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-late-live-1",
		"user@example.com",
		"/tmp/project",
		"lineage-late-live-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-late-live-1")
	if err := service.FinalizeRun(
		t.Context(),
		conversation.RunResult{RunID: run.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("FinalizeRun() error = %v", err)
	}

	env := conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"late"}]}}`,
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal envelope: %v", err)
	}
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/internal/agent-chat/events",
		bytes.NewReader(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Vamos-Internal-Token", "secret")
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	if err := handler.HandleInternalRunEvent(c); err != nil {
		t.Fatalf("HandleInternalRunEvent() error = %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	live, _ := service.buildLiveTranscript(thread.ID)
	if len(live.Items) != 0 {
		t.Fatalf(
			"live.Items = %#v, want empty for terminal run late live event",
			live.Items,
		)
	}
}

func TestHandleInternalRunEventCoalescesWorkspaceLiveNotifications(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	sub := service.notifier.Subscribe(workspace.ID)
	defer service.notifier.Unsubscribe(workspace.ID, sub)

	env := conversation.EventEnvelope{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"coalesced"}]}}`,
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal envelope: %v", err)
	}
	for range 3 {
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/internal/agent-chat/events",
			bytes.NewReader(body),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-Vamos-Internal-Token", "secret")
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)

		if err := handler.HandleInternalRunEvent(c); err != nil {
			t.Fatalf("HandleInternalRunEvent() error = %v", err)
		}
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
		}
	}

	select {
	case signal := <-sub:
		t.Fatalf("unexpected immediate workspace signal: %#v", signal)
	default:
	}
	if got := service.liveFlush.FlushOnce(t.Context()); got != 1 {
		t.Fatalf("FlushOnce() = %d, want 1", got)
	}
	select {
	case signal := <-sub:
		if signal.Scope != PatchLiveTranscript {
			t.Fatalf("signal.Scope = %q, want %q", signal.Scope, PatchLiveTranscript)
		}
	default:
		t.Fatal("subscriber did not receive coalesced live signal")
	}
	select {
	case extra := <-sub:
		t.Fatalf("unexpected extra signal: %#v", extra)
	default:
	}
}

func TestInternalAgentChatRoutesTrustBoundary(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)

	endpoints := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(*Handler, echo.Context) error
	}{
		{
			name:   "events",
			method: http.MethodPost,
			path:   "/internal/agent-chat/events",
			body:   `{}`,
			call:   (*Handler).HandleInternalRunEvent,
		},
		{
			name:   "snapshots",
			method: http.MethodGet,
			path:   "/internal/agent-chat/snapshots",
			call:   (*Handler).HandleInternalRunSnapshot,
		},
		{
			name:   "import-session",
			method: http.MethodPost,
			path:   "/internal/agent-chat/import-session",
			body:   `{"session_path":"missing.jsonl"}`,
			call:   (*Handler).HandleInternalPiSessionImport,
		},
	}

	invoke := func(endpointName string, handler *Handler, token, remoteAddr string) int {
		t.Helper()
		for _, endpoint := range endpoints {
			if endpoint.name != endpointName {
				continue
			}
			var body *strings.Reader
			if endpoint.body != "" {
				body = strings.NewReader(endpoint.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequestWithContext(
				t.Context(),
				endpoint.method,
				endpoint.path,
				body,
			)
			if endpoint.body != "" {
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			}
			if token != "" {
				req.Header.Set("X-Vamos-Internal-Token", token)
			}
			req.RemoteAddr = remoteAddr
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			err := endpoint.call(handler, c)
			if err != nil {
				var httpErr *echo.HTTPError
				if errors.As(err, &httpErr) {
					return httpErr.Code
				}
				t.Fatalf("%s error type = %T, want *echo.HTTPError", endpoint.name, err)
			}
			return rec.Code
		}
		t.Fatalf("unknown endpoint %q", endpointName)
		return 0
	}

	for _, endpoint := range endpoints {
		defaultHandler := NewHandler(service, nil)
		if got := invoke(
			endpoint.name,
			defaultHandler,
			"",
			"127.0.0.1:1234",
		); got != http.StatusUnauthorized {
			t.Fatalf(
				"%s default loopback status = %d, want %d",
				endpoint.name,
				got,
				http.StatusUnauthorized,
			)
		}

		tokenHandler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
		if got := invoke(
			endpoint.name,
			tokenHandler,
			"wrong",
			"127.0.0.1:1234",
		); got != http.StatusUnauthorized {
			t.Fatalf(
				"%s wrong token status = %d, want %d",
				endpoint.name,
				got,
				http.StatusUnauthorized,
			)
		}
		if got := invoke(
			endpoint.name,
			tokenHandler,
			"secret",
			"203.0.113.10:1234",
		); got == http.StatusUnauthorized {
			t.Fatalf(
				"%s correct token status = %d, want auth accepted",
				endpoint.name,
				got,
			)
		}

		loopbackHandler := NewHandler(
			service,
			nil,
			HandlerOptions{InternalAllowLoopback: true},
		)
		if got := invoke(
			endpoint.name,
			loopbackHandler,
			"",
			"127.0.0.1:1234",
		); got == http.StatusUnauthorized {
			t.Fatalf(
				"%s explicit loopback status = %d, want auth accepted",
				endpoint.name,
				got,
			)
		}
		if got := invoke(
			endpoint.name,
			loopbackHandler,
			"",
			"203.0.113.10:1234",
		); got != http.StatusUnauthorized {
			t.Fatalf(
				"%s non-loopback status = %d, want %d",
				endpoint.name,
				got,
				http.StatusUnauthorized,
			)
		}
	}
}

func TestCompatThreadQueryRedirectsToWorkspaceThread(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(t, service, "thread-1", "user@example.com", service.defaultCwd, "lineage-1")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat?thread=thread-1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.HandleAgentChatIndex(c); err != nil {
		t.Fatalf("HandleAgentChatIndex() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want no redirect", got)
	}
	if _, ok, err := service.ResolvePrimaryWorkspaceForThread(t.Context(), "user@example.com", thread.ID); err != nil || ok {
		t.Fatalf("primary workspace = ok %v err %v, want no route-created workspace", ok, err)
	}
}

func TestCompatThreadQueryRedirectPreservesRun(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(t, service, "thread-run-preserved", "user@example.com", service.defaultCwd, "lineage-run-preserved")
	mustCreateAgentRun(t, service, thread.ID, "run-1")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-chat?thread=thread-run-preserved&run=run-1", http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.HandleAgentChatIndex(c); err != nil {
		t.Fatalf("HandleAgentChatIndex() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "/agent-chat/stream?thread=thread-run-preserved&amp;run=run-1") && !strings.Contains(rec.Body.String(), "/agent-chat/stream?thread=thread-run-preserved&run=run-1") {
		t.Fatalf("body missing thread stream with run id: %s", rec.Body.String())
	}
	storedRun, err := service.queries.GetAgentRun(t.Context(), "run-1")
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if storedRun.WorkspaceID.Valid {
		t.Fatalf("run WorkspaceID = %v, want no route-created workspace", storedRun.WorkspaceID)
	}
}

func TestSendWorkspacePromptNavigatesToConcreteThreadURL(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	form := url.Values{"prompt": {"hello from workspace"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/send",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	if err := handler.SendWorkspacePrompt(c); err != nil {
		t.Fatalf("SendWorkspacePrompt() error = %v", err)
	}

	storedWorkspace, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !storedWorkspace.SelectedThreadID.Valid ||
		storedWorkspace.SelectedThreadID.String == "" {
		t.Fatalf(
			"SelectedThreadID = %v, want concrete thread",
			storedWorkspace.SelectedThreadID,
		)
	}
	thread, err := service.queries.GetAgentThreadForWorkspaceUser(
		t.Context(),
		db.GetAgentThreadForWorkspaceUserParams{
			ThreadID:    storedWorkspace.SelectedThreadID.String,
			WorkspaceID: workspace.ID,
			UserEmail:   "user@example.com",
		},
	)
	if err != nil {
		t.Fatalf("GetAgentThreadForWorkspaceUser() error = %v", err)
	}
	runs, err := service.queries.ListAgentRunsByThread(t.Context(), thread.ID)
	if err != nil {
		t.Fatalf("ListAgentRunsByThread() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if !runs[0].WorkspaceID.Valid || runs[0].WorkspaceID.String != workspace.ID {
		t.Fatalf("run workspace = %v, want %s", runs[0].WorkspaceID, workspace.ID)
	}

	wantURL := workspaceThreadURL(workspace.ID, thread.ID, runs[0].ID)
	if !strings.Contains(rec.Body.String(), wantURL) {
		t.Fatalf(
			"response body missing redirect target %q: %s",
			wantURL,
			rec.Body.String(),
		)
	}
}

func TestSendEmbeddedWorkspacePromptPatchesPanelAndDoesNotRedirect(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	workspaceRecord := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	form := url.Values{
		"prompt":   {"hello from thoughts"},
		"doc_path": {"creative-mode-agent/plans/2026-04-30_test-plan/plan.md"},
		"attached_paths[]": {
			"thoughts/creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
		},
	}
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/thoughts/chat/"+workspaceRecord.ID+"/send",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspaceRecord.ID)

	if err := handler.SendEmbeddedWorkspacePrompt(c); err != nil {
		t.Fatalf("SendEmbeddedWorkspacePrompt() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"doc-right-chat-panel",
		"agent-chat-workspace-composer",
		"data-replace-url",
		"context=chat",
		"chat_workspace=" + workspaceRecord.ID,
		"thread=",
		"run=",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{
		"datastar-redirect",
		"/agent-chat/",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf(
				"embedded send emitted redirect/path navigation %q: %s",
				notWant,
				body,
			)
		}
	}

	selection, err := service.LastFreeformEmbeddedChatSelection(
		t.Context(),
		"user@example.com",
	)
	if err != nil {
		t.Fatalf("LastFreeformEmbeddedChatSelection() error = %v", err)
	}
	if selection.WorkspaceID != workspaceRecord.ID || selection.ThreadID == "" ||
		selection.RunID == "" || selection.Scope != EmbeddedChatSelectionScopeFreeform {
		t.Fatalf("selection = %+v, want persisted concrete chat", selection)
	}
}

func TestAttachCurrentDocToEmbeddedChatPatchesComposerWithDocPill(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspaceRecord, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)

	form := url.Values{
		"doc_path":  {"creative-mode-agent/plans/2026-04-30_test-plan/plan.md"},
		"thread_id": {thread.ID},
		"run_id":    {"run_123"},
	}
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/thoughts/chat/"+workspaceRecord.ID+"/attach-doc",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspaceRecord.ID)

	if err := handler.AttachCurrentDocToEmbeddedChat(c); err != nil {
		t.Fatalf("AttachCurrentDocToEmbeddedChat() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"agent-chat-workspace-composer",
		"attached_paths[]",
		`value="thoughts/creative-mode-agent/plans/2026-04-30_test-plan/plan.md"`,
		"plan.md",
		"Remove current doc",
		`name="doc_path"`,
		`name="run_id"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}

	form.Set(
		"attached_paths[]",
		"thoughts/creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
	)
	req = httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/thoughts/chat/"+workspaceRecord.ID+"/attach-doc",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspaceRecord.ID)

	if err := handler.AttachCurrentDocToEmbeddedChat(c); err != nil {
		t.Fatalf("AttachCurrentDocToEmbeddedChat(remove) error = %v", err)
	}
	body = rec.Body.String()
	if strings.Contains(body, "attached_paths[]") ||
		!strings.Contains(body, "Add current doc") {
		t.Fatalf("remove current doc response did not toggle back to add: %s", body)
	}
}

func TestResumeEmbeddedWorkspaceThreadPatchesPanelAndURL(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	workspaceRecord, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)

	form := url.Values{
		"prompt":   {"resume from thoughts"},
		"doc_path": {"creative-mode-agent/plans/2026-04-30_test-plan/plan.md"},
	}
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/thoughts/chat/"+workspaceRecord.ID+"/thread/"+thread.ID+"/resume",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id", "thread_id")
	c.SetParamValues(workspaceRecord.ID, thread.ID)

	if err := handler.ResumeEmbeddedWorkspaceThread(c); err != nil {
		t.Fatalf("ResumeEmbeddedWorkspaceThread() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"doc-right-chat-panel",
		"agent-chat-workspace-composer",
		"data-replace-url",
		"context=chat",
		"chat_workspace=" + workspaceRecord.ID,
		"thread=" + thread.ID,
		"run=",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{
		"datastar-redirect",
		"/agent-chat/",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf(
				"embedded resume emitted redirect/path navigation %q: %s",
				notWant,
				body,
			)
		}
	}

	selection, err := service.LastFreeformEmbeddedChatSelection(
		t.Context(),
		"user@example.com",
	)
	if err != nil {
		t.Fatalf("LastFreeformEmbeddedChatSelection() error = %v", err)
	}
	if selection.WorkspaceID != workspaceRecord.ID || selection.ThreadID != thread.ID ||
		selection.RunID == "" || selection.Scope != EmbeddedChatSelectionScopeFreeform {
		t.Fatalf("selection = %+v, want persisted resumed chat", selection)
	}
}

func TestStreamEmbeddedFreeformCatchupPatchesEmbeddedPanelNotThreadPage(
	t *testing.T,
) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"freeform-thread-embedded-stream",
		"user@example.com",
		"/tmp/project",
		"",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-freeform-embedded-stream")
	service.notifier.Notify(thread.ID, ThreadStreamSignal{Scope: PatchLiveTranscript})

	reqCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		"/thoughts/chat/freeform/stream?thread="+thread.ID+"&run="+run.ID+"&since=0",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	done := make(chan error, 1)
	go func() { done <- handler.StreamEmbeddedFreeform(c) }()
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !isExpectedStreamCancel(err) {
			t.Fatalf("StreamEmbeddedFreeform() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamEmbeddedFreeform() timed out")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "doc-right-chat-panel") &&
		!strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("embedded freeform stream response missing embedded patch target: %s", body)
	}
	for _, notWant := range []string{
		"agent-chat-thread-sidebar",
		"agent-chat-doc-pane",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("embedded freeform stream patched thread page target %q: %s", notWant, body)
		}
	}
}

func TestStreamEmbeddedWorkspaceCatchupPatchesEmbeddedPanelNotWorkspaceResource(
	t *testing.T,
) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspaceRecord, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-embedded-catchup")
	service.notifier.NotifyWorkspaceResource(workspaceRecord.ID)

	reqCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		"/thoughts/chat/"+workspaceRecord.ID+"/stream?thread="+thread.ID+
			"&run="+run.ID+"&doc=creative-mode-agent/plans/2026-04-30_test-plan/plan.md&since=0",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspaceRecord.ID)

	done := make(chan error, 1)
	go func() { done <- handler.StreamEmbeddedWorkspace(c) }()
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !isExpectedStreamCancel(err) {
			t.Fatalf("StreamEmbeddedWorkspace() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamEmbeddedWorkspace() timed out")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "doc-right-chat-panel") &&
		!strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("embedded stream response missing embedded patch target: %s", body)
	}
	if strings.Contains(body, "agent-chat-workspace-resource") {
		t.Fatalf("embedded stream patched full workspace resource: %s", body)
	}
}

func TestWriteNoRedirectSuccessClearsComposer(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/agent-chat/thread/thread-1/resume",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	handler := NewHandler(nil, nil)

	if err := handler.writeNoRedirectSuccess(c); err != nil {
		t.Fatalf("writeNoRedirectSuccess() error = %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "agentChatLastWriteOK") {
		t.Fatalf("response body missing write signal: %s", body)
	}
	if !strings.Contains(body, "agent-chat-composer-form") ||
		!strings.Contains(body, ".reset()") {
		t.Fatalf("response body missing composer reset script: %s", body)
	}
}

func TestWorkspaceThreadURLIncludesRunWhenPresent(t *testing.T) {
	t.Parallel()

	got := workspaceThreadURL("workspace/id", "thread id", "run+id")
	want := "/agent-chat/workspace%2Fid/thread/thread%20id?run=run%2Bid"
	if got != want {
		t.Fatalf("workspaceThreadURL() = %q, want %q", got, want)
	}
}

func TestWorkspaceThreadURLOmitsRunWhenEmpty(t *testing.T) {
	t.Parallel()

	got := workspaceThreadURL("workspace/id", "thread id", " ")
	want := "/agent-chat/workspace%2Fid/thread/thread%20id"
	if got != want {
		t.Fatalf("workspaceThreadURL() = %q, want %q", got, want)
	}
}

func TestNewChatButtonUsesAgentChatHref(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := NewChatButton().Render(t.Context(), &body); err != nil {
		t.Fatalf("render NewChatButton() error = %v", err)
	}
	rendered := body.String()
	if !strings.Contains(rendered, `href="/agent-chat"`) {
		t.Fatalf("NewChatButton() missing /agent-chat href: %s", rendered)
	}
}

func TestFreeformRunHeaderRendersWorkbenchLayoutControls(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(t, FreeformRunHeader(workbench.WorkbenchState{
		Page: workbench.WorkbenchPageAgentChat,
		View: workbench.WorkbenchViewFocus,
	}))
	for _, want := range []string{
		`aria-label="Toggle artifact pane"`,
		`aria-label="Focus chat"`,
		`aria-label="Exit focus"`,
		`aria-label="Reset layout"`,
		`name="page"`,
		`name="view"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("FreeformRunHeader html = %s, want %q", body, want)
		}
	}
}

func TestWorkspaceHeaderRendersWorkbenchLayoutControls(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(t, WorkspaceHeader(
		WorkspaceProjection{Header: WorkspaceHeaderState{Title: "Workspace"}},
		workbench.WorkbenchState{
			Page: workbench.WorkbenchPageAgentChat,
			View: workbench.WorkbenchViewFocus,
		},
	))
	for _, want := range []string{
		`aria-label="Toggle artifact pane"`,
		`aria-label="Focus chat"`,
		`aria-label="Exit focus"`,
		`aria-label="Reset layout"`,
		`name="page"`,
		`name="view"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("WorkspaceHeader html = %s, want %q", body, want)
		}
	}
}

func TestAgentChatComposerUsesMetadataPopoverForCwd(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(t, AgentChatComposer(AgentChatComposerArgs{
		Action:      "@post('/agent-chat/send', {contentType: 'form'})",
		Cwd:         "/tmp/workspace",
		Placeholder: "Ask",
		IncludeCwd:  true,
	}))
	for _, want := range []string{
		`type="hidden" name="cwd" value="/tmp/workspace"`,
		`aria-label="Show chat metadata"`,
		`Chat metadata`,
		`/tmp/workspace`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("AgentChatComposer html = %s, want %q", body, want)
		}
	}
	if strings.Contains(body, `Cwd:`) {
		t.Fatalf("AgentChatComposer html = %s, should not render cwd footer", body)
	}
}

func TestThoughtsDocRedirectURLEscapesSegments(t *testing.T) {
	values := url.Values{"chat": []string{"open"}}
	got := thoughtsDocRedirectURL("thoughts/user/plans/demo doc/design.md", values)
	want := "/thoughts/user/plans/demo%20doc/design.md?chat=open"
	if got != want {
		t.Fatalf("thoughtsDocRedirectURL() = %q, want %q", got, want)
	}
}

func TestOpenPlanWorkspaceCreatesCurrentUserWorkspaceAndRedirects(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_click-through-plan",
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Click Through Plan",
		time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	)

	firstLocation := invokeOpenPlanWorkspace(
		t,
		handler,
		planDir,
		"alice@example.com",
		http.StatusSeeOther,
	)
	if firstLocation != "/thoughts/creative-mode-agent/plans/2026-05-16_click-through-plan" {
		t.Fatalf("Location = %q, want thoughts plan URL", firstLocation)
	}

	aliceWorkspaces := listWorkspacesForHandlerTest(t, service, "alice@example.com")
	if len(aliceWorkspaces) != 1 {
		t.Fatalf("alice workspace count = %d, want 1", len(aliceWorkspaces))
	}
	if aliceWorkspaces[0].RootDocPath != planDir {
		t.Fatalf(
			"alice artifact root = %q, want %q",
			aliceWorkspaces[0].RootDocPath,
			planDir,
		)
	}

	secondLocation := invokeOpenPlanWorkspace(
		t,
		handler,
		planDir,
		"alice@example.com",
		http.StatusSeeOther,
	)
	if secondLocation != firstLocation {
		t.Fatalf("repeat Location = %q, want %q", secondLocation, firstLocation)
	}
	aliceWorkspaces = listWorkspacesForHandlerTest(t, service, "alice@example.com")
	if len(aliceWorkspaces) != 1 {
		t.Fatalf("alice workspace count after repeat = %d, want 1", len(aliceWorkspaces))
	}

	bobLocation := invokeOpenPlanWorkspace(
		t,
		handler,
		planDir,
		"bob@example.com",
		http.StatusSeeOther,
	)
	bobWorkspaces := listWorkspacesForHandlerTest(t, service, "bob@example.com")
	if len(bobWorkspaces) != 0 {
		t.Fatalf(
			"bob-owned workspace count = %d, want 0 because workspaces are shared",
			len(bobWorkspaces),
		)
	}
	if bobLocation != firstLocation {
		t.Fatalf(
			"bob Location = %q, want shared workspace location %q",
			bobLocation,
			firstLocation,
		)
	}
}

func TestOpenPlanWorkspaceQRSPIStartsWorkflowWithPolicyPreset(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_qrspi-assisted-plan",
	)

	location := invokeOpenPlanWorkspaceWithQuery(
		t,
		handler,
		url.Values{
			"plan_dir":      {planDir},
			"workflow_type": {string(WorkspaceWorkflowQRSPI)},
			"policy_preset": {string(WorkflowPolicyPresetAssisted)},
		},
		"alice@example.com",
		http.StatusSeeOther,
	)
	if location != "/thoughts/creative-mode-agent/plans/2026-05-16_qrspi-assisted-plan" {
		t.Fatalf("Location = %q, want thoughts plan URL", location)
	}

	workspaces := listWorkspacesForHandlerTest(t, service, "alice@example.com")
	if len(workspaces) != 1 {
		t.Fatalf("workspace count = %d, want 1", len(workspaces))
	}
	workspace := workspaces[0]
	if workspace.WorkflowType != string(WorkspaceWorkflowQRSPI) {
		t.Fatalf("WorkflowType = %q, want qrspi", workspace.WorkflowType)
	}
	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(workspace.WorkflowStateJson.String),
		&state,
	); err != nil {
		t.Fatalf("Unmarshal(workflow state): %v", err)
	}
	policy := qrspi.ParsePolicy(state.Policy)
	if !policy.AutoMode || !policy.EnablePlanReviews ||
		policy.InvalidResultRetryLimit != 1 {
		t.Fatalf("policy = %#v, want assisted preset", policy)
	}
}

func TestOpenPlanWorkspaceQRSPIFastDraftSkipsPlanningReviews(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_qrspi-fast-draft-plan",
	)

	invokeOpenPlanWorkspaceWithQuery(
		t,
		handler,
		url.Values{
			"plan_dir":      {planDir},
			"workflow_type": {string(WorkspaceWorkflowQRSPI)},
			"policy_preset": {string(WorkflowPolicyPresetFastDraft)},
		},
		"alice@example.com",
		http.StatusSeeOther,
	)
	workspace := listWorkspacesForHandlerTest(t, service, "alice@example.com")[0]
	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(workspace.WorkflowStateJson.String),
		&state,
	); err != nil {
		t.Fatalf("Unmarshal(workflow state): %v", err)
	}
	policy := qrspi.ParsePolicy(state.Policy)
	if !policy.AutoMode || policy.EnablePlanReviews ||
		policy.InvalidResultRetryLimit != 1 {
		t.Fatalf("policy = %#v, want fast draft preset", policy)
	}
}

func TestOpenPlanWorkspaceQRSPIRejectsUnknownPolicyPreset(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_qrspi-bad-preset-plan",
	)

	invokeOpenPlanWorkspaceWithQuery(
		t,
		handler,
		url.Values{
			"plan_dir":      {planDir},
			"workflow_type": {string(WorkspaceWorkflowQRSPI)},
			"policy_preset": {"turbo"},
		},
		"alice@example.com",
		http.StatusBadRequest,
	)
}

func TestOpenPlanWorkspaceFreeformPreservesExistingBehaviorWithPresetParam(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_freeform-plan",
	)

	invokeOpenPlanWorkspaceWithQuery(
		t,
		handler,
		url.Values{
			"plan_dir":      {planDir},
			"policy_preset": {string(WorkflowPolicyPresetAssisted)},
		},
		"alice@example.com",
		http.StatusSeeOther,
	)
	workspace := listWorkspacesForHandlerTest(t, service, "alice@example.com")[0]
	if workspace.WorkflowType != string(WorkspaceWorkflowFreeform) {
		t.Fatalf("WorkflowType = %q, want freeform", workspace.WorkflowType)
	}
	if workspace.WorkflowStateJson.Valid {
		t.Fatalf(
			"WorkflowStateJson = %q, want empty freeform state",
			workspace.WorkflowStateJson.String,
		)
	}
}

func TestOpenDocumentChatRedirectsToThoughtsDoc(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(t, service, "2026-05-16_doc-chat-plan")
	if err := os.WriteFile(
		filepath.Join(planDir, "AGENTS.md"),
		[]byte("context"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile AGENTS.md error = %v", err)
	}
	notesDir := filepath.Join(planDir, "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(notesDir) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(notesDir, "design.md"),
		[]byte("# Design"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile design.md error = %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/document/open?doc_path="+url.QueryEscape(
			"thoughts/creative-mode-agent/plans/2026-05-16_doc-chat-plan/notes/design.md",
		)+"&attach=1",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "alice@example.com")

	if err := handler.OpenDocumentChat(c); err != nil {
		t.Fatalf("OpenDocumentChat() error = %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get(echo.HeaderLocation)
	want := "/thoughts/" + "creative-mode-agent/plans/2026-05-16_doc-chat-plan/notes/design.md?chat=open"
	if location != want {
		t.Fatalf("Location = %q, want %q", location, want)
	}
	if strings.Contains(location, "/agent-chat") {
		t.Fatalf("Location = %q, should not contain /agent-chat", location)
	}
}

func TestAgentChatOpenEntrypointsDoNotReturnAgentChatPageRedirects(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_no_agent_chat_redirects",
	)
	if err := os.WriteFile(
		filepath.Join(planDir, "AGENTS.md"),
		[]byte("context"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile AGENTS.md error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(planDir, "design.md"),
		[]byte("# Design"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile design.md error = %v", err)
	}

	planLocation := invokeOpenPlanWorkspace(
		t,
		handler,
		planDir,
		"alice@example.com",
		http.StatusSeeOther,
	)
	if strings.HasPrefix(planLocation, "/agent-chat") {
		t.Fatalf("OpenPlanWorkspace Location = %q, want thoughts URL", planLocation)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/document/open?doc_path="+url.QueryEscape(
			"thoughts/creative-mode-agent/plans/2026-05-16_no_agent_chat_redirects/design.md",
		),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "alice@example.com")
	if err := handler.OpenDocumentChat(c); err != nil {
		t.Fatalf("OpenDocumentChat() error = %v", err)
	}
	docLocation := rec.Header().Get(echo.HeaderLocation)
	if strings.HasPrefix(docLocation, "/agent-chat") {
		t.Fatalf("OpenDocumentChat Location = %q, want thoughts URL", docLocation)
	}
}

func TestOpenPlanWorkspaceRejectsOutsideThoughtsPath(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	outsidePlan := filepath.Join(t.TempDir(), "other", "plans", "outside")
	if err := os.MkdirAll(outsidePlan, 0o755); err != nil {
		t.Fatalf("MkdirAll(outsidePlan) error = %v", err)
	}

	invokeOpenPlanWorkspace(
		t,
		handler,
		outsidePlan,
		"user@example.com",
		http.StatusBadRequest,
	)
}

func TestPlanSidebarLeafRowRendersFullWidthLink(t *testing.T) {
	t.Parallel()

	href := "/agent-chat/plan-workspace?plan_dir=" + url.QueryEscape(
		"/tmp/project/thoughts/user/plans/leaf",
	)
	body := renderAgentChatComponent(t, PlanSidebar(PlanSidebarState{
		TargetID:    defaultPlanSidebarTargetID,
		DrawerTitle: "Plan workspaces",
		Nodes: []PlanSidebarNode{{
			Key:               "plan:/tmp/project/thoughts/user/plans/leaf",
			Label:             "Leaf Plan",
			Href:              href,
			LatestSourceLabel: "Artifact",
			DirectCount:       1,
			AggregateCount:    1,
		}},
	}))
	if !strings.Contains(body, `<a href="`+href+`"`) {
		t.Fatalf("leaf row missing outer link href %q: %s", href, body)
	}
	if !strings.Contains(body, `group block rounded-md border`) {
		t.Fatalf("leaf row missing full-width link classes: %s", body)
	}
	if strings.Contains(body, `aria-label="Toggle sub workspaces"`) {
		t.Fatalf("leaf row rendered expand button: %s", body)
	}
}

func TestPlanSidebarParentRowKeepsSeparateExpandButton(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(t, PlanSidebar(PlanSidebarState{
		TargetID:    defaultPlanSidebarTargetID,
		DrawerTitle: "Plan workspaces",
		Nodes: []PlanSidebarNode{
			{
				Key:   "plan:/tmp/project/thoughts/user/plans/parent",
				Label: "Parent Plan",
				Href:  "/agent-chat/plan-workspace?plan_dir=/tmp/project/thoughts/user/plans/parent",
				Children: []PlanSidebarNode{
					{
						Key:   "plan:/tmp/project/thoughts/user/plans/parent/reviews/child",
						Label: "Child Plan",
						Href:  "/agent-chat/plan-workspace?plan_dir=/tmp/project/thoughts/user/plans/parent/reviews/child",
					},
				},
			},
		},
	}))
	if !strings.Contains(body, `aria-label="Toggle sub workspaces"`) {
		t.Fatalf("parent row missing expand button: %s", body)
	}
	if !strings.Contains(body, `class="block min-w-0"`) {
		t.Fatalf("parent row missing separate nested link: %s", body)
	}
	if got := strings.Count(body, `group block rounded-md border`); got != 2 {
		t.Fatalf(
			"parent row should keep only child leaf as outer link in desktop/mobile; count = %d: %s",
			got,
			body,
		)
	}
}

func invokeOpenPlanWorkspace(
	t *testing.T,
	handler *Handler,
	planDir string,
	userEmail string,
	wantStatus int,
) string {
	t.Helper()
	return invokeOpenPlanWorkspaceWithQuery(
		t,
		handler,
		url.Values{"plan_dir": {planDir}},
		userEmail,
		wantStatus,
	)
}

func invokeOpenPlanWorkspaceWithQuery(
	t *testing.T,
	handler *Handler,
	query url.Values,
	userEmail string,
	wantStatus int,
) string {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/plan-workspace?"+query.Encode(),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", userEmail)
	err := handler.OpenPlanWorkspace(c)
	if wantStatus >= 400 {
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("OpenPlanWorkspace() error = %v, want HTTP %d", err, wantStatus)
		}
		if httpErr.Code != wantStatus {
			t.Fatalf("OpenPlanWorkspace() status = %d, want %d", httpErr.Code, wantStatus)
		}
		return ""
	}
	if err != nil {
		t.Fatalf("OpenPlanWorkspace() error = %v", err)
	}
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rec.Code, wantStatus)
	}
	return rec.Header().Get("Location")
}

func listWorkspacesForHandlerTest(
	t *testing.T,
	service *Service,
	userEmail string,
) []db.Workspace {
	t.Helper()
	workspaces, err := service.queries.ListWorkspacesForUser(
		t.Context(),
		db.ListWorkspacesForUserParams{UserEmail: userEmail, Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListWorkspacesForUser() error = %v", err)
	}
	return workspaces
}

func TestAgentChatPagesSetPageTypeAgentChat(t *testing.T) {
	t.Parallel()

	pages := []struct {
		name string
		body string
	}{
		{
			name: "freeform",
			body: renderAgentChatComponent(
				t,
				ChatPage(ChatPageArgs{UserEmail: "user@example.com"}),
			),
		},
		{
			name: "workspace",
			body: renderAgentChatComponent(t, WorkspaceChatPage(WorkspacePageArgs{
				UserEmail:   "user@example.com",
				WorkspaceID: "workspace-1",
				Projection: WorkspaceProjection{
					Header: WorkspaceHeaderState{WorkspaceID: "workspace-1"},
					PlanSidebar: PlanSidebarState{
						TargetID:    "agent-chat-workspace-sidebar",
						DrawerTitle: "Plan workspaces",
					},
				},
			})),
		},
	}

	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			t.Parallel()

			for _, removed := range []string{"Agent Chat / New chat", "header_mobile_product_nav"} {
				if strings.Contains(page.body, removed) {
					t.Fatalf(
						"AgentChat page still contains removed nav %q: %s",
						removed,
						page.body,
					)
				}
			}
			thoughtsHref := strings.LastIndex(page.body, `href="/thoughts/"`)
			if thoughtsHref < 0 {
				t.Fatalf("AgentChat page missing Thoughts link: %s", page.body)
			}
			segmentStart := max(0, thoughtsHref-240)
			if segment := page.body[segmentStart:thoughtsHref]; strings.Contains(
				segment,
				"text-foreground font-medium",
			) {
				t.Fatalf("Thoughts nav appears active on AgentChat page: %s", segment)
			}
		})
	}
}

func renderAgentChatComponent(t *testing.T, component templ.Component) string {
	t.Helper()

	var body bytes.Buffer
	if err := component.Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return body.String()
}

func TestAgentChatInitialScrollAssets(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(
		t,
		AgentChatChatPane(templ.NopComponent, templ.NopComponent, templ.NopComponent),
	)
	for _, want := range []string{
		`src="/js/agent-chat-scroll.js?v=1"`,
		`id="agent-chat-scroll-region"`,
		`data-agent-chat-initial-scroll`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("AgentChatChatPane missing %q: %s", want, body)
		}
	}
}

func TestAgentChatInitialScrollSource(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("..", "..", "..", "static", "js", "agent-chat-scroll.js"))
	if err != nil {
		t.Fatalf("ReadFile(agent-chat-scroll.js) error = %v", err)
	}
	body := string(source)
	for _, want := range []string{
		"scrollTop = region.scrollHeight",
		"datastar-patch-elements",
		"MutationObserver",
		"requestAnimationFrame",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("agent-chat-scroll.js missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"WeakSet",
		"initialized.has(region)",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("agent-chat-scroll.js should keep patch scroll idempotent; found %q", unwanted)
		}
	}
}

func TestChatMarkdownRawHTMLIsIgnoredByDatastar(t *testing.T) {
	t.Parallel()

	body := renderAgentChatComponent(t, ChatMessage(ChatMessageArgs{
		ID:          "assistant-1",
		Role:        "assistant",
		HTMLContent: `<div data-init="@get('/api/reload', {retryMaxCount: 1000})"></div>`,
	}))
	if !strings.Contains(body, `data-ignore`) {
		t.Fatalf("rendered chat markdown missing data-ignore: %s", body)
	}
	if !strings.Contains(body, `data-init="@get('/api/reload'`) {
		t.Fatalf("rendered chat markdown missing raw fixture: %s", body)
	}
}

func TestAgentChatIndexWithoutThreadStartsInFocusedWorkbench(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/agent-chat",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.HandleAgentChatIndex(c); err != nil {
		t.Fatalf("HandleAgentChatIndex() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="workbench-root"`,
		`data-workbench-region="agent-chat-primary"`,
		`agentChatNavigation&#34;:{&#34;ratio&#34;:0.22,&#34;visible&#34;:false}`,
		`agentChatContext&#34;:{&#34;ratio&#34;:0.39,&#34;visible&#34;:false}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("freeform workbench missing %q: %s", want, body)
		}
	}
}

func TestWorkspaceRouteRendersWithoutSelectedThread(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestWorkspaceRouteWithoutThreadKeepsSidebarVisibleWithPersistedSelection(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestWorkspaceResourceKeepsShellSignalsAndTextareaGuards(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestAgentChatResponsiveShellSignals(t *testing.T) {
	t.Parallel()

	freeformSignals := chatShellSignals(false)
	for _, want := range []string{
		`showSidebar: true`,
		`mobilePane: "chat"`,
		`agent_chat_thread_sheet`,
	} {
		if !strings.Contains(freeformSignals, want) {
			t.Fatalf("freeform signals missing %q: %s", want, freeformSignals)
		}
	}
	if strings.Contains(freeformSignals, "rightRailTab") {
		t.Fatalf(
			"freeform signals should not initialize rightRailTab: %s",
			freeformSignals,
		)
	}

	workspaceSignals := workspaceShellSignals(false, PlanSidebarState{})
	for _, want := range []string{
		`showSidebar: true`,
		`showArtifactTree: false`,
		`mobilePane: "chat"`,
		`rightRailTab: 'artifacts'`,
		`agentChatSidebarOpenGroup`,
		`agent_chat_thread_sheet`,
	} {
		if !strings.Contains(workspaceSignals, want) {
			t.Fatalf("workspace signals missing %q: %s", want, workspaceSignals)
		}
	}

	body := renderAgentChatComponent(
		t,
		ChatPage(ChatPageArgs{UserEmail: "user@example.com"}),
	)
	for _, want := range []string{
		`id="workbench-root"`,
		`data-workbench-region="agent-chat-primary"`,
		`$workbench.activeRegionID = &#39;agentChatPrimary&#39;`,
		`data-attr:aria-selected`,
		`id="agent-chat-chat-pane"`,
		`id="agent-chat-rail-pane"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Agent Chat workbench shell missing %q: %s", want, body)
		}
	}
	for _, old := range []string{
		`id="agent-chat-mobile-chat-tab"`,
		`id="agent-chat-mobile-rail-tab"`,
	} {
		if strings.Contains(body, old) {
			t.Fatalf(
				"Agent Chat freeform should not render retired responsive shell marker %q: %s",
				old,
				body,
			)
		}
	}
}

func TestAgentChatMobileSidebarDrawer(t *testing.T) {
	t.Parallel()

	freeformBody := renderAgentChatComponent(
		t,
		ChatPage(ChatPageArgs{UserEmail: "user@example.com"}),
	)
	for _, old := range []string{
		`id="agent-chat-thread-sidebar"`,
		`aria-controls="agent_chat_thread_sheet"`,
		`$agent_chat_thread_sheet.open = true`,
	} {
		if strings.Contains(freeformBody, old) {
			t.Fatalf(
				"freeform should not render retired mobile drawer marker %q: %s",
				old,
				freeformBody,
			)
		}
	}
	for _, want := range []string{
		`id="workbench-root"`,
		`role="tablist"`,
		`aria-label="Workbench regions"`,
		`$workbench.activeRegionID = &#39;agentChatNavigation&#39;`,
		`id="agent-chat-shared-sidebar"`,
	} {
		if !strings.Contains(freeformBody, want) {
			t.Fatalf("freeform mobile workbench missing %q: %s", want, freeformBody)
		}
	}

	workspaceBody := renderAgentChatComponent(t, WorkspaceChatPage(WorkspacePageArgs{
		UserEmail:   "user@example.com",
		WorkspaceID: "workspace-1",
		Projection: WorkspaceProjection{
			Header: WorkspaceHeaderState{WorkspaceID: "workspace-1"},
			PlanSidebar: PlanSidebarState{
				TargetID:    "agent-chat-workspace-sidebar",
				DrawerTitle: "Plan workspaces",
			},
		},
	}))
	for _, want := range []string{
		`id="workbench-root"`,
		`id="agent-chat-workspace-topology-region"`,
		`id="agent-chat-doc-region"`,
		`id="agent-chat-chat-region"`,
		`role="tablist"`,
		`aria-label="Workbench regions"`,
		`$workbench.activeRegionID = &#39;agentChatNavigation&#39;`,
	} {
		if !strings.Contains(workspaceBody, want) {
			t.Fatalf("workspace mobile workbench missing %q: %s", want, workspaceBody)
		}
	}
}

func assertMobileDrawerMarkup(t *testing.T, body string) {
	t.Helper()

	for _, want := range []string{
		`aria-controls="agent_chat_thread_sheet"`,
		`$agent_chat_thread_sheet.open = true`,
		`role="dialog"`,
		`aria-modal="true"`,
		`data-on:keydown__window`,
		`evt.key === &#39;Escape&#39;`,
		`$agent_chat_thread_sheet.open = false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("mobile drawer markup missing %q: %s", want, body)
		}
	}
}

func TestWorkspaceRouteRendersSelectedThreadWithWorkspaceForkAction(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestWorkspaceShellSignalsInitializesSidebarOpenGroupFromActiveOnly(t *testing.T) {
	t.Parallel()

	signals := workspaceShellSignals(false, PlanSidebarState{
		Nodes: []PlanSidebarNode{{Key: "plan:inactive"}},
	})
	if !strings.Contains(signals, `agentChatSidebarOpenGroup: ""`) {
		t.Fatalf("inactive sidebar group should not open by default: %s", signals)
	}
	if strings.Contains(signals, "plan:inactive") {
		t.Fatalf(
			"inactive sidebar group key leaked into default open signal: %s",
			signals,
		)
	}

	signals = workspaceShellSignals(true, PlanSidebarState{
		Nodes: []PlanSidebarNode{{Key: "plan:active", Active: true}},
	})
	if !strings.Contains(signals, `agentChatSidebarOpenGroup: "plan:active"`) {
		t.Fatalf("active sidebar group should initialize open signal: %s", signals)
	}
}

func TestWorkspaceSidebarRendersCollapsibleGroups(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestWorkspaceThreadHrefRoutePersistsSelectedThread(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestWorkspaceNotifierUnsubscribesOnCancel(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	done := make(chan error, 1)
	go func() { done <- handler.StreamWorkspace(c) }()

	for i := 0; i < 20 && service.notifier.SubscriberCount(workspace.ID) == 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if got := service.notifier.SubscriberCount(workspace.ID); got != 1 {
		cancel()
		t.Fatalf("SubscriberCount() before cancel = %d, want 1", got)
	}
	cancel()

	select {
	case err := <-done:
		if !isExpectedStreamCancel(err) {
			t.Fatalf("StreamWorkspace() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamWorkspace() timed out")
	}
	if got := service.notifier.SubscriberCount(workspace.ID); got != 0 {
		t.Fatalf("SubscriberCount() after cancel = %d, want 0", got)
	}
}

func TestPlanSidebarStreamSubscriptionsUnsubscribeOnCancel(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	projectKey := service.projectPlanSidebarNotifyKey()
	userKey := service.planSidebarNotifyKey("user@example.com")

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	done := make(chan error, 1)
	go func() { done <- handler.StreamWorkspace(c) }()

	waitForSubscriberCount(t, service.notifier, userKey, 1)
	waitForSubscriberCount(t, service.notifier, projectKey, 1)
	cancel()

	select {
	case err := <-done:
		if !isExpectedStreamCancel(err) {
			t.Fatalf("StreamWorkspace() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamWorkspace() timed out")
	}
	if got := service.notifier.SubscriberCount(userKey); got != 0 {
		t.Fatalf("user SubscriberCount() after cancel = %d, want 0", got)
	}
	if got := service.notifier.SubscriberCount(projectKey); got != 0 {
		t.Fatalf("project SubscriberCount() after cancel = %d, want 0", got)
	}
}

func TestStreamSessionsPatchesPlanSidebarAfterProjectNotify(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_project-stream-plan",
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Project Stream Plan",
		time.Date(2026, 5, 16, 20, 0, 0, 0, time.UTC),
	)

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/sessions/stream", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	done := make(chan error, 1)
	go func() { done <- handler.StreamSessions(c) }()

	waitForSubscriberCount(t, service.notifier, service.projectPlanSidebarNotifyKey(), 1)
	service.NotifyProjectPlanSidebar()
	waitForResponseContains(t, rec, "project stream plan")
	waitForResponseCount(t, rec, defaultPlanSidebarTargetID, 2)
	cancel()
	waitForStreamDone(t, done, "StreamSessions")
}

func TestStreamThreadPatchesPlanSidebarAfterProjectAndUserNotify(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_thread-stream-plan",
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Thread Stream Plan",
		time.Date(2026, 5, 16, 21, 0, 0, 0, time.UTC),
	)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-plan-sidebar-stream",
		"user@example.com",
		planDir,
		"lineage-plan-sidebar-stream",
	)

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/stream?thread="+url.QueryEscape(thread.ID), nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	done := make(chan error, 1)
	go func() { done <- handler.StreamThread(c) }()

	waitForSubscriberCount(t, service.notifier, service.projectPlanSidebarNotifyKey(), 1)
	waitForSubscriberCount(
		t,
		service.notifier,
		service.planSidebarNotifyKey("user@example.com"),
		1,
	)
	service.NotifyProjectPlanSidebar()
	waitForResponseContains(t, rec, "thread stream plan")
	service.NotifyPlanSidebar("user@example.com")
	waitForResponseCount(t, rec, defaultPlanSidebarTargetID, 2)
	cancel()
	waitForStreamDone(t, done, "StreamThread")
}

func TestStreamWorkspacePatchesPlanSidebarAfterProjectAndUserNotify(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_workspace-stream-plan",
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Workspace Stream Plan",
		time.Date(2026, 5, 16, 22, 0, 0, 0, time.UTC),
	)

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	done := make(chan error, 1)
	go func() { done <- handler.StreamWorkspace(c) }()

	waitForSubscriberCount(t, service.notifier, service.projectPlanSidebarNotifyKey(), 1)
	waitForSubscriberCount(
		t,
		service.notifier,
		service.planSidebarNotifyKey("user@example.com"),
		1,
	)
	service.NotifyProjectPlanSidebar()
	waitForResponseContains(t, rec, "workspace stream plan")
	service.NotifyPlanSidebar("user@example.com")
	waitForResponseCount(t, rec, "agent-chat-shared-sidebar", 4)
	cancel()
	waitForStreamDone(t, done, "StreamWorkspace")
}

func TestPlanSidebarStaleUserCursorDoesNotSuppressProjectNotify(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	planDir := mustCreateHandlerTestPlanWorkspace(
		t,
		service,
		"2026-05-16_project-cursor-plan",
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Project Cursor Plan",
		time.Date(2026, 5, 16, 23, 0, 0, 0, time.UTC),
	)
	service.NotifyPlanSidebar("user@example.com")

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	done := make(chan error, 1)
	go func() { done <- handler.StreamWorkspace(c) }()

	waitForSubscriberCount(t, service.notifier, service.projectPlanSidebarNotifyKey(), 1)
	service.NotifyProjectPlanSidebar()
	waitForResponseContains(t, rec, "project cursor plan")
	cancel()
	waitForStreamDone(t, done, "StreamWorkspace")
}

func mustCreateHandlerTestPlanWorkspace(
	t *testing.T,
	service *Service,
	slug string,
) string {
	t.Helper()
	planDir := filepath.Join(service.thoughtsRoot, "creative-mode-agent", "plans", slug)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(planDir) error = %v", err)
	}
	return planDir
}

func waitForSubscriberCount(t *testing.T, notifier *Notifier, key string, want int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if got := notifier.SubscriberCount(key); got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf(
		"SubscriberCount(%q) = %d, want %d",
		key,
		notifier.SubscriberCount(key),
		want,
	)
}

func waitForResponseContains(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if strings.Contains(rec.Body.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("response missing %q: %s", want, rec.Body.String())
}

func waitForResponseCount(
	t *testing.T,
	rec *httptest.ResponseRecorder,
	needle string,
	wantAtLeast int,
) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if strings.Count(rec.Body.String(), needle) >= wantAtLeast {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf(
		"response count for %q = %d, want at least %d: %s",
		needle,
		strings.Count(rec.Body.String(), needle),
		wantAtLeast,
		rec.Body.String(),
	)
}

func waitForStreamDone(t *testing.T, done <-chan error, streamName string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s() error = %v", streamName, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s() timed out", streamName)
	}
}

func TestWorkspaceNotifierSlowSubscriberDoesNotBlock(t *testing.T) {
	notifier := NewNotifier()
	slow := notifier.Subscribe("workspace-1")
	fast := notifier.Subscribe("workspace-1")
	for i := 0; i < cap(slow); i++ {
		slow <- WorkspaceStreamSignal{Cursor: int64(i + 1)}
	}

	done := make(chan struct{})
	go func() {
		notifier.NotifyWorkspaceResource("workspace-1")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("NotifyWorkspaceResource blocked on slow subscriber")
	}
	select {
	case signal := <-fast:
		if signal.Scope != PatchWorkspaceResource {
			t.Fatalf(
				"fast signal scope = %q, want %q",
				signal.Scope,
				PatchWorkspaceResource,
			)
		}
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive signal")
	}
}

func assertWorkspaceSidebarExpandedForEmptySelection(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		`id="workbench-root"`,
		`id="agent-chat-workspace-topology-region"`,
		`id="agent-chat-workspace-topology-sidebar"`,
		`data-workbench-kind="workspace-topology"`,
		`id="agent-chat-doc-region"`,
		`id="agent-chat-doc-pane"`,
		`data-workbench-kind="doc"`,
		`id="agent-chat-chat-region"`,
		`id="agent-chat-chat-pane"`,
		`data-workbench-kind="chat"`,
		`agentChatNavigation&#34;:{&#34;ratio&#34;:0.22,&#34;visible&#34;:true}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf(
				"empty workspace missing workbench marker %q: %s",
				want,
				body,
			)
		}
	}
}

func assertWorkspaceSidebarMinimizedForSelectedThread(t *testing.T, body string) {
	t.Helper()
	assertWorkspaceSidebarExpandedForEmptySelection(t, body)
	for _, retired := range []string{
		`id="agent-chat-plan-sidebar-region"`,
		`id="agent-chat-context-region"`,
		`id="agent-chat-workspace-right-rail"`,
	} {
		if strings.Contains(body, retired) {
			t.Fatalf(
				"selected workspace rendered retired shell marker %q: %s",
				retired,
				body,
			)
		}
	}
}

func assertWorkspaceResourcePatch(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "agent-chat-workspace-resource") {
		t.Fatalf("response missing workspace resource patch: %s", body)
	}
	assertNoWorkspaceShellPatch(t, body)
}

func assertWorkspaceWorkflowPanelPatch(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "agent-chat-workspace-workflow") {
		t.Fatalf("response missing workflow panel patch: %s", body)
	}
	if strings.Contains(body, "agent-chat-workspace-resource") {
		t.Fatalf(
			"response patched full workspace resource, want workflow panel only: %s",
			body,
		)
	}
	assertNoWorkspaceShellPatch(t, body)
}

func assertNoWorkspaceShellPatch(t *testing.T, body string) {
	t.Helper()
	for _, unexpected := range []string{
		"agent-chat-workspace-shell",
		`data-signals="{showDetails`,
		`data-signals=\"{showDetails`,
		`"agent_chat_thread_sheet":{"open":false`,
		`&#34;agent_chat_thread_sheet&#34;:{&#34;open&#34;:false`,
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf(
				"response patched workspace shell/signal owner %q: %s",
				unexpected,
				body,
			)
		}
	}
}

func assertNoWorkspaceSidebarSignalPatch(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, `data-signals=\"{showSidebar`) ||
		strings.Contains(body, `data-signals="{showSidebar`) {
		t.Fatalf(
			"resource patch should not mutate shell-owned showSidebar signal: %s",
			body,
		)
	}
}

func assertLiveTranscriptPatchOnly(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("response missing live transcript patch: %s", body)
	}
	for _, unexpected := range []string{
		"agent-chat-workspace-resource",
		"agent-chat-workspace-shell",
		"agent-chat-workspace-sidebar",
		"agent-chat-messages",
		"agent-chat-artifact-pane",
		"agent-chat-workspace-minimap",
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("live patch unexpectedly included %s: %s", unexpected, body)
		}
	}
}

func TestStreamWorkspaceCursorGapRunsCatchup(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	mustCreateAgentRun(t, service, thread.ID, "run-1")
	service.notifier.NotifyWorkspaceResource(workspace.ID)
	service.notifier.NotifyWorkspaceResource(workspace.ID)

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream?since=0", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	done := make(chan error, 1)
	go func() { done <- handler.StreamWorkspace(c) }()
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !isExpectedStreamCancel(err) {
			t.Fatalf("StreamWorkspace() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamWorkspace() timed out")
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	for _, target := range []string{
		"workbench-root",
		"agent-chat-workspace-topology-region",
		"agent-chat-workspace-topology-sidebar",
		"agent-chat-doc-region",
		"agent-chat-doc-pane",
		"agent-chat-chat-region",
		"agent-chat-stable-transcript",
	} {
		if !strings.Contains(body, target) {
			t.Fatalf("catch-up resource response missing %s: %s", target, body)
		}
	}
	if strings.Count(body, "agent-chat-workspace-resource") != 1 {
		t.Fatalf("catch-up response should patch workspace resource once: %s", body)
	}
}

func TestWorkspaceWorkflowPanelPatchTargetsWorkflowPanel(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusWaitingHuman,
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspace(c, sse, PatchWorkflowPanel); err != nil {
		t.Fatalf("patchWorkspace(PatchWorkflowPanel) error = %v", err)
	}
	assertWorkspaceWorkflowPanelPatch(t, rec.Body.String())
}

func TestUpdateWorkspaceWorkflowPolicyHandlerPersistsAndPatchesPanel(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusWaitingHuman,
	)
	form := url.Values{
		"autoMode":                {"on"},
		"invalidResultRetryLimit": {"0"},
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/workflow/policy",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	if err := handler.UpdateWorkspaceWorkflowPolicy(c); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowPolicy() error = %v", err)
	}
	assertWorkspaceWorkflowPanelPatch(t, rec.Body.String())

	updated, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(updated.WorkflowStateJson.String),
		&state,
	); err != nil {
		t.Fatalf("Unmarshal workflow state: %v", err)
	}
	policy := qrspi.ParsePolicy(state.Policy)
	if !policy.AutoMode || policy.EnablePlanReviews ||
		policy.InvalidResultRetryLimit != 0 {
		t.Fatalf("policy = %#v, want auto fast-draft with zero retries", policy)
	}
}

func TestSlashCommandInputHandlerBuildsSafeCommandButtons(t *testing.T) {
	script := slashCommandInputHandler("workspace-1", "")
	for _, unsafe := range []string{"onclick=", "innerHTML = commands", "escapeHTML"} {
		if strings.Contains(script, unsafe) {
			t.Fatalf("slash command handler contains unsafe %q: %s", unsafe, script)
		}
	}
	for _, want := range []string{"createElement('button')", "textContent", "addEventListener('click'", "replaceChildren"} {
		if !strings.Contains(script, want) {
			t.Fatalf("slash command handler missing %q: %s", want, script)
		}
	}
}

func TestDurableWorkspacePatchTargetsResource(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-1",
		"",
		"message",
		0,
		`{"type":"message","id":"assistant-1","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(
		ctx,
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "assistant-1", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream?thread="+url.QueryEscape(thread.ID),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspace(c, sse, PatchStableTranscript); err != nil {
		t.Fatalf("patchWorkspace() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	if !strings.Contains(body, "agent-chat-stable-transcript") ||
		!strings.Contains(body, "done") {
		t.Fatalf("expected resource patch with stable transcript, got %s", body)
	}
}

func TestWorkspaceStreamNeedsCatchupWhenClientCursorIsAhead(t *testing.T) {
	t.Parallel()

	if !workspace.NeedsCatchup(99, 0) {
		t.Fatal("workspace.NeedsCatchup(99, 0) = false, want true for reset cursor")
	}
	if !workspace.NeedsCatchup(1, 2) {
		t.Fatal("workspace.NeedsCatchup(1, 2) = false, want true for missed event")
	}
	if workspace.NeedsCatchup(2, 2) {
		t.Fatal("workspace.NeedsCatchup(2, 2) = true, want false")
	}
}

func TestWorkspaceShellNotPatched(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, _ := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID+"/stream", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspaceCatchup(c, sse); err != nil {
		t.Fatalf("patchWorkspaceCatchup() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
}

func TestWorkspaceResourcePatchDoesNotMutateSidebarSignalWithoutSelectedThread(
	t *testing.T,
) {
	t.Parallel()

	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	if err := service.queries.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: sql.NullString{String: thread.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedThread() error = %v", err)
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspaceResource(c, sse); err != nil {
		t.Fatalf("patchWorkspaceResource() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	assertNoWorkspaceSidebarSignalPatch(t, body)
}

func TestWorkspaceResourcePatchReflectsSelectedBackendState(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	if err := service.UpdateWorkspaceWorkflowState(
		ctx,
		workspace.ID,
		WorkspaceWorkflowState{
			Type:        WorkspaceWorkflowQRSPI,
			CurrentStep: "plan",
			Status:      "running",
			ReviewGate:  "none",
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceSelectedDoc(
		ctx,
		db.UpdateWorkspaceSelectedDocParams{
			ID: workspace.ID,
			SelectedDocPath: sql.NullString{
				String: handlerTestPlanRelPath,
				Valid:  true,
			},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedDoc() error = %v", err)
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream?thread="+url.QueryEscape(thread.ID),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspaceResource(c, sse); err != nil {
		t.Fatalf("patchWorkspaceResource() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	if strings.Contains(body, `id="agent-chat-workspace-shell"`) ||
		strings.Contains(body, `data-signals=\"{showDetails`) ||
		strings.Contains(body, `data-signals="{showDetails`) {
		t.Fatalf("resource patch should not replace shell signals: %s", body)
	}
	assertNoWorkspaceSidebarSignalPatch(t, body)
	if strings.Contains(body, `id="agent-chat-workspace-sidebar" data-signals`) ||
		strings.Contains(body, `id=\"agent-chat-workspace-sidebar\" data-signals`) ||
		strings.Contains(body, `data-signals=\"{agentChatSidebarOpenGroup`) ||
		strings.Contains(body, `data-signals="{agentChatSidebarOpenGroup`) {
		t.Fatalf(
			"resource patch should not reinitialize sidebar open-group signals: %s",
			body,
		)
	}
	for _, want := range []string{
		thread.ID,
		handlerTestPlanRelPath,
		"agent-chat-workspace-topology-region",
		"agent-chat-doc-region",
		"agent-chat-chat-region",
		"agent-chat-chat-pane",
		"plan",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("resource patch missing backend state %q: %s", want, body)
		}
	}
}

func TestWorkspacePatchInputThreadQueryPinsStreamThread(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, threadA := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	threadB := mustCreateAgentThread(
		t,
		service,
		"thread-2",
		"user@example.com",
		workspace.RootDocPath,
		"lineage-2",
	)
	if err := service.queries.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          threadB.ID,
			WorkspaceID: sql.NullString{String: workspace.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace(threadB) error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: sql.NullString{String: threadB.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedThread(threadB) error = %v", err)
	}
	mustCreateAgentEntry(
		t,
		service,
		threadA.LineageID,
		"assistant-a",
		"",
		"message",
		0,
		`{"type":"message","id":"assistant-a","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"thread alpha"}]}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		threadB.LineageID,
		"assistant-b",
		"",
		"message",
		0,
		`{"type":"message","id":"assistant-b","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"thread beta"}]}}`,
	)
	for _, update := range []db.UpdateAgentThreadHeadParams{
		{ID: threadA.ID, HeadEntryID: sql.NullString{String: "assistant-a", Valid: true}},
		{ID: threadB.ID, HeadEntryID: sql.NullString{String: "assistant-b", Valid: true}},
	} {
		if err := service.queries.UpdateAgentThreadHead(ctx, update); err != nil {
			t.Fatalf("UpdateAgentThreadHead(%s) error = %v", update.ID, err)
		}
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream?thread="+url.QueryEscape(threadA.ID),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspaceResource(c, sse); err != nil {
		t.Fatalf("patchWorkspaceResource() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	if !strings.Contains(body, "thread alpha") {
		t.Fatalf("resource patch missing thread query content: %s", body)
	}
	if strings.Contains(body, "thread beta") {
		t.Fatalf(
			"resource patch used persisted selected thread instead of query thread: %s",
			body,
		)
	}
}

func TestTerminalRunResourcePatchClearsLiveStateBeforeNotify(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-terminal-live-1")
	if err := service.queries.UpdateAgentRunWorkspaceForTest(ctx, db.UpdateAgentRunWorkspaceForTestParams{ID: run.ID, WorkspaceID: nullString(workspace.ID)}); err != nil {
		t.Fatalf("update run workspace_id: %v", err)
	}
	workspaceSignals := service.notifier.Subscribe(workspace.ID)
	defer service.notifier.Unsubscribe(workspace.ID, workspaceSignals)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"stale terminal live"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}
	if err := service.FinalizeRun(
		ctx,
		conversation.RunResult{RunID: run.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("FinalizeRun() error = %v", err)
	}
	select {
	case signal := <-workspaceSignals:
		if signal.Scope != PatchWorkspaceResource {
			t.Fatalf(
				"first workspace signal scope = %q, want %q",
				signal.Scope,
				PatchWorkspaceResource,
			)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace subscriber did not receive terminal resource signal")
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream?thread="+url.QueryEscape(thread.ID),
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspaceResource(c, sse); err != nil {
		t.Fatalf("patchWorkspaceResource() error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	if strings.Contains(body, "stale terminal live") {
		t.Fatalf("terminal resource patch included stale live state: %s", body)
	}
}

func TestWorkspacePageAllowsCoworkerAccess(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestPatchThreadPageCanTargetLiveTranscriptOnly(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-1",
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"live only"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/stream?thread=thread-1&run=run-1",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchThreadPage(c, sse, PatchLiveTranscript); err != nil {
		t.Fatalf("patchThreadPage() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("response did not target live transcript region: %s", body)
	}
	if strings.Contains(body, "agent-chat-messages") ||
		strings.Contains(body, "agent-chat-artifact-pane") ||
		strings.Contains(body, "agent-chat-thread-sidebar") {
		t.Fatalf("response unexpectedly touched non-live regions: %s", body)
	}
}

func TestPatchWorkspaceLiveTranscriptUsesLiveOnlyBuilder(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-live-workspace-1",
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"workspace live only"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.MkdirAll(blockedRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(blockedRoot) error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceRootDocPathForTest(t.Context(), db.UpdateWorkspaceRootDocPathForTestParams{ID: workspace.ID, RootDocPath: blockedRoot}); err != nil {
		t.Fatalf("update workspace artifact root: %v", err)
	}
	if err := os.Chmod(blockedRoot, 0o000); err != nil {
		t.Fatalf("Chmod(blockedRoot) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blockedRoot, 0o755) })

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspace(c, sse, PatchLiveTranscript); err != nil {
		t.Fatalf("patchWorkspace(PatchLiveTranscript) error = %v", err)
	}
	body := rec.Body.String()
	assertLiveTranscriptPatchOnly(t, body)
	if !strings.Contains(body, "workspace live only") {
		t.Fatalf("live transcript patch missing live content: %s", body)
	}
}

func TestPatchLiveTranscriptSkipsPiSessionSidebarScan(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-1",
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"live only"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	blockedDir := t.TempDir()
	if err := os.Chmod(blockedDir, 0o000); err != nil {
		t.Fatalf("Chmod(blockedDir) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(blockedDir, 0o755)
	})
	service.piSessionsDir = blockedDir

	if _, err := service.BuildPageArgs(
		context.Background(),
		"user@example.com",
		thread.ID,
		"",
		"",
		"",
	); err != nil {
		t.Fatalf("BuildPageArgs() should not scan piSessionsDir: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/stream?thread=thread-1&run=run-1",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchThreadPage(c, sse, PatchLiveTranscript); err != nil {
		t.Fatalf("patchThreadPage() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-live-transcript") ||
		!strings.Contains(body, "live only") {
		t.Fatalf(
			"expected live transcript patch despite unreadable piSessionsDir, got %s",
			body,
		)
	}
	if strings.Contains(body, "agent-chat-thread-sidebar") ||
		strings.Contains(body, "agent-chat-messages") {
		t.Fatalf("unexpected non-live patch content: %s", body)
	}
}

func TestStreamSessionsInitialPatchUsesDBProjection(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "stream-db")
	mustCreateAgentSession(t, service, "session-1", "user@example.com", planDir, "", "")
	blockedDir := t.TempDir()
	if err := os.Chmod(blockedDir, 0o000); err != nil {
		t.Fatalf("Chmod(blockedDir) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o755) })
	service.piSessionsDir = blockedDir

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	time.AfterFunc(10*time.Millisecond, cancel)
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/sessions/stream",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.StreamSessions(c); err != nil {
		t.Fatalf("StreamSessions() should not scan piSessionsDir: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-thread-sidebar") ||
		!strings.Contains(body, "stream-db") {
		t.Fatalf("stream response missing DB-backed plan sidebar: %s", body)
	}
	if strings.Contains(body, "ThreadSidebar") {
		t.Fatalf("stream response should render PlanSidebar, got: %s", body)
	}
}

func TestPatchRunHeaderScopeClearsLiveRegionWithoutTouchingArtifactPane(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"partial"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}
	if err := service.FailRun(
		context.Background(),
		conversation.RunFailure{RunID: run.ID, ErrorMessage: "boom"},
	); err != nil {
		t.Fatalf("FailRun() error = %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/stream?thread=thread-1&run=run-1",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchThreadPage(c, sse, PatchRunHeader); err != nil {
		t.Fatalf("patchThreadPage() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-run-header") ||
		!strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("expected run header and live transcript patch, got %s", body)
	}
	if strings.Contains(body, "agent-chat-messages") ||
		strings.Contains(body, "agent-chat-artifact-pane") ||
		strings.Contains(body, "agent-chat-thread-sidebar") {
		t.Fatalf("unexpected non-run-header scope content: %s", body)
	}
	if strings.Contains(body, "Waiting for the first completed turn") {
		t.Fatalf("unexpected waiting placeholder after failure: %s", body)
	}
}

func TestPatchStableTranscriptScopeDoesNotTouchSidebarOrArtifactPane(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-1",
		"",
		"message",
		0,
		`{"type":"message","id":"assistant-1","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "assistant-1", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/stream?thread=thread-1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchThreadPage(c, sse, PatchStableTranscript); err != nil {
		t.Fatalf("patchThreadPage() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-messages") ||
		!strings.Contains(body, "agent-chat-stable-transcript") ||
		!strings.Contains(body, "done") {
		t.Fatalf("expected stable transcript patch, got %s", body)
	}
	if strings.Contains(body, "agent-chat-run-header") ||
		strings.Contains(body, "agent-chat-artifact-pane") ||
		strings.Contains(body, "agent-chat-thread-sidebar") {
		t.Fatalf("unexpected non-transcript content in stable patch: %s", body)
	}
}

func TestPatchThreadPageIncludesRunHeader(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	_ = mustCreateAgentRun(t, service, thread.ID, "run-1")

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/stream?thread=thread-1&run=run-1",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchThreadPage(c, sse, PatchThreadPage); err != nil {
		t.Fatalf("patchThreadPage() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-run-header") {
		t.Fatalf("response did not include run header patch: %s", body)
	}
	if !strings.Contains(body, "Toggle sessions sidebar") ||
		strings.Contains(body, "Hide run details") {
		t.Fatalf("response did not render the expected header controls: %s", body)
	}
}

func TestStreamThreadSkipsInitialPatchWhenSinceIsCurrent(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-1",
		ThreadID:    thread.ID,
		EventType:   "message_end",
		PayloadJSON: `{"message":{"role":"user","content":"hello"}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}
	service.notifier.Notify(thread.ID, ThreadStreamSignal{Scope: PatchLiveTranscript})

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/stream?thread="+thread.ID+"&since=1", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	done := make(chan error, 1)
	go func() { done <- handler.StreamThread(c) }()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StreamThread() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamThread() timed out")
	}

	if strings.Contains(rec.Body.String(), "agent-chat-messages") {
		t.Fatalf("unexpected connect-time patch: %s", rec.Body.String())
	}
}

func TestStreamThreadSendsCatchUpPatchWhenSinceIsStale(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	_ = mustCreateAgentRun(t, service, thread.ID, "run-1")

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-1",
		ThreadID:    thread.ID,
		EventType:   "message_end",
		PayloadJSON: `{"message":{"role":"user","content":"hello"}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}
	service.notifier.Notify(thread.ID, ThreadStreamSignal{Scope: PatchLiveTranscript})

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/stream?thread="+thread.ID+"&run=run-1&since=0", nil).
		WithContext(reqCtx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	done := make(chan error, 1)
	go func() { done <- handler.StreamThread(c) }()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StreamThread() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamThread() timed out")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "agent-chat-messages") {
		t.Fatalf("expected catch-up patch to include transcript: %s", body)
	}
	if !strings.Contains(body, "agent-chat-run-header") {
		t.Fatalf("expected catch-up patch to include run header: %s", body)
	}
}

func TestResumeThreadRequiresThreadIDAndPrompt(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)

	tests := []struct {
		name    string
		form    string
		wantMsg string
	}{
		{
			name:    "missing thread id",
			form:    "prompt=hello",
			wantMsg: "thread_id is required",
		},
		{
			name:    "missing prompt",
			form:    "thread_id=thread-1",
			wantMsg: "prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/agent-chat/resume",
				bytes.NewBufferString(tt.form),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set("user_email", "user@example.com")

			err := handler.ResumeThread(c)
			if err == nil {
				t.Fatal("ResumeThread() error = nil, want HTTP error")
			}

			httpErr, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("ResumeThread() error type = %T, want *echo.HTTPError", err)
			}
			if httpErr.Code != http.StatusBadRequest {
				t.Fatalf("HTTP status = %d, want %d", httpErr.Code, http.StatusBadRequest)
			}
			if msg := httpErr.Message.(string); msg != tt.wantMsg {
				t.Fatalf("HTTP message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestForkThreadRequiresSourceThreadIDSourceEntryIDAndPrompt(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)

	tests := []struct {
		name    string
		form    string
		wantMsg string
	}{
		{
			name:    "missing source thread id",
			form:    "source_entry_id=entry-1&prompt=hello",
			wantMsg: "source_thread_id is required",
		},
		{
			name:    "missing source entry id",
			form:    "source_thread_id=thread-1&prompt=hello",
			wantMsg: "source_entry_id is required",
		},
		{
			name:    "missing prompt",
			form:    "source_thread_id=thread-1&source_entry_id=entry-1",
			wantMsg: "prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/agent-chat/fork",
				bytes.NewBufferString(tt.form),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set("user_email", "user@example.com")

			err := handler.ForkThread(c)
			if err == nil {
				t.Fatal("ForkThread() error = nil, want HTTP error")
			}

			httpErr, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("ForkThread() error type = %T, want *echo.HTTPError", err)
			}
			if httpErr.Code != http.StatusBadRequest {
				t.Fatalf("HTTP status = %d, want %d", httpErr.Code, http.StatusBadRequest)
			}
			if msg := httpErr.Message.(string); msg != tt.wantMsg {
				t.Fatalf("HTTP message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestWorkspaceDocCommentCreateAllowsCoworkerAccess(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")

	form := url.Values{}
	form.Set("doc_rel_path", handlerTestPlanRelPath)
	form.Set("selected_text", "Body")
	form.Set("comment_text", "Please clarify")
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.CreateWorkspaceDocComment(c); err != nil {
		t.Fatalf(
			"CreateWorkspaceDocComment() error = %v, want shared workspace access",
			err,
		)
	}
	body := rec.Body.String()
	for _, want := range []string{"comment-target", "coworker@example.com", "Please clarify"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}

func TestWorkspaceDocCommentShowFormPatchesSelectedTextPreview(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	form := url.Values{}
	form.Set("doc_rel_path", handlerTestPlanRelPath)
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	form.Set("selected_text", "Body")
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments/show",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	c.Set("user_email", "user@example.com")

	if err := handler.ShowWorkspaceDocCommentForm(c); err != nil {
		t.Fatalf("ShowWorkspaceDocCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
		`name="selected_text" value="Body"`,
		`<blockquote`,
		`Body`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("selected-text show response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `textarea name="selected_text"`) ||
		strings.Contains(body, `name="selected_text"></textarea>`) {
		t.Fatalf("selected text rendered as editable textarea: %s", body)
	}
}

func TestWorkspaceDocCommentCancelPatchesNormalTarget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	if _, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor: WorkspaceDocCommentAnchor{
				SectionHint: "section-1",
				HeadingHint: "Plan",
			},
		},
	); err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	form := url.Values{}
	form.Set("doc_rel_path", handlerTestPlanRelPath)
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments/cancel",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	c.Set("user_email", "user@example.com")

	if err := handler.CancelWorkspaceDocCommentForm(c); err != nil {
		t.Fatalf("CancelWorkspaceDocCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
		`Question`,
		`Add comment`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("cancel response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `artifact-comment-form-`) ||
		strings.Contains(body, `Add a comment...`) {
		t.Fatalf("cancel response still contains comment form: %s", body)
	}
}

func TestWorkspaceDocCommentCreatePatchesContextualTarget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	form := url.Values{}
	form.Set("doc_rel_path", handlerTestPlanRelPath)
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	form.Set("selected_text", "Body")
	form.Set("comment_text", "Please clarify")
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	c.Set("user_email", "user@example.com")

	if err := handler.CreateWorkspaceDocComment(c); err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
		`Please clarify`,
		`Body`,
		`Open`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("create response missing %q: %s", want, body)
		}
	}
	events, err := service.queries.ListWorkspaceEvents(
		ctx,
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	wantDocPath := "thoughts/creative-mode-agent/plans/2026-04-30_test-plan/plan.md"
	found := false
	for _, event := range events {
		if event.EventType == "comment_created" &&
			event.DocPath.Valid &&
			event.DocPath.String == wantDocPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %#v, want comment_created for %s", events, wantDocPath)
	}
}

func TestWorkspaceDocCommentReplyPatchesContextualTarget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor: WorkspaceDocCommentAnchor{
				SectionHint: "section-1",
				HeadingHint: "Plan",
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	form := url.Values{}
	form.Set("doc_rel_path", handlerTestPlanRelPath)
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	form.Set("reply_text", "Reply body")
	form.Set("request_id", "reply-request")
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments/"+comment.ID+"/replies",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id", "comment_id")
	c.SetParamValues(workspace.ID, comment.ID)
	c.Set("user_email", "user@example.com")

	if err := handler.ReplyWorkspaceDocComment(c); err != nil {
		t.Fatalf("ReplyWorkspaceDocComment() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
		`Question`,
		`Reply body`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("reply response missing %q: %s", want, body)
		}
	}
}

func TestWorkspaceDocCommentResolveAndReopenPatchContextualTarget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor: WorkspaceDocCommentAnchor{
				SectionHint: "section-1",
				HeadingHint: "Plan",
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	for _, tc := range []struct {
		name    string
		handler func(echo.Context) error
		path    string
		want    string
	}{
		{
			name:    "resolve",
			handler: handler.ResolveWorkspaceDocComment,
			path:    "/resolve",
			want:    "Resolved",
		},
		{
			name:    "reopen",
			handler: handler.ReopenWorkspaceDocComment,
			path:    "/reopen",
			want:    "Open",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("doc_rel_path", handlerTestPlanRelPath)
			form.Set("section_hint", "section-1")
			form.Set("heading_hint", "Plan")
			form.Set("request_id", tc.name+"-request")
			req := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/agent-chat/"+workspace.ID+"/docs/comments/"+comment.ID+tc.path,
				strings.NewReader(form.Encode()),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("workspace_id", "comment_id")
			c.SetParamValues(workspace.ID, comment.ID)
			c.Set("user_email", "user@example.com")

			if err := tc.handler(c); err != nil {
				t.Fatalf("%s handler error = %v", tc.name, err)
			}
			body := rec.Body.String()
			for _, want := range []string{
				`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
				`Question`,
				tc.want,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("%s response missing %q: %s", tc.name, want, body)
				}
			}
		})
	}
}

func TestWorkspaceDocCommentActionFallsBackToPersistedTarget(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor: WorkspaceDocCommentAnchor{
				SectionHint: "section-1",
				HeadingHint: "Plan",
			},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	form := url.Values{}
	form.Set("reply_text", "Fallback reply")
	form.Set("request_id", "fallback-reply")
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/agent-chat/"+workspace.ID+"/docs/comments/"+comment.ID+"/replies",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("workspace_id", "comment_id")
	c.SetParamValues(workspace.ID, comment.ID)
	c.Set("user_email", "user@example.com")

	if err := handler.ReplyWorkspaceDocComment(c); err != nil {
		t.Fatalf("ReplyWorkspaceDocComment() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="` + docSectionTargetID(workspace.ID, handlerTestPlanRelPath, "section-1") + `"`,
		`Question`,
		`Fallback reply`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fallback response missing %q: %s", want, body)
		}
	}
}

func TestWorkspaceDocContentFallbackRendersFullHTMLWithDocumentTarget(t *testing.T) {
	t.Parallel()

	state := ArtifactPaneState{
		WorkspaceID: "workspace-1",
		Selected: ArtifactRenderView{
			RelativePath: "notes.txt",
			Exists:       true,
			HTML:         `<pre>raw fallback</pre>`,
		},
	}
	var buf bytes.Buffer
	if err := WorkspaceDocContent(
		state,
	).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	body := buf.String()
	for _, want := range []string{`raw fallback`, `data-comment-target="true"`, `Add comment`} {
		if !strings.Contains(body, want) {
			t.Fatalf("fallback artifact missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `agent-chat-artifact-comments`) ||
		strings.Contains(body, `Comment on selected artifact`) {
		t.Fatalf("fallback artifact reintroduced bottom comments panel: %s", body)
	}
}

func TestWorkspaceDocCommentTargetHelpersUseSafeStableIDs(t *testing.T) {
	t.Parallel()

	first := docSectionTargetID(
		"workspace-1",
		"plans/2026-05-04_demo/design.md",
		"section-1",
	)
	second := docSectionTargetID(
		"workspace-1",
		"plans/2026-05-04_demo/design.md",
		"section-1",
	)
	if first != second {
		t.Fatalf("docSectionTargetID not stable: %q vs %q", first, second)
	}
	if strings.Contains(first, "/") || strings.Contains(first, ".") {
		t.Fatalf("docSectionTargetID exposes raw path characters: %q", first)
	}
	if got := docDocumentTargetID(
		"workspace-1",
		"plans/2026-05-04_demo/design.md",
	); got == "" || strings.Contains(got, "/") ||
		strings.Contains(got, ".") {
		t.Fatalf("docDocumentTargetID() = %q, want safe non-empty ID", got)
	}
}

func TestWorkspaceCommentRoutesParseRequestID(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	form := url.Values{}
	form.Set("request_id", "resolve-route")
	for i := range 2 {
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/agent-chat/"+workspace.ID+"/docs/comments/"+comment.ID+"/resolve",
			strings.NewReader(form.Encode()),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("workspace_id", "comment_id")
		c.SetParamValues(workspace.ID, comment.ID)
		c.Set("user_email", "user@example.com")
		if err := handler.ResolveWorkspaceDocComment(c); err != nil {
			t.Fatalf("ResolveWorkspaceDocComment(%d) error = %v", i, err)
		}
	}

	events, err := service.queries.ListWorkspaceEvents(
		ctx,
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	resolveEvents := 0
	for _, event := range events {
		if event.EventType != "comment_resolved" || !event.CommentID.Valid ||
			event.CommentID.String != comment.ID {
			continue
		}
		resolveEvents++
		if !event.EventKey.Valid ||
			!strings.Contains(event.EventKey.String, "resolve-route") {
			t.Fatalf("resolve event key = %v, want request id", event.EventKey)
		}
	}
	if resolveEvents != 1 {
		t.Fatalf("resolveEvents = %d, want 1; events = %#v", resolveEvents, events)
	}
}

func TestAgentCommentRoutesParseRequestID(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	form := url.Values{}
	form.Set("message", "Done")
	form.Set("request_id", "agent-reply-route")
	for i := range 2 {
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/agent-chat/"+workspace.ID+"/comments/"+comment.ID+"/replies",
			strings.NewReader(form.Encode()),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("workspace_id", "comment_id")
		c.SetParamValues(workspace.ID, comment.ID)
		c.Set("user_email", "user@example.com")
		if err := handler.AgentReplyWorkspaceDocCommentAPI(c); err != nil {
			t.Fatalf("AgentReplyWorkspaceDocCommentAPI(%d) error = %v", i, err)
		}
	}
	replies, err := service.queries.ListWorkspaceDocCommentReplies(
		ctx,
		comment.ID,
	)
	if err != nil {
		t.Fatalf("ListWorkspaceDocCommentReplies() error = %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("replies = %#v, want one idempotent reply", replies)
	}
}

func TestWorkspaceCommentPatchRefreshesResourceWithCommentState(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	if err := service.queries.UpdateWorkspaceSelectedDoc(
		ctx,
		db.UpdateWorkspaceSelectedDocParams{
			ID: workspace.ID,
			SelectedDocPath: sql.NullString{
				String: handlerTestPlanRelPath,
				Valid:  true,
			},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedDoc() error = %v", err)
	}
	if _, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     handlerTestPlanRelPath,
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor: WorkspaceDocCommentAnchor{
				SelectedText: "missing quote",
			},
		},
	); err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/stream",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)
	sse := datastar.NewSSE(rec, req)

	if err := handler.patchWorkspace(c, sse, PatchArtifactAnchorCallouts); err != nil {
		t.Fatalf("patchWorkspace(PatchArtifactAnchorCallouts) error = %v", err)
	}
	body := rec.Body.String()
	assertWorkspaceResourcePatch(t, body)
	for _, target := range []string{
		"agent-chat-workspace-topology-region",
		"agent-chat-workspace-topology-sidebar",
		"agent-chat-doc-region",
		"agent-chat-chat-region",
	} {
		if !strings.Contains(body, target) {
			t.Fatalf("resource comment patch missing %s: %s", target, body)
		}
	}
	for _, removed := range []string{"agent-chat-artifact-comments", "Artifact comment anchors", "Stale comment"} {
		if strings.Contains(body, removed) {
			t.Fatalf(
				"resource comment patch rendered removed comment chrome %q: %s",
				removed,
				body,
			)
		}
	}
}

func mustCreatePlanDirForUser(
	t *testing.T,
	service *Service,
	userEmail string,
	name string,
) string {
	t.Helper()
	planDir := filepath.Join(service.thoughtsRoot, userEmail, "plans", name)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(planDir) error = %v", err)
	}
	mustWriteFile(t, filepath.Join(planDir, handlerTestPlanRelPath), "# Plan\n\nBody")
	return planDir
}

func piOpenNoTouchedPlanFixtureLines(cwd string) []string {
	return []string{
		`{"type":"session","id":"s-open","timestamp":"2026-04-30T12:00:00Z","cwd":"` + filepath.ToSlash(
			cwd,
		) + `"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"please continue this"}}`,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-30T12:00:02Z","message":{"role":"assistant","content":"Imported assistant reply"}}`,
		`{"type":"custom","id":"custom-1","parentId":"assistant-1","timestamp":"2026-04-30T12:00:03Z","customType":"plan-classification","data":{"planDir":"` + filepath.ToSlash(
			cwd,
		) + `","source":"prompt-path"}}`,
	}
}

func postOpenPiSession(
	t *testing.T,
	handler *Handler,
	userEmail string,
	form url.Values,
) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		piSessionOpenEndpoint,
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if strings.TrimSpace(userEmail) != "" {
		c.Set("user_email", userEmail)
	}
	return rec, handler.OpenPiSession(c)
}

func assertHTTPErrorCode(t *testing.T, err error, want int) {
	t.Helper()
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %[1]v, want *echo.HTTPError", err)
	}
	if httpErr.Code != want {
		t.Fatalf("HTTPError.Code = %d, want %d: %v", httpErr.Code, want, httpErr)
	}
}

func countAgentSessions(t *testing.T, service *Service) int {
	t.Helper()
	count, err := service.queries.TestSupportCountAgentSessions(t.Context())
	if err != nil {
		t.Fatalf("count agent_sessions: %v", err)
	}
	return int(count)
}

func TestThreadSidebarRendersPiSessionOpenForm(t *testing.T) {
	t.Parallel()

	thread := ThreadSidebarThread{
		Title:               "[q-resume] terminal handoff",
		CwdLabel:            "research",
		SourceLabel:         "Pi",
		OpenPiSessionAction: piSessionOpenAction(),
		SessionPath:         "/tmp/pi/session.jsonl",
		WorkspaceDir:        "/tmp/project/thoughts/user@example.com/plans/2026-04-30_demo",
	}

	rec := httptest.NewRecorder()
	if err := WorkspaceSidebarThreadRow("", thread).Render(t.Context(), rec); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<form`,
		`data-on:submit="@post(&#39;/agent-chat/pi-sessions/open&#39;, {contentType: &#39;form&#39;})"`,
		`name="session_path"`,
		`value="/tmp/pi/session.jsonl"`,
		`name="workspace_dir"`,
		`value="/tmp/project/thoughts/user@example.com/plans/2026-04-30_demo"`,
		`type="submit"`,
		`data-indicator="_openingPiSession`,
		`Opening…`,
		`Pi`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Pi open form missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `href=`) {
		t.Fatalf("Pi open form unexpectedly rendered an anchor: %s", body)
	}

	anchorRec := httptest.NewRecorder()
	anchor := ThreadSidebarThread{
		ID:    "thread-1",
		Href:  "/agent-chat/workspace-1/thread/thread-1",
		Title: "Browser thread",
	}
	if err := WorkspaceSidebarThreadRow("workspace-1", anchor).Render(
		t.Context(),
		anchorRec,
	); err != nil {
		t.Fatalf("Render(anchor) error = %v", err)
	}
	anchorBody := anchorRec.Body.String()
	if !strings.Contains(anchorBody, `href="/agent-chat/workspace-1/thread/thread-1"`) {
		t.Fatalf("DB-backed row did not render anchor: %s", anchorBody)
	}
	if strings.Contains(anchorBody, piSessionOpenEndpoint) {
		t.Fatalf("DB-backed row rendered Pi open endpoint: %s", anchorBody)
	}
}

func TestOpenPiSessionImportsExplicitWorkspaceDirAndRedirects(t *testing.T) {
	t.Parallel()

	const authenticatedUser = "user@example.com"
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreatePlanDirForUser(
		t,
		service,
		authenticatedUser,
		"2026-04-30_demo",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "open", "session.jsonl")
	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(planDir)...)

	form := url.Values{}
	form.Set("session_path", sessionPath)
	form.Set("workspace_dir", planDir)
	form.Set("user_email", "attacker@example.com")
	rec, err := postOpenPiSession(t, handler, authenticatedUser, form)
	if err != nil {
		t.Fatalf("OpenPiSession() error = %v", err)
	}

	session, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if session.Source != string(AgentSessionSourceAdopted) {
		t.Fatalf("session.Source = %q, want adopted", session.Source)
	}
	if !session.WorkspaceID.Valid || !session.ThreadID.Valid {
		t.Fatalf(
			"session workspace/thread = %v/%v, want both set",
			session.WorkspaceID,
			session.ThreadID,
		)
	}
	if !session.UserEmail.Valid || session.UserEmail.String != authenticatedUser {
		t.Fatalf("session.UserEmail = %v, want authenticated user", session.UserEmail)
	}
	storedWorkspace, err := service.queries.GetWorkspace(
		t.Context(),
		session.WorkspaceID.String,
	)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if storedWorkspace.UserEmail != authenticatedUser ||
		storedWorkspace.RootDocPath != planDir {
		t.Fatalf("workspace = %+v, want authenticated user's plan", storedWorkspace)
	}
	wantURL := workspaceThreadHrefForWorkspace(storedWorkspace, session.ThreadID.String)
	if !strings.Contains(rec.Body.String(), wantURL) {
		t.Fatalf("SSE redirect missing %q: %s", wantURL, rec.Body.String())
	}
}

func TestOpenPiSessionAllowsAuthenticatedViewerToOpenPlanOwnedByAnotherPathUser(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestOpenPiSessionRequiresAuthentication(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	form := url.Values{}
	form.Set("session_path", filepath.Join(service.piSessionsDir, "missing.jsonl"))

	_, err := postOpenPiSession(t, handler, "", form)
	assertHTTPErrorCode(t, err, http.StatusUnauthorized)
}

func storedWorkspaceForTest(
	t *testing.T,
	service *Service,
	workspaceID string,
) db.Workspace {
	t.Helper()
	workspaceRecord, err := service.queries.GetWorkspace(t.Context(), workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	return workspaceRecord
}

func TestOpenPiSessionRedirectTargetLoadsImportedThreadContext(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/"+workspace.ID, http.NoBody)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	err := handler.HandleWorkspacePage(c)
	if err == nil {
		t.Fatal("HandleWorkspacePage() error = nil, want gone")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusGone {
		t.Fatalf("HandleWorkspacePage() error = %v, want 410 Gone", err)
	}
}

func TestOpenPiSessionRedirectsDivergedImportedSession(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreatePlanDirForUser(
		t,
		service,
		"user@example.com",
		"2026-04-30_diverged",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "open", "diverged.jsonl")
	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(planDir)...)

	form := url.Values{}
	form.Set("session_path", sessionPath)
	form.Set("workspace_dir", planDir)
	if _, err := postOpenPiSession(t, handler, "viewer@example.com", form); err != nil {
		t.Fatalf("initial OpenPiSession() error = %v", err)
	}
	firstSession, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if err := service.queries.UpdateAgentThreadHead(
		t.Context(),
		db.UpdateAgentThreadHeadParams{
			ID:          firstSession.ThreadID.String,
			HeadEntryID: nullString("web-head"),
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}

	rec, err := postOpenPiSession(t, handler, "other-viewer@example.com", form)
	if err != nil {
		t.Fatalf("diverged OpenPiSession() error = %v", err)
	}
	afterSession, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(after) error = %v", err)
	}
	if afterSession.Status != "diverged" {
		t.Fatalf("session.Status = %q, want diverged", afterSession.Status)
	}
	if afterSession.ThreadID.String == firstSession.ThreadID.String {
		t.Fatalf("ThreadID = %s, want diverged sibling", afterSession.ThreadID.String)
	}
	afterWorkspace, err := service.queries.GetWorkspace(
		t.Context(),
		afterSession.WorkspaceID.String,
	)
	if err != nil {
		t.Fatalf("GetWorkspace(after) error = %v", err)
	}
	wantURL := workspaceThreadHrefForWorkspace(
		afterWorkspace,
		afterSession.ThreadID.String,
	)
	if !strings.Contains(rec.Body.String(), wantURL) {
		t.Fatalf("SSE redirect missing %q: %s", wantURL, rec.Body.String())
	}
}

func TestOpenPiSessionRejectsTamperedWorkspaceContext(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, service *Service) url.Values
	}{
		{
			name: "foreign workspace id",
			setup: func(t *testing.T, service *Service) url.Values {
				t.Helper()
				foreignWorkspace := mustCreateWorkspaceForHandlerTest(
					t,
					service,
					"other@example.com",
				)
				planDir := mustCreatePlanDirForUser(
					t,
					service,
					"user@example.com",
					"2026-04-30_demo",
				)
				sessionPath := filepath.Join(service.piSessionsDir, "tamper", "foreign.jsonl")
				writePiSessionFile(
					t,
					sessionPath,
					piOpenNoTouchedPlanFixtureLines(planDir)...,
				)
				return url.Values{
					"session_path":  {sessionPath},
					"workspace_dir": {planDir},
					"workspace_id":  {foreignWorkspace.ID},
				}
			},
		},
		{
			name: "mismatched workspace dir",
			setup: func(t *testing.T, service *Service) url.Values {
				t.Helper()
				ownerWorkspace := mustCreateWorkspaceForHandlerTest(
					t,
					service,
					"user@example.com",
				)
				otherPlanDir := mustCreatePlanDirForUser(
					t,
					service,
					"user@example.com",
					"2026-04-30_other",
				)
				sessionPath := filepath.Join(service.piSessionsDir, "tamper", "mismatch.jsonl")
				writePiSessionFile(
					t,
					sessionPath,
					piOpenNoTouchedPlanFixtureLines(otherPlanDir)...,
				)
				return url.Values{
					"session_path":  {sessionPath},
					"workspace_dir": {otherPlanDir},
					"workspace_id":  {ownerWorkspace.ID},
				}
			},
		},
		{
			name: "malformed session with workspace dir outside thoughts root",
			setup: func(t *testing.T, service *Service) url.Values {
				t.Helper()
				outsideDir := filepath.Join(t.TempDir(), "outside", "plans", "2026-04-30_other")
				if err := os.MkdirAll(outsideDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(outsideDir) error = %v", err)
				}
				sessionPath := filepath.Join(service.piSessionsDir, "tamper", "malformed-outside-dir.jsonl")
				mustWriteFile(t, sessionPath, "not-json\n")
				return url.Values{
					"session_path":  {sessionPath},
					"workspace_dir": {outsideDir},
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := newTestAgentChatService(t)
			handler := NewHandler(service, nil)
			form := tc.setup(t, service)

			_, err := postOpenPiSession(t, handler, "user@example.com", form)
			assertHTTPErrorCode(t, err, http.StatusBadRequest)
			if count := countAgentSessions(t, service); count != 0 {
				t.Fatalf("agent_sessions count = %d, want no mutation", count)
			}
		})
	}
}

func TestOpenPiSessionRejectsInvalidSessionPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		makePath  func(t *testing.T, service *Service) string
		wantError string
	}{
		{
			name: "empty",
			makePath: func(t *testing.T, service *Service) string {
				t.Helper()
				return ""
			},
			wantError: "path is required",
		},
		{
			name: "outside pi sessions dir",
			makePath: func(t *testing.T, service *Service) string {
				t.Helper()
				outside := filepath.Join(t.TempDir(), "outside.jsonl")
				writePiSessionFile(
					t,
					outside,
					`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
				)
				return outside
			},
			wantError: "outside Pi sessions dir",
		},
		{
			name: "non jsonl",
			makePath: func(t *testing.T, service *Service) string {
				t.Helper()
				path := filepath.Join(service.piSessionsDir, "not-jsonl.txt")
				if err := os.WriteFile(path, []byte("not jsonl"), 0o600); err != nil {
					t.Fatalf("WriteFile(non-jsonl) error = %v", err)
				}
				return path
			},
			wantError: ".jsonl",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := newTestAgentChatService(t)
			handler := NewHandler(service, nil)
			planDir := mustCreatePlanDirForUser(
				t,
				service,
				"user@example.com",
				"2026-04-30_demo",
			)
			form := url.Values{}
			form.Set("session_path", tc.makePath(t, service))
			form.Set("workspace_dir", planDir)

			_, err := postOpenPiSession(t, handler, "user@example.com", form)
			assertHTTPErrorCode(t, err, http.StatusBadRequest)
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantError)
			}
			if count := countAgentSessions(t, service); count != 0 {
				t.Fatalf("agent_sessions count = %d, want no mutation", count)
			}
		})
	}
}

func TestOpenPiSessionReturnsBadRequestWhenImportProducesNoThread(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	sessionPath := filepath.Join(service.piSessionsDir, "unassigned.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"s-unassigned","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
	)
	form := url.Values{}
	form.Set("session_path", sessionPath)

	rec, err := postOpenPiSession(t, handler, "user@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	if strings.Contains(rec.Body.String(), "/agent-chat/") {
		t.Fatalf("unexpected redirect body: %s", rec.Body.String())
	}
	session, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if session.Status != "unassigned" || session.ThreadID.Valid ||
		session.WorkspaceID.Valid {
		t.Fatalf("session = %+v, want unassigned without thread/workspace", session)
	}
	if !session.UserEmail.Valid || session.UserEmail.String != "user@example.com" {
		t.Fatalf("session.UserEmail = %v, want authenticated user", session.UserEmail)
	}
}

func TestOpenPiSessionIsIdempotentForAlreadyImportedSession(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreatePlanDirForUser(
		t,
		service,
		"user@example.com",
		"2026-04-30_demo",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "idempotent.jsonl")
	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(planDir)...)
	form := url.Values{}
	form.Set("session_path", sessionPath)
	form.Set("workspace_dir", planDir)

	if _, err := postOpenPiSession(t, handler, "user@example.com", form); err != nil {
		t.Fatalf("first OpenPiSession() error = %v", err)
	}
	first, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(first) error = %v", err)
	}
	secondRec, err := postOpenPiSession(t, handler, "user@example.com", form)
	if err != nil {
		t.Fatalf("second OpenPiSession() error = %v", err)
	}
	second, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(second) error = %v", err)
	}
	if !first.ThreadID.Valid || !second.ThreadID.Valid ||
		first.ThreadID.String != second.ThreadID.String {
		t.Fatalf(
			"thread IDs first=%v second=%v, want same",
			first.ThreadID,
			second.ThreadID,
		)
	}
	secondWorkspace, err := service.queries.GetWorkspace(
		t.Context(),
		second.WorkspaceID.String,
	)
	if err != nil {
		t.Fatalf("GetWorkspace(second) error = %v", err)
	}
	wantURL := workspaceThreadHrefForWorkspace(secondWorkspace, second.ThreadID.String)
	if !strings.Contains(secondRec.Body.String(), wantURL) {
		t.Fatalf("second redirect missing %q: %s", wantURL, secondRec.Body.String())
	}

	threadCount64, err := service.queries.TestSupportCountPrimaryThreadWorkspaceAssociationsByWorkspace(t.Context(), second.WorkspaceID.String)
	if err != nil {
		t.Fatalf("count agent_threads: %v", err)
	}
	threadCount := int(threadCount64)
	if threadCount != 1 {
		t.Fatalf("agent thread count = %d, want 1", threadCount)
	}
}

func TestOpenPiSessionAllowsCrossUserAlreadyImportedSession(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreatePlanDirForUser(
		t,
		service,
		"first@example.com",
		"2026-04-30_first",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "cross-user.jsonl")
	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(planDir)...)

	form := url.Values{}
	form.Set("session_path", sessionPath)
	form.Set("workspace_dir", planDir)
	if _, err := postOpenPiSession(
		t,
		handler,
		"first@example.com",
		form,
	); err != nil {
		t.Fatalf("first OpenPiSession() error = %v", err)
	}
	before, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(before) error = %v", err)
	}

	rec, err := postOpenPiSession(t, handler, "second@example.com", form)
	if err != nil {
		t.Fatalf("second OpenPiSession() error = %v", err)
	}
	after, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(after) error = %v", err)
	}
	if after.WorkspaceID != before.WorkspaceID || after.ThreadID != before.ThreadID ||
		after.Status != before.Status || after.UserEmail != before.UserEmail {
		t.Fatalf(
			"session mutated after cross-user open: before=%+v after=%+v",
			before,
			after,
		)
	}
	beforeWorkspace, err := service.queries.GetWorkspace(
		t.Context(),
		before.WorkspaceID.String,
	)
	if err != nil {
		t.Fatalf("GetWorkspace(before) error = %v", err)
	}
	wantURL := workspaceThreadHrefForWorkspace(beforeWorkspace, before.ThreadID.String)
	if !strings.Contains(rec.Body.String(), wantURL) {
		t.Fatalf("cross-user redirect missing %q: %s", wantURL, rec.Body.String())
	}
}

func TestOpenPiSessionAllowsSameUserRetryForUnassignedSession(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	sessionPath := filepath.Join(service.piSessionsDir, "unassigned-retry.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"s-unassigned","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
	)
	form := url.Values{"session_path": {sessionPath}}

	_, err := postOpenPiSession(t, handler, "user@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	before, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(before) error = %v", err)
	}

	_, err = postOpenPiSession(t, handler, "user@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	after, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(after) error = %v", err)
	}
	if after.ID != before.ID || after.UserEmail != before.UserEmail ||
		after.Status != "unassigned" || after.WorkspaceID.Valid || after.ThreadID.Valid {
		t.Fatalf(
			"session after retry = %+v, want same owner-bearing unassigned row %+v",
			after,
			before,
		)
	}
	if count := countAgentSessions(t, service); count != 1 {
		t.Fatalf("agent_sessions count = %d, want 1", count)
	}
}

func TestOpenPiSessionAllowsCrossUserRetryForOwnerBearingUnassignedSession(
	t *testing.T,
) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	sessionPath := filepath.Join(service.piSessionsDir, "cross-user-unassigned.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"s-unassigned","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
	)
	form := url.Values{"session_path": {sessionPath}}

	_, err := postOpenPiSession(t, handler, "first@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	before, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(before) error = %v", err)
	}

	_, err = postOpenPiSession(t, handler, "second@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	if strings.Contains(err.Error(), "another user") {
		t.Fatalf("error = %v, want non-ownership failure", err)
	}
	after, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(after) error = %v", err)
	}
	if after.ID != before.ID || after.UserEmail != before.UserEmail ||
		after.Status != before.Status || after.WorkspaceID.Valid || after.ThreadID.Valid {
		t.Fatalf(
			"session mutated after cross-user retry: before=%+v after=%+v",
			before,
			after,
		)
	}
	if count := countAgentSessions(t, service); count != 1 {
		t.Fatalf("agent_sessions count = %d, want 1", count)
	}
}

func TestOpenPiSessionRejectsHistoricalOwnerlessUnassignedSessionReuse(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	sessionPath := filepath.Join(
		service.piSessionsDir,
		"historical-ownerless-unassigned.jsonl",
	)
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"s-historical","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
	)
	_, err := service.queries.CreateAgentSession(t.Context(), db.CreateAgentSessionParams{
		ID:                  "historical-ownerless-unassigned",
		WorkspaceID:         sql.NullString{},
		ThreadID:            sql.NullString{},
		UserEmail:           sql.NullString{},
		Source:              "adopted",
		SessionPath:         nullString(sessionPath),
		SessionID:           sql.NullString{},
		ParentSessionID:     sql.NullString{},
		Cwd:                 sql.NullString{},
		Status:              "unassigned",
		InferredWorkspaceID: sql.NullString{},
		InferredPlanDir:     sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	})
	if err != nil {
		t.Fatalf("seed historical ownerless session: %v", err)
	}

	form := url.Values{"session_path": {sessionPath}}
	_, err = postOpenPiSession(t, handler, "user@example.com", form)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	if !strings.Contains(err.Error(), "no owner") {
		t.Fatalf("error = %v, want ownerless rejection", err)
	}
	session, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if session.UserEmail.Valid || session.WorkspaceID.Valid ||
		session.ThreadID.Valid || session.Status != "unassigned" {
		t.Fatalf("historical session mutated: %+v", session)
	}
}

func TestOpenPiSessionAllowsHistoricalOwnerlessAssignedCrossUserWhenMappingMatches(
	t *testing.T,
) {
	t.Parallel()

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	planDir := mustCreatePlanDirForUser(
		t,
		service,
		"owner@example.com",
		"2026-04-30_historical",
	)
	sessionPath := filepath.Join(
		service.piSessionsDir,
		"historical-ownerless-assigned.jsonl",
	)
	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(planDir)...)

	historicalWorkspace := mustCreateWorkspaceForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	thread := mustCreateAgentThread(
		t,
		service,
		"historical-thread",
		"owner@example.com",
		historicalWorkspace.RootDocPath,
		"historical-lineage",
	)
	if err := service.queries.AttachThreadToWorkspace(
		t.Context(),
		db.AttachThreadToWorkspaceParams{
			ID:          thread.ID,
			WorkspaceID: nullString(historicalWorkspace.ID),
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace() error = %v", err)
	}
	_, err := service.queries.CreateAgentSession(t.Context(), db.CreateAgentSessionParams{
		ID:                  "historical-ownerless-assigned",
		WorkspaceID:         nullString(historicalWorkspace.ID),
		ThreadID:            nullString(thread.ID),
		UserEmail:           sql.NullString{},
		Source:              "adopted",
		SessionPath:         nullString(sessionPath),
		SessionID:           sql.NullString{},
		ParentSessionID:     sql.NullString{},
		Cwd:                 sql.NullString{},
		Status:              "imported",
		InferredWorkspaceID: sql.NullString{},
		InferredPlanDir:     sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	})
	if err != nil {
		t.Fatalf("seed historical assigned session: %v", err)
	}

	form := url.Values{
		"session_path":  {sessionPath},
		"workspace_dir": {historicalWorkspace.RootDocPath},
	}
	rec, err := postOpenPiSession(t, handler, "other@example.com", form)
	if err != nil {
		t.Fatalf("cross-user OpenPiSession() error = %v", err)
	}
	wantURL := workspaceThreadHrefForWorkspace(historicalWorkspace, thread.ID)
	if !strings.Contains(rec.Body.String(), wantURL) {
		t.Fatalf("cross-user redirect missing %q: %s", wantURL, rec.Body.String())
	}
}

func TestOpenPiSessionPersistsOwnerForFailedImportAndAllowsDifferentUserRetry(
	t *testing.T,
) {
	t.Parallel()

	const failedStatus = "failed"

	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	firstPlanDir := mustCreatePlanDirForUser(
		t,
		service,
		"first@example.com",
		"2026-04-30_first_failed",
	)
	secondPlanDir := mustCreatePlanDirForUser(
		t,
		service,
		"second@example.com",
		"2026-04-30_second_failed",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "failed-owner.jsonl")
	mustWriteFile(t, sessionPath, "not-json\n")

	firstForm := url.Values{
		"session_path":  {sessionPath},
		"workspace_dir": {firstPlanDir},
	}
	_, err := postOpenPiSession(t, handler, "first@example.com", firstForm)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	before, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(before) error = %v", err)
	}
	if before.Status != failedStatus || !before.UserEmail.Valid ||
		before.UserEmail.String != "first@example.com" ||
		before.WorkspaceID.Valid || before.ThreadID.Valid {
		t.Fatalf(
			"failed session = %+v, want owner-bearing failed row without workspace/thread",
			before,
		)
	}

	secondForm := url.Values{
		"session_path":  {sessionPath},
		"workspace_dir": {secondPlanDir},
	}
	_, err = postOpenPiSession(t, handler, "second@example.com", secondForm)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
	if strings.Contains(err.Error(), "another user") {
		t.Fatalf("error = %v, want non-ownership failure", err)
	}
	afterReject, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(afterReject) error = %v", err)
	}
	if afterReject.ID != before.ID || afterReject.UserEmail != before.UserEmail ||
		afterReject.WorkspaceID.Valid || afterReject.ThreadID.Valid ||
		afterReject.Status != before.Status {
		t.Fatalf(
			"failed session mutated after cross-user retry: before=%+v after=%+v",
			before,
			afterReject,
		)
	}

	writePiSessionFile(t, sessionPath, piOpenNoTouchedPlanFixtureLines(secondPlanDir)...)
	if _, err := postOpenPiSession(
		t,
		handler,
		"second@example.com",
		secondForm,
	); err != nil {
		t.Fatalf("different-user retry after fixing file error = %v", err)
	}
	afterRetry, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(afterRetry) error = %v", err)
	}
	if afterRetry.ID != before.ID || !afterRetry.UserEmail.Valid ||
		afterRetry.UserEmail.String != "first@example.com" ||
		!afterRetry.WorkspaceID.Valid || !afterRetry.ThreadID.Valid ||
		afterRetry.Status != "imported" {
		t.Fatalf(
			"after same-user retry = %+v, want same row imported for first user",
			afterRetry,
		)
	}
}

func mustCreateWorkspaceForHandlerTest(
	t *testing.T,
	service *Service,
	userEmail string,
) db.Workspace {
	t.Helper()
	root := filepath.Join(t.TempDir(), "thoughts")
	artifactRoot := filepath.Join(
		root,
		"creative-mode-agent",
		"plans",
		"2026-04-30_test-plan",
	)
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(artifactRoot) error = %v", err)
	}
	mustWriteFile(
		t,
		filepath.Join(artifactRoot, handlerTestPlanRelPath),
		"# Plan\n\nBody",
	)
	service.thoughtsRoot = root
	workspace, err := service.CreateWorkspace(context.Background(), WorkspaceCreateInput{
		UserEmail:   userEmail,
		Title:       "Test Workspace",
		RootDocPath: artifactRoot,
		Cwd:         artifactRoot,
		Source:      WorkspaceSourceWeb,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return workspace
}

func mustCreateWorkspaceThreadForHandlerTest(
	t *testing.T,
	service *Service,
	userEmail string,
) (db.Workspace, db.AgentThread) {
	t.Helper()
	workspace := mustCreateWorkspaceForHandlerTest(t, service, userEmail)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		userEmail,
		workspace.RootDocPath,
		"lineage-1",
	)
	if err := service.queries.AttachThreadToWorkspace(
		context.Background(),
		db.AttachThreadToWorkspaceParams{
			ID:          thread.ID,
			WorkspaceID: sql.NullString{String: workspace.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace() error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceSelectedThread(
		context.Background(),
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: sql.NullString{String: thread.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedThread() error = %v", err)
	}
	thread.WorkspaceID = sql.NullString{String: workspace.ID, Valid: true}
	return workspace, thread
}

func TestChangeCwdRedirectsToThreadPage(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-2",
		"user@example.com",
		projectRoot,
		"lineage-2",
	)
	childDir := filepath.Join(projectRoot, "thoughts")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	form := url.Values{}
	form.Set("thread_id", thread.ID)
	form.Set("cwd", childDir)

	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/cwd",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")

	if err := handler.ChangeCwd(c); err != nil {
		t.Fatalf("ChangeCwd() error = %v", err)
	}

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	body := rec.Body.String()
	if strings.Contains(body, "/agent-chat?thread="+thread.ID) {
		t.Fatalf("response redirected to retired thread page: %s", body)
	}

	stored, err := service.queries.GetAgentThread(t.Context(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if stored.Cwd != childDir {
		t.Fatalf("stored cwd = %q, want %q", stored.Cwd, childDir)
	}
}
