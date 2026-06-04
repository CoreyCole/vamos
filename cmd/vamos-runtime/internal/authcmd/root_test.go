package authcmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	profile          Profile
	secret           string
	savedProfileName string
	savedProfile     Profile
	savedSecret      string
}

func (s *fakeStore) Save(profileName string, profile Profile, secret string) error {
	s.savedProfileName = profileName
	s.savedProfile = profile
	s.savedSecret = secret
	return nil
}

func (s *fakeStore) Load(profileName string) (Profile, string, error) {
	return s.profile, s.secret, nil
}

type fakeClient struct {
	keyID        string
	secret       string
	req          MintRequest
	statusCalled bool
}

func (c *fakeClient) MintBrowserToken(ctx context.Context, keyID, secret string, req MintRequest) (MintResponse, error) {
	c.keyID = keyID
	c.secret = secret
	c.req = req
	return MintResponse{Token: "minted-token", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (c *fakeClient) Status(ctx context.Context, keyID, secret string, req MintRequest) error {
	c.statusCalled = true
	_, err := c.MintBrowserToken(ctx, keyID, secret, req)
	return err
}

func TestPlaywrightEnvPrintsBothExports(t *testing.T) {
	store := &fakeStore{profile: Profile{ManagerURL: "https://manager.test", KeyID: "mc_1"}, secret: "secret"}
	client := &fakeClient{}
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Store: store, Client: client, Out: &out})
	cmd.SetArgs([]string{"playwright-env", "--slug", "stage", "--email", "lead@example.test", "--ttl", "2m"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, name := range []string{"VAMOS_E2E_AUTH_TOKEN", "VAMOS_PLAYWRIGHT_AUTH_TOKEN"} {
		if !strings.Contains(got, "export "+name+"=\"minted-token\"") {
			t.Fatalf("missing %s export in %q", name, got)
		}
	}
	if client.keyID != "mc_1" || client.secret != "secret" {
		t.Fatalf("client credential = %q/%q", client.keyID, client.secret)
	}
	if client.req.Slug != "stage" || client.req.Purpose != purposeE2EPlaywright || client.req.Email != "lead@example.test" || client.req.RedirectPath != "/" || client.req.TTLSeconds != 120 {
		t.Fatalf("unexpected mint request: %+v", client.req)
	}
}

func TestLoginMachineWritesCredentialFile0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Store: FileCredentialStore{Path: path}, Out: &out})
	cmd.SetArgs([]string{"login-machine", "--manager-url", "https://manager.test/", "--key-id", "mc_1", "--secret", "secret"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credential file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	store := FileCredentialStore{Path: path}
	profile, secret, err := store.Load("default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if profile.ManagerURL != "https://manager.test" || profile.KeyID != "mc_1" || secret != "secret" {
		t.Fatalf("loaded profile = %+v secret=%q", profile, secret)
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatalf("login output leaked secret: %q", out.String())
	}
}

func TestResolveManagerURLPrecedence(t *testing.T) {
	t.Setenv("VAMOS_WORKSPACE_MANAGER_URL", "https://env.test/")
	if got, err := ResolveManagerURL("https://flag.test/", "", Profile{ManagerURL: "https://profile.test"}); err != nil || got != "https://flag.test" {
		t.Fatalf("explicit = %q, %v", got, err)
	}
	if got, err := ResolveManagerURL("", "", Profile{ManagerURL: "https://profile.test"}); err != nil || got != "https://env.test" {
		t.Fatalf("env = %q, %v", got, err)
	}

	t.Setenv("VAMOS_WORKSPACE_MANAGER_URL", "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".vamos", "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".vamos", "run", "workspace.env"), []byte("VAMOS_WORKSPACE_MANAGER_URL='https://workspace.test/'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveManagerURL("", child, Profile{ManagerURL: "https://profile.test"}); err != nil || got != "https://workspace.test" {
		t.Fatalf("workspace env = %q, %v", got, err)
	}

	other := t.TempDir()
	if got, err := ResolveManagerURL("", other, Profile{ManagerURL: "https://profile.test/"}); err != nil || got != "https://profile.test" {
		t.Fatalf("profile = %q, %v", got, err)
	}
}

func TestStatusUsesStoredCredential(t *testing.T) {
	store := &fakeStore{profile: Profile{ManagerURL: "https://manager.test", KeyID: "mc_1"}, secret: "secret"}
	client := &fakeClient{}
	var out bytes.Buffer
	cmd := newCommand(commandDeps{Store: store, Client: client, Out: &out})
	cmd.SetArgs([]string{"status", "--slug", "stage"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !client.statusCalled || client.keyID != "mc_1" || client.secret != "secret" {
		t.Fatalf("status client not called with stored credential: %+v", client)
	}
	if strings.TrimSpace(out.String()) != "ok" {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestClientMintBrowserToken(t *testing.T) {
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/agent-auth/mint-browser-token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer vamos_machine_mc_1.secret" {
			t.Fatalf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"browser-token","expires_at":"2026-06-04T12:00:00Z"}`))
	})
	ts := httptest.NewServer(server)
	defer ts.Close()
	resp, err := (Client{HTTPClient: ts.Client(), ManagerURL: ts.URL}).MintBrowserToken(context.Background(), "mc_1", "secret", MintRequest{Slug: "stage", Purpose: purposeE2EPlaywright})
	if err != nil {
		t.Fatalf("MintBrowserToken: %v", err)
	}
	if resp.Token != "browser-token" {
		t.Fatalf("token = %q", resp.Token)
	}
}
