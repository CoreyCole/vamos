package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func newPlaywrightAuthTestService(t *testing.T) *Service {
	t.Helper()
	database, err := dbsvc.NewService(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
	credentialsJSON := `{"web":{"client_id":"client","client_secret":"secret","redirect_uris":["http://localhost:4200/auth/callback"]}}`
	if err := os.WriteFile(credentialsPath, []byte(credentialsJSON), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	service, err := NewService(database.Queries, credentialsPath, nil, nil)
	if err != nil {
		t.Fatalf("NewService auth returned error: %v", err)
	}
	return service
}

func TestRegisterPlaywrightAuthRoutesDisabledByDefault(t *testing.T) {
	t.Parallel()

	e := echo.New()
	RegisterPlaywrightAuthRoutes(e, &Service{}, PlaywrightAuthConfig{})

	req := httptest.NewRequest(http.MethodGet, "/internal/playwright-auth", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled route, got %d", rec.Code)
	}
}

func TestHandlePlaywrightAuthRejectsNonLoopback(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{Enabled: true})

	req := httptest.NewRequest(http.MethodGet, "/internal/playwright-auth", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback host, got %d", rec.Code)
	}
}

func TestHandlePlaywrightAuthRejectsSpoofedLoopbackForwardedHost(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{Enabled: true})

	req := httptest.NewRequest(http.MethodGet, "/internal/playwright-auth", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Host", "localhost:4200")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for spoofed forwarded loopback host, got %d", rec.Code)
	}
}

func TestHandlePlaywrightAuthRequiresOptionalToken(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(
		e,
		service,
		PlaywrightAuthConfig{Enabled: true, Token: "secret"},
	)

	req := httptest.NewRequest(http.MethodGet, "/internal/playwright-auth", nil)
	req.Host = "localhost:4200"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing token, got %d", rec.Code)
	}
}

func TestHandlePlaywrightAuthAcceptsLoopbackToken(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(
		e,
		service,
		PlaywrightAuthConfig{Enabled: true, Token: "secret"},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/playwright-auth?token=secret&redirect=/workspaces",
		nil,
	)
	req.Host = "localhost:4200"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/workspaces" {
		t.Fatalf("expected /workspaces redirect, got %q", location)
	}
}

func TestHandlePlaywrightAuthRejectsExternalRedirect(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{Enabled: true})

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/playwright-auth?redirect=https://example.com/agent-chat",
		nil,
	)
	req.Host = "localhost:4200"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for external redirect, got %d", rec.Code)
	}
}

func TestHandlePlaywrightAuthCreatesSessionAndCookie(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{Enabled: true})

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/playwright-auth?redirect=/agent-chat?thread=thread-1",
		nil,
	)
	req.Host = "localhost:4200"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rec.Code)
	}
	if location := rec.Header().
		Get("Location"); location != defaultThoughtsRedirectPath {
		t.Fatalf("expected local redirect location, got %q", location)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Fatalf("expected %s cookie, got %s", SessionCookieName, cookie.Name)
	}
	if !cookie.Secure {
		t.Fatalf("expected secure cookie when forwarded proto is https")
	}
	session, err := service.GetSession(req.Context(), cookie.Value)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.UserEmail != "playwright@localhost" {
		t.Fatalf("expected playwright email, got %q", session.UserEmail)
	}
}

func TestHandlePlaywrightAuthCreatesSecureCookieForPublicWorkspaceDomain(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{
		Enabled:         true,
		Token:           "public-secret",
		PublicHostToken: true,
		WorkspaceDomain: "cn-agents.test",
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/playwright-auth?token=public-secret&redirect=/workspaces",
		nil,
	)
	req.Host = "main.cn-agents.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/workspaces" {
		t.Fatalf("expected /workspaces redirect, got %q", location)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if !cookies[0].Secure {
		t.Fatalf("expected secure cookie for forwarded https public host")
	}
}

func TestHandlePlaywrightAuthRejectsPublicTokenOutsideWorkspaceDomain(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{
		Enabled:         true,
		Token:           "public-secret",
		PublicHostToken: true,
		WorkspaceDomain: "cn-agents.test",
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/playwright-auth?token=public-secret&redirect=/workspaces",
		nil,
	)
	req.Host = "evil.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf(
			"expected 403 for public host outside workspace domain, got %d",
			rec.Code,
		)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("expected no cookies for rejected public host, got %d", len(cookies))
	}
}

func TestHandlePlaywrightAuthPlainLoopbackCookieIsNotSecure(t *testing.T) {
	t.Parallel()

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	RegisterPlaywrightAuthRoutes(e, service, PlaywrightAuthConfig{Enabled: true})

	req := httptest.NewRequest(http.MethodGet, "/internal/playwright-auth", nil)
	req.Host = "127.0.0.1:4200"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if cookies[0].Secure {
		t.Fatalf("expected non-secure cookie for plain loopback HTTP")
	}
}

func TestNormalizePlaywrightRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", want: defaultThoughtsRedirectPath},
		{name: "path", raw: "/agent-chat", want: defaultThoughtsRedirectPath},
		{
			name: "query",
			raw:  "/agent-chat?thread=thread-1",
			want: defaultThoughtsRedirectPath,
		},
		{name: "absolute", raw: "https://example.com/agent-chat", wantErr: true},
		{name: "scheme relative", raw: "//example.com/agent-chat", wantErr: true},
		{name: "relative", raw: "agent-chat", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizePlaywrightRedirect(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePlaywrightRedirect returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
