package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
)

func TestMachineSecretHashVerify(t *testing.T) {
	hash, err := HashMachineSecret(" secret ")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyMachineSecret("secret", hash) {
		t.Fatal("expected correct secret to verify")
	}
	if VerifyMachineSecret("wrong", hash) {
		t.Fatal("expected wrong secret to fail")
	}
}

func TestMemoryMachineCredentialStoreSecretHandling(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(ctx, CreateMachineCredentialInput{
		Name:              "Agent Laptop",
		DefaultActorEmail: "lead@example.test",
		AllowedSlugs:      []string{"stage"},
		AllowedPurposes:   []agentbrowser.Purpose{agentbrowser.PurposeE2EPlaywright},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || !strings.HasPrefix(created.Secret, "vamos_machine_") {
		t.Fatalf("unexpected raw secret %q", created.Secret)
	}
	if created.Credential.ID == "" {
		t.Fatal("expected credential id")
	}
	if string(created.Credential.SecretHash) == created.Secret {
		t.Fatal("secret hash must not be raw secret")
	}

	authenticated, err := store.Authenticate(ctx, created.Credential.ID, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID != created.Credential.ID {
		t.Fatalf("authenticated id = %q", authenticated.ID)
	}
	if authenticated.LastUsedAt == nil {
		t.Fatal("expected last used timestamp")
	}
	if VerifyMachineSecret("wrong", authenticated.SecretHash) {
		t.Fatal("returned credential should not verify wrong secret")
	}
	if _, err := store.Authenticate(ctx, created.Credential.ID, "wrong"); err == nil {
		t.Fatal("expected wrong secret to fail")
	}

	credentials, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 1 {
		t.Fatalf("credential count = %d", len(credentials))
	}
	if credentials[0].ID != created.Credential.ID {
		t.Fatalf("listed id = %q", credentials[0].ID)
	}
	if string(credentials[0].SecretHash) == created.Secret {
		t.Fatal("list must not expose raw secret")
	}
}

func TestMachineCredentialPolicyAllowsDefaultActor(t *testing.T) {
	credential := MachineCredential{
		DefaultActorEmail:  "lead@example.test",
		AllowedActorEmails: []string{"lead@example.test"},
		AllowedSlugs:       []string{"stage"},
		AllowedPurposes:    []agentbrowser.Purpose{agentbrowser.PurposeE2EPlaywright},
	}
	if err := credential.Allows(BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeE2EPlaywright}); err != nil {
		t.Fatal(err)
	}
}

func TestMachineCredentialPolicyDeniesDisallowedFields(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	revoked := time.Now()
	base := MachineCredential{
		DefaultActorEmail:  "lead@example.test",
		AllowedActorEmails: []string{"lead@example.test"},
		AllowedSlugs:       []string{"stage"},
		AllowedPurposes:    []agentbrowser.Purpose{agentbrowser.PurposeE2EPlaywright},
	}

	tests := []struct {
		name       string
		credential MachineCredential
		req        BrowserTokenRequest
	}{
		{
			name:       "email",
			credential: base,
			req:        BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeE2EPlaywright, Email: "other@example.test"},
		},
		{
			name:       "slug",
			credential: base,
			req:        BrowserTokenRequest{Slug: "main", Purpose: agentbrowser.PurposeE2EPlaywright},
		},
		{
			name:       "purpose",
			credential: base,
			req:        BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeHermesChat},
		},
		{
			name:       "revoked",
			credential: MachineCredential{DefaultActorEmail: "lead@example.test", RevokedAt: &revoked},
			req:        BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeE2EPlaywright},
		},
		{
			name:       "expired",
			credential: MachineCredential{DefaultActorEmail: "lead@example.test", ExpiresAt: &expired},
			req:        BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeE2EPlaywright},
		},
		{
			name:       "missing-email",
			credential: MachineCredential{},
			req:        BrowserTokenRequest{Slug: "stage", Purpose: agentbrowser.PurposeE2EPlaywright},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.credential.Allows(tt.req); err == nil {
				t.Fatal("expected policy denial")
			}
		})
	}
}

func TestMemoryMachineCredentialStoreRevokeAndExpire(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryMachineCredentialStore()
	created, err := store.Create(ctx, CreateMachineCredentialInput{DefaultActorEmail: "lead@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, created.Credential.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, created.Credential.ID, created.Secret); err == nil {
		t.Fatal("expected revoked credential to fail")
	}

	expiresAt := time.Now().Add(-time.Minute)
	expired, err := store.Create(ctx, CreateMachineCredentialInput{DefaultActorEmail: "lead@example.test", ExpiresAt: &expiresAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, expired.Credential.ID, expired.Secret); err == nil {
		t.Fatal("expected expired credential to fail")
	}
}
