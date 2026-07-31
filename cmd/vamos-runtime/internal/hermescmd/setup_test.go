package hermescmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupPersistsNormalizedGatewayBaseAndChecksHealth(t *testing.T) {
	requestPath := ""
	requestMethod := ""
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			requestPath = request.URL.Path
			requestMethod = request.Method
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	cmd := newSetupCommand()
	cmd.SetArgs([]string{
		"--gateway-url", server.URL + "/",
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
	if config.GatewayURL != server.URL {
		t.Fatalf(
			"gateway URL = %q, want normalized base %q",
			config.GatewayURL,
			server.URL,
		)
	}
	if requestMethod != http.MethodGet || requestPath != "/health" {
		t.Fatalf(
			"verification request = %s %s, want GET /health",
			requestMethod,
			requestPath,
		)
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

func TestSetupRejectsUnhealthyAdapter(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/health" {
				t.Errorf("request path = %q, want /health", request.URL.Path)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}),
	)
	defer server.Close()

	cmd := newSetupCommand()
	cmd.SetArgs([]string{
		"--gateway-url", server.URL,
		"--vamos-url", server.URL,
		"--ingress-token", "ingress-secret",
		"--callback-token", "callback-secret",
		"--config", filepath.Join(t.TempDir(), "hermes.yaml"),
	})
	if err := cmd.Execute(); err == nil ||
		!strings.Contains(err.Error(), "GET /health: 503 Service Unavailable") {
		t.Fatalf("error = %v, want rejected health status", err)
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
