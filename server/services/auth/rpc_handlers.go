package auth

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	authv1 "github.com/CoreyCole/vamos/pkg/proto/auth/v1"
)

// Logout terminates the user's session
func (s *Service) Logout(
	ctx context.Context,
	req *connect.Request[authv1.LogoutRequest],
) (*connect.Response[authv1.LogoutResponse], error) {
	// Get session cookie from request headers
	cookies := req.Header().Get("Cookie")

	// Parse cookies to find session
	cookie := parseCookieValue(cookies, SessionCookieName)
	if cookie != "" {
		// Delete session from database
		_ = s.DeleteSession(ctx, cookie)
	}

	// Return HTML redirect for datastar
	html := `<meta http-equiv="refresh" content="0; url=/login">`

	return connect.NewResponse(&authv1.LogoutResponse{
		Html:    html,
		Success: true,
	}), nil
}

// GetCurrentUser returns the currently authenticated user's information
func (s *Service) GetCurrentUser(
	ctx context.Context,
	req *connect.Request[authv1.GetCurrentUserRequest],
) (*connect.Response[authv1.GetCurrentUserResponse], error) {
	// Get session cookie from request headers
	cookies := req.Header().Get("Cookie")
	sessionID := parseCookieValue(cookies, SessionCookieName)

	if sessionID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Get session from database
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return connect.NewResponse(&authv1.GetCurrentUserResponse{
		Email:     session.UserEmail,
		ExpiresAt: session.ExpiresAt.Format(http.TimeFormat),
	}), nil
}

// ValidateSession checks if the current session is valid
func (s *Service) ValidateSession(
	ctx context.Context,
	req *connect.Request[authv1.ValidateSessionRequest],
) (*connect.Response[authv1.ValidateSessionResponse], error) {
	// Get session cookie from request headers
	cookies := req.Header().Get("Cookie")
	sessionID := parseCookieValue(cookies, SessionCookieName)

	if sessionID == "" {
		return connect.NewResponse(&authv1.ValidateSessionResponse{
			Valid: false,
			Error: "No session cookie found",
		}), nil
	}

	// Get session from database
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return connect.NewResponse(&authv1.ValidateSessionResponse{
			Valid: false,
			Error: err.Error(),
		}), nil
	}

	return connect.NewResponse(&authv1.ValidateSessionResponse{
		Valid: true,
		Email: session.UserEmail,
	}), nil
}

// parseCookieValue extracts a cookie value from a Cookie header string
func parseCookieValue(cookieHeader, name string) string {
	if cookieHeader == "" {
		return ""
	}

	// Simple cookie parsing (Echo will handle this better in middleware)
	cookies := splitCookies(cookieHeader)
	for _, cookie := range cookies {
		parts := splitCookie(cookie)
		if len(parts) == 2 && parts[0] == name {
			return parts[1]
		}
	}
	return ""
}

func splitCookies(s string) []string {
	var result []string
	var current string
	for i := 0; i < len(s); i++ {
		if s[i] == ';' && i+1 < len(s) && s[i+1] == ' ' {
			result = append(result, current)
			current = ""
			i++ // skip space
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func splitCookie(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
