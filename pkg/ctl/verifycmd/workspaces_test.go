package verifycmd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

const testRestartToken = "restart-secret"

func TestLoadWorkspaceVerifyConfigFromEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"VAMOS_PUBLIC_BASE_URL=https://main.vamos.test",
		"VAMOS_WORKSPACE_DOMAIN=vamos.test",
		"VAMOS_WORKSPACE_RESTART_TOKEN=" + testRestartToken,
		"VAMOS_PLAYWRIGHT_AUTH_TOKEN=playwright-secret",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWorkspaceVerifyConfig(
		[]string{"--env", envPath, "--slug", "demo", "--domain", "override.test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "override.test" {
		t.Fatalf("Domain = %q", cfg.Domain)
	}
	if cfg.BaseURL != "https://main.vamos.test" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.RestartToken != testRestartToken {
		t.Fatalf("RestartToken = %q", cfg.RestartToken)
	}
	if cfg.PlaywrightAuthToken != "playwright-secret" {
		t.Fatalf("PlaywrightAuthToken = %q", cfg.PlaywrightAuthToken)
	}
}

func TestRunServerLifecyclePhaseSendsRestartToken(t *testing.T) {
	t.Parallel()
	var posted workspaces.VerifyWorkspaceRequest
	polls := 0
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(
				"X-Vamos-Workspace-Restart-Token",
			); got != testRestartToken {
				t.Fatalf("restart token header = %q", got)
			}
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/internal/workspaces/verify":
				if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).
					Encode(workspaces.VerifyWorkspaceRun{ID: "run-1", Slug: posted.Slug, Status: workspaces.VerifyRunRunning})
			case r.Method == http.MethodGet && r.URL.Path == "/internal/workspaces/verify/run-1":
				polls++
				_ = json.NewEncoder(w).
					Encode(workspaces.VerifyWorkspaceRun{ID: "run-1", Slug: posted.Slug, Status: workspaces.VerifyRunPassed})
			default:
				http.NotFound(w, r)
			}
		}),
	)
	defer server.Close()

	run, err := RunServerLifecyclePhase(
		t.Context(),
		WorkspaceVerifyConfig{BaseURL: server.URL, RestartToken: testRestartToken},
		workspaces.VerifyWorkspaceRequest{Slug: "demo", Start: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != workspaces.VerifyRunPassed || posted.Slug != "demo" ||
		!posted.Start ||
		polls == 0 {
		t.Fatalf("run=%+v posted=%+v polls=%d", run, posted, polls)
	}
}

func TestBrowserAuthUsesPlaywrightTokenNotRestartToken(t *testing.T) {
	t.Parallel()
	var gotToken string
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotToken = r.Header.Get("X-Vamos-Workspace-Restart-Token")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).
				Encode(workspaces.VerifyWorkspaceRun{ID: "run-1", Status: workspaces.VerifyRunPassed})
		}),
	)
	defer server.Close()
	_, err := StartServerVerifyRun(
		t.Context(),
		WorkspaceVerifyConfig{
			BaseURL:             server.URL,
			RestartToken:        testRestartToken,
			PlaywrightAuthToken: "playwright-secret",
		},
		workspaces.VerifyWorkspaceRequest{Slug: "demo"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotToken != testRestartToken || gotToken == "playwright-secret" {
		t.Fatalf("server API token header = %q", gotToken)
	}
}

//nolint:paralleltest // Mutates package-level test hooks.
func TestRunWorkspaceVerifyBrowserOrdering(t *testing.T) {
	oldResolve, oldHTTPS, oldHost, oldRun, oldBrowser := resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runBrowserVerifyFn
	defer func() {
		resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runBrowserVerifyFn = oldResolve, oldHTTPS, oldHost, oldRun, oldBrowser
	}()
	resolveHostFn = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	probeHTTPSFn = func(ctx context.Context, rawURL string, out io.Writer) error {
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	var events []string
	probeHostPreservationFn = func(ctx context.Context, baseURL, childURL string, out io.Writer) error {
		events = append(events, "caddy-host-dispatch")
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	var requests []workspaces.VerifyWorkspaceRequest
	runServerLifecyclePhaseFn = func(ctx context.Context, cfg WorkspaceVerifyConfig, req workspaces.VerifyWorkspaceRequest) (workspaces.VerifyWorkspaceRun, error) {
		requests = append(requests, req)
		if req.Stop {
			events = append(events, "server-stop")
		} else {
			events = append(events, "server-start-restart")
		}
		return workspaces.VerifyWorkspaceRun{
			ID:     "run",
			Status: workspaces.VerifyRunPassed,
			Phases: []workspaces.VerifyWorkspacePhase{
				{
					Layer:  workspaces.VerificationLayerLifecycle,
					Status: workspaces.VerifyPhasePassed,
				},
			},
		}, nil
	}
	runBrowserVerifyFn = func(ctx context.Context, cfg BrowserVerifyConfig) (ClientVerifyStep, error) {
		if cfg.ExpectStopped {
			events = append(events, "browser-stopped")
			return ClientVerifyStep{
				Name:   "browser-unavailable-after-stop",
				Layer:  workspaces.VerificationLayerBrowser,
				Status: statusPassed,
			}, nil
		}
		events = append(events, "browser-running")
		return ClientVerifyStep{
			Name:   "browser",
			Layer:  workspaces.VerificationLayerBrowser,
			Status: statusPassed,
		}, nil
	}
	report, err := RunWorkspaceVerify(
		t.Context(),
		WorkspaceVerifyConfig{
			BaseURL:             "https://main.vamos.test",
			Domain:              "vamos.test",
			Slug:                "demo",
			RestartToken:        "restart",
			PlaywrightAuthToken: "playwright",
			Start:               true,
			Restart:             true,
			Stop:                true,
			Browser:             true,
			ReportDir:           t.TempDir(),
			Timeout:             time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests len = %d", len(requests))
	}
	if requests[0].Stop || !requests[0].Start || !requests[0].Restart {
		t.Fatalf("first request = %+v", requests[0])
	}
	if !requests[1].Stop || requests[1].Start || requests[1].Restart {
		t.Fatalf("second request = %+v", requests[1])
	}
	if got := strings.Join(
		events,
		",",
	); got != "server-start-restart,caddy-host-dispatch,browser-running,server-stop,browser-stopped" {
		t.Fatalf("events = %s", got)
	}
	if report.Summary.Layers[workspaces.VerificationLayerBrowser] != statusPassed {
		t.Fatalf(
			"browser layer = %q",
			report.Summary.Layers[workspaces.VerificationLayerBrowser],
		)
	}
}

//nolint:paralleltest // Mutates package-level test hooks.
func TestRunWorkspaceVerifyStopsAfterBrowserFailure(t *testing.T) {
	oldResolve, oldHTTPS, oldHost, oldRun, oldBrowser := resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runBrowserVerifyFn
	defer func() {
		resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runBrowserVerifyFn = oldResolve, oldHTTPS, oldHost, oldRun, oldBrowser
	}()
	resolveHostFn = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	probeHTTPSFn = func(ctx context.Context, rawURL string, out io.Writer) error {
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	probeHostPreservationFn = func(ctx context.Context, baseURL, childURL string, out io.Writer) error {
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	var requests []workspaces.VerifyWorkspaceRequest
	runServerLifecyclePhaseFn = func(ctx context.Context, cfg WorkspaceVerifyConfig, req workspaces.VerifyWorkspaceRequest) (workspaces.VerifyWorkspaceRun, error) {
		requests = append(requests, req)
		return workspaces.VerifyWorkspaceRun{
			ID:     "run",
			Status: workspaces.VerifyRunPassed,
		}, nil
	}
	runBrowserVerifyFn = func(ctx context.Context, cfg BrowserVerifyConfig) (ClientVerifyStep, error) {
		return ClientVerifyStep{
				Name:   "browser",
				Layer:  workspaces.VerificationLayerBrowser,
				Status: statusFailed,
			}, stringError(
				"browser failed",
			)
	}

	_, err := RunWorkspaceVerify(
		t.Context(),
		WorkspaceVerifyConfig{
			BaseURL:             "https://main.vamos.test",
			Domain:              "vamos.test",
			Slug:                "demo",
			RestartToken:        "restart",
			PlaywrightAuthToken: "playwright",
			Start:               true,
			Stop:                true,
			Browser:             true,
			ReportDir:           t.TempDir(),
			Timeout:             time.Second,
		},
	)
	if err == nil {
		t.Fatal("expected browser verification error")
	}
	if len(requests) != 2 {
		t.Fatalf("requests len = %d, want start plus cleanup stop", len(requests))
	}
	if !requests[1].Stop || requests[1].Start || requests[1].Restart {
		t.Fatalf("cleanup request = %+v", requests[1])
	}
}

//nolint:paralleltest // Mutates package-level command runner.
func TestRunBrowserVerifyCommandConstruction(t *testing.T) {
	oldRunner := runBrowserCommand
	defer func() { runBrowserCommand = oldRunner }()
	var gotScript string
	var gotArgs []string
	var gotOut string
	runBrowserCommand = func(ctx context.Context, script string, args []string, outPath string) error {
		gotScript = script
		gotArgs = append([]string(nil), args...)
		gotOut = outPath
		return os.WriteFile(outPath, []byte("ok"), 0o600)
	}
	step, err := RunBrowserVerify(t.Context(), BrowserVerifyConfig{
		BaseURL:   "https://main.vamos.test",
		Domain:    "vamos.test",
		Slug:      "demo",
		AuthToken: "playwright-secret",
		ReportDir: t.TempDir(),
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if step.Status != statusPassed || step.OutputPath == "" || gotOut != step.OutputPath {
		t.Fatalf("step=%+v gotOut=%q", step, gotOut)
	}
	if !strings.HasSuffix(
		gotScript,
		filepath.Join("scripts", "workspace-verify-playwright.mjs"),
	) {
		t.Fatalf("script = %q", gotScript)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"--base-url https://main.vamos.test", "--domain vamos.test", "--slug demo", "--token playwright-secret"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestPlaywrightScriptPath(t *testing.T) {
	t.Parallel()
	path, err := playwrightScriptPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(
		path,
		filepath.Join("scripts", "workspace-verify-playwright.mjs"),
	) {
		t.Fatalf("path = %q", path)
	}
}

func TestWriteWorkspaceVerifyReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	layers := make(map[workspaces.VerificationLayer]string)
	layers[workspaces.VerificationLayerConfig] = statusPassed
	layers[workspaces.VerificationLayerDNSRemoteSystem] = statusPassed
	layers[workspaces.VerificationLayerTLSRemote] = statusPassed
	report := WorkspaceVerifyReport{
		Summary: ReportSummary{
			Status: statusPassed,
			Slug:   "demo",
			Scope:  "test",
			Layers: layers,
		},
		ServerRuns: []workspaces.VerifyWorkspaceRun{{
			ID: "run",
			Snapshots: []workspaces.VerifyWorkspaceSnapshot{{
				Metadata: &workspaces.WorkspaceMetadata{RestartToken: "restart-secret"},
			}},
			Diagnostics: workspaces.WorkspaceDiagnostics{
				Metadata: &workspaces.WorkspaceMetadata{RestartToken: "restart-secret"},
			},
		}},
		ClientSteps: []ClientVerifyStep{
			{
				Name:   "dns-remote-system-main",
				Layer:  workspaces.VerificationLayerDNSRemoteSystem,
				Status: statusPassed,
			},
			{
				Name:   "remote-curl-main-https",
				Layer:  workspaces.VerificationLayerTLSRemote,
				Status: statusPassed,
			},
		},
		Artifacts: map[string]string{"report_dir": dir},
	}
	if err := WriteWorkspaceVerifyReport(report, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"summary.json", "summary.md", "server-runs.json", "manager-log-tail.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "```workspace-verification") ||
		!strings.Contains(string(data), "`config`: passed") ||
		!strings.Contains(string(data), "`dns-remote-system`: passed") ||
		!strings.Contains(string(data), "`tls-remote`: passed") {
		t.Fatalf("summary.md missing expected content:\n%s", data)
	}
	jsonData, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dns-remote-system", "tls-remote"} {
		if !strings.Contains(string(jsonData), want) {
			t.Fatalf("summary.json missing %q:\n%s", want, jsonData)
		}
	}
	for _, name := range []string{"summary.json", "server-runs.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "restart-secret") ||
			strings.Contains(string(data), "RestartToken") {
			t.Fatalf("%s leaked restart token:\n%s", name, data)
		}
	}
}

func TestLoadWorkspaceVerifyConfigRemoteFlags(t *testing.T) {
	t.Parallel()
	cfg, err := LoadWorkspaceVerifyConfig([]string{
		"--env", filepath.Join(t.TempDir(), ".env"),
		"--slug", "demo",
		"--domain", "vamos.test",
		"--remote-ssh", "linux-client",
		"--remote-shell", "ssh -J jump {host} -- {command}",
		"--dns-server", "100.126.72.21",
		"--expect-ip", "100.126.72.21",
		"--require-remote-tailnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RemoteProof.SSHHost != "linux-client" ||
		cfg.RemoteProof.ShellTemplate != "ssh -J jump {host} -- {command}" ||
		cfg.RemoteProof.DNSServer != "100.126.72.21" ||
		cfg.RemoteProof.ExpectIP != "100.126.72.21" ||
		!cfg.RemoteProof.RequireRemoteTailnet {
		t.Fatalf("RemoteProof = %+v", cfg.RemoteProof)
	}
}

func TestRequireRemoteTailnetNeedsSSH(t *testing.T) {
	t.Parallel()
	report, err := RunWorkspaceVerify(t.Context(), WorkspaceVerifyConfig{
		BaseURL:      "https://main.vamos.test",
		Domain:       "vamos.test",
		Slug:         "demo",
		RestartToken: "restart",
		ReportDir:    t.TempDir(),
		RemoteProof: RemoteProofConfig{
			RequireRemoteTailnet: true,
		},
	})
	if err == nil {
		t.Fatal("expected remote tailnet config error")
	}
	if report.Failure == nil ||
		report.Failure.Layer != workspaces.VerificationLayerConfig {
		t.Fatalf("failure = %+v", report.Failure)
	}
	if !strings.Contains(err.Error(), "--require-remote-tailnet requires --remote-ssh") {
		t.Fatalf("err = %v", err)
	}
}

//nolint:paralleltest // Mutates package-level test hooks.
func TestRemoteCommandConstruction(t *testing.T) {
	oldResolve, oldHTTPS, oldHost, oldRun, oldRemote := resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runRemoteCommandFn
	defer func() {
		resolveHostFn, probeHTTPSFn, probeHostPreservationFn, runServerLifecyclePhaseFn, runRemoteCommandFn = oldResolve, oldHTTPS, oldHost, oldRun, oldRemote
	}()
	resolveHostFn = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("100.126.72.21")}, nil
	}
	probeHTTPSFn = func(ctx context.Context, rawURL string, out io.Writer) error {
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	probeHostPreservationFn = func(ctx context.Context, baseURL, childURL string, out io.Writer) error {
		_, _ = out.Write([]byte("ok"))
		return nil
	}
	runServerLifecyclePhaseFn = func(ctx context.Context, cfg WorkspaceVerifyConfig, req workspaces.VerifyWorkspaceRequest) (workspaces.VerifyWorkspaceRun, error) {
		return workspaces.VerifyWorkspaceRun{
			ID:     "run",
			Status: workspaces.VerifyRunPassed,
			Phases: []workspaces.VerifyWorkspacePhase{
				{
					Layer:  workspaces.VerificationLayerLifecycle,
					Status: workspaces.VerifyPhasePassed,
				},
			},
		}, nil
	}
	var got [][]string
	runRemoteCommandFn = func(ctx context.Context, template, host string, command []string) (RemoteCommandResult, error) {
		if host != "linux-client" {
			t.Fatalf("host = %q", host)
		}
		got = append(got, append([]string(nil), command...))
		return RemoteCommandResult{
			Command: shellJoin(command),
			Output:  "100.126.72.21\n",
		}, nil
	}
	_, err := RunWorkspaceVerify(t.Context(), WorkspaceVerifyConfig{
		BaseURL:      "https://main.vamos.test",
		Domain:       "vamos.test",
		Slug:         "demo",
		RestartToken: "restart",
		Start:        true,
		Restart:      true,
		ReportDir:    t.TempDir(),
		Timeout:      time.Second,
		RemoteProof: RemoteProofConfig{
			SSHHost:       "linux-client",
			ShellTemplate: "ssh {host} -- {command}",
			DNSServer:     "100.126.72.21",
			ExpectIP:      "100.126.72.21",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := make([]string, 0, len(got))
	for _, cmd := range got {
		joined = append(joined, strings.Join(cmd, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{
		"dig @100.126.72.21 main.vamos.test +short",
		"dig @100.126.72.21 demo.vamos.test +short",
		"getent hosts main.vamos.test",
		"getent hosts demo.vamos.test",
		"curl -I --fail --show-error --silent https://main.vamos.test",
		"curl -I --fail --show-error --silent https://demo.vamos.test/",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("commands missing %q:\n%s", want, all)
		}
	}
}

func TestRunRemoteCommandRejectsUnsafeHost(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", "-oProxyCommand=evil", "bad host", "bad;host", "bad$(host)"} {
		_, err := RunRemoteCommand(
			t.Context(),
			"ssh {host} -- {command}",
			host,
			[]string{"true"},
		)
		if err == nil {
			t.Fatalf("host %q accepted", host)
		}
	}
}

func TestRunRemoteCommandCustomTemplateQuotesArgv(t *testing.T) {
	t.Parallel()
	cmd := shellJoin([]string{"curl", "-I", "https://main.vamos.test/a path?x='y'"})
	if !strings.Contains(cmd, "'curl' '-I'") ||
		!strings.Contains(cmd, "'https://main.vamos.test/a path?x='\\''y'\\'''") {
		t.Fatalf("shellJoin = %q", cmd)
	}
}

func TestClassifyPreflightFailures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg   string
		input workspaces.VerificationLayer
		want  workspaces.VerificationLayer
	}{
		{
			"lookup demo.test: no such host",
			workspaces.VerificationLayerConfig,
			workspaces.VerificationLayerDNS,
		},
		{
			"dig: no servers could be reached",
			workspaces.VerificationLayerDNSDirectCoreDNS,
			workspaces.VerificationLayerDNSDirectCoreDNS,
		},
		{
			"main.vamos.test did not resolve to expected IP 100.1.2.3",
			workspaces.VerificationLayerDNSDirectCoreDNS,
			workspaces.VerificationLayerDNSDirectCoreDNS,
		},
		{
			"getent hosts failed; resolvectl query failed",
			workspaces.VerificationLayerDNSRemoteSystem,
			workspaces.VerificationLayerDNSRemoteSystem,
		},
		{
			"bind: address already in use on port 443; tailscale serve status has HTTPS",
			workspaces.VerificationLayerCaddy,
			workspaces.VerificationLayerCaddyPort,
		},
		{
			"x509: certificate signed by unknown authority",
			workspaces.VerificationLayerTLSRemote,
			workspaces.VerificationLayerTLSRemote,
		},
		{
			"502 Bad Gateway",
			workspaces.VerificationLayerCaddy,
			workspaces.VerificationLayerCaddyProxy,
		},
		{
			"Workspace unavailable: child stopped",
			workspaces.VerificationLayerTLS,
			workspaces.VerificationLayerLifecycle,
		},
		{
			"handoff invalid signature",
			workspaces.VerificationLayerAuth,
			workspaces.VerificationLayerHandoff,
		},
		{
			"tls: failed to verify certificate",
			workspaces.VerificationLayerTLS,
			workspaces.VerificationLayerTLS,
		},
		{
			"get server verify returned HTTP 401",
			workspaces.VerificationLayerAuth,
			workspaces.VerificationLayerAuth,
		},
	}
	for _, tc := range cases {
		got := classifyError(tc.input, stringError(tc.msg))
		if got.Layer != tc.want {
			t.Fatalf("%q classified as %s, want %s", tc.msg, got.Layer, tc.want)
		}
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }
