package workspaces

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNewHandlerAllowsNilManagerServicePointer(t *testing.T) {
	var manager *ManagerService
	handler := NewHandler(manager, "https://main.cn-agents.test", "child")
	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestProxyByHostFailedWorkspaceRendersRecoveryWithRetryAndBuildWarning(t *testing.T) {
	e := echo.New()
	e.Use((&ManagerService{
		runtime:   RuntimeConfig{ManagerURL: "https://main.cn-agents.test"},
		discovery: DiscoveryConfig{Domain: "cn-agents.test"},
		workspaces: map[string]Workspace{
			"foo": {
				Slug:        "foo",
				DisplayName: "Foo",
				Host:        "foo.cn-agents.test",
				Status:      StatusFailed,
				Phase:       PhaseStartingWeb,
				Error:       "web readiness failed",
				BuildStatus: BuildStatus{Error: "templ failed", LogPath: "/tmp/build.log"},
			},
		},
	}).HostDispatchMiddleware("main.cn-agents.test"))
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(http.MethodGet, "/agent-chat", nil)
	req.Host = "foo.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Workspace recovery",
		"Foo",
		"status: failed",
		"phase: starting_web",
		"web readiness failed",
		"Last build failed: templ failed",
		`name="action" value="retry"`,
		"Open Manager",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("recovery page missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"stack", "lineage", "Delete"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("recovery page contains %q: %s", forbidden, body)
		}
	}
}

func TestProxyByHostUnsafeUnavailableRequestIsNotProxied(t *testing.T) {
	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			"http://127.0.0.1:1",
			StatusStopped,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(
		http.MethodPost,
		"/forms/comments",
		strings.NewReader("hello"),
	)
	req.Host = "foo.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() == "manager" {
		t.Fatalf("request fell through to manager route: %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "The original request was not forwarded") {
		t.Fatalf("body missing unsafe warning: %q", rec.Body.String())
	}
}

func TestProxyByHostUnknownWorkspaceRenders404Recovery(t *testing.T) {
	e := echo.New()
	e.Use((&ManagerService{
		runtime:    RuntimeConfig{ManagerURL: "https://main.cn-agents.test"},
		discovery:  DiscoveryConfig{Domain: "cn-agents.test"},
		workspaces: map[string]Workspace{},
	}).HostDispatchMiddleware("main.cn-agents.test"))
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "missing.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Unknown workspace") ||
		!strings.Contains(rec.Body.String(), "https://main.cn-agents.test/workspaces") {
		t.Fatalf("unknown recovery body: %q", rec.Body.String())
	}
}

func TestWorkspaceHostActionRouteRequiresAuthAndStartsWorkspace(t *testing.T) {
	manager := &fakeHostActionManager{workspaces: map[string]Workspace{
		"foo": {Slug: "foo", Status: StatusStopped},
	}}
	handler := NewHandler(manager, "https://main.cn-agents.test", "main")
	e := echo.New()
	authMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("X-Test-Auth") == "yes" {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusUnauthorized)
		}
	}
	handler.RegisterRoutes(e, authMiddleware)

	form := url.Values{"slug": {"foo"}, "action": {"start"}, "return_to": {"/agent-chat"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/workspaces/host-action",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", rec.Code)
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/workspaces/host-action",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("X-Test-Auth", "yes")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/agent-chat" {
		t.Fatalf(
			"auth status=%d location=%q body=%q",
			rec.Code,
			rec.Header().Get("Location"),
			rec.Body.String(),
		)
	}
	if manager.started != "foo" {
		t.Fatalf("started = %q, want foo", manager.started)
	}
}

type fakeHostActionManager struct {
	workspaces map[string]Workspace
	started    string
	stopped    string
}

func (m *fakeHostActionManager) Refresh(context.Context) error { return nil }
func (m *fakeHostActionManager) List() []Workspace {
	out := make([]Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		out = append(out, ws)
	}
	return out
}

func (m *fakeHostActionManager) Lookup(slug string) (Workspace, bool) {
	ws, ok := m.workspaces[slug]
	return ws, ok
}

func (m *fakeHostActionManager) LookupHost(host string) (Workspace, bool) {
	return Workspace{}, false
}

func (m *fakeHostActionManager) Start(
	ctx context.Context,
	slug string,
) (Workspace, error) {
	m.started = slug
	ws := m.workspaces[slug]
	ws.Status = StatusRunning
	m.workspaces[slug] = ws
	return ws, nil
}

func (m *fakeHostActionManager) Stop(
	ctx context.Context,
	slug string,
) (Workspace, error) {
	m.stopped = slug
	ws := m.workspaces[slug]
	ws.Status = StatusStopped
	m.workspaces[slug] = ws
	return ws, nil
}

func (m *fakeHostActionManager) Restart(
	ctx context.Context,
	slug string,
) (Workspace, error) {
	return m.Start(ctx, slug)
}
