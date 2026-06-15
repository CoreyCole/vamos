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
	"time"
)

func TestTryPrintWorkspacePreflightNoWorkspaceEnvNoop(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handled, err := TryPrintWorkspacePreflight(t.Context(), WorkspaceDiagnosticsOptions{CheckoutPath: t.TempDir(), Stdout: &out})
	if err != nil {
		t.Fatalf("TryPrintWorkspacePreflight: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false")
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestTryPrintWorkspacePreflightFetchesManagerDiagnostics(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	var saw bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = true
		if r.URL.Path != "/internal/workspaces/feature/diagnostics" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("project_id"); got != "github.com/coreycole/vamos" {
			t.Errorf("project_id = %q", got)
		}
		if got := r.Header.Get("X-Vamos-Workspace-Restart-Token"); got != "secret" {
			t.Errorf("token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspace":      map[string]any{"slug": "feature", "status": "crashed", "checkout_path": checkout},
			"runtime_status": map[string]any{"status": "crashed"},
			"lifecycle_diagnostic": map[string]any{
				"project_id":       "github.com/coreycole/vamos",
				"workspace_slug":   "feature",
				"checkout_path":    checkout,
				"lifecycle":        "merged",
				"lifecycle_source": "manager_db_lifecycle",
				"runtime_status":   "crashed",
				"runtime_source":   "local_runtime_diagnostics",
				"sync": map[string]any{
					"status":           "ok",
					"last_finished_at": time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano),
				},
				"diagnostics": []map[string]any{{
					"severity": "warning",
					"message":  "Local runtime diagnostics may be stale for this non-active workspace.",
				}},
				"cleanup_message": "Cleanup requires human approval. Do not clean up or delete this checkout unless explicitly approved.",
			},
		})
	}))
	defer server.Close()
	writeWorkspaceMetadataWithProject(t, checkout, "feature", "github.com/coreycole/vamos", checkout, server.URL, "secret")

	var out bytes.Buffer
	handled, err := TryPrintWorkspacePreflight(t.Context(), WorkspaceDiagnosticsOptions{CheckoutPath: checkout, Stdout: &out})
	if err != nil {
		t.Fatalf("TryPrintWorkspacePreflight: %v", err)
	}
	if !handled || !saw {
		t.Fatalf("handled=%v saw=%v, want true true", handled, saw)
	}
	text := out.String()
	for _, want := range []string{
		"Workspace diagnostics (preflight)",
		"Manager lifecycle: merged (source: manager DB)",
		"Scheduled sync: ok",
		"warnings: 1",
		"Local runtime diagnostics: crashed (source: .vamos/run/status.json; diagnostic only)",
		"Cleanup: Cleanup requires human approval",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestTryPrintWorkspacePreflightMissingProjectIDFallsBackLocal(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, "https://manager.test", "secret")
	writeRuntimeStatus(t, checkout, "crashed")
	var out bytes.Buffer
	handled, err := TryPrintWorkspacePreflight(t.Context(), WorkspaceDiagnosticsOptions{CheckoutPath: checkout, Stdout: &out})
	if err != nil {
		t.Fatalf("TryPrintWorkspacePreflight: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	text := out.String()
	if !strings.Contains(text, "workspace.env missing VAMOS_WORKSPACE_PROJECT_ID") || !strings.Contains(text, "Local runtime diagnostics: crashed") {
		t.Fatalf("output = %s", text)
	}
}

func TestTryPrintWorkspacePreflightManagerTimeoutFallsBackLocal(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()
	writeWorkspaceMetadataWithProject(t, checkout, "feature", "github.com/coreycole/vamos", checkout, server.URL, "secret")
	writeRuntimeStatus(t, checkout, "crashed")

	var out bytes.Buffer
	handled, err := TryPrintWorkspacePreflight(t.Context(), WorkspaceDiagnosticsOptions{
		CheckoutPath: checkout,
		Stdout:       &out,
		Timeout:      time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("TryPrintWorkspacePreflight: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	text := out.String()
	if !strings.Contains(text, "Manager lifecycle unavailable") || !strings.Contains(text, "Local runtime diagnostics: crashed") {
		t.Fatalf("output = %s", text)
	}
}

func writeWorkspaceMetadataWithProject(t *testing.T, checkout, slug, projectID, checkoutPath, managerURL, token string) {
	t.Helper()
	content := "VAMOS_WORKSPACE_SLUG=" + slug + "\n" +
		"VAMOS_WORKSPACE_PROJECT_ID=" + projectID + "\n" +
		"VAMOS_WORKSPACE_CHECKOUT=" + checkoutPath + "\n" +
		"VAMOS_WORKSPACE_MANAGER_URL=" + managerURL + "\n" +
		"VAMOS_WORKSPACE_RESTART_TOKEN=" + token + "\n"
	path := workspaceEnvPath(checkout)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(metadata dir): %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(metadata): %v", err)
	}
}

func writeRuntimeStatus(t *testing.T, checkout, status string) {
	t.Helper()
	path := workspaceRuntimePaths(checkout).statusJSON
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(status dir): %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"status":"`+status+`"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(status): %v", err)
	}
}
