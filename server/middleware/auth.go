package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/CoreyCole/vamos/server/services/auth"
	"github.com/labstack/echo/v4"
)

type AuthRedirectConfig struct {
	ManagerURL      string
	WorkspaceDomain string
	CurrentSlug     string
}

// AuthMiddleware validates user sessions and redirects unauthenticated users to login.
func AuthMiddleware(authService *auth.Service, configs ...AuthRedirectConfig) echo.MiddlewareFunc {
	cfg := AuthRedirectConfig{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get session cookie
			cookie, err := c.Cookie(auth.SessionCookieName)
			if err != nil {
				// No session cookie, redirect to login with original URL
				return redirectUnauthenticated(c, cfg)
			}

			// Validate session
			session, err := authService.GetSession(c.Request().Context(), cookie.Value)
			if err != nil {
				// Invalid or expired session, redirect to login with original URL
				return redirectUnauthenticated(c, cfg)
			}

			// Store session in context for handlers to use
			c.Set("session", session)
			c.Set("user_email", session.UserEmail)

			return next(c)
		}
	}
}

func redirectUnauthenticated(c echo.Context, cfg AuthRedirectConfig) error {
	if target := managerSwitchRedirect(c, cfg); target != "" {
		return c.Redirect(http.StatusTemporaryRedirect, target)
	}
	redirectURL := "/login?redirect=" + url.QueryEscape(c.Request().URL.String())
	return c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

func managerSwitchRedirect(c echo.Context, cfg AuthRedirectConfig) string {
	managerURL := strings.TrimRight(strings.TrimSpace(cfg.ManagerURL), "/")
	slug := strings.TrimSpace(cfg.CurrentSlug)
	domain := strings.Trim(strings.TrimSpace(cfg.WorkspaceDomain), ".")
	if managerURL == "" || slug == "" || slug == "main" || domain == "" {
		return ""
	}
	host := requestHost(c.Request().Host)
	if !isChildWorkspaceHost(host, slug, domain) {
		return ""
	}
	redirectPath := c.Request().URL.RequestURI()
	if redirectPath == "" || !strings.HasPrefix(redirectPath, "/") || strings.HasPrefix(redirectPath, "//") {
		redirectPath = "/"
	}
	return managerURL + "/workspaces/switch/" + url.PathEscape(slug) + "?redirect=" + url.QueryEscape(redirectPath)
}

func requestHost(hostport string) string {
	host := strings.TrimSpace(hostport)
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			return strings.Trim(host[:end+1], "[]")
		}
	}
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}

func isChildWorkspaceHost(host, slug, domain string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	slug = strings.ToLower(strings.TrimSpace(slug))
	domain = strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
	return host == slug+"."+domain
}
