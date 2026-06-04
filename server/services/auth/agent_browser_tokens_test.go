package auth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
)

func TestAgentBrowserMintAndLoginSetsSessionCookie(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), CreateMachineCredentialInput{
		Name:               "agent laptop",
		DefaultActorEmail:  "agent@example.test",
		AllowedSlugs:       []string{"stage"},
		AllowedPurposes:    []agentbrowser.Purpose{agentbrowser.PurposeHermesChat},
		AllowedActorEmails: []string{"agent@example.test"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	e := echo.New()
	service := newPlaywrightAuthTestService(t)
	cfg := AgentBrowserAuthConfig{
		Enabled:            true,
		WorkspaceSlug:      "stage",
		SigningKey:         privateKey,
		VerifyKey:          publicKey,
		MachineCredentials: store,
		Replay:             agentbrowser.NewMemoryReplayCache(),
	}
	RegisterAgentBrowserAuthRoutes(e, service, cfg)

	mintBody := map[string]any{
		"slug":          "stage",
		"purpose":       string(agentbrowser.PurposeHermesChat),
		"redirect_path": "/agent-chat?thread=thread-1",
		"ttl_seconds":   60,
	}
	body, err := json.Marshal(mintBody)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	mintReq := httptest.NewRequest(http.MethodPost, "/internal/agent-auth/mint-browser-token", bytes.NewReader(body))
	mintReq.Header.Set("Content-Type", "application/json")
	mintReq.Header.Set("Authorization", "Bearer vamos_machine_"+created.Credential.ID+"."+created.Secret)
	mintRec := httptest.NewRecorder()
	e.ServeHTTP(mintRec, mintReq)
	if mintRec.Code != http.StatusOK {
		t.Fatalf("expected mint status 200, got %d: %s", mintRec.Code, mintRec.Body.String())
	}
	var mintResp BrowserTokenResponse
	if err := json.NewDecoder(mintRec.Body).Decode(&mintResp); err != nil {
		t.Fatalf("Decode mint response returned error: %v", err)
	}
	if mintResp.Token == "" || mintResp.ExpiresAt.IsZero() {
		t.Fatalf("expected token and expiry, got %+v", mintResp)
	}

	loginReq := httptest.NewRequest(
		http.MethodGet,
		"/internal/agent-auth/browser-login?purpose=hermes_chat&token="+url.QueryEscape(mintResp.Token),
		nil,
	)
	loginReq.Host = "stage.workspaces.example.test"
	loginReq.Header.Set("X-Forwarded-Proto", "https")
	loginRec := httptest.NewRecorder()
	e.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected login redirect 307, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	if location := loginRec.Header().Get("Location"); location != "/agent-chat?thread=thread-1" {
		t.Fatalf("expected /agent-chat redirect, got %q", location)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Fatalf("expected %s cookie, got %s", SessionCookieName, cookie.Name)
	}
	if !cookie.Secure {
		t.Fatalf("expected secure cookie for forwarded https")
	}
	session, err := service.GetSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if session.UserEmail != "agent@example.test" {
		t.Fatalf("expected agent email, got %q", session.UserEmail)
	}
}

func TestHandleMintBrowserTokenRejectsMissingOrWrongMachineCredential(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), CreateMachineCredentialInput{
		DefaultActorEmail: "agent@example.test",
		AllowedSlugs:      []string{"stage"},
		AllowedPurposes:   []agentbrowser.Purpose{agentbrowser.PurposeE2EPlaywright},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

each:
	for _, tc := range []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "restart token is not browser mint auth", header: ""},
		{name: "wrong secret", header: "Bearer vamos_machine_" + created.Credential.ID + ".wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			RegisterAgentBrowserAuthRoutes(e, newPlaywrightAuthTestService(t), AgentBrowserAuthConfig{
				Enabled:            true,
				WorkspaceSlug:      "stage",
				SigningKey:         privateKey,
				MachineCredentials: store,
			})
			body := bytes.NewBufferString(`{"slug":"stage","purpose":"e2e_playwright","redirect_path":"/"}`)
			req := httptest.NewRequest(http.MethodPost, "/internal/agent-auth/mint-browser-token", body)
			req.Header.Set("Content-Type", "application/json")
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.name == "restart token is not browser mint auth" {
				req.Header.Set("X-Vamos-Workspace-Restart-Token", "restart-token")
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
		continue each
	}
}

func TestAgentBrowserLoginRejectsWrongSlugPurposeExpiredAndReplay(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	service := newPlaywrightAuthTestService(t)

	mintToken := func(t *testing.T, claims agentbrowser.Claims) string {
		t.Helper()
		signer := agentbrowser.NewSigner(privateKey, nil, time.Minute, nil)
		token, err := signer.Sign(claims)
		if err != nil {
			t.Fatalf("Sign returned error: %v", err)
		}
		return token
	}

	tests := []struct {
		name         string
		tokenClaims  agentbrowser.Claims
		loginPurpose string
		workspace    string
		wantStatus   int
	}{
		{
			name:         "wrong slug",
			tokenClaims:  agentbrowser.Claims{Purpose: agentbrowser.PurposeHermesChat, Email: "agent@example.test", TargetSlug: "other", RedirectPath: "/agent-chat", ExpiresAt: time.Now().Add(time.Minute).Unix(), KeyID: "mc_1"},
			loginPurpose: "hermes_chat",
			workspace:    "stage",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "wrong purpose",
			tokenClaims:  agentbrowser.Claims{Purpose: agentbrowser.PurposeE2EPlaywright, Email: "agent@example.test", TargetSlug: "stage", RedirectPath: "/agent-chat", ExpiresAt: time.Now().Add(time.Minute).Unix(), KeyID: "mc_1"},
			loginPurpose: "hermes_chat",
			workspace:    "stage",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "expired",
			tokenClaims:  agentbrowser.Claims{Purpose: agentbrowser.PurposeHermesChat, Email: "agent@example.test", TargetSlug: "stage", RedirectPath: "/agent-chat", ExpiresAt: time.Now().Add(-time.Minute).Unix(), KeyID: "mc_1"},
			loginPurpose: "hermes_chat",
			workspace:    "stage",
			wantStatus:   http.StatusForbidden,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			RegisterAgentBrowserAuthRoutes(e, service, AgentBrowserAuthConfig{Enabled: true, WorkspaceSlug: tc.workspace, VerifyKey: publicKey, Replay: agentbrowser.NewMemoryReplayCache()})
			req := httptest.NewRequest(http.MethodGet, "/internal/agent-auth/browser-login?purpose="+tc.loginPurpose+"&token="+url.QueryEscape(mintToken(t, tc.tokenClaims)), nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}

	t.Run("hermes token is one use", func(t *testing.T) {
		t.Parallel()

		replay := agentbrowser.NewMemoryReplayCache()
		e := echo.New()
		RegisterAgentBrowserAuthRoutes(e, newPlaywrightAuthTestService(t), AgentBrowserAuthConfig{Enabled: true, WorkspaceSlug: "stage", VerifyKey: publicKey, Replay: replay})
		token := mintToken(t, agentbrowser.Claims{Purpose: agentbrowser.PurposeHermesChat, Email: "agent@example.test", TargetSlug: "stage", RedirectPath: "/agent-chat", ExpiresAt: time.Now().Add(time.Minute).Unix(), KeyID: "mc_1"})
		for i, want := range []int{http.StatusTemporaryRedirect, http.StatusForbidden} {
			req := httptest.NewRequest(http.MethodGet, "/internal/agent-auth/browser-login?purpose=hermes_chat&token="+url.QueryEscape(token), nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("attempt %d: expected %d, got %d: %s", i+1, want, rec.Code, rec.Body.String())
			}
		}
	})
}
