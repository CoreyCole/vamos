package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func TestSQLMachineCredentialStorePersistsAuthenticateListAndRevoke(t *testing.T) {
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	svc, err := dbsvc.NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	store := NewSQLMachineCredentialStore(svc.Queries)
	created, err := store.Create(ctx, CreateMachineCredentialInput{
		Name:               "Agent Laptop",
		DefaultActorEmail:  "lead@example.test",
		AllowedActorEmails: []string{"lead@example.test"},
		AllowedSlugs:       []string{"stage"},
		AllowedPurposes:    []agentbrowser.Purpose{agentbrowser.PurposeE2EPlaywright},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Secret == "" || created.Credential.ID == "" || string(created.Credential.SecretHash) == created.Secret {
		t.Fatalf("bad created credential: %+v secret=%q", created.Credential, created.Secret)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := dbsvc.NewService(dbPath)
	if err != nil {
		t.Fatalf("reopen NewService: %v", err)
	}
	defer reopened.Close()
	reopenedStore := NewSQLMachineCredentialStore(reopened.Queries)
	authenticated, err := reopenedStore.Authenticate(ctx, created.Credential.ID, created.Secret)
	if err != nil {
		t.Fatalf("Authenticate after reopen: %v", err)
	}
	if authenticated.ID != created.Credential.ID || authenticated.LastUsedAt == nil {
		t.Fatalf("authenticated = %+v", authenticated)
	}
	if _, err := reopenedStore.Authenticate(ctx, created.Credential.ID, "wrong"); err == nil {
		t.Fatal("expected wrong secret failure")
	}
	listed, err := reopenedStore.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.Credential.ID || string(listed[0].SecretHash) == created.Secret {
		t.Fatalf("listed = %+v", listed)
	}
	if err := reopenedStore.Revoke(ctx, created.Credential.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := reopenedStore.Authenticate(ctx, created.Credential.ID, created.Secret); err == nil {
		t.Fatal("expected revoked credential failure")
	}
}

func TestSQLMachineCredentialStoreRejectsExpiredCredential(t *testing.T) {
	ctx := t.Context()
	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()
	expiresAt := time.Now().Add(-time.Minute)
	store := NewSQLMachineCredentialStore(svc.Queries)
	created, err := store.Create(ctx, CreateMachineCredentialInput{DefaultActorEmail: "lead@example.test", ExpiresAt: &expiresAt})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.Authenticate(ctx, created.Credential.ID, created.Secret); err == nil {
		t.Fatal("expected expired credential failure")
	}
}
