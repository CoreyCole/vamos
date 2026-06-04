package agentbrowser

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Signer struct {
	signingKey ed25519.PrivateKey
	verifyKey  ed25519.PublicKey
	ttl        time.Duration
	replay     ReplayCache
	now        func() time.Time
}

func NewSigner(signingKey ed25519.PrivateKey, verifyKey ed25519.PublicKey, ttl time.Duration, replay ReplayCache) *Signer {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if replay == nil {
		replay = NewMemoryReplayCache()
	}
	return &Signer{signingKey: signingKey, verifyKey: verifyKey, ttl: ttl, replay: replay, now: time.Now}
}

func (s *Signer) Sign(claims Claims) (string, error) {
	if len(s.signingKey) != ed25519.PrivateKeySize {
		return "", errors.New("agent browser signing key is required")
	}
	normalized, err := s.normalizeClaims(claims)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(s.signingKey, []byte(payloadB64))
	return payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Signer) Verify(token, expectedSlug string, purpose Purpose) (Claims, error) {
	if len(s.verifyKey) != ed25519.PublicKeySize {
		return Claims{}, errors.New("agent browser verify key is required")
	}
	payloadB64, sigB64, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || payloadB64 == "" || sigB64 == "" {
		return Claims{}, errors.New("invalid token format")
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || !ed25519.Verify(s.verifyKey, []byte(payloadB64), sig) {
		return Claims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, err
	}
	if strings.TrimSpace(claims.TargetSlug) != strings.TrimSpace(expectedSlug) {
		return Claims{}, fmt.Errorf("token target %q does not match %q", claims.TargetSlug, expectedSlug)
	}
	if claims.Purpose != purpose {
		return Claims{}, fmt.Errorf("token purpose %q does not match %q", claims.Purpose, purpose)
	}
	if s.now().Unix() > claims.ExpiresAt {
		return Claims{}, errors.New("agent browser token expired")
	}
	redirect, err := ValidateRedirectPath(claims.RedirectPath)
	if err != nil {
		return Claims{}, err
	}
	claims.RedirectPath = redirect
	if !s.replay.Use(claims.JTI, claims.ExpiresAtTime(), replayPolicyForPurpose(purpose)) {
		return Claims{}, errors.New("agent browser token replayed")
	}
	return claims, nil
}

func replayPolicyForPurpose(purpose Purpose) ReplayPolicy {
	switch purpose {
	case PurposeE2EPlaywright:
		return ReplayPolicy{MaxUses: 5}
	default:
		return ReplayPolicy{MaxUses: 1}
	}
}

func (s *Signer) normalizeClaims(claims Claims) (Claims, error) {
	claims.Email = strings.TrimSpace(claims.Email)
	claims.TargetSlug = strings.TrimSpace(claims.TargetSlug)
	claims.KeyID = strings.TrimSpace(claims.KeyID)
	if claims.Email == "" {
		return Claims{}, errors.New("email is required")
	}
	if claims.TargetSlug == "" {
		return Claims{}, errors.New("target slug is required")
	}
	if claims.Purpose == "" {
		return Claims{}, errors.New("purpose is required")
	}
	if claims.KeyID == "" {
		return Claims{}, errors.New("key id is required")
	}
	redirect, err := ValidateRedirectPath(claims.RedirectPath)
	if err != nil {
		return Claims{}, err
	}
	claims.RedirectPath = redirect
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = s.now().Add(s.ttl).Unix()
	}
	if claims.JTI == "" {
		claims.JTI, err = randomJTI()
		if err != nil {
			return Claims{}, err
		}
	}
	return claims, nil
}

func randomJTI() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
