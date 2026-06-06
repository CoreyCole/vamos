package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAuthMiddlewareRedirectsChildWorkspaceDirectLinkThroughManagerSwitch(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/plan.md?context=chat&thread=abc", nil)
	req.Host = "feature.workspaces.test"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := AuthMiddleware(nil, AuthRedirectConfig{
		ManagerURL:      "https://main.workspaces.test",
		WorkspaceDomain: "workspaces.test",
		CurrentSlug:     "feature",
	})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	if err := h(c); err != nil {
		t.Fatalf("AuthMiddleware() error = %v", err)
	}
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}
	want := "https://main.workspaces.test/workspaces/switch/feature?redirect=%2Fthoughts%2Fplan.md%3Fcontext%3Dchat%26thread%3Dabc"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestAuthMiddlewareRedirectsChildWorkspaceDirectLinkThroughManagerSwitchBehindProxy(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	req.Host = "127.0.0.1:45923"
	req.Header.Set("X-Forwarded-Host", "feature.workspaces.test")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := AuthMiddleware(nil, AuthRedirectConfig{
		ManagerURL:      "https://main.workspaces.test",
		WorkspaceDomain: "workspaces.test",
		CurrentSlug:     "feature",
	})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	if err := h(c); err != nil {
		t.Fatalf("AuthMiddleware() error = %v", err)
	}
	want := "https://main.workspaces.test/workspaces/switch/feature?redirect=%2Fworkspaces"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestAuthMiddlewareRedirectsChildWorkspaceDirectLinkThroughManagerSwitchWhenDomainOmitted(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	req.Host = "feature.workspaces.test"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := AuthMiddleware(nil, AuthRedirectConfig{
		ManagerURL:  "https://main.workspaces.test",
		CurrentSlug: "feature",
	})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	if err := h(c); err != nil {
		t.Fatalf("AuthMiddleware() error = %v", err)
	}
	want := "https://main.workspaces.test/workspaces/switch/feature?redirect=%2Fworkspaces"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestAuthMiddlewareUsesLocalLoginOnManagerHost(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/thoughts/?context=chat", nil)
	req.Host = "main.workspaces.test"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := AuthMiddleware(nil, AuthRedirectConfig{
		ManagerURL:      "https://main.workspaces.test",
		WorkspaceDomain: "workspaces.test",
		CurrentSlug:     "main",
	})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	if err := h(c); err != nil {
		t.Fatalf("AuthMiddleware() error = %v", err)
	}
	want := "/login?redirect=%2Fthoughts%2F%3Fcontext%3Dchat"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}
