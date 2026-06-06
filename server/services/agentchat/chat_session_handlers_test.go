package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/db"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

func mustCreateHandlerChatSession(
	t *testing.T,
	service *Service,
	workspace db.Workspace,
) db.ChatSession {
	t.Helper()
	session, err := service.queries.CreateChatSession(
		context.Background(),
		db.CreateChatSessionParams{
			ID:                 "chat-session-1",
			WorkspaceID:        workspace.ID,
			CreatedByUserEmail: workspace.UserEmail,
			BranchID:           "branch-1",
			WorkflowAttempt:    0,
			TopologyKind:       string(chatsession.TopologyRoot),
		},
	)
	if err != nil {
		t.Fatalf("CreateChatSession() error = %v", err)
	}
	return session
}

func TestGetChatSessionSnapshotAuthorizesSharedWorkspace(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	_, err := chatsession.NewService(service.db, service.queries).AppendEvent(
		context.Background(),
		chatsession.AppendEventInput{
			SessionID:   session.ID,
			EventType:   chatsession.EventMessageCreated,
			PayloadJSON: []byte(`{"id":"m1","role":"user","content":"hello"}`),
		},
	)
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/chat-sessions/"+session.ID, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.GetChatSessionSnapshot(c); err != nil {
		t.Fatalf("GetChatSessionSnapshot() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response chatSessionSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if response.LastSeq != 1 || len(response.Projection.Messages) != 1 {
		t.Fatalf("snapshot = %+v, want one message at seq 1", response)
	}
}

func TestStreamChatSessionEventsReplaysAfterCursor(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	svc := chatsession.NewService(service.db, service.queries)
	for i := 1; i <= 3; i++ {
		if _, err := svc.AppendEvent(
			context.Background(),
			chatsession.AppendEventInput{
				SessionID:   session.ID,
				EventType:   chatsession.EventMessageCreated,
				PayloadJSON: []byte(`{"id":"m"}`),
			},
		); err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/chat-sessions/"+session.ID+"/events?after=1&tail=false",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.StreamChatSessionEvents(c); err != nil {
		t.Fatalf("StreamChatSessionEvents() error = %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "id: 1") || !strings.Contains(body, "id: 2") ||
		!strings.Contains(body, "id: 3") {
		t.Fatalf("SSE body = %q, want ids 2 and 3 only", body)
	}
}

func TestStreamChatSessionEventsUsesLastEventID(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	svc := chatsession.NewService(service.db, service.queries)
	for i := 1; i <= 2; i++ {
		if _, err := svc.AppendEvent(
			context.Background(),
			chatsession.AppendEventInput{
				SessionID: session.ID,
				EventType: chatsession.EventMessageCreated,
			},
		); err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/chat-sessions/"+session.ID+"/events?tail=false",
		nil,
	)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.StreamChatSessionEvents(c); err != nil {
		t.Fatalf("StreamChatSessionEvents() error = %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "id: 1") || !strings.Contains(body, "id: 2") {
		t.Fatalf("SSE body = %q, want id 2 only", body)
	}
}

func TestGetCLIChatSessionReturnsAuthorizedMachineSnapshot(t *testing.T) {
	service := newTestAgentChatService(t)
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		Name:              "cli",
		DefaultActorEmail: "agent@example.test",
	})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}
	handler := NewHandler(service, nil, HandlerOptions{MachineCredentials: store})
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	if _, err := service.chatSessions.AppendEvent(
		context.Background(),
		chatsession.AppendEventInput{
			SessionID:   session.ID,
			EventType:   chatsession.EventMessageCreated,
			PayloadJSON: []byte(`{"id":"m1","role":"user","content":"hello"}`),
		},
	); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/api/chat-sessions/"+session.ID, nil)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)

	if err := handler.GetCLIChatSession(c); err != nil {
		t.Fatalf("GetCLIChatSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response chatSessionSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if response.LastSeq != 1 || len(response.Projection.Messages) != 1 {
		t.Fatalf("snapshot = %+v, want one message at seq 1", response)
	}
}

func TestStreamCLIChatSessionEventsReplaysAfterCursorAndLastEventID(t *testing.T) {
	service := newTestAgentChatService(t)
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		Name:              "cli",
		DefaultActorEmail: "agent@example.test",
	})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}
	handler := NewHandler(service, nil, HandlerOptions{MachineCredentials: store})
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	for i := 1; i <= 3; i++ {
		if _, err := service.chatSessions.AppendEvent(
			context.Background(),
			chatsession.AppendEventInput{
				SessionID:   session.ID,
				EventType:   chatsession.EventMessageCreated,
				PayloadJSON: []byte(`{"id":"m"}`),
			},
		); err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/api/chat-sessions/"+session.ID+"/events?tail=false", nil)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	req.Header.Set("Last-Event-ID", "2")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)

	if err := handler.StreamCLIChatSessionEvents(c); err != nil {
		t.Fatalf("StreamCLIChatSessionEvents() error = %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "id: 1") || strings.Contains(body, "id: 2") || !strings.Contains(body, "id: 3") {
		t.Fatalf("SSE body = %q, want id 3 only", body)
	}
}

func TestCLIChatSessionEndpointsRejectMissingBearer(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{MachineCredentials: serverauth.NewMemoryMachineCredentialStore()})
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)

	req := httptest.NewRequest(http.MethodGet, "/agent-chat/api/chat-sessions/"+session.ID, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)

	err := handler.GetCLIChatSession(c)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("GetCLIChatSession error = %v, want 401", err)
	}
}

func TestPostChatSessionCommandStartsMessageSendRun(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	body := bytes.NewBufferString(
		`{"type":"message.send","idempotency_key":"idem-1","payload":{"prompt":"hello"}}`,
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/chat-sessions/"+session.ID+"/commands",
		body,
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.PostChatSessionCommand(c); err != nil {
		t.Fatalf("PostChatSessionCommand() error = %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if response["status"] != string(chatsession.CommandApplied) || strings.TrimSpace(response["run_id"]) == "" || response["session_id"] != session.ID {
		t.Fatalf("response = %+v", response)
	}
	eventCount, err := service.queries.TestSupportCountChatSessionEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 3 {
		t.Fatalf("event count = %d, want submitted+accepted+applied", eventCount)
	}
}
