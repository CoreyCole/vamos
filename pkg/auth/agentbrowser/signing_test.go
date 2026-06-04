package agentbrowser

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"
)

func TestSignerRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	signer := NewSigner(priv, nil, 5*time.Minute, nil)
	signer.now = func() time.Time { return now }

	token, err := signer.Sign(Claims{
		Purpose:      PurposeE2EPlaywright,
		Email:        " playwright@example.test ",
		TargetSlug:   " stage ",
		RedirectPath: "/agent-chat?x=1",
		KeyID:        " machine-1 ",
	})
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewSigner(nil, pub, 5*time.Minute, NewMemoryReplayCache())
	verifier.now = func() time.Time { return now }
	claims, err := verifier.Verify(token, "stage", PurposeE2EPlaywright)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "playwright@example.test" {
		t.Fatalf("email = %q", claims.Email)
	}
	if claims.TargetSlug != "stage" {
		t.Fatalf("target slug = %q", claims.TargetSlug)
	}
	if claims.RedirectPath != "/agent-chat?x=1" {
		t.Fatalf("redirect = %q", claims.RedirectPath)
	}
	if claims.KeyID != "machine-1" {
		t.Fatalf("key id = %q", claims.KeyID)
	}
	if claims.ExpiresAt != now.Add(5*time.Minute).Unix() {
		t.Fatalf("expires at = %s", claims.ExpiresAtTime())
	}
	if claims.JTI == "" {
		t.Fatal("expected signer to fill JTI")
	}
}

func TestSignerRejectsWrongSlugPurposeExpiredReplay(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	signer := NewSigner(priv, nil, 5*time.Minute, nil)
	signer.now = func() time.Time { return now }
	hermesToken, err := signer.Sign(Claims{
		Purpose:      PurposeHermesChat,
		Email:        "lead@example.test",
		TargetSlug:   "stage",
		RedirectPath: "/agent-chat",
		KeyID:        "machine-1",
		JTI:          "hermes-jti",
	})
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewSigner(nil, pub, 5*time.Minute, NewMemoryReplayCache())
	verifier.now = func() time.Time { return now }
	if _, err := verifier.Verify(hermesToken, "other", PurposeHermesChat); err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("expected wrong slug error, got %v", err)
	}
	if _, err := verifier.Verify(hermesToken, "stage", PurposeE2EPlaywright); err == nil || !strings.Contains(err.Error(), "purpose") {
		t.Fatalf("expected wrong purpose error, got %v", err)
	}
	if _, err := verifier.Verify(hermesToken, "stage", PurposeHermesChat); err != nil {
		t.Fatalf("first hermes use: %v", err)
	}
	if _, err := verifier.Verify(hermesToken, "stage", PurposeHermesChat); err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("expected hermes replay error, got %v", err)
	}

	expiredToken, err := signer.Sign(Claims{
		Purpose:      PurposeHermesChat,
		Email:        "lead@example.test",
		TargetSlug:   "stage",
		RedirectPath: "/agent-chat",
		ExpiresAt:    now.Add(-time.Second).Unix(),
		KeyID:        "machine-1",
		JTI:          "expired-jti",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(expiredToken, "stage", PurposeHermesChat); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}

	e2eToken, err := signer.Sign(Claims{
		Purpose:      PurposeE2EPlaywright,
		Email:        "playwright@example.test",
		TargetSlug:   "stage",
		RedirectPath: "/",
		KeyID:        "machine-1",
		JTI:          "e2e-jti",
	})
	if err != nil {
		t.Fatal(err)
	}
	e2eVerifier := NewSigner(nil, pub, 5*time.Minute, NewMemoryReplayCache())
	e2eVerifier.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if _, err := e2eVerifier.Verify(e2eToken, "stage", PurposeE2EPlaywright); err != nil {
			t.Fatalf("e2e use %d: %v", i+1, err)
		}
	}
	if _, err := e2eVerifier.Verify(e2eToken, "stage", PurposeE2EPlaywright); err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("expected bounded e2e replay error, got %v", err)
	}
}

func TestValidateRedirectPathRejectsExternalAndInternalAuth(t *testing.T) {
	accepted, err := ValidateRedirectPath("/agent-chat?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if accepted != "/agent-chat?x=1" {
		t.Fatalf("accepted redirect = %q", accepted)
	}

	for _, path := range []string{
		"https://example.test/agent-chat",
		"//example.test/agent-chat",
		"agent-chat",
		"/internal/agent-auth/browser-login",
		"/internal/dev-auth/callback",
	} {
		t.Run(path, func(t *testing.T) {
			if _, err := ValidateRedirectPath(path); err == nil {
				t.Fatal("expected redirect rejection")
			}
		})
	}
}
