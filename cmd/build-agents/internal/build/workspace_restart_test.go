package build

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWorkspaceMetadata(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	path := workspaceEnvPath(checkout)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "VAMOS_WORKSPACE_SLUG=feature\n" +
		"VAMOS_WORKSPACE_CHECKOUT='/tmp/vamos feature'\n" +
		"VAMOS_WORKSPACE_MANAGER_URL=https://main.vamos.test\n" +
		"VAMOS_WORKSPACE_RESTART_TOKEN='tok'\\''en'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := readWorkspaceMetadata(path)
	if err != nil {
		t.Fatalf("readWorkspaceMetadata: %v", err)
	}
	if got.Slug != "feature" || got.CheckoutPath != "/tmp/vamos feature" ||
		got.ManagerURL != "https://main.vamos.test" || got.RestartToken != "tok'en" {
		t.Fatalf("metadata = %+v", got)
	}
}

func TestTryWorkspaceRestartMissingMetadataReturnsUnhandled(t *testing.T) {
	t.Parallel()

	handled, err := tryWorkspaceRestart(
		t.Context(),
		WorkspaceRestartOptions{CheckoutPath: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("tryWorkspaceRestart: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false")
	}
}

func TestTryWorkspaceRestartIgnoresLegacyRootMetadata(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(checkout, ".vamos-workspace.env"),
		[]byte("VAMOS_WORKSPACE_SLUG=feature\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile(legacy metadata): %v", err)
	}
	handled, err := tryWorkspaceRestart(
		t.Context(),
		WorkspaceRestartOptions{CheckoutPath: checkout},
	)
	if err != nil {
		t.Fatalf("tryWorkspaceRestart: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for legacy metadata")
	}
}

func TestTryWorkspaceRestartPostsRestartRequest(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	var saw bool
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			saw = true
			if r.URL.Path != "/internal/workspaces/restart" {
				t.Errorf("path = %s", r.URL.Path)
			}
			if r.Header.Get("X-Vamos-Workspace-Restart-Token") != "secret" {
				t.Errorf(
					"token header = %q",
					r.Header.Get("X-Vamos-Workspace-Restart-Token"),
				)
			}
			var body struct {
				Slug         string   `json:"slug"`
				CheckoutPath string   `json:"checkout_path"`
				Components   []string `json:"components"`
				Force        bool     `json:"force"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Decode: %v", err)
			}
			if body.Slug != "feature" || body.CheckoutPath != checkout {
				t.Errorf("body = %#v", body)
			}
			if got, want := body.Components, []string{
				"web",
				"ts_worker",
			}; len(got) != len(want) || got[0] != want[0] ||
				got[1] != want[1] {
				t.Errorf("components = %#v, want %#v", got, want)
			}
			if body.Force {
				t.Error("force = true, want false")
			}
			w.WriteHeader(http.StatusAccepted)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")

	handled, err := tryWorkspaceRestart(
		t.Context(),
		WorkspaceRestartOptions{
			CheckoutPath: checkout,
			Components:   []string{"web", "ts_worker"},
		},
	)
	if err != nil {
		t.Fatalf("tryWorkspaceRestart: %v", err)
	}
	if !handled || !saw {
		t.Fatalf("handled=%t saw=%t, want true/true", handled, saw)
	}
}

func TestTryWorkspaceRestartPrintsWorkspaceURLFromManagerResponse(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write(
				[]byte(`{"Slug":"feature","URL":"https://feature.vamos.test/"}`),
			)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")

	var out bytes.Buffer
	handled, err := tryWorkspaceRestart(
		t.Context(),
		WorkspaceRestartOptions{CheckoutPath: checkout, Stdout: &out},
	)
	if err != nil {
		t.Fatalf("tryWorkspaceRestart: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if want := "workspace URL: https://feature.vamos.test/"; !strings.Contains(
		out.String(),
		want,
	) {
		t.Fatalf("stdout missing %q: %s", want, out.String())
	}
}

func TestTryWorkspaceRestartWithRecoveryForceSucceeds(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	attempts := []bool{}
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Force bool `json:"force"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Decode: %v", err)
			}
			attempts = append(attempts, body.Force)
			if !body.Force {
				http.Error(w, "graceful failed", http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")

	var out bytes.Buffer
	result, err := TryWorkspaceRestartWithRecovery(
		t.Context(),
		WorkspaceRestartOptions{
			CheckoutPath: checkout,
			Components:   []string{"web"},
			Stdout:       &out,
		},
	)
	if err != nil {
		t.Fatalf("TryWorkspaceRestartWithRecovery: %v", err)
	}
	if !result.Handled || !result.ForceAttempted || !result.ForceSucceeded {
		t.Fatalf("result = %+v", result)
	}
	if len(attempts) != 2 || attempts[0] || !attempts[1] {
		t.Fatalf("force attempts = %#v", attempts)
	}
	for _, want := range []string{
		"restart: graceful workspace restart failed",
		"--- attempting forceful workspace restart ---",
		"restart: vamos workspace restarted after force retry",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, out.String())
		}
	}
}

func TestTryWorkspaceRestartWithRecoveryForceFails(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Force bool `json:"force"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Force {
				http.Error(w, "force failed", http.StatusBadGateway)
				return
			}
			http.Error(w, "graceful failed", http.StatusBadGateway)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")

	var out bytes.Buffer
	result, err := TryWorkspaceRestartWithRecovery(
		t.Context(),
		WorkspaceRestartOptions{CheckoutPath: checkout, Stdout: &out},
	)
	if err == nil {
		t.Fatal("err = nil, want force failure")
	}
	if !result.Handled || !result.ForceAttempted || result.ForceSucceeded {
		t.Fatalf("result = %+v", result)
	}
	for _, want := range []string{
		"workspace graceful restart failed",
		"workspace force restart failed",
		"vamos ctl workspace doctor",
		"vamos ctl workspace logs web --tail 120",
		"vamos ctl workspace restart --force",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %s", want, err.Error())
		}
	}
}

func TestTryWorkspaceRestartBadStatusReturnsHandledError(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	webLog := filepath.Join(checkout, ".vamos", "log", "web.log")
	if err := os.MkdirAll(filepath.Dir(webLog), 0o755); err != nil {
		t.Fatalf("MkdirAll(web log): %v", err)
	}
	if err := os.WriteFile(
		webLog,
		[]byte("startup\npanic: runtime error: nil pointer\nstack line\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile(web log): %v", err)
	}
	statusPath := workspaceRuntimePaths(checkout).statusJSON
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(status): %v", err)
	}
	status := `{"status":"failed","error":"workspace crashed","logs":{"web":"` + webLog + `"}}`
	if err := os.WriteFile(statusPath, []byte(status), 0o600); err != nil {
		t.Fatalf("WriteFile(status): %v", err)
	}
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "restart denied", http.StatusUnauthorized)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")

	handled, err := tryWorkspaceRestart(
		t.Context(),
		WorkspaceRestartOptions{CheckoutPath: checkout},
	)
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if err == nil {
		t.Fatal("err = nil, want bad status error")
	}
	errText := err.Error()
	for _, want := range []string{
		"workspace restart API returned 401 Unauthorized",
		"response body:\nrestart denied",
		"status: failed",
		"error: workspace crashed",
		"panic: runtime error: nil pointer",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error %q does not contain %q", errText, want)
		}
	}
}

func TestFindCheckoutRootFromPackageDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pkg := filepath.Join(root, "pkg", "agents")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(pkg, "go.mod"),
		[]byte("module test\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := findCheckoutRoot(pkg); got != root {
		t.Fatalf("findCheckoutRoot(%q) = %q, want %q", pkg, got, root)
	}
}

func writeWorkspaceMetadata(
	t *testing.T,
	checkout, slug, metaCheckout, managerURL, token string,
) {
	t.Helper()
	content := "VAMOS_WORKSPACE_SLUG=" + slug + "\n" +
		"VAMOS_WORKSPACE_CHECKOUT=" + metaCheckout + "\n" +
		"VAMOS_WORKSPACE_MANAGER_URL=" + managerURL + "\n" +
		"VAMOS_WORKSPACE_RESTART_TOKEN=" + token + "\n"
	path := workspaceEnvPath(checkout)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(metadata dir): %v", err)
	}
	if err := os.WriteFile(
		path,
		[]byte(content),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile(metadata): %v", err)
	}
}
