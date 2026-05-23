package middleware

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
	"github.com/CoreyCole/vamos/server/services/auth"
)

// AuthMiddleware validates user sessions and redirects unauthenticated users to login
func AuthMiddleware(authService *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get session cookie
			cookie, err := c.Cookie(auth.SessionCookieName)
			if err != nil {
				// No session cookie, redirect to login with original URL
				redirectURL := "/login?redirect=" + url.QueryEscape(c.Request().URL.String())
				return c.Redirect(http.StatusTemporaryRedirect, redirectURL)
			}

			// Validate session
			session, err := authService.GetSession(c.Request().Context(), cookie.Value)
			if err != nil {
				// Invalid or expired session, redirect to login with original URL
				redirectURL := "/login?redirect=" + url.QueryEscape(c.Request().URL.String())
				return c.Redirect(http.StatusTemporaryRedirect, redirectURL)
			}

			// Store session in context for handlers to use
			c.Set("session", session)
			c.Set("user_email", session.UserEmail)

			return next(c)
		}
	}
}
