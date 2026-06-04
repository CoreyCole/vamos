package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	"golang.org/x/crypto/bcrypt"
)

type MachineCredential struct {
	ID                 string
	Name               string
	SecretHash         []byte
	DefaultActorEmail  string
	AllowedActorEmails []string
	AllowedSlugs       []string
	AllowedPurposes    []agentbrowser.Purpose
	ExpiresAt          *time.Time
	RevokedAt          *time.Time
	CreatedAt          time.Time
	LastUsedAt         *time.Time
}

type CreateMachineCredentialInput struct {
	Name               string
	DefaultActorEmail  string
	AllowedActorEmails []string
	AllowedSlugs       []string
	AllowedPurposes    []agentbrowser.Purpose
	ExpiresAt          *time.Time
}

type CreatedMachineCredential struct {
	Credential MachineCredential
	Secret     string
}

type BrowserTokenRequest struct {
	Slug         string
	Purpose      agentbrowser.Purpose
	Email        string
	RedirectPath string
	TTL          time.Duration
}

type MachineCredentialStore interface {
	Create(ctx context.Context, input CreateMachineCredentialInput) (CreatedMachineCredential, error)
	Authenticate(ctx context.Context, keyID, secret string) (MachineCredential, error)
	Revoke(ctx context.Context, keyID string) error
	List(ctx context.Context) ([]MachineCredential, error)
}

type MemoryMachineCredentialStore struct {
	mu    sync.Mutex
	creds map[string]MachineCredential
	now   func() time.Time
}

func NewMemoryMachineCredentialStore() *MemoryMachineCredentialStore {
	return &MemoryMachineCredentialStore{creds: map[string]MachineCredential{}, now: time.Now}
}

func HashMachineSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("machine secret is required")
	}
	return bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
}

func VerifyMachineSecret(secret string, hash []byte) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(strings.TrimSpace(secret))) == nil
}

func NewMachineSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "vamos_machine_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *MemoryMachineCredentialStore) Create(ctx context.Context, input CreateMachineCredentialInput) (CreatedMachineCredential, error) {
	select {
	case <-ctx.Done():
		return CreatedMachineCredential{}, ctx.Err()
	default:
	}

	secret, err := NewMachineSecret()
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	hash, err := HashMachineSecret(secret)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	id, err := newMachineCredentialID()
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	credential := MachineCredential{
		ID:                 id,
		Name:               strings.TrimSpace(input.Name),
		SecretHash:         hash,
		DefaultActorEmail:  strings.TrimSpace(input.DefaultActorEmail),
		AllowedActorEmails: normalizeStringList(input.AllowedActorEmails),
		AllowedSlugs:       normalizeStringList(input.AllowedSlugs),
		AllowedPurposes:    append([]agentbrowser.Purpose(nil), input.AllowedPurposes...),
		ExpiresAt:          input.ExpiresAt,
		CreatedAt:          s.now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds[credential.ID] = credential
	return CreatedMachineCredential{Credential: cloneMachineCredential(credential), Secret: secret}, nil
}

func (s *MemoryMachineCredentialStore) Authenticate(ctx context.Context, keyID, secret string) (MachineCredential, error) {
	select {
	case <-ctx.Done():
		return MachineCredential{}, ctx.Err()
	default:
	}

	keyID = strings.TrimSpace(keyID)
	if keyID == "" || strings.TrimSpace(secret) == "" {
		return MachineCredential{}, errors.New("machine credential id and secret are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	credential, ok := s.creds[keyID]
	if !ok || !VerifyMachineSecret(secret, credential.SecretHash) {
		return MachineCredential{}, errors.New("invalid machine credential")
	}
	if credential.RevokedAt != nil {
		return MachineCredential{}, errors.New("machine credential revoked")
	}
	if credential.ExpiresAt != nil && !credential.ExpiresAt.After(s.now()) {
		return MachineCredential{}, errors.New("machine credential expired")
	}
	now := s.now()
	credential.LastUsedAt = &now
	s.creds[keyID] = credential
	return cloneMachineCredential(credential), nil
}

func (s *MemoryMachineCredentialStore) Revoke(ctx context.Context, keyID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return errors.New("machine credential id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	credential, ok := s.creds[keyID]
	if !ok {
		return errors.New("machine credential not found")
	}
	now := s.now()
	credential.RevokedAt = &now
	s.creds[keyID] = credential
	return nil
}

func (s *MemoryMachineCredentialStore) List(ctx context.Context) ([]MachineCredential, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	credentials := make([]MachineCredential, 0, len(s.creds))
	for _, credential := range s.creds {
		credentials = append(credentials, cloneMachineCredential(credential))
	}
	return credentials, nil
}

func (c MachineCredential) Allows(req BrowserTokenRequest) error {
	if c.RevokedAt != nil {
		return errors.New("machine credential revoked")
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()) {
		return errors.New("machine credential expired")
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = strings.TrimSpace(c.DefaultActorEmail)
	}
	if email == "" {
		return errors.New("email is required")
	}
	if len(c.AllowedActorEmails) > 0 && !containsString(c.AllowedActorEmails, email) {
		return errors.New("email not allowed")
	}
	if len(c.AllowedSlugs) > 0 && !containsString(c.AllowedSlugs, strings.TrimSpace(req.Slug)) {
		return errors.New("slug not allowed")
	}
	if len(c.AllowedPurposes) > 0 && !containsPurpose(c.AllowedPurposes, req.Purpose) {
		return errors.New("purpose not allowed")
	}
	return nil
}

func newMachineCredentialID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "mc_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func containsPurpose(values []agentbrowser.Purpose, target agentbrowser.Purpose) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneMachineCredential(credential MachineCredential) MachineCredential {
	credential.SecretHash = append([]byte(nil), credential.SecretHash...)
	credential.AllowedActorEmails = append([]string(nil), credential.AllowedActorEmails...)
	credential.AllowedSlugs = append([]string(nil), credential.AllowedSlugs...)
	credential.AllowedPurposes = append([]agentbrowser.Purpose(nil), credential.AllowedPurposes...)
	return credential
}
