package authcmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	serverauth "github.com/CoreyCole/vamos/server/services/auth"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func TestCreateMachineKeyWritesDBAndPrintsSecretOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Out: &out})
	cmd.SetArgs([]string{"create-machine-key", "--database-path", dbPath, "--manager-url", "https://main.test", "--name", "laptop", "--email", "agent@example.test", "--slug", "stage", "--purpose", purposeE2EPlaywright, "--purpose", purposeHermesChat, "--purpose", purposeVerify})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	if strings.Count(got, "vamos_machine_") != 2 { // secret line + printed login command
		t.Fatalf("expected secret only in create output and login command, got %q", got)
	}
	keyID := extractOutputValue(t, got, "key_id:")
	secret := extractOutputValue(t, got, "secret:")
	svc, err := dbsvc.NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()
	credential, err := serverauth.NewSQLMachineCredentialStore(svc.Queries).Authenticate(t.Context(), keyID, secret)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if credential.DefaultActorEmail != "agent@example.test" || len(credential.AllowedPurposes) != 3 || string(credential.AllowedPurposes[2]) != purposeVerify {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestCreateMachineKeySaveProfileAndListOmitSecrets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	profilePath := filepath.Join(t.TempDir(), "credentials.json")
	store := FileCredentialStore{Path: profilePath}
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Store: store, Out: &out})
	cmd.SetArgs([]string{"create-machine-key", "--database-path", dbPath, "--manager-url", "https://main.test", "--email", "agent@example.test", "--save-profile"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute create: %v", err)
	}
	keyID := extractOutputValue(t, out.String(), "key_id:")
	secret := extractOutputValue(t, out.String(), "secret:")
	profile, storedSecret, err := store.Load("default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if profile.ManagerURL != "https://main.test" || profile.KeyID != keyID || storedSecret != secret {
		t.Fatalf("profile = %+v secret=%q", profile, storedSecret)
	}

	out.Reset()
	cmd = newCommand(commandDeps{Out: &out})
	cmd.SetArgs([]string{"list-machine-keys", "--database-path", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute list: %v", err)
	}
	if strings.Contains(out.String(), secret) || strings.Contains(out.String(), "secret_hash") {
		t.Fatalf("list leaked secret material: %q", out.String())
	}
}

func TestRevokeMachineKeyPreventsAuth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Out: &out})
	cmd.SetArgs([]string{"create-machine-key", "--database-path", dbPath, "--manager-url", "https://main.test", "--email", "agent@example.test"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute create: %v", err)
	}
	keyID := extractOutputValue(t, out.String(), "key_id:")
	secret := extractOutputValue(t, out.String(), "secret:")
	out.Reset()
	cmd = newCommand(commandDeps{Out: &out})
	cmd.SetArgs([]string{"revoke-machine-key", "--database-path", dbPath, "--key-id", keyID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute revoke: %v", err)
	}
	svc, err := dbsvc.NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()
	if _, err := serverauth.NewSQLMachineCredentialStore(svc.Queries).Authenticate(t.Context(), keyID, secret); err == nil {
		t.Fatal("expected revoked credential to fail")
	}
}

func TestResolveMachineKeyDatabasePathUsesEnv(t *testing.T) {
	t.Setenv("VAMOS_DATABASE_PATH", "/tmp/vamos-test-agents.db")
	if got, err := resolveMachineKeyDatabasePath(""); err != nil || got != "/tmp/vamos-test-agents.db" {
		t.Fatalf("env path = %q, %v", got, err)
	}
	if got, err := resolveMachineKeyDatabasePath(" /explicit.db "); err != nil || got != "/explicit.db" {
		t.Fatalf("explicit path = %q, %v", got, err)
	}
}

func TestParseMachineCredentialPurposeAcceptsVerify(t *testing.T) {
	purpose, err := parseMachineCredentialPurpose(purposeVerify)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(purpose) != purposeVerify {
		t.Fatalf("purpose = %q", purpose)
	}
}

func extractOutputValue(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("missing %s in %q", prefix, output)
	return ""
}
