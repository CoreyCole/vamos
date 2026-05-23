package workspacecmd

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

func TestLoadConfigWalksUpFromPkgAgents(t *testing.T) {
	root := writeWorkspaceFixture(t)

	cfg, err := LoadConfig(filepath.Join(root, "pkg", "agents"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.CheckoutPath != root || cfg.Metadata.CheckoutPath != root {
		t.Fatalf(
			"checkout paths = %q / %q, want %q",
			cfg.CheckoutPath,
			cfg.Metadata.CheckoutPath,
			root,
		)
	}
	if cfg.Metadata.Slug != "feature" ||
		cfg.ManagerURL != "https://main.cn-agents.test" ||
		cfg.RestartToken != "secret" {
		t.Fatalf("metadata = %+v", cfg.Metadata)
	}
	if cfg.Status.Status != "running" || cfg.Status.Ports["web"] != 4217 ||
		cfg.Status.PIDs["web"] != 1234 {
		t.Fatalf("status = %+v", cfg.Status)
	}
}

func TestLoadConfigMissingEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "pkg", "agents", "go.mod"),
		[]byte("module example\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(filepath.Join(root, "pkg", "agents"))
	if err == nil || !strings.Contains(err.Error(), "not a managed workspace checkout") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestRunStatus(t *testing.T) {
	cfg := testConfig(t)
	var out bytes.Buffer
	if err := RunStatus(t.Context(), cfg, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	for _, want := range []string{
		"slug: feature",
		"checkout: " + cfg.CheckoutPath,
		"manager_url: https://main.cn-agents.test",
		"status: running",
		"phase: ready",
		"ports.web: 4217",
		"pids.web: 1234",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunLogsTailsSelectedLog(t *testing.T) {
	cfg := testConfig(t)
	var out bytes.Buffer
	if err := RunLogs(t.Context(), cfg, WorkspaceLogWeb, 2, &out); err != nil {
		t.Fatalf("RunLogs: %v", err)
	}
	if strings.Contains(out.String(), "line1") ||
		!strings.Contains(out.String(), "line2\nline3") {
		t.Fatalf("log output = %q", out.String())
	}
}

func TestMainLogsAcceptsTailAfterTarget(t *testing.T) {
	cfg := testConfig(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(filepath.Join(cfg.CheckoutPath, "pkg", "agents")); err != nil {
		t.Fatal(err)
	}
	if err := Main([]string{"logs", "web", "--tail", "1"}); err != nil {
		t.Fatalf("Main logs: %v", err)
	}
}

func TestRunLogsRejectsPathOutsideWorkspaceLogDir(t *testing.T) {
	cfg := testConfig(t)
	cfg.Status.Logs["web"] = filepath.Join(cfg.CheckoutPath, "outside.log")
	var out bytes.Buffer
	err := RunLogs(t.Context(), cfg, WorkspaceLogWeb, 2, &out)
	if err == nil ||
		!strings.Contains(err.Error(), "outside managed workspace log directory") {
		t.Fatalf("RunLogs error = %v", err)
	}
}

func TestRunRestartPostsTokenComponentsAndForce(t *testing.T) {
	cfg := testConfig(t)
	type restartRequest struct {
		Slug         string   `json:"slug"`
		CheckoutPath string   `json:"checkout_path"`
		Components   []string `json:"components"`
		Force        bool     `json:"force"`
	}
	var got restartRequest
	var gotToken string
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/internal/workspaces/restart" {
				t.Errorf("path = %s", r.URL.Path)
			}
			gotToken = r.Header.Get("X-CN-Agents-Workspace-Restart-Token")
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("Decode: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}),
	)
	defer server.Close()
	cfg.ManagerURL = server.URL
	cfg.Metadata.ManagerURL = server.URL

	var out bytes.Buffer
	if err := RunRestart(
		t.Context(),
		cfg,
		[]string{"web", "ts_worker"},
		true,
		&out,
	); err != nil {
		t.Fatalf("RunRestart: %v", err)
	}
	if gotToken != "secret" || got.Slug != "feature" ||
		got.CheckoutPath != cfg.Metadata.CheckoutPath ||
		!got.Force {
		t.Fatalf("request token=%q body=%+v", gotToken, got)
	}
	if len(got.Components) != 2 || got.Components[0] != "web" ||
		got.Components[1] != "ts_worker" {
		t.Fatalf("components = %#v", got.Components)
	}
	if !strings.Contains(out.String(), "restart accepted: 202 Accepted") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestComponentsFromWorkspaceCLIFlags(t *testing.T) {
	got, err := componentsFromWorkspaceCLIFlags([]string{"web", "ts-worker", "ts_worker"})
	if err != nil {
		t.Fatalf("componentsFromWorkspaceCLIFlags: %v", err)
	}
	want := []string{"web", "ts_worker", "ts_worker"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("components = %#v, want %#v", got, want)
	}
	if _, err := componentsFromWorkspaceCLIFlags(
		[]string{"temporal"},
	); err == nil ||
		!strings.Contains(err.Error(), "allowed") {
		t.Fatalf("unknown component error = %v", err)
	}
}

func testConfig(t *testing.T) WorkspaceCLIConfig {
	t.Helper()
	root := writeWorkspaceFixture(t)
	cfg, err := LoadConfig(filepath.Join(root, "pkg", "agents"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

func writeWorkspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "pkg", "agents", "go.mod"),
		[]byte("module example\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, ".cn-agents", "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(root, ".cn-agents", "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	webLog := filepath.Join(logDir, "web.log")
	if err := os.WriteFile(webLog, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "CN_AGENTS_WORKSPACE_SLUG=feature\n" +
		"CN_AGENTS_WORKSPACE_CHECKOUT=" + root + "\n" +
		"CN_AGENTS_WORKSPACE_MANAGER_URL=https://main.cn-agents.test\n" +
		"CN_AGENTS_WORKSPACE_RESTART_TOKEN=secret\n"
	if err := os.WriteFile(
		filepath.Join(runDir, "workspace.env"),
		[]byte(env),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	status := map[string]any{
		"status": "running",
		"phase":  "ready",
		"logs":   map[string]string{"web": webLog},
		"ports":  map[string]int{"web": 4217},
		"pids":   map[string]int{"web": 1234},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "status.json"),
		data,
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	return root
}
