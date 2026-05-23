package auth

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

type PlaywrightAuthConfig struct {
	Enabled         bool
	Email           string
	Token           string
	PublicHostToken bool
	WorkspaceDomain string
}

func RegisterPlaywrightAuthRoutes(e *echo.Echo, svc *Service, cfg PlaywrightAuthConfig) {
	if !cfg.Enabled {
		return
	}
	e.GET("/internal/playwright-auth", func(c echo.Context) error {
		return svc.HandlePlaywrightAuth(c, cfg)
	})
}

func (s *Service) HandlePlaywrightAuth(c echo.Context, cfg PlaywrightAuthConfig) error {
	if !cfg.Enabled {
		return echo.NewHTTPError(http.StatusNotFound, "playwright auth disabled")
	}
	if !isAllowedPlaywrightBootstrap(c.Request(), cfg, c.QueryParam("token")) {
		return echo.NewHTTPError(
			http.StatusForbidden,
			"playwright auth requires loopback or verifier token on workspace domain",
		)
	}
	redirect, err := normalizePlaywrightRedirect(c.QueryParam("redirect"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	email := strings.TrimSpace(cfg.Email)
	if email == "" {
		email = "playwright@localhost"
	}
	session, err := s.CreateSession(c.Request().Context(), email)
	if err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			"failed to create playwright session",
		)
	}
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecureForRequest(c.Request()),
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})
	return c.Redirect(http.StatusTemporaryRedirect, redirect)
}

func isAllowedPlaywrightBootstrap(
	r *http.Request,
	cfg PlaywrightAuthConfig,
	token string,
) bool {
	if isLoopbackRequest(r) {
		if strings.TrimSpace(cfg.Token) == "" {
			return true
		}
		return constantTimeEqual(token, cfg.Token)
	}
	if !cfg.PublicHostToken || strings.TrimSpace(cfg.Token) == "" || token == "" {
		return false
	}
	if !constantTimeEqual(token, cfg.Token) {
		return false
	}
	return isWorkspaceDomainHost(requestHost(r), cfg.WorkspaceDomain)
}

func constantTimeEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func isLoopbackRequest(r *http.Request) bool {
	hostOnly := hostWithoutPort(r.Host)
	if strings.EqualFold(hostOnly, "localhost") {
		return true
	}
	ip := net.ParseIP(hostOnly)
	return ip != nil && ip.IsLoopback()
}

func requestHost(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		return forwarded
	}
	return r.Host
}

func hostWithoutPort(host string) string {
	host = strings.TrimSpace(host)
	hostOnly, _, err := net.SplitHostPort(host)
	if err == nil {
		return hostOnly
	}
	return host
}

func isWorkspaceDomainHost(host, domain string) bool {
	host = strings.ToLower(strings.TrimSuffix(hostWithoutPort(host), "."))
	domain = strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" || host == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieSecureForRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func normalizePlaywrightRedirect(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultThoughtsRedirectPath, nil
	}
	if strings.HasPrefix(raw, "//") {
		return "", errors.New("redirect must be a local path")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("invalid redirect")
	}
	if parsed.IsAbs() || parsed.Host != "" {
		return "", errors.New("redirect must be a local path")
	}
	if !strings.HasPrefix(parsed.Path, "/") {
		return "", errors.New("redirect must start with /")
	}
	if parsed.Path == "/agent-chat" || strings.HasPrefix(parsed.Path, "/agent-chat/") {
		return defaultThoughtsRedirectPath, nil
	}
	return parsed.RequestURI(), nil
}
