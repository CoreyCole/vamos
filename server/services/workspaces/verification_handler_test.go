package workspaces

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestVerificationHandlerRejectsMissingOrBadToken(t *testing.T) {
	t.Parallel()

	handler := newDiagnosticsHandlerForTest(t)
	for _, token := range []string{"", "bad"} {
		rec, err := runWorkspaceDiagnosticsRequest(handler, token, "demo", "")
		if err == nil {
			t.Fatalf("token %q error = nil, want unauthorized", token)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf(
				"recorder status = %d before Echo error handling, want default OK",
				rec.Code,
			)
		}
	}
}

func TestVerificationHandlerDiagnostics(t *testing.T) {
	t.Parallel()

	handler := newDiagnosticsHandlerForTest(t)
	rec, err := runWorkspaceDiagnosticsRequest(handler, "secret", "demo", "")
	if err != nil {
		t.Fatalf("HandleWorkspaceDiagnostics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var diagnostics WorkspaceDiagnostics
	if err := json.Unmarshal(rec.Body.Bytes(), &diagnostics); err != nil {
		t.Fatalf("Unmarshal diagnostics: %v", err)
	}
	if diagnostics.Workspace.Slug != "demo" ||
		!strings.Contains(
			diagnostics.MetadataPath,
			filepath.Join(".vamos", "run", "workspace.env"),
		) {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if diagnostics.LogTail != "log line 2" {
		t.Fatalf("log tail = %q, want log line 2", diagnostics.LogTail)
	}
}

func TestVerificationHandlerVerifyRunRoutes(t *testing.T) {
	t.Parallel()

	handler := newRunHandlerForTest(t)
	rec, err := runVerifyRequest(handler, "", `{"slug":"demo"}`)
	if err == nil {
		t.Fatal("missing token error = nil, want unauthorized")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"recorder status = %d before Echo error handling, want default OK",
			rec.Code,
		)
	}

	rec, err = runVerifyRequest(handler, "secret", `{"slug":"demo","start":true}`)
	if err != nil {
		t.Fatalf("HandleVerifyWorkspace: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	var started VerifyWorkspaceRun
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatalf("Unmarshal start response: %v", err)
	}
	if started.ID == "" {
		t.Fatal("run id is empty")
	}

	var got VerifyWorkspaceRun
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got = getVerifyRunForTest(t, handler, started.ID)
		if got.Status == VerifyRunPassed || got.Status == VerifyRunFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got.ID != started.ID {
		t.Fatalf("get run id = %q, want %q", got.ID, started.ID)
	}
}

func TestVerificationHandlerEvents(t *testing.T) {
	t.Parallel()

	handler := newRunHandlerForTest(t)
	store := handler.verifier.Runs.(*MemoryVerifyRunStore)
	run, err := store.Create(context.Background(), VerifyWorkspaceRequest{Slug: "demo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	run.Status = VerifyRunPassed
	if err := store.Update(context.Background(), run); err != nil {
		t.Fatalf("Update: %v", err)
	}
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/workspaces/verify/"+run.ID+"/events",
		nil,
	)
	req.Header.Set("X-Vamos-Workspace-Restart-Token", "secret")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("run_id")
	ctx.SetParamValues(run.ID)
	if err := handler.HandleStreamVerifyWorkspaceRun(ctx); err != nil {
		t.Fatalf("HandleStreamVerifyWorkspaceRun: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "data:") {
		t.Fatalf("events body = %q, want data line", rec.Body.String())
	}
}

func TestVerificationHandlerLogs(t *testing.T) {
	t.Parallel()

	handler := newDiagnosticsHandlerForTest(t)
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/workspaces/demo/logs?tail=1",
		nil,
	)
	req.Header.Set("X-Vamos-Workspace-Restart-Token", "secret")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("slug")
	ctx.SetParamValues("demo")
	if err := handler.HandleWorkspaceLogs(ctx); err != nil {
		t.Fatalf("HandleWorkspaceLogs: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != "log line 2" {
		t.Fatalf("body = %q, want log line 2", rec.Body.String())
	}
}

func newRunHandlerForTest(t *testing.T) *Handler {
	t.Helper()
	manager := newVerificationFakeManager(t, "demo")
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	handler.verifier = NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		&verificationFakeProber{
			pidAlive:   true,
			portOpen:   true,
			statusCode: http.StatusOK,
		},
	)
	return handler
}

func runVerifyRequest(
	handler *Handler,
	token, body string,
) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/workspaces/verify",
		bytes.NewBufferString(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if token != "" {
		req.Header.Set("X-Vamos-Workspace-Restart-Token", token)
	}
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	return rec, handler.HandleVerifyWorkspace(ctx)
}

func getVerifyRunForTest(t *testing.T, handler *Handler, id string) VerifyWorkspaceRun {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/internal/workspaces/verify/"+id, nil)
	req.Header.Set("X-Vamos-Workspace-Restart-Token", "secret")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("run_id")
	ctx.SetParamValues(id)
	if err := handler.HandleGetVerifyWorkspaceRun(ctx); err != nil {
		t.Fatalf("HandleGetVerifyWorkspaceRun: %v", err)
	}
	var run VerifyWorkspaceRun
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("Unmarshal get response: %v", err)
	}
	return run
}

func newDiagnosticsHandlerForTest(t *testing.T) *Handler {
	t.Helper()
	checkout := t.TempDir()
	stateDir := t.TempDir()
	logPath := filepath.Join(stateDir, "agents-server.log")
	writeTestFile(t, logPath, "log line 1\nlog line 2\n")
	if err := WriteMetadata(WorkspaceMetadataPath(checkout), WorkspaceMetadata{
		Slug:         "demo",
		CheckoutPath: checkout,
		ManagerURL:   "https://main.cn-agents.test",
		PID:          111,
		Port:         4222,
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	manager := &diagnosticsFakeManager{workspaces: map[string]Workspace{
		"demo": {
			Slug:         "demo",
			CheckoutPath: checkout,
			URL:          "https://demo.cn-agents.test/",
			PID:          111,
			Port:         4222,
			StateDir:     stateDir,
			LogPath:      logPath,
		},
	}}
	handler := NewHandler(
		manager,
		"https://main.cn-agents.test",
		"main",
		WithRestartAPI("secret", "/repo/cn-agents"),
	)
	handler.verifier = NewVerifier(
		manager,
		":4200",
		nil,
		NewFileLogTailer(),
		diagnosticsFakeProber{pidAlive: true, portOpen: true},
	)
	return handler
}

func runWorkspaceDiagnosticsRequest(
	handler *Handler,
	token, slug, query string,
) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	if query == "" {
		query = "tail=1"
	}
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/workspaces/"+slug+"/diagnostics?"+query,
		nil,
	)
	if token != "" {
		req.Header.Set("X-Vamos-Workspace-Restart-Token", token)
	}
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("slug")
	ctx.SetParamValues(slug)
	return rec, handler.HandleWorkspaceDiagnostics(ctx)
}
