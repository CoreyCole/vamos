package workspaces

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/auth"
)

func newTestHandoffSigner(t *testing.T) (*HandoffSigner, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return NewHandoffSigner(privateKey, publicKey, time.Minute, NewMemoryReplayCache()),
		EncodeHandoffVerifyKey(publicKey)
}

func newTestHandoffVerifier(t *testing.T, verifyKey string) *HandoffSigner {
	t.Helper()
	verifier, err := NewHandoffVerifierFromVerifyKey(
		verifyKey,
		time.Minute,
		NewMemoryReplayCache(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func TestHandoffSignVerify(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	t0 := time.Unix(1000, 0)
	signer.now = func() time.Time { return t0 }
	verifier.now = func() time.Time { return t0 }

	token, err := signer.Sign(HandoffClaims{
		Email:        "user@example.com",
		TargetSlug:   "feature",
		RedirectPath: "/agent-chat?x=1",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims, err := verifier.Verify(token, "feature")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.Email != "user@example.com" || claims.RedirectPath != "/agent-chat?x=1" ||
		claims.ExpiresAt != t0.Add(time.Minute).Unix() {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestHandoffRejectsTamper(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/",
		},
	)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token parts = %d", len(parts))
	}
	tampered := parts[0] + "." + strings.TrimRight(parts[1], "A") + "A"
	if tampered == token {
		tampered += "x"
	}
	if _, err := verifier.Verify(tampered, "feature"); err == nil {
		t.Fatal("Verify(tampered) error = nil")
	}
}

func TestHandoffRejectsWrongTargetSlug(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/",
		},
	)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := verifier.Verify(token, "main"); err == nil {
		t.Fatal("Verify(wrong slug) error = nil")
	}
}

func TestHandoffRejectsExpired(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	t0 := time.Unix(1000, 0)
	signer.now = func() time.Time { return t0 }
	token, err := signer.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/",
		},
	)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	verifier.now = func() time.Time { return t0.Add(2 * time.Minute) }
	if _, err := verifier.Verify(token, "feature"); err == nil {
		t.Fatal("Verify(expired) error = nil")
	}
}

func TestHandoffRejectsReplay(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/",
		},
	)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := verifier.Verify(token, "feature"); err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}
	if _, err := verifier.Verify(token, "feature"); err == nil {
		t.Fatal("second Verify() error = nil")
	}
}

func TestParseHandoffSigningKeyAcceptsSeedAndPrivateKey(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	encodedSeed := base64.RawURLEncoding.EncodeToString(seed)
	keyFromSeed, err := ParseHandoffSigningKey(encodedSeed)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(keyFromSeed) != ed25519.PrivateKeySize {
		t.Fatalf("seed key len=%d", len(keyFromSeed))
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encodedPrivate := base64.RawURLEncoding.EncodeToString(privateKey)
	keyFromPrivate, err := ParseHandoffSigningKey(encodedPrivate)
	if err != nil {
		t.Fatalf("private: %v", err)
	}
	if len(keyFromPrivate) != ed25519.PrivateKeySize {
		t.Fatalf("private key len=%d", len(keyFromPrivate))
	}
}

func TestParseHandoffSigningKeyRejectsMismatchedPrivateKey(t *testing.T) {
	raw := make([]byte, ed25519.PrivateKeySize)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := ParseHandoffSigningKey(encoded); err == nil {
		t.Fatal("mismatched private key error=nil")
	}
}

func TestParseHandoffVerifyKeyRequiresPublicKeySize(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifyKey := privateKey.Public().(ed25519.PublicKey)
	encoded := EncodeHandoffVerifyKey(verifyKey)
	parsed, err := ParseHandoffVerifyKey(encoded)
	if err != nil {
		t.Fatalf("valid verify key: %v", err)
	}
	if string(parsed) != string(verifyKey) {
		t.Fatal("parsed verify key mismatch")
	}

	bad := base64.RawURLEncoding.EncodeToString([]byte("short"))
	if _, err := ParseHandoffVerifyKey(bad); err == nil {
		t.Fatal("short verify key error=nil")
	}
}

func TestVerifyOnlyHandoffSignerCannotSign(t *testing.T) {
	_, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	if _, err := verifier.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/",
		},
	); err == nil {
		t.Fatal("verify-only Sign() error=nil")
	}
}

func TestValidateLocalRedirectPath(t *testing.T) {
	accepted := map[string]string{
		"":                  "/",
		"/":                 "/",
		"/agent-chat?x=1":   "/agent-chat?x=1",
		"/thoughts/foo#bar": "/thoughts/foo",
	}
	for in, want := range accepted {
		got, err := ValidateLocalRedirectPath(in)
		if err != nil {
			t.Fatalf("ValidateLocalRedirectPath(%q) error = %v", in, err)
		}
		if got != want {
			t.Fatalf("ValidateLocalRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"https://evil", "//evil", "agent-chat", "/internal/dev-auth/handoff"} {
		if _, err := ValidateLocalRedirectPath(in); err == nil {
			t.Fatalf("ValidateLocalRedirectPath(%q) error = nil", in)
		}
	}
}

type authAttempt struct {
	email        string
	success      bool
	errorMessage string
}

type fakeSessionCreator struct {
	email          string
	validateEmail  func(string) error
	createSession  func(context.Context, string) (*auth.Session, error)
	logAuthAttempt func(context.Context, string, bool, string) error
	attempts       []authAttempt
}

func (f *fakeSessionCreator) ValidateEmail(email string) error {
	if f.validateEmail != nil {
		return f.validateEmail(email)
	}
	return nil
}

func (f *fakeSessionCreator) CreateSession(
	ctx context.Context,
	email string,
) (*auth.Session, error) {
	if f.createSession != nil {
		return f.createSession(ctx, email)
	}
	f.email = email
	return &auth.Session{
		ID:        "session-id",
		UserEmail: email,
		ExpiresAt: time.Unix(2000, 0),
	}, nil
}

func (f *fakeSessionCreator) LogAuthAttempt(
	ctx context.Context,
	email string,
	success bool,
	errorMessage string,
) error {
	f.attempts = append(f.attempts, authAttempt{
		email:        email,
		success:      success,
		errorMessage: errorMessage,
	})
	if f.logAuthAttempt != nil {
		return f.logAuthAttempt(ctx, email, success, errorMessage)
	}
	return nil
}

func TestHandleDevAuthHandoffSetsSessionCookie(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(
		HandoffClaims{
			Email:        "user@example.com",
			TargetSlug:   "feature",
			RedirectPath: "/agent-chat",
		},
	)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	authSvc := &fakeSessionCreator{}
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"feature",
		WithDevAuth(authSvc, verifier),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/dev-auth/handoff?token="+url.QueryEscape(token),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler.HandleDevAuthHandoff(c); err != nil {
		t.Fatalf("HandleDevAuthHandoff() error = %v", err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/agent-chat" {
		t.Fatalf("status/location = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != auth.SessionCookieName || cookie.Value != "session-id" ||
		cookie.Domain != "" {
		t.Fatalf("cookie = %+v", cookie)
	}
	if authSvc.email != "user@example.com" {
		t.Fatalf("created session email = %q", authSvc.email)
	}
	assertAuthAttempt(t, authSvc.attempts, 0, "user@example.com", true, "")
}

func TestHandleDevAuthHandoffRejectsUnauthorizedEmail(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(HandoffClaims{
		Email:        "intruder@example.net",
		TargetSlug:   "feature",
		RedirectPath: "/",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	authSvc := &fakeSessionCreator{
		validateEmail: func(string) error { return errors.New("email not authorized") },
	}
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"feature",
		WithDevAuth(authSvc, verifier),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/dev-auth/handoff?token="+url.QueryEscape(token),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err = handler.HandleDevAuthHandoff(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok ||
		httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("HandleDevAuthHandoff() error = %#v", err)
	}
	if authSvc.email != "" {
		t.Fatalf("created session for unauthorized email %q", authSvc.email)
	}
	assertAuthAttempt(
		t,
		authSvc.attempts,
		0,
		"intruder@example.net",
		false,
		"email not authorized",
	)
}

func TestHandleDevAuthHandoffLogsUnknownEmailForInvalidToken(t *testing.T) {
	_, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	authSvc := &fakeSessionCreator{}
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"feature",
		WithDevAuth(authSvc, verifier),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/dev-auth/handoff?token=bad-token",
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := handler.HandleDevAuthHandoff(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok ||
		httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("HandleDevAuthHandoff() error = %#v", err)
	}
	if authSvc.email != "" {
		t.Fatalf("created session for invalid token email %q", authSvc.email)
	}
	assertAuthAttempt(t, authSvc.attempts, 0, "unknown", false, "invalid token format")
	if strings.Contains(authSvc.attempts[0].errorMessage, "bad-token") {
		t.Fatalf("logged token contents: %#v", authSvc.attempts[0])
	}
}

func TestHandleDevAuthHandoffLogsSessionCreationFailure(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(HandoffClaims{
		Email:        "user@example.com",
		TargetSlug:   "feature",
		RedirectPath: "/",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	authSvc := &fakeSessionCreator{
		createSession: func(context.Context, string) (*auth.Session, error) {
			return nil, errors.New("session failed")
		},
	}
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"feature",
		WithDevAuth(authSvc, verifier),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/dev-auth/handoff?token="+url.QueryEscape(token),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler.HandleDevAuthHandoff(
		c,
	); err == nil ||
		err.Error() != "session failed" {
		t.Fatalf("HandleDevAuthHandoff() error = %v", err)
	}
	if authSvc.email != "" {
		t.Fatalf("created session despite failure for email %q", authSvc.email)
	}
	assertAuthAttempt(t, authSvc.attempts, 0, "user@example.com", false, "session failed")
}

func TestHandleDevAuthHandoffIgnoresLoggingErrorOnSuccess(t *testing.T) {
	signer, verifyKey := newTestHandoffSigner(t)
	verifier := newTestHandoffVerifier(t, verifyKey)
	token, err := signer.Sign(HandoffClaims{
		Email:        "user@example.com",
		TargetSlug:   "feature",
		RedirectPath: "/",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	authSvc := &fakeSessionCreator{
		logAuthAttempt: func(context.Context, string, bool, string) error {
			return errors.New("log failed")
		},
	}
	handler := NewHandler(
		nil,
		"https://main.cn-agents.test",
		"feature",
		WithDevAuth(authSvc, verifier),
	)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/dev-auth/handoff?token="+url.QueryEscape(token),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler.HandleDevAuthHandoff(c); err != nil {
		t.Fatalf("HandleDevAuthHandoff() error = %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if authSvc.email != "user@example.com" {
		t.Fatalf("created session email = %q", authSvc.email)
	}
	assertAuthAttempt(t, authSvc.attempts, 0, "user@example.com", true, "")
}

func assertAuthAttempt(
	t *testing.T,
	attempts []authAttempt,
	index int,
	email string,
	success bool,
	errorContains string,
) {
	t.Helper()
	if len(attempts) <= index {
		t.Fatalf("attempts=%#v, missing index %d", attempts, index)
	}
	attempt := attempts[index]
	if attempt.email != email || attempt.success != success ||
		!strings.Contains(attempt.errorMessage, errorContains) {
		t.Fatalf("attempts[%d]=%#v", index, attempt)
	}
	if len(attempts) != index+1 {
		t.Fatalf("attempts=%#v, want %d attempts", attempts, index+1)
	}
}
