package generic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/labstack/echo/v4"
)

func TestRepositoryExampleManifestsRegister(t *testing.T) {
	repoExamplesRoot := filepath.Clean(filepath.Join("..", "..", "..", "..", "examples"))
	if _, err := os.Stat(filepath.Join(repoExamplesRoot, "streamlit", "AGENTS.md")); err != nil {
		t.Skipf("repo examples root unavailable: %v", err)
	}
	service, err := NewService(Options{ExamplesRoot: repoExamplesRoot, AppletRuntime: &fakeRuntime{}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/examples/streamlit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("streamlit")
	if err := service.HandlePage(c); err != nil {
		t.Fatalf("HandlePage(streamlit) error = %v", err)
	}
	if !strings.Contains(rec.Body.String(), "/examples/streamlit/app/") {
		t.Fatalf("streamlit page missing app iframe route:\n%s", rec.Body.String())
	}
	if err := service.RegisterRoutes(e, nil); err != nil {
		t.Fatalf("RegisterRoutes() error = %v", err)
	}
}

func TestHandlePageRendersDocumentWorkbenchApplet(t *testing.T) {
	root := writeExampleManifest(t)
	service, err := NewService(Options{ExamplesRoot: root, AppletRuntime: &fakeRuntime{state: appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusHealthy}}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/examples/wordle", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("wordle")

	if err := service.HandlePage(c); err != nil {
		t.Fatalf("HandlePage() error = %v", err)
	}
	html := rec.Body.String()
	for _, want := range []string{"doc-workbench-sidebar", "doc-workbench-center", "doc-workbench-right", "examples/wordle/AGENTS.md", "/examples/wordle/app/"} {
		if !strings.Contains(html, want) {
			t.Fatalf("page HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "wordle-files-app-region") {
		t.Fatalf("page still rendered legacy two-region applet shell:\n%s", html)
	}
}

func TestHandleAppProxiesScopedRouteThroughRuntime(t *testing.T) {
	root := writeExampleManifest(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer backend.Close()
	service, err := NewService(Options{ExamplesRoot: root, AppletRuntime: &fakeRuntime{target: backend.URL}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/examples/wordle/app/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("wordle")

	if err := service.HandleApp(c); err != nil {
		t.Fatalf("HandleApp() error = %v", err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "/events" {
		t.Fatalf("proxied path = %q, want /events", got)
	}
}

func TestHandleAliasProxiesRootAliasThroughRuntime(t *testing.T) {
	root := writeExampleManifest(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Method + " " + r.URL.Path))
	}))
	defer backend.Close()
	service, err := NewService(Options{ExamplesRoot: root, AppletRuntime: &fakeRuntime{target: backend.URL}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	e := echo.New()
	if err := service.applets.RegisterStartupAliases(e); err != nil {
		t.Fatalf("RegisterStartupAliases() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/guesses", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("alias status = %d body=%q", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "POST /guesses" {
		t.Fatalf("alias proxy = %q, want POST /guesses", got)
	}
}

func TestRegisterRoutesReturnsStartupAliasConflict(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
vamos_artifact: applet
applet:
  id: bad
  title: Bad Streamlit
  kind: streamlit
  source_dir: .
  start_command: [streamlit, run, app.py]
  root_aliases:
    - pattern: /static/*
---
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(Options{ExamplesRoot: root, AppletRuntime: &fakeRuntime{}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	err = service.RegisterRoutes(echo.New(), nil)
	if err == nil || !strings.Contains(err.Error(), "reserved Vamos prefix") {
		t.Fatalf("RegisterRoutes() error = %v, want reserved alias conflict", err)
	}
}

func writeExampleManifest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "wordle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
vamos_artifact: applet
applet:
  id: wordle
  title: Daily Wordle
  kind: datastar
  files_root: files
  app_dir: .
  start_command: [just, build]
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /guesses
      methods: [POST]
---
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type fakeRuntime struct {
	target string
	state  appletruntime.AppletProcessState
}

func (m *fakeRuntime) EnsureStarted(context.Context, appletruntime.RuntimeConfig) (appletruntime.AppletProcessState, error) {
	if m.state.Status == "" {
		m.state.Status = appletruntime.ProcessStatusHealthy
	}
	return m.state, nil
}

func (m *fakeRuntime) Start(ctx context.Context, cfg appletruntime.RuntimeConfig) (appletruntime.ProcessState, error) {
	return m.EnsureStarted(ctx, cfg)
}

func (m *fakeRuntime) Stop(context.Context, string) error { return nil }

func (m *fakeRuntime) Health(context.Context, string) (appletruntime.AppletProcessState, error) {
	if m.state.Status == "" {
		return appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusHealthy}, nil
	}
	return m.state, nil
}

func (m *fakeRuntime) ProxyTarget(string) (string, bool) {
	if m.target == "" {
		return "", false
	}
	return m.target, true
}

func (m *fakeRuntime) Touch(string, int) {}

func (m *fakeRuntime) SweepInactive(context.Context, time.Time) ([]appletruntime.AppletProcessState, error) {
	return nil, nil
}
