package verifycmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type WorkspaceVerifyConfig struct {
	EnvPath             string
	BaseURL             string
	Domain              string
	Slug                string
	Start               bool
	Restart             bool
	Stop                bool
	Browser             bool
	AgentChatProbe      bool
	ReportDir           string
	RestartToken        string
	PlaywrightAuthToken string
	PlaywrightURL       string
	Timeout             time.Duration
	RemoteProof         RemoteProofConfig
}

type WorkspaceVerifyReport struct {
	Summary     ReportSummary                    `json:"summary"`
	ServerRuns  []workspaces.VerifyWorkspaceRun  `json:"serverRuns"`
	ClientSteps []ClientVerifyStep               `json:"clientSteps"`
	Artifacts   map[string]string                `json:"artifacts"`
	Failure     *workspaces.VerifyWorkspaceError `json:"failure,omitempty"`
}

type ReportSummary struct {
	Status string                                  `json:"status"`
	Slug   string                                  `json:"slug"`
	Scope  string                                  `json:"scope"`
	Layers map[workspaces.VerificationLayer]string `json:"layers"`
}

type ClientVerifyStep struct {
	Name       string                       `json:"name"`
	Layer      workspaces.VerificationLayer `json:"layer"`
	Status     string                       `json:"status"`
	OutputPath string                       `json:"outputPath,omitempty"`
	Error      string                       `json:"error,omitempty"`
}

const (
	defaultVerifyTimeout = 2 * time.Minute
	defaultTailLines     = 200
	statusFailed         = "failed"
	statusPassed         = "passed"
)

var (
	resolveHostFn             = ResolveHost
	probeHTTPSFn              = ProbeHTTPS
	probeHostPreservationFn   = ProbeHostPreservation
	runServerLifecyclePhaseFn = RunServerLifecyclePhase
	runBrowserVerifyFn        = RunBrowserVerify
	runRemoteCommandFn        = RunRemoteCommand
)

func Main(args []string) error {
	cfg, err := LoadWorkspaceVerifyConfig(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	report, runErr := RunWorkspaceVerify(ctx, cfg)
	if err := WriteWorkspaceVerifyReport(report, cfg.ReportDir); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	fmt.Printf("workspace verification passed; report: %s\n", cfg.ReportDir)
	return nil
}

func LoadWorkspaceVerifyConfig(args []string) (WorkspaceVerifyConfig, error) {
	cfg := WorkspaceVerifyConfig{Timeout: defaultVerifyTimeout}
	fs := flag.NewFlagSet("agentsctl verify workspaces", flag.ContinueOnError)
	fs.StringVar(&cfg.EnvPath, "env", "../../.env", "path to env file")
	fs.StringVar(&cfg.BaseURL, "base-url", "", "manager public base URL")
	fs.StringVar(&cfg.Domain, "domain", "", "workspace DNS domain")
	fs.StringVar(&cfg.Slug, "slug", "", "workspace slug")
	fs.BoolVar(&cfg.Start, "start", false, "start workspace before verification")
	fs.BoolVar(&cfg.Restart, "restart", false, "restart workspace during verification")
	fs.BoolVar(&cfg.Stop, "stop", false, "stop workspace after verification")
	fs.BoolVar(&cfg.Browser, "browser", false, "run browser verification")
	fs.BoolVar(
		&cfg.AgentChatProbe,
		"agent-chat-probe",
		false,
		"run child Agent Chat callback/snapshot isolation probe",
	)
	fs.StringVar(&cfg.ReportDir, "report", "", "report directory")
	fs.StringVar(&cfg.RestartToken, "restart-token", "", "workspace restart/API token")
	fs.StringVar(
		&cfg.PlaywrightAuthToken,
		"playwright-auth-token",
		"",
		"Playwright auth bootstrap token",
	)
	fs.StringVar(
		&cfg.PlaywrightURL,
		"playwright-url",
		"",
		"reserved Playwright URL override",
	)
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "verification timeout")
	fs.StringVar(
		&cfg.RemoteProof.SSHHost,
		"remote-ssh",
		"",
		"SSH host for remote tailnet verification",
	)
	fs.StringVar(
		&cfg.RemoteProof.ShellTemplate,
		"remote-shell",
		"ssh {host} -- {command}",
		"remote shell template with {host} and {command}",
	)
	fs.StringVar(
		&cfg.RemoteProof.DNSServer,
		"dns-server",
		"",
		"explicit CoreDNS server IP for direct DNS proof",
	)
	fs.StringVar(
		&cfg.RemoteProof.ExpectIP,
		"expect-ip",
		"",
		"expected A record IP for workspace hosts",
	)
	fs.BoolVar(
		&cfg.RemoteProof.RequireRemoteTailnet,
		"require-remote-tailnet",
		false,
		"fail if remote tailnet proof is skipped",
	)
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	env, err := readDotEnv(cfg.EnvPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if cfg.Domain == "" {
		cfg.Domain = env["VAMOS_WORKSPACE_DOMAIN"]
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = env["VAMOS_PUBLIC_BASE_URL"]
	}
	if cfg.BaseURL == "" && cfg.Domain != "" {
		cfg.BaseURL = "https://main." + strings.Trim(cfg.Domain, ".")
	}
	if cfg.RestartToken == "" {
		cfg.RestartToken = env["VAMOS_WORKSPACE_RESTART_TOKEN"]
	}
	if cfg.PlaywrightAuthToken == "" {
		cfg.PlaywrightAuthToken = env["VAMOS_PLAYWRIGHT_AUTH_TOKEN"]
	}
	cfg.Slug = strings.TrimSpace(cfg.Slug)
	if cfg.ReportDir == "" {
		slug := cfg.Slug
		if slug == "" {
			slug = "workspace"
		}
		cfg.ReportDir = filepath.Join(
			"tmp",
			"workspace-verification",
			time.Now().UTC().Format("20060102T150405Z")+"_"+slug,
		)
	}
	return cfg, nil
}

func RunWorkspaceVerify(
	ctx context.Context,
	cfg WorkspaceVerifyConfig,
) (report WorkspaceVerifyReport, retErr error) {
	report = WorkspaceVerifyReport{
		Summary: ReportSummary{
			Status: statusPassed,
			Slug:   cfg.Slug,
			Scope:  "workspace verification",
			Layers: map[workspaces.VerificationLayer]string{},
		},
		Artifacts: map[string]string{"report_dir": cfg.ReportDir},
	}
	cleanupOnFailure := false
	stopWorkspace := func(name string) error {
		stopRun, err := runServerLifecyclePhaseFn(
			ctx,
			cfg,
			workspaces.VerifyWorkspaceRequest{
				Slug:      cfg.Slug,
				Stop:      true,
				TailLines: defaultTailLines,
				ReportDir: cfg.ReportDir,
			},
		)
		report.ServerRuns = append(report.ServerRuns, stopRun)
		if err != nil {
			layer := workspaces.VerificationLayerLifecycle
			if stopRun.Error != nil {
				layer = stopRun.Error.Layer
			}
			vwErr := classifyError(layer, err)
			report.Summary.Status = statusFailed
			report.Summary.Layers[vwErr.Layer] = statusFailed
			report.Failure = &vwErr
			report.ClientSteps = append(
				report.ClientSteps,
				ClientVerifyStep{
					Name:   name,
					Layer:  vwErr.Layer,
					Status: statusFailed,
					Error:  vwErr.Message,
				},
			)
			return fmt.Errorf(
				"%s verification failed: %s",
				vwErr.Layer,
				vwErr.Message,
			)
		}
		markServerLayers(&report, stopRun)
		return nil
	}
	defer func() {
		if retErr != nil && cleanupOnFailure {
			_ = stopWorkspace("server-stop-after-failure")
		}
	}()
	fail := func(layer workspaces.VerificationLayer, name string, err error) (WorkspaceVerifyReport, error) {
		vwErr := classifyError(layer, err)
		report.Summary.Status = statusFailed
		report.Summary.Layers[vwErr.Layer] = statusFailed
		report.Failure = &vwErr
		report.ClientSteps = append(
			report.ClientSteps,
			ClientVerifyStep{
				Name:   name,
				Layer:  vwErr.Layer,
				Status: statusFailed,
				Error:  vwErr.Message,
			},
		)
		return report, fmt.Errorf(
			"%s verification failed: %s",
			vwErr.Layer,
			vwErr.Message,
		)
	}
	if cfg.Slug == "" {
		return fail(
			workspaces.VerificationLayerConfig,
			"config",
			errors.New("--slug is required"),
		)
	}
	if cfg.Domain == "" {
		return fail(
			workspaces.VerificationLayerConfig,
			"config",
			errors.New(
				"workspace domain is required via --domain or VAMOS_WORKSPACE_DOMAIN",
			),
		)
	}
	if cfg.BaseURL == "" {
		return fail(
			workspaces.VerificationLayerConfig,
			"config",
			errors.New(
				"base URL is required via --base-url, VAMOS_PUBLIC_BASE_URL, or --domain",
			),
		)
	}
	if cfg.RestartToken == "" {
		return fail(
			workspaces.VerificationLayerConfig,
			"config",
			errors.New(
				"restart token is required via --restart-token or VAMOS_WORKSPACE_RESTART_TOKEN",
			),
		)
	}

	mainHost, err := hostFromURL(cfg.BaseURL)
	if err != nil {
		return fail(workspaces.VerificationLayerConfig, "config", err)
	}
	childHost := cfg.Slug + "." + strings.Trim(cfg.Domain, ".")
	if cfg.RemoteProof.RequireRemoteTailnet && cfg.RemoteProof.SSHHost == "" {
		return fail(
			workspaces.VerificationLayerConfig,
			"remote-tailnet-config",
			errors.New("--require-remote-tailnet requires --remote-ssh"),
		)
	}
	if cfg.RemoteProof.DNSServer != "" {
		if err := runDirectCoreDNS(ctx, &report, cfg.RemoteProof, mainHost); err != nil {
			return fail(
				workspaces.VerificationLayerDNSDirectCoreDNS,
				"dns-direct-coredns-main",
				err,
			)
		}
		if err := runDirectCoreDNS(ctx, &report, cfg.RemoteProof, childHost); err != nil {
			return fail(
				workspaces.VerificationLayerDNSDirectCoreDNS,
				"dns-direct-coredns-child",
				err,
			)
		}
	}
	if cfg.RemoteProof.SSHHost != "" {
		if err := runRemoteSystemDNS(
			ctx,
			&report,
			cfg.RemoteProof,
			mainHost,
		); err != nil {
			return fail(
				workspaces.VerificationLayerDNSRemoteSystem,
				"dns-remote-system-main",
				err,
			)
		}
		if err := runRemoteSystemDNS(
			ctx,
			&report,
			cfg.RemoteProof,
			childHost,
		); err != nil {
			return fail(
				workspaces.VerificationLayerDNSRemoteSystem,
				"dns-remote-system-child",
				err,
			)
		}
	}
	if err := runDNS(ctx, &report, "dns-main", mainHost); err != nil {
		return fail(workspaces.VerificationLayerDNS, "dns-main", err)
	}
	if err := runDNS(ctx, &report, "dns-child", childHost); err != nil {
		return fail(workspaces.VerificationLayerDNS, "dns-child", err)
	}
	if err := captureStep(
		&report,
		"curl-main-https",
		workspaces.VerificationLayerTLS,
		"curl-main-https.txt",
		func(f *os.File) error {
			return probeHTTPSFn(ctx, cfg.BaseURL, f)
		},
	); err != nil {
		return fail(workspaces.VerificationLayerTLS, "curl-main-https", err)
	}
	if cfg.RemoteProof.SSHHost != "" {
		if err := runRemoteHTTPS(
			ctx,
			&report,
			cfg.RemoteProof,
			cfg.BaseURL,
			"remote-curl-main-https",
		); err != nil {
			return fail(
				workspaces.VerificationLayerTLSRemote,
				"remote-curl-main-https",
				err,
			)
		}
	}
	childURL := "https://" + childHost + "/"
	req := workspaces.VerifyWorkspaceRequest{
		Slug:           cfg.Slug,
		Start:          cfg.Start,
		Restart:        cfg.Restart,
		TailLines:      defaultTailLines,
		ReportDir:      cfg.ReportDir,
		AgentChatProbe: cfg.AgentChatProbe,
	}
	run, err := runServerLifecyclePhaseFn(ctx, cfg, req)
	report.ServerRuns = append(report.ServerRuns, run)
	if err != nil {
		layer := workspaces.VerificationLayerLifecycle
		if run.Error != nil {
			layer = run.Error.Layer
		}
		return fail(layer, "server-lifecycle", err)
	}
	markServerLayers(&report, run)
	report.Summary.Layers[workspaces.VerificationLayerAuth] = statusPassed
	cleanupOnFailure = cfg.Stop

	if err := captureStep(
		&report,
		"curl-child-https",
		workspaces.VerificationLayerTLS,
		"curl-child-https.txt",
		func(f *os.File) error {
			return probeHTTPSFn(ctx, childURL, f)
		},
	); err != nil {
		return fail(workspaces.VerificationLayerTLS, "curl-child-https", err)
	}
	if cfg.RemoteProof.SSHHost != "" {
		if err := runRemoteHTTPS(
			ctx,
			&report,
			cfg.RemoteProof,
			childURL,
			"remote-curl-child-https",
		); err != nil {
			return fail(
				workspaces.VerificationLayerTLSRemote,
				"remote-curl-child-https",
				err,
			)
		}
	}
	if err := captureStep(
		&report,
		"curl-host-dispatch",
		workspaces.VerificationLayerCaddy,
		"curl-host-dispatch.txt",
		func(f *os.File) error {
			return probeHostPreservationFn(ctx, cfg.BaseURL, childURL, f)
		},
	); err != nil {
		return fail(workspaces.VerificationLayerCaddy, "curl-host-dispatch", err)
	}

	if cfg.Browser {
		step, err := runBrowserVerifyFn(ctx, BrowserVerifyConfig{
			BaseURL:   cfg.BaseURL,
			Domain:    cfg.Domain,
			Slug:      cfg.Slug,
			AuthToken: cfg.PlaywrightAuthToken,
			ReportDir: cfg.ReportDir,
			Timeout:   cfg.Timeout,
		})
		report.ClientSteps = append(report.ClientSteps, step)
		if err != nil {
			return fail(workspaces.VerificationLayerBrowser, step.Name, err)
		}
		report.Summary.Layers[workspaces.VerificationLayerBrowser] = statusPassed
		report.Summary.Layers[workspaces.VerificationLayerHandoff] = statusPassed
		if cfg.Stop {
			cleanupOnFailure = false
			if err := stopWorkspace("server-stop-after-browser"); err != nil {
				return report, err
			}
			stoppedStep, err := runBrowserVerifyFn(ctx, BrowserVerifyConfig{
				BaseURL:       cfg.BaseURL,
				Domain:        cfg.Domain,
				Slug:          cfg.Slug,
				AuthToken:     cfg.PlaywrightAuthToken,
				ReportDir:     cfg.ReportDir,
				ExpectStopped: true,
				Timeout:       cfg.Timeout,
			})
			report.ClientSteps = append(report.ClientSteps, stoppedStep)
			if err != nil {
				return fail(workspaces.VerificationLayerBrowser, stoppedStep.Name, err)
			}
		}
	} else if cfg.Stop {
		cleanupOnFailure = false
		if err := stopWorkspace("server-stop"); err != nil {
			return report, err
		}
	}
	return report, nil
}

func runDirectCoreDNS(
	ctx context.Context,
	report *WorkspaceVerifyReport,
	remote RemoteProofConfig,
	host string,
) error {
	name := "dns-direct-coredns-" + host
	return captureStep(
		report,
		name,
		workspaces.VerificationLayerDNSDirectCoreDNS,
		name+".txt",
		func(f *os.File) error {
			var result RemoteCommandResult
			var err error
			if remote.SSHHost == "" {
				result, err = runLocalCommand(
					ctx,
					directDNSCommand(remote.DNSServer, host),
				)
			} else {
				result, err = runRemoteCommandFn(
					ctx,
					remote.ShellTemplate,
					remote.SSHHost,
					directDNSCommand(remote.DNSServer, host),
				)
			}
			writeRemoteResult(f, result)
			if err != nil {
				return err
			}
			if remote.ExpectIP != "" &&
				!strings.Contains(result.Output, remote.ExpectIP) {
				return fmt.Errorf(
					"%s did not resolve to expected IP %s: %s",
					host,
					remote.ExpectIP,
					result.Output,
				)
			}
			return nil
		},
	)
}

func runRemoteSystemDNS(
	ctx context.Context,
	report *WorkspaceVerifyReport,
	remote RemoteProofConfig,
	host string,
) error {
	name := "dns-remote-system-" + host
	return captureStep(
		report,
		name,
		workspaces.VerificationLayerDNSRemoteSystem,
		name+".txt",
		func(f *os.File) error {
			primary, primaryErr := runRemoteCommandFn(
				ctx,
				remote.ShellTemplate,
				remote.SSHHost,
				remoteDNSCommand(host),
			)
			writeRemoteResult(f, primary)
			if primaryErr == nil {
				if remote.ExpectIP != "" &&
					!strings.Contains(primary.Output, remote.ExpectIP) {
					return fmt.Errorf(
						"%s did not resolve to expected IP %s: %s",
						host,
						remote.ExpectIP,
						primary.Output,
					)
				}
				return nil
			}
			fallback, fallbackErr := runRemoteCommandFn(
				ctx,
				remote.ShellTemplate,
				remote.SSHHost,
				remoteDNSFallbackCommand(host),
			)
			fmt.Fprintln(f)
			writeRemoteResult(f, fallback)
			if fallbackErr != nil {
				return fmt.Errorf(
					"getent hosts failed: %w; resolvectl query failed: %v",
					primaryErr,
					fallbackErr,
				)
			}
			if remote.ExpectIP != "" &&
				!strings.Contains(fallback.Output, remote.ExpectIP) {
				return fmt.Errorf(
					"%s did not resolve to expected IP %s: %s",
					host,
					remote.ExpectIP,
					fallback.Output,
				)
			}
			return nil
		},
	)
}

func runRemoteHTTPS(
	ctx context.Context,
	report *WorkspaceVerifyReport,
	remote RemoteProofConfig,
	rawURL, name string,
) error {
	return captureStep(
		report,
		name,
		workspaces.VerificationLayerTLSRemote,
		name+".txt",
		func(f *os.File) error {
			result, err := runRemoteCommandFn(
				ctx,
				remote.ShellTemplate,
				remote.SSHHost,
				remoteHTTPSCommand(rawURL),
			)
			writeRemoteResult(f, result)
			return err
		},
	)
}

func runLocalCommand(ctx context.Context, command []string) (RemoteCommandResult, error) {
	if len(command) == 0 {
		return RemoteCommandResult{}, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	out, err := cmd.CombinedOutput()
	return RemoteCommandResult{Command: shellJoin(command), Output: string(out)}, err
}

func runDNS(ctx context.Context, report *WorkspaceVerifyReport, name, host string) error {
	return captureStep(
		report,
		name,
		workspaces.VerificationLayerDNS,
		name+".txt",
		func(f *os.File) error {
			ips, err := resolveHostFn(ctx, host)
			if err != nil {
				return err
			}
			for _, ip := range ips {
				fmt.Fprintln(f, ip.String())
			}
			return nil
		},
	)
}

func captureStep(
	report *WorkspaceVerifyReport,
	name string,
	layer workspaces.VerificationLayer,
	filename string,
	fn func(*os.File) error,
) error {
	if err := os.MkdirAll(reportDirFromArtifacts(report), 0o755); err != nil {
		return err
	}
	path := filepath.Join(reportDirFromArtifacts(report), filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = fn(f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	step := ClientVerifyStep{
		Name:       name,
		Layer:      layer,
		Status:     statusPassed,
		OutputPath: path,
	}
	if err != nil {
		step.Status = statusFailed
		step.Error = err.Error()
	} else {
		report.Summary.Layers[layer] = statusPassed
	}
	report.ClientSteps = append(report.ClientSteps, step)
	report.Artifacts[name] = path
	return err
}

func reportDirFromArtifacts(report *WorkspaceVerifyReport) string {
	if report.Artifacts == nil {
		report.Artifacts = map[string]string{}
	}
	if dir := report.Artifacts["report_dir"]; dir != "" {
		return dir
	}
	return "."
}

func markServerLayers(report *WorkspaceVerifyReport, run workspaces.VerifyWorkspaceRun) {
	if strings.TrimSpace(run.Diagnostics.LogTail) != "" {
		report.Summary.Layers[workspaces.VerificationLayerLogs] = statusPassed
	}
	if run.AgentChatProbe != nil {
		report.Summary.Layers[workspaces.VerificationLayerAgentChat] = statusPassed
	}
	for _, phase := range run.Phases {
		if phase.Status == workspaces.VerifyPhasePassed {
			report.Summary.Layers[phase.Layer] = statusPassed
		}
		if phase.Status == workspaces.VerifyPhaseFailed {
			report.Summary.Layers[phase.Layer] = statusFailed
		}
	}
}

func classifyError(
	layer workspaces.VerificationLayer,
	err error,
) workspaces.VerifyWorkspaceError {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "restricted dns") || strings.Contains(lower, "resolvectl") || strings.Contains(lower, "getent"):
		layer = workspaces.VerificationLayerDNSRemoteSystem
	case strings.Contains(lower, "no servers could be reached") || strings.Contains(lower, "connection timed out") && layer == workspaces.VerificationLayerDNSDirectCoreDNS:
		layer = workspaces.VerificationLayerDNSDirectCoreDNS
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "server misbehaving"):
		layer = workspaces.VerificationLayerDNS
	case strings.Contains(lower, "expected ip") || strings.Contains(lower, "did not resolve to expected ip"):
		layer = workspaces.VerificationLayerDNSDirectCoreDNS
	case strings.Contains(lower, "address already in use") || strings.Contains(lower, "port 443") || strings.Contains(lower, "tailscale serve"):
		layer = workspaces.VerificationLayerCaddyPort
	case strings.Contains(lower, "x509") || strings.Contains(lower, "certificate signed by unknown authority") || strings.Contains(lower, "unknown ca"):
		layer = workspaces.VerificationLayerTLSRemote
	case strings.Contains(lower, "http: server gave http response to https client") || strings.Contains(lower, "certificate") || strings.Contains(lower, "tls"):
		layer = workspaces.VerificationLayerTLS
	case strings.Contains(lower, "502") || strings.Contains(lower, "bad gateway"):
		layer = workspaces.VerificationLayerCaddyProxy
	case strings.Contains(lower, "workspace unavailable") || strings.Contains(lower, "child") && strings.Contains(lower, "stopped"):
		layer = workspaces.VerificationLayerLifecycle
	case strings.Contains(lower, "invalid signature") || strings.Contains(lower, "target mismatch") || strings.Contains(lower, "replay") || strings.Contains(lower, "handoff"):
		layer = workspaces.VerificationLayerHandoff
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		layer = workspaces.VerificationLayerAuth
	}
	return workspaces.VerifyWorkspaceError{
		Layer:      layer,
		Message:    msg,
		Suggestion: suggestionForLayer(layer),
	}
}

func suggestionForLayer(layer workspaces.VerificationLayer) string {
	switch layer {
	case workspaces.VerificationLayerDNSDirectCoreDNS:
		return "Check cn-agents-coredns.service, Corefile tailnet bind IP, and UFW tailnet DNS rules."
	case workspaces.VerificationLayerDNSRemoteSystem:
		return "Check Tailscale restricted nameserver for vamos.test and that the client accepts Tailscale DNS."
	case workspaces.VerificationLayerTLSRemote:
		return "Install or re-trust the exported Caddy internal root CA on the remote client."
	case workspaces.VerificationLayerCaddyPort:
		return "Check cn-agents-caddy.service, UFW TCP 443, and Tailscale Serve or other :443 listeners."
	case workspaces.VerificationLayerCaddyProxy:
		return "Check Caddy reverse_proxy target 127.0.0.1:4200 and manager host dispatch."
	}
	return ""
}

func hostFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("base URL %q has no host", raw)
	}
	return u.Hostname(), nil
}

func readDotEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, "'\"")
		out[key] = val
	}
	return out, nil
}
