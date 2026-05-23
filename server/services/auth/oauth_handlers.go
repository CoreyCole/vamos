package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// HandleLogout logs out the user by clearing the session cookie
func (s *Service) HandleLogout(c echo.Context) error {
	// Get session ID from cookie
	cookie, err := c.Cookie(SessionCookieName)
	if err == nil && cookie.Value != "" {
		// Delete session from database
		_ = s.DeleteSession(c.Request().Context(), cookie.Value)
	}

	// Clear the session cookie
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to login page
	return c.Redirect(http.StatusFound, "/login")
}

// HandleLoginPage renders the login page
func (s *Service) HandleLoginPage(c echo.Context) error {
	args := LoginPageArgs{
		ErrorMessage: "",
		RedirectURL:  NormalizeRedirectURL(c.QueryParam("redirect")),
	}
	return Render(c, http.StatusOK, LoginPage(args))
}

// HandleGoogleLogin redirects to Google OAuth login
func (s *Service) HandleGoogleLogin(c echo.Context) error {
	// Generate state token for CSRF protection
	state, err := generateSecureToken(16)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to generate state")
	}

	// Store state in cookie for validation
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Store redirect URL in cookie if provided (for post-login redirect)
	redirectURL := NormalizeRedirectURL(c.QueryParam("redirect"))
	if redirectURL != "" {
		c.SetCookie(&http.Cookie{
			Name:     "oauth_redirect",
			Value:    redirectURL,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300, // 5 minutes (same as state)
		})
	}

	// Redirect to Google OAuth
	authURL := s.GetAuthURL(state)
	return c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// HandleOAuthCallback handles the OAuth callback from Google
func (s *Service) HandleOAuthCallback(c echo.Context) error {
	ctx := c.Request().Context()

	// Verify state token
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != c.QueryParam("state") {
		return s.renderUnauthorized(c, "", "Invalid state parameter")
	}

	// Clear state cookie
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Exchange code for token
	code := c.QueryParam("code")
	token, err := s.ExchangeCodeForToken(ctx, code)
	if err != nil {
		return s.renderUnauthorized(c, "", "Failed to exchange code")
	}

	// Get user info
	email, err := s.getUserEmail(ctx, token.AccessToken)
	if err != nil {
		_ = s.LogAuthAttempt(ctx, "unknown", false, "Failed to get user info")
		return s.renderUnauthorized(c, "", "Failed to get user info")
	}

	// Validate email domain
	if err := s.ValidateEmail(email); err != nil {
		_ = s.LogAuthAttempt(ctx, email, false, err.Error())
		return s.renderUnauthorized(c, email, err.Error())
	}

	// Create session
	session, err := s.CreateSession(ctx, email)
	if err != nil {
		_ = s.LogAuthAttempt(ctx, email, false, "Failed to create session")
		return s.renderUnauthorized(c, email, "Failed to create session")
	}

	// Log successful auth
	_ = s.LogAuthAttempt(ctx, email, true, "")

	// Set session cookie
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})

	// Get redirect URL from cookie (default to /thoughts/)
	finalRedirect := "/thoughts/"
	if redirectCookie, err := c.Cookie("oauth_redirect"); err == nil && redirectCookie.Value != "" {
		if redirectURL := NormalizeRedirectURL(redirectCookie.Value); redirectURL != "" {
			finalRedirect = redirectURL
		}
		// Clear the redirect cookie
		c.SetCookie(&http.Cookie{
			Name:     "oauth_redirect",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Redirect to original page or default
	return c.Redirect(http.StatusTemporaryRedirect, finalRedirect)
}

// getUserEmail fetches user email from Google
func (s *Service) getUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var userInfo struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", err
	}

	if !userInfo.VerifiedEmail {
		return "", fmt.Errorf("email %s is not verified", userInfo.Email)
	}

	return userInfo.Email, nil
}

// renderUnauthorized renders the unauthorized page
func (s *Service) renderUnauthorized(c echo.Context, email, errorMsg string) error {
	args := UnauthorizedPageArgs{
		Email:          email,
		ErrorMessage:   errorMsg,
		AllowedDomains: s.allowedDomains,
	}
	return Render(c, http.StatusUnauthorized, UnauthorizedPage(args))
}
