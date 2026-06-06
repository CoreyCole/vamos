//go:build !integration || unit

package agentchat

import (
	"bytes"
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
	group := e.Group("/agent-chat")
	handler.RegisterRuntimeRoutes(group)

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

func TestPostCLIChatRunRejectsUnauthorizedAndUnknownProject(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	store := serverauth.NewMemoryMachineCredentialStore()
	handler := NewHandler(service, nil, HandlerOptions{
		MachineCredentials: store,
		ProjectsConfig:     servercfg.ProjectsConfig{Repos: map[string]servercfg.RepoConfig{}},
	})
	e := echo.New()
	group := e.Group("/agent-chat")
	handler.RegisterRuntimeRoutes(group)

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
