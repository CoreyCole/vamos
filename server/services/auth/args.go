package auth

import "time"

// LoginPageArgs contains data for rendering the login page
type LoginPageArgs struct {
	ErrorMessage string
	RedirectURL  string // URL to redirect to after successful login
}

// UnauthorizedPageArgs contains data for rendering unauthorized page
type UnauthorizedPageArgs struct {
	Email          string
	ErrorMessage   string
	AllowedDomains []string
}

// Session represents an authenticated user session
type Session struct {
	ID             string
	UserEmail      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastAccessedAt time.Time
}
