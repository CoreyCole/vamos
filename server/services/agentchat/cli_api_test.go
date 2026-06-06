//go:build !integration || unit

package agentchat

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
	servercfg "github.com/CoreyCole/vamos/server"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

func TestPostCLIChatRunStartsRunWithProjectCheckoutCWD(t *testing.T) {
	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	service.defaultCwd = filepath.Join(t.TempDir(), "wrong-default")
	service.projectRoot = service.defaultCwd

	checkoutRoot := t.TempDir()
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		Name:              "cli",
		DefaultActorEmail: "agent@example.test",
	})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}

	handler := NewHandler(service, nil, HandlerOptions{
		MachineCredentials: store,
		ProjectsConfig: servercfg.ProjectsConfig{
			DefaultCheckout: "stage",
			Repos: map[string]servercfg.RepoConfig{
				"github.com/coreycole/vamos": {
					DefaultCheckout:  "stage",
					BaselineCheckout: "main",
					Checkouts: map[string]servercfg.CheckoutConfig{
						"stage": {RootPath: checkoutRoot, Role: servercfg.CheckoutRoleStage},
						"main":  {RootPath: t.TempDir(), Role: servercfg.CheckoutRoleMain, MustBeClean: true},
					},
				},
			},
		},
		PublicBaseURL: "https://vamos.example.test",
	})
	e := echo.New()
	group := e.Group("/agent-chat/api")
	handler.RegisterMachineAPIRoutes(group)

	body := bytes.NewBufferString(`{"project_id":"github.com/coreycole/vamos","prompt":"hello from cli"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/runs", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response ChatAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "started" {
		t.Fatalf("type = %q, want started", response.Type)
	}
	ref := response.Ref
	if ref.ProjectID != "github.com/coreycole/vamos" || ref.WorkspaceID == "" || ref.ThreadID == "" || ref.RunID == "" || ref.ChatSessionID == "" {
		t.Fatalf("incomplete ref: %+v", ref)
	}
	if ref.CWD != checkoutRoot {
		t.Fatalf("ref cwd = %q, want %q", ref.CWD, checkoutRoot)
	}
	if !strings.HasPrefix(ref.WebURL, "https://vamos.example.test/thoughts/?") || !strings.Contains(ref.WebURL, "thread=") || !strings.Contains(ref.WebURL, "run=") {
		t.Fatalf("web_url = %q", ref.WebURL)
	}

	thread, err := service.queries.GetAgentThread(t.Context(), ref.ThreadID)
	if err != nil {
		t.Fatalf("GetAgentThread: %v", err)
	}
	if thread.Cwd != checkoutRoot {
		t.Fatalf("thread cwd = %q, want project checkout %q", thread.Cwd, checkoutRoot)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf("temporal input = %T, want conversation.RunInput", fakeTemporal.lastInput)
	}
	if input.Cwd != checkoutRoot {
		t.Fatalf("temporal cwd = %q, want %q", input.Cwd, checkoutRoot)
	}
	if input.ChatSessionID != ref.ChatSessionID || input.RunID != ref.RunID || input.ThreadID != ref.ThreadID {
		t.Fatalf("temporal refs = %+v, response = %+v", input, ref)
	}

	events, err := service.queries.ListChatSessionEventsAfter(t.Context(), db.ListChatSessionEventsAfterParams{
		SessionID: ref.ChatSessionID,
		AfterSeq:  0,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter: %v", err)
	}
	var sawMessage, sawRunStarted bool
	for _, event := range events {
		if event.EventType == string(chatsession.EventMessageCompleted) && event.RunID.Valid && event.RunID.String == ref.RunID {
			sawMessage = true
		}
		if event.EventType == string(chatsession.EventRunStarted) && event.RunID.Valid && event.RunID.String == ref.RunID {
			sawRunStarted = true
		}
	}
	if !sawMessage || !sawRunStarted {
		t.Fatalf("events missing message/run.started: sawMessage=%v sawRunStarted=%v events=%+v", sawMessage, sawRunStarted, events)
	}

	workspace, err := service.queries.GetWorkspace(t.Context(), ref.WorkspaceID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if !workspace.CurrentSessionID.Valid || workspace.CurrentSessionID.String != ref.ChatSessionID {
		t.Fatalf("workspace current session = %v, want %s", workspace.CurrentSessionID, ref.ChatSessionID)
	}
}

func TestPostCLIChatSteerStartsFollowUpRun(t *testing.T) {
	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	checkoutRoot := t.TempDir()
	store, secret := newCLITestMachineStore(t)
	handler := newCLITestHandler(t, service, store, checkoutRoot)
	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))

	start := postCLIRun(t, e, secret, `{"project_id":"github.com/coreycole/vamos","prompt":"initial"}`)
	if err := service.queries.CompleteAgentRun(t.Context(), db.CompleteAgentRunParams{ID: start.Ref.RunID, ResultHeadEntryID: sql.NullString{String: "entry-1", Valid: true}}); err != nil {
		t.Fatalf("CompleteAgentRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/steer", strings.NewReader(`{"thread_id":"`+start.Ref.ThreadID+`","prompt":"follow up"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+secret.Credential.ID+"."+secret.Secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response ChatAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "steer_accepted" || !response.InfluencesLatest {
		t.Fatalf("response = %+v", response)
	}
	if response.Ref.ThreadID != start.Ref.ThreadID || response.Ref.RunID == "" || response.Ref.RunID == start.Ref.RunID || response.Ref.ChatSessionID == "" {
		t.Fatalf("ref = %+v start = %+v", response.Ref, start.Ref)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok || input.ThreadID != start.Ref.ThreadID || input.RunID != response.Ref.RunID {
		t.Fatalf("temporal input = %+v (%T), response = %+v", fakeTemporal.lastInput, fakeTemporal.lastInput, response.Ref)
	}
}

func TestPostCLIChatSteerRejectsActiveRunWithRefs(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	checkoutRoot := t.TempDir()
	store, secret := newCLITestMachineStore(t)
	handler := newCLITestHandler(t, service, store, checkoutRoot)
	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))

	start := postCLIRun(t, e, secret, `{"project_id":"github.com/coreycole/vamos","prompt":"initial"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/steer", strings.NewReader(`{"thread_id":"`+start.Ref.ThreadID+`","prompt":"follow up"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+secret.Credential.ID+"."+secret.Secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response ChatAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "steer_rejected" || response.Reason != "run_in_progress" || response.Ref.RunID != start.Ref.RunID || response.LatestThreadID != start.Ref.ThreadID {
		t.Fatalf("response = %+v, start = %+v", response, start.Ref)
	}
}

func newCLITestMachineStore(t *testing.T) (*serverauth.MemoryMachineCredentialStore, serverauth.CreatedMachineCredential) {
	t.Helper()
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		Name:              "cli",
		DefaultActorEmail: "agent@example.test",
	})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}
	return store, created
}

func newCLITestHandler(t *testing.T, service *Service, store serverauth.MachineCredentialStore, checkoutRoot string) *Handler {
	t.Helper()
	return NewHandler(service, nil, HandlerOptions{
		MachineCredentials: store,
		ProjectsConfig: servercfg.ProjectsConfig{
			DefaultCheckout: "stage",
			Repos: map[string]servercfg.RepoConfig{
				"github.com/coreycole/vamos": {
					DefaultCheckout:  "stage",
					BaselineCheckout: "main",
					Checkouts: map[string]servercfg.CheckoutConfig{
						"stage": {RootPath: checkoutRoot, Role: servercfg.CheckoutRoleStage},
						"main":  {RootPath: t.TempDir(), Role: servercfg.CheckoutRoleMain, MustBeClean: true},
					},
				},
			},
		},
		PublicBaseURL: "https://vamos.example.test",
	})
}

func postCLIRun(t *testing.T, e *echo.Echo, secret serverauth.CreatedMachineCredential, body string) ChatAPIResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/runs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+secret.Credential.ID+"."+secret.Secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response ChatAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return response
}

func TestPostCLIChatRunRejectsDisallowedCheckoutSlug(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	checkoutRoot := t.TempDir()
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		Name:              "cli",
		DefaultActorEmail: "agent@example.test",
		AllowedSlugs:      []string{"other"},
	})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}
	handler := newCLITestHandler(t, service, store, checkoutRoot)
	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))

	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/runs", strings.NewReader(`{"project_id":"github.com/coreycole/vamos","prompt":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPostCLIChatRunRejectsUnauthorizedAndUnknownProject(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	store := serverauth.NewMemoryMachineCredentialStore()
	handler := NewHandler(service, nil, HandlerOptions{
		MachineCredentials: store,
		ProjectsConfig:     servercfg.ProjectsConfig{Repos: map[string]servercfg.RepoConfig{}},
	})
	e := echo.New()
	group := e.Group("/agent-chat/api")
	handler.RegisterMachineAPIRoutes(group)

	req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/runs", strings.NewReader(`{"project_id":"missing","prompt":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status = %d, want 401", rec.Code)
	}

	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{Name: "cli", DefaultActorEmail: "agent@example.test"})
	if err != nil {
		t.Fatalf("Create machine credential: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/agent-chat/api/runs", strings.NewReader(`{"project_id":"missing","prompt":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown project status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStartCLIChatRunReturnsRelativeURLWhenPublicBaseBlank(t *testing.T) {
	got := absoluteChatURL("", "thread id", "run id")
	if !strings.HasPrefix(got, "/thoughts/?") || !strings.Contains(got, "thread=thread+id") || !strings.Contains(got, "run=run+id") {
		t.Fatalf("absoluteChatURL blank base = %q", got)
	}
}
