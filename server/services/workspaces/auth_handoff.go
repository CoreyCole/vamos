package workspaces

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type HandoffClaims struct {
	Email        string `json:"email"`
	TargetSlug   string `json:"target_slug"`
	RedirectPath string `json:"redirect_path"`
	ExpiresAt    int64  `json:"expires_at"`
	JTI          string `json:"jti"`
}

type ReplayCache interface {
	Use(jti string, expiresAt time.Time) bool
}

type MemoryReplayCache struct {
	mu   sync.Mutex
	used map[string]time.Time
	now  func() time.Time
}

func NewMemoryReplayCache() *MemoryReplayCache {
	return &MemoryReplayCache{
		used: map[string]time.Time{},
		now:  time.Now,
	}
}

func (c *MemoryReplayCache) Use(jti string, expiresAt time.Time) bool {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for key, exp := range c.used {
		if !exp.After(now) {
			delete(c.used, key)
		}
	}
	if _, exists := c.used[jti]; exists {
		return false
	}
	c.used[jti] = expiresAt
	return true
}

type HandoffSigner struct {
	signingKey ed25519.PrivateKey
	verifyKey  ed25519.PublicKey
	ttl        time.Duration
	replay     ReplayCache
	now        func() time.Time
}

func NewHandoffSigner(
	signingKey ed25519.PrivateKey,
	verifyKey ed25519.PublicKey,
	ttl time.Duration,
	replay ReplayCache,
) *HandoffSigner {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if replay == nil {
		replay = NewMemoryReplayCache()
	}
	return &HandoffSigner{
		signingKey: signingKey,
		verifyKey:  verifyKey,
		ttl:        ttl,
		replay:     replay,
		now:        time.Now,
	}
}

func NewHandoffSignerFromSigningKey(
	encoded string,
	ttl time.Duration,
	replay ReplayCache,
) (*HandoffSigner, string, error) {
	signingKey, err := ParseHandoffSigningKey(encoded)
	if err != nil {
		return nil, "", err
	}
	verifyKey, ok := signingKey.Public().(ed25519.PublicKey)
	if !ok || len(verifyKey) != ed25519.PublicKeySize {
		return nil, "", errors.New("derive handoff verify key")
	}
	return NewHandoffSigner(
			signingKey,
			verifyKey,
			ttl,
			replay,
		), EncodeHandoffVerifyKey(
			verifyKey,
		), nil
}

func NewHandoffVerifierFromVerifyKey(
	encoded string,
	ttl time.Duration,
	replay ReplayCache,
) (*HandoffSigner, error) {
	verifyKey, err := ParseHandoffVerifyKey(encoded)
	if err != nil {
		return nil, err
	}
	return NewHandoffSigner(nil, verifyKey, ttl, replay), nil
}

func ParseHandoffSigningKey(encoded string) (ed25519.PrivateKey, error) {
	raw, err := decodeRawURLBase64(
		strings.TrimSpace(encoded),
		"VAMOS_DEV_AUTH_SIGNING_KEY",
	)
	if err != nil {
		return nil, err
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		key := ed25519.PrivateKey(raw)
		derived := ed25519.NewKeyFromSeed(key.Seed())
		if !bytes.Equal(derived[ed25519.SeedSize:], key[ed25519.SeedSize:]) {
			return nil, errors.New("invalid VAMOS_DEV_AUTH_SIGNING_KEY public key")
		}
		return key, nil
	default:
		return nil, fmt.Errorf(
			"VAMOS_DEV_AUTH_SIGNING_KEY must decode to %d-byte seed or %d-byte private key; got %d bytes",
			ed25519.SeedSize,
			ed25519.PrivateKeySize,
			len(raw),
		)
	}
}

func ParseHandoffVerifyKey(encoded string) (ed25519.PublicKey, error) {
	raw, err := decodeRawURLBase64(
		strings.TrimSpace(encoded),
		"VAMOS_DEV_AUTH_VERIFY_KEY",
	)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf(
			"VAMOS_DEV_AUTH_VERIFY_KEY must decode to %d bytes; got %d bytes",
			ed25519.PublicKeySize,
			len(raw),
		)
	}
	return ed25519.PublicKey(raw), nil
}

func EncodeHandoffVerifyKey(key ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

func decodeRawURLBase64(encoded, name string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return raw, nil
}

func (s *HandoffSigner) Sign(claims HandoffClaims) (string, error) {
	if len(s.signingKey) != ed25519.PrivateKeySize {
		return "", errors.New("dev auth signing key is required")
	}
	if strings.TrimSpace(claims.Email) == "" {
		return "", errors.New("email is required")
	}
	if err := ValidateSlug(claims.TargetSlug); err != nil {
		return "", err
	}
	redirect, err := ValidateLocalRedirectPath(claims.RedirectPath)
	if err != nil {
		return "", err
	}
	claims.Email = strings.TrimSpace(claims.Email)
	claims.RedirectPath = redirect
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = s.now().Add(s.ttl).Unix()
	}
	if claims.JTI == "" {
		claims.JTI, err = randomJTI()
		if err != nil {
			return "", err
		}
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(s.signingKey, []byte(payloadB64))
	return payloadB64 + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *HandoffSigner) Verify(token, expectedSlug string) (HandoffClaims, error) {
	if len(s.verifyKey) != ed25519.PublicKeySize {
		return HandoffClaims{}, errors.New("dev auth verify key is required")
	}
	payloadB64, sigB64, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || payloadB64 == "" || sigB64 == "" {
		return HandoffClaims{}, errors.New("invalid token format")
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return HandoffClaims{}, errors.New("invalid token signature")
	}
	if !ed25519.Verify(s.verifyKey, []byte(payloadB64), sig) {
		return HandoffClaims{}, errors.New("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return HandoffClaims{}, err
	}
	var claims HandoffClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return HandoffClaims{}, err
	}
	if claims.TargetSlug != expectedSlug {
		return HandoffClaims{}, fmt.Errorf(
			"handoff token target %q does not match %q",
			claims.TargetSlug,
			expectedSlug,
		)
	}
	if s.now().Unix() > claims.ExpiresAt {
		return HandoffClaims{}, errors.New("handoff token expired")
	}
	redirect, err := ValidateLocalRedirectPath(claims.RedirectPath)
	if err != nil {
		return HandoffClaims{}, err
	}
	claims.RedirectPath = redirect
	if !s.replay.Use(claims.JTI, time.Unix(claims.ExpiresAt, 0)) {
		return HandoffClaims{}, errors.New("handoff token replayed")
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

func ValidateLocalRedirectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/", nil
	}
	if strings.HasPrefix(path, "//") {
		return "", errors.New("redirect must be local path")
	}
	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if u.IsAbs() || u.Host != "" {
		return "", errors.New("redirect must be local path")
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", errors.New("redirect must start with /")
	}
	if strings.HasPrefix(u.Path, "/internal/dev-auth") {
		return "", errors.New("redirect to handoff endpoint not allowed")
	}
	return u.RequestURI(), nil
}
