package auth

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
)

type BrowserTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AgentBrowserAuthConfig struct {
	Enabled            bool
	WorkspaceSlug      string
	SigningKey         ed25519.PrivateKey
	VerifyKey          ed25519.PublicKey
	MachineCredentials MachineCredentialStore
	Replay             agentbrowser.ReplayCache
	Now                func() time.Time
}

func RegisterAgentBrowserAuthRoutes(e *echo.Echo, svc *Service, cfg AgentBrowserAuthConfig) {
	if !cfg.Enabled {
		return
	}
	e.POST("/internal/agent-auth/mint-browser-token", func(c echo.Context) error {
		return svc.HandleMintBrowserToken(c, cfg)
	})
	e.GET("/internal/agent-auth/browser-login", func(c echo.Context) error {
		return svc.HandleAgentBrowserLogin(c, cfg)
	})
}

func (s *Service) HandleMintBrowserToken(c echo.Context, cfg AgentBrowserAuthConfig) error {
	if cfg.MachineCredentials == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "machine credentials unavailable")
	}
	keyID, secret, err := machineCredentialFromRequest(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	credential, err := cfg.MachineCredentials.Authenticate(c.Request().Context(), keyID, secret)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid machine credential")
	}
	var req BrowserTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	resp, err := MintBrowserToken(credential, req, cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *Service) HandleAgentBrowserLogin(c echo.Context, cfg AgentBrowserAuthConfig) error {
	purpose := agentbrowser.Purpose(strings.TrimSpace(c.QueryParam("purpose")))
	if purpose == "" {
		purpose = agentbrowser.PurposeHermesChat
	}
	signer := agentbrowser.NewSigner(nil, cfg.VerifyKey, 5*time.Minute, cfg.Replay)
	claims, err := signer.Verify(c.QueryParam("token"), cfg.WorkspaceSlug, purpose)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	session, err := s.CreateSession(c.Request().Context(), claims.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create agent browser session")
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
	return c.Redirect(http.StatusTemporaryRedirect, claims.RedirectPath)
}

func MintBrowserToken(credential MachineCredential, req BrowserTokenRequest, cfg AgentBrowserAuthConfig) (BrowserTokenResponse, error) {
	if err := credential.Allows(req); err != nil {
		return BrowserTokenResponse{}, err
	}
	ttl := req.TTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	switch req.Purpose {
	case agentbrowser.PurposeE2EPlaywright:
		if ttl > 15*time.Minute {
			ttl = 15 * time.Minute
		}
	case agentbrowser.PurposeHermesChat:
		if ttl > 5*time.Minute {
			ttl = 5 * time.Minute
		}
	}
	redirect, err := agentbrowser.ValidateRedirectPath(req.RedirectPath)
	if err != nil {
		return BrowserTokenResponse{}, err
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = strings.TrimSpace(credential.DefaultActorEmail)
	}
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	expiresAt := now().Add(ttl)
	signer := agentbrowser.NewSigner(cfg.SigningKey, nil, ttl, nil)
	token, err := signer.Sign(agentbrowser.Claims{
		Purpose:      req.Purpose,
		Email:        email,
		TargetSlug:   strings.TrimSpace(req.Slug),
		RedirectPath: redirect,
		ExpiresAt:    expiresAt.Unix(),
		KeyID:        credential.ID,
	})
	if err != nil {
		return BrowserTokenResponse{}, err
	}
	return BrowserTokenResponse{Token: token, ExpiresAt: expiresAt}, nil
}

func machineCredentialFromRequest(r *http.Request) (string, string, error) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(raw, "Bearer ") {
		return "", "", errors.New("machine bearer token required")
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	if strings.HasPrefix(token, "vamos_machine_") {
		token = strings.TrimPrefix(token, "vamos_machine_")
	}
	keyID, secret, ok := strings.Cut(token, ".")
	if !ok || strings.TrimSpace(keyID) == "" || strings.TrimSpace(secret) == "" {
		return "", "", errors.New("invalid machine bearer token")
	}
	return strings.TrimSpace(keyID), strings.TrimSpace(secret), nil
}
