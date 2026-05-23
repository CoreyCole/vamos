package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	SessionCookieName = "thoughts_session"
	SessionDuration   = 24 * time.Hour
)

// Service handles authentication operations
type Service struct {
	dbQueries         *db.Queries
	oauthConfig       *oauth2.Config
	allowedDomains    []string
	whitelistedEmails []string
}

// GoogleCredentials represents the structure of Google OAuth credentials JSON file
type GoogleCredentials struct {
	Web struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
	} `json:"web"`
}

// NewService creates a new auth service
func NewService(dbQueries *db.Queries, credentialsFile string, allowedDomains, whitelistedEmails []string) (*Service, error) {
	// Load credentials from JSON file
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds GoogleCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Use the first redirect URI from the credentials file
	redirectURL := ""
	if len(creds.Web.RedirectURIs) > 0 {
		redirectURL = creds.Web.RedirectURIs[0]
	}

	oauthConfig := &oauth2.Config{
		ClientID:     creds.Web.ClientID,
		ClientSecret: creds.Web.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}

	return &Service{
		dbQueries:         dbQueries,
		oauthConfig:       oauthConfig,
		allowedDomains:    allowedDomains,
		whitelistedEmails: whitelistedEmails,
	}, nil
}

// GetAuthURL returns the Google OAuth URL for login
func (s *Service) GetAuthURL(state string) string {
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOnline}
	// Only hint domain when there's exactly one allowed domain and no whitelisted emails
	if len(s.allowedDomains) == 1 && len(s.whitelistedEmails) == 0 {
		opts = append(opts, oauth2.SetAuthURLParam("hd", s.allowedDomains[0]))
	}
	return s.oauthConfig.AuthCodeURL(state, opts...)
}

// ValidateEmail checks if email is from an allowed domain or is whitelisted
func (s *Service) ValidateEmail(email string) error {
	email = strings.ToLower(email)

	// Check whitelisted emails first
	for _, allowed := range s.whitelistedEmails {
		if strings.ToLower(allowed) == email {
			return nil
		}
	}

	// Check allowed domains
	for _, domain := range s.allowedDomains {
		if strings.HasSuffix(email, "@"+strings.ToLower(domain)) {
			return nil
		}
	}

	return fmt.Errorf("email not authorized")
}

// AllowedDomains returns the list of allowed email domains (for templates)
func (s *Service) AllowedDomains() []string {
	return s.allowedDomains
}

// CreateSession creates a new session for the user
func (s *Service) CreateSession(ctx context.Context, email string) (*Session, error) {
	// Generate secure session ID
	sessionID, err := generateSecureToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	expiresAt := time.Now().Add(SessionDuration)

	// Create session in database
	dbSession, err := s.dbQueries.CreateSession(ctx, db.CreateSessionParams{
		ID:        sessionID,
		UserEmail: email,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		ID:             dbSession.ID,
		UserEmail:      dbSession.UserEmail,
		CreatedAt:      dbSession.CreatedAt,
		ExpiresAt:      dbSession.ExpiresAt,
		LastAccessedAt: dbSession.LastAccessedAt,
	}, nil
}

// GetSession retrieves a valid session by ID
func (s *Service) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	dbSession, err := s.dbQueries.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found or expired")
	}

	// Update last accessed time
	_ = s.dbQueries.UpdateSessionAccess(ctx, sessionID)

	return &Session{
		ID:             dbSession.ID,
		UserEmail:      dbSession.UserEmail,
		CreatedAt:      dbSession.CreatedAt,
		ExpiresAt:      dbSession.ExpiresAt,
		LastAccessedAt: dbSession.LastAccessedAt,
	}, nil
}

// DeleteSession removes a session
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	return s.dbQueries.DeleteSession(ctx, sessionID)
}

// LogAuthAttempt logs an authentication attempt
func (s *Service) LogAuthAttempt(ctx context.Context, email string, success bool, errorMsg string) error {
	var errorMessage sql.NullString
	if errorMsg != "" {
		errorMessage = sql.NullString{String: errorMsg, Valid: true}
	}

	_, err := s.dbQueries.LogAuthAttempt(ctx, db.LogAuthAttemptParams{
		Email:        email,
		Success:      success,
		ErrorMessage: errorMessage,
	})
	return err
}

// ExchangeCodeForToken exchanges an OAuth code for a token
func (s *Service) ExchangeCodeForToken(ctx context.Context, code string) (*oauth2.Token, error) {
	return s.oauthConfig.Exchange(ctx, code)
}

// GetOAuthConfig returns the OAuth configuration
func (s *Service) GetOAuthConfig() *oauth2.Config {
	return s.oauthConfig
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
