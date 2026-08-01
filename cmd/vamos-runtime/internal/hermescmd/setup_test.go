package hermescmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const setupHealthPath = "/health"

func TestSetupPersistsNormalizedGatewayBaseAndChecksCapability(t *testing.T) {
	t.Parallel()
	requests := make([]string, 0, 2)
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			requests = append(requests, request.Method+" "+request.URL.Path)
			if request.URL.Path == setupHealthPath {
				w.WriteHeader(http.StatusOK)

				return
			}
			if request.Header.Get("Authorization") != "Bearer ingress-secret" {
				t.Errorf("authorization header was not the ingress credential")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sessioningress.CapabilityResponse{
				Capabilities: []string{sessioningress.ExactSessionCapability},
				Code:         "capabilities", MaxFrameBytes: sessioningress.MaxFrameBytes,
				ProtocolVersions: []int{sessioningress.ProtocolVersion},
				Version:          sessioningress.ProtocolVersion,
			})
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
	wantRequests := []string{
		"GET /health",
		"POST /vamos/session-ingress/v1/capabilities",
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("verification requests = %#v, want %#v", requests, wantRequests)
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
			if request.URL.Path != setupHealthPath {
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

func TestSetupRejectsHealthyAdapterWithoutExactSessionCapability(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == setupHealthPath {
				w.WriteHeader(http.StatusOK)

				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(sessioningress.CapabilityResponse{
				Capabilities: []string{"other-capability"}, Code: "capabilities",
				MaxFrameBytes:    sessioningress.MaxFrameBytes,
				ProtocolVersions: []int{sessioningress.ProtocolVersion},
				Version:          sessioningress.ProtocolVersion,
			})
		}),
	)
	defer server.Close()

	cmd := newSetupCommand()
	cmd.SetArgs([]string{
		"--gateway-url", server.URL, "--vamos-url", server.URL,
		"--ingress-token", "ingress-secret", "--callback-token", "callback-secret",
		"--config", filepath.Join(t.TempDir(), "hermes.yaml"),
	})
	if err := cmd.Execute(); err == nil ||
		!strings.Contains(
			err.Error(),
			"healthy but exact-session capability preflight failed",
		) {
		t.Fatalf("error = %v, want capability failure distinct from health", err)
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
