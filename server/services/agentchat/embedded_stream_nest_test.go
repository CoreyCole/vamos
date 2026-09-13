package agentchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/db"
)

func setupEmbeddedWorkspaceStreamFixture(t *testing.T) (*Service, *Handler, string) {
	t.Helper()
	service, queries := newThreadDraftService(t)
	thoughtsRoot := t.TempDir()
	service.thoughtsRoot = thoughtsRoot
	service.defaultCwd = thoughtsRoot
	service.notifier = NewNotifier()
	service.liveThreads = make(map[string]*liveThreadState)
	service.chatSessions = chatsession.NewService(service.db, queries)
	service.piIndexRunning = make(map[string]bool)
	service.piIndexQueued = make(map[string]PiSessionIndexRequest)

	createDraftThread(t, queries, "thread_1")
	if _, err := queries.CreateWorkspace(t.Context(), db.CreateWorkspaceParams{
		ID:           "workspace_1",
		UserEmail:    "owner@example.com",
		Title:        "Embedded nest",
		RootDocPath:  "",
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
	if err := queries.AttachThreadToWorkspace(t.Context(), db.AttachThreadToWorkspaceParams{
		ID:          "thread_1",
		WorkspaceID: nullString("workspace_1"),
	}); err != nil {
		t.Fatal(err)
	}
	return service, NewHandler(service, nil), "workspace_1"
}

func startEmbeddedWorkspaceStream(
	t *testing.T,
	handler *Handler,
	workspaceID, since string,
) (context.CancelFunc, *httptest.ResponseRecorder, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	target := "/thoughts/chat/workspace/" + workspaceID + "/stream?thread=thread_1&since=" + since
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/thoughts/chat/workspace/:workspace_id/stream")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspaceID)
	c.Set("user_email", "owner@example.com")
	done := make(chan error, 1)
	go func() { done <- handler.StreamEmbeddedWorkspace(c) }()
	return cancel, rec, done
}

func waitEmbeddedWorkspaceSubscriber(t *testing.T, service *Service, workspaceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if service.notifier.SubscriberCount(workspaceID) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("stream did not subscribe")
}

func waitStreamBodyContains(t *testing.T, rec *httptest.ResponseRecorder, needle string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := rec.Body.String()
		if strings.Contains(body, needle) {
			return body
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("stream body never contained %q:\n%s", needle, rec.Body.String())
	return ""
}

func assertNoPanelLiveNest(t *testing.T, body, label string) {
	t.Helper()
	if strings.Contains(body, "doc-right-chat-panel") &&
		strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("%s nested live via panel REPLACE:\n%s", label, body)
	}
	if strings.Contains(body, "selector #doc-right-chat-panel") ||
		strings.Contains(body, `id="doc-right-chat-panel"`) {
		t.Fatalf("%s emitted #doc-right-chat-panel REPLACE:\n%s", label, body)
	}
}

func TestStreamEmbeddedWorkspaceIncrementalSignalsDoNotNestLive(t *testing.T) {
	service, handler, workspaceID := setupEmbeddedWorkspaceStreamFixture(t)

	for _, tc := range []struct {
		name  string
		scope WorkspacePatchScope
		want  string
	}{
		{
			name:  "PatchWorkspaceResource",
			scope: PatchWorkspaceResource,
			want:  "agent-chat-stable-transcript",
		},
		{
			name:  "PatchStableTranscript",
			scope: PatchStableTranscript,
			want:  "agent-chat-stable-transcript",
		},
		{
			name:  "defaultPatchRunHeader",
			scope: PatchRunHeader,
			want:  "agent-chat-stable-transcript",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			since := service.CurrentCursor(workspaceID)
			cancel, rec, done := startEmbeddedWorkspaceStream(
				t, handler, workspaceID, formatCursor(since),
			)
			defer cancel()
			waitEmbeddedWorkspaceSubscriber(t, service, workspaceID)
			before := rec.Body.Len()
			service.notifier.Notify(workspaceID, WorkspaceStreamSignal{Scope: tc.scope})
			deadline := time.Now().Add(2 * time.Second)
			var body string
			for time.Now().Before(deadline) {
				body = rec.Body.String()
				if len(body) > before && strings.Contains(body[before:], tc.want) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if len(body) <= before {
				t.Fatalf("no incremental patch emitted for %s:\n%s", tc.scope, body)
			}
			incremental := body[before:]
			if !strings.Contains(incremental, tc.want) {
				t.Fatalf("missing %q in incremental patch:\n%s", tc.want, incremental)
			}
			assertNoPanelLiveNest(t, incremental, string(tc.scope))
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("stream did not stop")
			}
		})
	}
}

func TestStreamEmbeddedWorkspacePatchLiveTranscriptIsLiveOnly(t *testing.T) {
	service, handler, workspaceID := setupEmbeddedWorkspaceStreamFixture(t)
	since := service.CurrentCursor(workspaceID)
	cancel, rec, done := startEmbeddedWorkspaceStream(
		t, handler, workspaceID, formatCursor(since),
	)
	defer cancel()
	waitEmbeddedWorkspaceSubscriber(t, service, workspaceID)
	before := rec.Body.Len()
	service.notifier.NotifyLiveTranscript(workspaceID)
	body := waitStreamBodyContains(t, rec, "agent-chat-live-transcript")
	incremental := body[before:]
	if !strings.Contains(incremental, "agent-chat-live-transcript") {
		t.Fatalf("live patch missing live region:\n%s", incremental)
	}
	if strings.Contains(incremental, "doc-right-chat-panel") {
		t.Fatalf("live patch must not panel REPLACE:\n%s", incremental)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop")
	}
}

func TestStreamEmbeddedWorkspaceCatchupMayFullPanel(t *testing.T) {
	service, handler, workspaceID := setupEmbeddedWorkspaceStreamFixture(t)
	service.notifier.NotifyWorkspaceResource(workspaceID) // advance cursor so since=0 catchup fires
	cancel, rec, done := startEmbeddedWorkspaceStream(t, handler, workspaceID, "0")
	defer cancel()
	body := waitStreamBodyContains(t, rec, "doc-right-chat-panel")
	if !strings.Contains(body, "agent-chat-live-transcript") {
		t.Fatalf("catchup panel should include live region shell:\n%s", body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop")
	}
}

func formatCursor(cursor int64) string {
	return strconv.FormatInt(cursor, 10)
}
