package hermescmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupPersistsIngressAndCallbackCredentialsSeparately(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	cmd := newSetupCommand()
	cmd.SetArgs([]string{
		"--gateway-url", server.URL,
		"--vamos-url", server.URL,
		"--ingress-token", "ingress-secret",
		"--callback-token", "callback-secret",
		"--config", configPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	config, err := readHostConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.IngressToken != "ingress-secret" ||
		config.CallbackToken != "callback-secret" {
		t.Fatalf("credentials were not persisted separately")
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", info.Mode())
	}
}

func TestSetupRequiresIngressToken(t *testing.T) {
	cmd := newSetupCommand()
	cmd.SetArgs([]string{
		"--gateway-url", "https://gateway.example",
		"--vamos-url", "https://vamos.example",
		"--callback-token", "callback-secret",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("setup accepted missing ingress token")
	}
}
