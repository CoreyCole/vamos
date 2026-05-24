package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	temporalclient "go.temporal.io/sdk/client"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
	agentworker "github.com/CoreyCole/vamos/pkg/agents/worker"
	conversationworkflow "github.com/CoreyCole/vamos/pkg/agents/workflows/conversation"
	"github.com/CoreyCole/vamos/pkg/git"
	"github.com/CoreyCole/vamos/pkg/proto/auth/v1/authv1connect"
	"github.com/CoreyCole/vamos/pkg/proto/comments/v1/commentsv1connect"
	"github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/config"
	"github.com/CoreyCole/vamos/server/handlers"
	"github.com/CoreyCole/vamos/server/layouts"
	authmw "github.com/CoreyCole/vamos/server/middleware"
	"github.com/CoreyCole/vamos/server/services/agentchat"
	"github.com/CoreyCole/vamos/server/services/auth"
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/db"
	"github.com/CoreyCole/vamos/server/services/layoutprefs"
	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/services/storybook"
	"github.com/CoreyCole/vamos/server/services/system"
	"github.com/CoreyCole/vamos/server/services/theme"
	"github.com/CoreyCole/vamos/server/services/webhook"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type proxyRewrite struct {
	contentType string
	old         string
	new         string
}

func resolveStaticRoot() string {
	if root := strings.TrimSpace(os.Getenv("VAMOS_STATIC_ROOT")); root != "" {
		return root
	}
	if _, err := os.Stat(filepath.Join("static", "css", "index.css")); err == nil {
		return "static"
	}
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		candidate := filepath.Join(filepath.Dir(exe), "static")
		if _, err := os.Stat(filepath.Join(candidate, "css", "index.css")); err == nil {
			return candidate
		}
	}
	return "static"
}

func newPrefixStrippingProxy(
	targetURL, prefix string,
	rewrites ...proxyRewrite,
) (echo.HandlerFunc, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	if len(rewrites) > 0 {
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Header.Del("Accept-Encoding")
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			contentType := resp.Header.Get("Content-Type")
			shouldRewrite := false
			for _, rewrite := range rewrites {
				if strings.Contains(contentType, rewrite.contentType) {
					shouldRewrite = true
					break
				}
			}
			if !shouldRewrite {
				return nil
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if err := resp.Body.Close(); err != nil {
				return err
			}

			bodyText := string(body)
			for _, rewrite := range rewrites {
				if strings.Contains(contentType, rewrite.contentType) {
					bodyText = strings.ReplaceAll(bodyText, rewrite.old, rewrite.new)
				}
			}

			resp.Body = io.NopCloser(strings.NewReader(bodyText))
			resp.ContentLength = int64(len(bodyText))
			resp.Header.Set("Content-Length", strconv.Itoa(len(bodyText)))
			return nil
		}
	}

	return func(c echo.Context) error {
		req := c.Request().Clone(c.Request().Context())
		strippedPath := strings.TrimPrefix(req.URL.Path, prefix)
		if strippedPath == "" {
			strippedPath = "/"
		}
		req.URL.Path = strippedPath
		if req.URL.RawPath != "" {
			strippedRawPath := strings.TrimPrefix(req.URL.RawPath, prefix)
			if strippedRawPath == "" {
				strippedRawPath = "/"
			}
			req.URL.RawPath = strippedRawPath
		}

		proxy.ServeHTTP(c.Response().Writer, req)
		return nil
	}, nil
}

// Config holds environment configuration
type Config struct {
	ListenAddress           string   `envconfig:"CN_AGENTS_LISTEN_ADDRESS"             default:":4200"`
	PublicBaseURL           string   `envconfig:"CN_AGENTS_PUBLIC_BASE_URL"            default:"http://localhost:4200"`
	InternalCallbackBaseURL string   `envconfig:"CN_AGENTS_INTERNAL_CALLBACK_BASE_URL" default:""`
	WorkspaceMode           string   `envconfig:"CN_AGENTS_WORKSPACE_MODE"             default:"standalone"`
	WorkspaceDomain         string   `envconfig:"CN_AGENTS_WORKSPACE_DOMAIN"           default:""`
	WorkspaceParentDir      string   `envconfig:"CN_AGENTS_WORKSPACE_PARENT_DIR"       default:""`
	WorkspaceStateDir       string   `envconfig:"CN_AGENTS_WORKSPACE_STATE_DIR"        default:"~/.local/state/cn-agents/workspaces"`
	WorkspaceSlug           string   `envconfig:"CN_AGENTS_WORKSPACE_SLUG"             default:"main"`
	WorkspaceManagerURL     string   `envconfig:"CN_AGENTS_WORKSPACE_MANAGER_URL"      default:""`
	WorkspaceRestartToken   string   `envconfig:"CN_AGENTS_WORKSPACE_RESTART_TOKEN"    default:""`
	DevAuthSigningKey       string   `envconfig:"CN_AGENTS_DEV_AUTH_SIGNING_KEY"       default:""`
	DevAuthVerifyKey        string   `envconfig:"CN_AGENTS_DEV_AUTH_VERIFY_KEY"        default:""`
	MarkdownBasePath        string   `envconfig:"MARKDOWN_BASE_PATH"                   default:""`
	DatabasePath            string   `envconfig:"DATABASE_PATH"                        default:"~/.local/state/cn-agents/agents.db"`
	GoogleCredentialsFile   string   `envconfig:"GOOGLE_CREDENTIALS_FILE"              default:""`
	RepoPath                string   `envconfig:"REPO_PATH"                            default:""`
	ThoughtsBaseURL         string   `envconfig:"THOUGHTS_BASE_URL"                    default:""`
	GitHubBaseURL           string   `envconfig:"GITHUB_BASE_URL"                      default:""`
	ConfigPath              string   `envconfig:"CN_AGENTS_CONFIG"                     default:""`
	WebhookSecret           string   `envconfig:"WEBHOOK_SECRET"                       default:""`
	RebuildScript           string   `envconfig:"REBUILD_SCRIPT"                       default:"scripts/webhook-rebuild.sh"`
	TemporalUIBaseURL       string   `envconfig:"TEMPORAL_UI_BASE_URL"                 default:"http://127.0.0.1:8233"`
	AuthAllowedDomains      []string `envconfig:"AUTH_ALLOWED_DOMAINS"                 default:""`
	AuthWhitelistedEmails   []string `envconfig:"AUTH_WHITELISTED_EMAILS"              default:""`
	PlaywrightAuthEnabled   bool     `envconfig:"CN_AGENTS_PLAYWRIGHT_AUTH_ENABLED"    default:"false"`
	PlaywrightAuthEmail     string   `envconfig:"CN_AGENTS_PLAYWRIGHT_AUTH_EMAIL"      default:"playwright@localhost"`
	PlaywrightAuthToken     string   `envconfig:"CN_AGENTS_PLAYWRIGHT_AUTH_TOKEN"      default:""`
	// Optional explicit default working directory for new agent-chat threads.
	AgentChatDefaultDir string `envconfig:"AGENT_CHAT_DEFAULT_DIR" default:""`
	// Detail cards collapse only when their content exceeds this many lines.
	AgentChatDetailCollapseLineLimit int    `envconfig:"AGENT_CHAT_DETAIL_COLLAPSE_LINE_LIMIT" default:"10"`
	InternalAgentChatToken           string `envconfig:"CN_AGENTS_INTERNAL_TOKEN"              default:""`
	InternalAgentChatAllowLoopback   bool   `envconfig:"AGENT_CHAT_INTERNAL_ALLOW_LOOPBACK"    default:"false"`
}

func loadHostConfigForLegacyStartup() (server.HostConfig, bool) {
	if strings.TrimSpace(os.Getenv(config.EnvConfigPath)) == "" {
		return server.HostConfig{}, false
	}
	hostCfg, err := config.LoadFromEnvAndFile()
	if err != nil {
		log.Fatalf("failed to load Vamos config: %v", err)
	}
	return hostCfg, true
}

func applyHostConfigToLegacyConfig(cfg Config, host server.HostConfig) Config {
	cfg.ListenAddress = firstNonEmpty(host.Web.ListenAddress, cfg.ListenAddress)
	cfg.PublicBaseURL = firstNonEmpty(host.Web.PublicBaseURL, cfg.PublicBaseURL)
	cfg.InternalCallbackBaseURL = firstNonEmpty(
		host.Web.InternalCallbackBaseURL,
		cfg.InternalCallbackBaseURL,
	)
	cfg.TemporalUIBaseURL = firstNonEmpty(
		host.Web.TemporalUIBaseURL,
		cfg.TemporalUIBaseURL,
	)
	cfg.RepoPath = firstNonEmpty(host.Runtime.ThoughtsRepo, cfg.RepoPath)
	cfg.MarkdownBasePath = firstNonEmpty(
		host.Runtime.ThoughtsRoot,
		cfg.MarkdownBasePath,
	)
	cfg.DatabasePath = firstNonEmpty(host.Runtime.DatabasePath, cfg.DatabasePath)
	cfg.GoogleCredentialsFile = firstNonEmpty(
		host.Auth.GoogleCredentialsFile,
		cfg.GoogleCredentialsFile,
	)
	if len(host.Auth.AllowedDomains) > 0 {
		cfg.AuthAllowedDomains = host.Auth.AllowedDomains
	}
	if len(host.Auth.WhitelistedEmails) > 0 {
		cfg.AuthWhitelistedEmails = host.Auth.WhitelistedEmails
	}
	cfg.DevAuthSigningKey = firstNonEmpty(
		host.Auth.DevAuthSigningKey,
		cfg.DevAuthSigningKey,
	)
	cfg.DevAuthVerifyKey = firstNonEmpty(
		host.Auth.DevAuthVerifyKey,
		cfg.DevAuthVerifyKey,
	)
	cfg.PlaywrightAuthEnabled = host.Auth.PlaywrightEnabled
	cfg.PlaywrightAuthEmail = firstNonEmpty(
		host.Auth.PlaywrightEmail,
		cfg.PlaywrightAuthEmail,
	)
	cfg.PlaywrightAuthToken = firstNonEmpty(
		host.Auth.PlaywrightToken,
		cfg.PlaywrightAuthToken,
	)
	cfg.InternalAgentChatToken = firstNonEmpty(
		host.Auth.InternalToken,
		cfg.InternalAgentChatToken,
	)
	cfg.InternalAgentChatAllowLoopback = host.Auth.InternalAllowLoopback
	cfg.WorkspaceMode = firstNonEmpty(host.Workspaces.Mode, cfg.WorkspaceMode)
	cfg.WorkspaceDomain = firstNonEmpty(host.Workspaces.Domain, cfg.WorkspaceDomain)
	cfg.WorkspaceParentDir = firstNonEmpty(
		host.Workspaces.ParentDir,
		cfg.WorkspaceParentDir,
	)
	cfg.WorkspaceStateDir = firstNonEmpty(
		host.Workspaces.StateDir,
		cfg.WorkspaceStateDir,
	)
	cfg.WorkspaceSlug = firstNonEmpty(host.Workspaces.Slug, cfg.WorkspaceSlug)
	cfg.WorkspaceManagerURL = firstNonEmpty(
		host.Workspaces.ManagerURL,
		cfg.WorkspaceManagerURL,
	)
	cfg.WorkspaceRestartToken = firstNonEmpty(
		host.Workspaces.RestartToken,
		cfg.WorkspaceRestartToken,
	)
	cfg.WebhookSecret = firstNonEmpty(host.Deploy.WebhookSecret, cfg.WebhookSecret)
	cfg.RebuildScript = firstNonEmpty(host.Deploy.RebuildScript, cfg.RebuildScript)
	cfg.ThoughtsBaseURL = firstNonEmpty(
		host.Deploy.ThoughtsBaseURL,
		cfg.ThoughtsBaseURL,
	)
	cfg.GitHubBaseURL = firstNonEmpty(host.Deploy.GitHubBaseURL, cfg.GitHubBaseURL)
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type pathPolicy int

const (
	pathPolicyHost pathPolicy = iota
	pathPolicyModule
	pathPolicyState
)

func applyVamosEnvOverrides(cfg Config) Config {
	overrides := []struct {
		name string
		set  func(string)
	}{
		{"VAMOS_LISTEN_ADDRESS", func(v string) { cfg.ListenAddress = v }},
		{"VAMOS_PUBLIC_BASE_URL", func(v string) { cfg.PublicBaseURL = v }},
		{"VAMOS_INTERNAL_CALLBACK_BASE_URL", func(v string) { cfg.InternalCallbackBaseURL = v }},
		{"VAMOS_WORKSPACE_MODE", func(v string) { cfg.WorkspaceMode = v }},
		{"VAMOS_WORKSPACE_DOMAIN", func(v string) { cfg.WorkspaceDomain = v }},
		{"VAMOS_WORKSPACE_PARENT_DIR", func(v string) { cfg.WorkspaceParentDir = v }},
		{"VAMOS_WORKSPACE_STATE_DIR", func(v string) { cfg.WorkspaceStateDir = v }},
		{"VAMOS_WORKSPACE_SLUG", func(v string) { cfg.WorkspaceSlug = v }},
		{"VAMOS_WORKSPACE_MANAGER_URL", func(v string) { cfg.WorkspaceManagerURL = v }},
		{"VAMOS_WORKSPACE_RESTART_TOKEN", func(v string) { cfg.WorkspaceRestartToken = v }},
		{"VAMOS_DEV_AUTH_SIGNING_KEY", func(v string) { cfg.DevAuthSigningKey = v }},
		{"VAMOS_DEV_AUTH_VERIFY_KEY", func(v string) { cfg.DevAuthVerifyKey = v }},
		{"VAMOS_THOUGHTS_ROOT", func(v string) { cfg.MarkdownBasePath = v }},
		{"VAMOS_THOUGHTS_REPO", func(v string) { cfg.RepoPath = v }},
		{"VAMOS_DATABASE_PATH", func(v string) { cfg.DatabasePath = v }},
		{"VAMOS_CONFIG", func(v string) { cfg.ConfigPath = v }},
		{"VAMOS_PLAYWRIGHT_AUTH_ENABLED", func(v string) { cfg.PlaywrightAuthEnabled = parseBoolEnv(v, cfg.PlaywrightAuthEnabled) }},
		{"VAMOS_PLAYWRIGHT_AUTH_EMAIL", func(v string) { cfg.PlaywrightAuthEmail = v }},
		{"VAMOS_PLAYWRIGHT_AUTH_TOKEN", func(v string) { cfg.PlaywrightAuthToken = v }},
		{"VAMOS_INTERNAL_TOKEN", func(v string) { cfg.InternalAgentChatToken = v }},
		{"VAMOS_DEFAULT_CWD", func(v string) { cfg.AgentChatDefaultDir = v }},
	}
	for _, override := range overrides {
		if value, ok := os.LookupEnv(override.name); ok {
			override.set(value)
		}
	}
	return cfg
}

func parseBoolEnv(raw string, fallback bool) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func validateInternalAgentChatConfig(cfg Config) error {
	if strings.TrimSpace(cfg.InternalAgentChatToken) != "" {
		return nil
	}
	if cfg.InternalAgentChatAllowLoopback {
		return nil
	}
	return errors.New(
		"CN_AGENTS_INTERNAL_TOKEN is required for AgentChat internal endpoints (or set AGENT_CHAT_INTERNAL_ALLOW_LOOPBACK=true for local/test only)",
	)
}

func expandRuntimePaths(cfg Config) (Config, error) {
	var err error
	if isPreCutoverRelativeDatabasePath(cfg.DatabasePath) {
		return cfg, errors.New(
			"DATABASE_PATH=data/thoughts.db is ambiguous after pkg/agents cwd cutover; set an absolute DATABASE_PATH or ~/.local/state/cn-agents/agents.db",
		)
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		cfg.DatabasePath, err = defaultStatePath("agents.db")
		if err != nil {
			return cfg, err
		}
	}
	if cfg.DatabasePath, err = expandRequiredPath(
		"DATABASE_PATH",
		cfg.DatabasePath,
		pathPolicyState,
	); err != nil {
		return cfg, err
	}
	if cfg.GoogleCredentialsFile, err = expandOptionalPath(
		"GOOGLE_CREDENTIALS_FILE",
		cfg.GoogleCredentialsFile,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	if cfg.MarkdownBasePath, err = expandOptionalPath(
		"MARKDOWN_BASE_PATH",
		cfg.MarkdownBasePath,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	if cfg.RepoPath, err = expandOptionalPath(
		"REPO_PATH",
		cfg.RepoPath,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	if cfg.AgentChatDefaultDir, err = expandOptionalPath(
		"AGENT_CHAT_DEFAULT_DIR",
		cfg.AgentChatDefaultDir,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	if cfg.WorkspaceParentDir, err = expandOptionalPath(
		"CN_AGENTS_WORKSPACE_PARENT_DIR",
		cfg.WorkspaceParentDir,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.WorkspaceStateDir) == "" {
		cfg.WorkspaceStateDir, err = defaultStatePath("workspaces")
		if err != nil {
			return cfg, err
		}
	}
	if cfg.WorkspaceStateDir, err = expandRequiredPath(
		"CN_AGENTS_WORKSPACE_STATE_DIR",
		cfg.WorkspaceStateDir,
		pathPolicyState,
	); err != nil {
		return cfg, err
	}
	if cfg.RebuildScript, err = expandOptionalPath(
		"REBUILD_SCRIPT",
		cfg.RebuildScript,
		pathPolicyModule,
	); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.ConfigPath) == "" {
		cfg.ConfigPath, err = config.DefaultUserConfigPath()
		if err != nil {
			return cfg, err
		}
	}
	if cfg.ConfigPath, err = expandOptionalPath(
		"CN_AGENTS_CONFIG",
		cfg.ConfigPath,
		pathPolicyHost,
	); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func defaultStatePath(filename string) (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "cn-agents", filename), nil
}

func expandOptionalPath(name, value string, policy pathPolicy) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return expandRequiredPath(name, value, policy)
}

func expandRequiredPath(name, value string, policy pathPolicy) (string, error) {
	expanded, err := config.ExpandPath(value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	switch policy {
	case pathPolicyModule:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, expanded), nil
	case pathPolicyState:
		base, err := defaultStatePath("")
		if err != nil {
			return "", err
		}
		return filepath.Join(base, expanded), nil
	default:
		return "", fmt.Errorf(
			"%s must be absolute or ~/ relative after pkg/agents cwd cutover; got %q",
			name,
			value,
		)
	}
}

func isPreCutoverRelativeDatabasePath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return path == "data/thoughts.db"
}

func normalizeListenAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ":4200", nil
	}
	if strings.HasPrefix(raw, ":") {
		if _, err := strconv.Atoi(strings.TrimPrefix(raw, ":")); err != nil {
			return "", fmt.Errorf("CN_AGENTS_LISTEN_ADDRESS has invalid port %q", raw)
		}
		return raw, nil
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return ":" + raw, nil
	}
	host, listenPort, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf(
			"CN_AGENTS_LISTEN_ADDRESS must be a port, :port, or host:port: %w",
			err,
		)
	}
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("CN_AGENTS_LISTEN_ADDRESS host cannot be empty in %q", raw)
	}
	if _, err := strconv.Atoi(listenPort); err != nil {
		return "", fmt.Errorf("CN_AGENTS_LISTEN_ADDRESS has invalid port %q", listenPort)
	}
	return raw, nil
}

func publicBaseURL(cfg Config, req *http.Request) string {
	if base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/"); base != "" {
		return base
	}
	if req == nil {
		return "http://localhost:4200"
	}
	scheme := req.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	host := req.Host
	if forwarded := req.Header.Get(
		"X-Forwarded-Host",
	); strings.TrimSpace(
		forwarded,
	) != "" {
		host = forwarded
	}
	return scheme + "://" + host
}

func agentChatCallbackBaseURL(cfg Config) string {
	if base := strings.TrimSpace(cfg.InternalCallbackBaseURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	if base := strings.TrimSpace(cfg.PublicBaseURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	return "http://localhost:4200"
}

func agentChatDefaultDir(cfg Config) string {
	if dir := strings.TrimSpace(cfg.AgentChatDefaultDir); dir != "" {
		return dir
	}
	return cfg.RepoPath
}

func hostFromBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func validateWorkspaceConfig(cfg Config) error {
	switch cfg.WorkspaceMode {
	case "standalone", "manager", "child":
	default:
		return fmt.Errorf(
			"CN_AGENTS_WORKSPACE_MODE must be standalone, manager, or child; got %q",
			cfg.WorkspaceMode,
		)
	}
	if cfg.WorkspaceMode == "manager" && strings.TrimSpace(cfg.WorkspaceDomain) == "" {
		return errors.New("CN_AGENTS_WORKSPACE_DOMAIN is required in manager mode")
	}
	if cfg.WorkspaceMode == "child" && strings.TrimSpace(cfg.WorkspaceSlug) == "" {
		return errors.New("CN_AGENTS_WORKSPACE_SLUG is required in child mode")
	}
	switch cfg.WorkspaceMode {
	case "manager":
		if _, err := workspaces.ParseHandoffSigningKey(
			cfg.DevAuthSigningKey,
		); err != nil {
			return err
		}
	case "child":
		if _, err := workspaces.ParseHandoffVerifyKey(cfg.DevAuthVerifyKey); err != nil {
			return err
		}
	}
	return nil
}

func thoughtsBaseURL(cfg Config) string {
	if strings.TrimSpace(cfg.ThoughtsBaseURL) != "" {
		return cfg.ThoughtsBaseURL
	}
	return cfg.GitHubBaseURL
}

func registerAgentChatEntryRoutes(
	e *echo.Echo,
	authMiddleware echo.MiddlewareFunc,
	handler *agentchat.Handler,
	markdownService *markdown.Service,
) {
	// Root opens the unified Thoughts workbench with Chat selected. Keep
	// /agent-chat page routes retired while runtime endpoints remain available.
	e.GET("/", func(c echo.Context) error {
		if markdownService == nil {
			return c.NoContent(http.StatusNoContent)
		}
		c.Set("thoughts_context_mode", "chat")
		c.SetParamNames("*")
		c.SetParamValues("")
		return markdownService.ServeMarkdown(c)
	}, authMiddleware)

	agentChatGroup := e.Group("/agent-chat")
	agentChatGroup.Use(authMiddleware)
	handler.RegisterRuntimeRoutes(agentChatGroup)
	handler.RegisterNotFoundPageRoutes(agentChatGroup)
}

func main() {
	// Load configuration from environment
	runtimeCtx, stopRuntime := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopRuntime()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatal(err)
	}
	cfg = applyVamosEnvOverrides(cfg)
	cfg, err := expandRuntimePaths(cfg)
	if err != nil {
		log.Fatal(err)
	}
	hostCfg, useHostConfig := loadHostConfigForLegacyStartup()
	if useHostConfig {
		cfg = applyHostConfigToLegacyConfig(cfg, hostCfg)
	}
	if err := validateWorkspaceConfig(cfg); err != nil {
		log.Fatal(err)
	}
	listenAddress, err := normalizeListenAddress(cfg.ListenAddress)
	if err != nil {
		log.Fatal(err)
	}
	if !useHostConfig {
		if _, err := config.LoadFileConfig(config.LoadFileConfigOptions{
			Path:         cfg.ConfigPath,
			AllowMissing: true,
		}); err != nil {
			log.Fatalf("failed to load config file: %v", err)
		}
	}

	// Log server startup with mode (NDJSON format for format-logs.sh)
	mode := "prod"
	if config.DevMode {
		mode = "dev"
	}
	startupLog := map[string]any{
		"time":            time.Now().UTC().Format(time.RFC3339),
		"event":           "server_startup",
		"mode":            mode,
		"listen_address":  listenAddress,
		"public_base_url": cfg.PublicBaseURL,
		"workspace_mode":  cfg.WorkspaceMode,
		"workspace_slug":  cfg.WorkspaceSlug,
		"dev":             config.DevMode,
		"message": fmt.Sprintf(
			"Server starting in %s mode on %s",
			mode,
			listenAddress,
		),
	}
	if logJSON, err := json.Marshal(startupLog); err == nil {
		fmt.Fprintln(os.Stderr, string(logJSON))
	}

	// Determine markdown base path
	basePath := cfg.MarkdownBasePath
	if useHostConfig {
		layouts.SetBranding(layouts.Branding{AppName: hostCfg.App.Name})
		auth.SetBranding(auth.Branding{
			AppName:      hostCfg.App.Name,
			AccountLabel: hostCfg.App.AccountLabel,
		})
		layouts.SetGitHubLinkProvider(func(currentPath string) string {
			url, ok := markdown.GitHubURLForPath(hostCfg.Projects, currentPath)
			if !ok {
				return ""
			}
			return url
		})
	}

	fmt.Printf("Serving markdown files from: %s\n", basePath)

	// Get current git commit at startup
	ctx := context.Background()
	gitCommit, err := git.GetCurrentCommit(ctx, cfg.RepoPath)
	if err != nil {
		log.Fatalf("failed to get git commit: %v", err)
	}
	log.Printf("Server starting with git commit: %s", gitCommit)

	// Create a new Echo instance
	e := echo.New()

	// Add middleware
	e.Use(authmw.LoggingMiddleware())
	e.Use(middleware.Recover())
	corsAllowedOrigins := []string{"http://localhost:4200"}
	if useHostConfig {
		corsAllowedOrigins = hostCfg.Web.CORSAllowedOrigins
		if len(corsAllowedOrigins) == 0 &&
			strings.TrimSpace(hostCfg.Web.PublicBaseURL) != "" {
			corsAllowedOrigins = []string{hostCfg.Web.PublicBaseURL}
		}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsAllowedOrigins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Content-Type",
			"Authorization",
			"Connect-Protocol-Version",
		},
		AllowCredentials: true,
	}))

	var workspaceManager *workspaces.ManagerService
	var handoffSigner *workspaces.HandoffSigner
	var childDevAuthVerifyKey string
	switch cfg.WorkspaceMode {
	case "manager":
		handoffSigner, childDevAuthVerifyKey, err = workspaces.NewHandoffSignerFromSigningKey(
			cfg.DevAuthSigningKey,
			60*time.Second,
			workspaces.NewMemoryReplayCache(),
		)
		if err != nil {
			log.Fatal(err)
		}
	case "child":
		handoffSigner, err = workspaces.NewHandoffVerifierFromVerifyKey(
			cfg.DevAuthVerifyKey,
			60*time.Second,
			workspaces.NewMemoryReplayCache(),
		)
		if err != nil {
			log.Fatal(err)
		}
	}
	workspaceParentDir := cfg.WorkspaceParentDir
	if workspaceParentDir == "" && cfg.RepoPath != "" {
		workspaceParentDir = filepath.Dir(cfg.RepoPath)
	}
	workspaceDiscovery := workspaces.DiscoveryConfig{
		ParentDir:        workspaceParentDir,
		Domain:           cfg.WorkspaceDomain,
		StateDir:         cfg.WorkspaceStateDir,
		MainCheckoutPath: cfg.RepoPath,
	}
	if useHostConfig {
		workspaceDiscovery.MetadataDirName = hostCfg.Workspaces.MetadataDirName
		workspaceDiscovery.CheckoutPrefixes = hostCfg.Workspaces.CheckoutPrefixes
		workspaceDiscovery.MainCheckoutName = hostCfg.Workspaces.MainCheckoutName
		workspaceDiscovery.MainCheckoutPath = firstNonEmpty(
			hostCfg.Workspaces.MainCheckoutPath,
			workspaceDiscovery.MainCheckoutPath,
		)
		workspaceDiscovery.ModuleMarker = hostCfg.Workspaces.ModuleMarker
		workspaceDiscovery.PackageSubdir = hostCfg.Workspaces.PackageSubdir
		if len(hostCfg.Workspaces.ConfiguredCheckouts) > 0 {
			workspaceDiscovery.ConfiguredCheckouts = map[string]workspaces.ConfiguredCheckout{}
			for slug, checkout := range hostCfg.Workspaces.ConfiguredCheckouts {
				workspaceDiscovery.ConfiguredCheckouts[slug] = workspaces.ConfiguredCheckout{
					RootPath:    checkout.RootPath,
					DisplayName: checkout.DisplayName,
					IsMain:      checkout.IsMain,
				}
			}
		}
	}
	workspaceManagerURL := strings.TrimRight(cfg.PublicBaseURL, "/")
	if cfg.WorkspaceMode == "child" && strings.TrimSpace(cfg.WorkspaceManagerURL) != "" {
		workspaceManagerURL = strings.TrimRight(cfg.WorkspaceManagerURL, "/")
	}
	if cfg.WorkspaceMode == "manager" {
		workspaceManager, err = workspaces.NewManager(
			workspaces.RuntimeConfig{
				ListenAddress:    cfg.ListenAddress,
				ManagerURL:       strings.TrimRight(cfg.PublicBaseURL, "/"),
				RestartToken:     cfg.WorkspaceRestartToken,
				DevAuthVerifyKey: childDevAuthVerifyKey,
				BaseEnv: map[string]string{
					"GOOGLE_CREDENTIALS_FILE":      cfg.GoogleCredentialsFile,
					"VAMOS_INTERNAL_TOKEN":         cfg.InternalAgentChatToken,
					"VAMOS_WORKSPACE_DOMAIN":       cfg.WorkspaceDomain,
					"VAMOS_WORKSPACE_PARENT_DIR":   workspaceParentDir,
					"VAMOS_WORKSPACE_STATE_DIR":    cfg.WorkspaceStateDir,
					"VAMOS_WORKSPACE_METADATA_DIR": workspaceDiscovery.MetadataDirName,
					"VAMOS_THOUGHTS_REPO":          cfg.RepoPath,
					"VAMOS_THOUGHTS_ROOT":          cfg.MarkdownBasePath,
				},
				ThoughtsRepo:    cfg.RepoPath,
				ThoughtsRoot:    cfg.MarkdownBasePath,
				MetadataDirName: workspaceDiscovery.MetadataDirName,
			},
			workspaceDiscovery,
		)
		if err != nil {
			log.Fatal(err)
		}
		e.Use(workspaceManager.HostDispatchMiddleware(
			hostFromBaseURL(cfg.PublicBaseURL),
			workspaces.HostForSlug("main", cfg.WorkspaceDomain),
		))
	}
	if cfg.WorkspaceMode == "manager" && workspaceManager != nil {
		layouts.SetWorkspaceNavProvider(
			func(currentPath string) []layouts.WorkspaceNavItem {
				_ = workspaceManager.Refresh(context.Background())
				return workspaces.BuildNavItems(
					workspaceManager.List(),
					cfg.WorkspaceSlug,
					workspaceManagerURL,
					currentPath,
				)
			},
		)
	} else if cfg.WorkspaceMode == "child" {
		layouts.SetWorkspaceNavProvider(
			func(currentPath string) []layouts.WorkspaceNavItem {
				items, err := workspaces.Discover(workspaceDiscovery)
				if err != nil {
					return nil
				}
				return workspaces.BuildNavItems(
					items,
					cfg.WorkspaceSlug,
					workspaceManagerURL,
					currentPath,
				)
			},
		)
	}

	// Setup hot reload routes (dev mode only, no-op in prod)
	handlers.SetupReloadRoutes(e)

	// Initialize database
	if isPreCutoverRelativeDatabasePath(os.Getenv("DATABASE_PATH")) {
		log.Fatal(
			"DATABASE_PATH=data/thoughts.db is ambiguous after pkg/agents cwd cutover; set an absolute DATABASE_PATH or ~/.local/state/cn-agents/agents.db",
		)
	}
	dbService, err := db.NewService(cfg.DatabasePath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer dbService.Close()

	fmt.Printf("Database initialized at: %s\n", cfg.DatabasePath)

	// Create auth service
	authService, err := auth.NewService(
		dbService.Queries,
		cfg.GoogleCredentialsFile,
		cfg.AuthAllowedDomains,
		cfg.AuthWhitelistedEmails,
	)
	if err != nil {
		log.Fatal("Failed to create auth service:", err)
	}

	fmt.Printf(
		"Auth service initialized with credentials: %s, allowed domains: %v, whitelisted emails: %d\n",
		cfg.GoogleCredentialsFile,
		cfg.AuthAllowedDomains,
		len(cfg.AuthWhitelistedEmails),
	)

	// Create comment service with cached git commit
	commentService := comments.NewService(
		dbService.DB(),
		gitCommit,
		thoughtsBaseURL(cfg),
		cfg.MarkdownBasePath,
	)
	commentHandler := comments.NewHandler(commentService)

	fmt.Printf("Comment service initialized with git commit: %s\n", gitCommit)

	// Create theme service with database access
	themeService := theme.NewService(dbService.Queries)
	layoutPrefsService := layoutprefs.NewService(dbService.Queries)

	// Create markdown service with comment service and theme service dependencies
	markdownService, err := markdown.NewServiceWithOptions(
		basePath,
		commentService,
		themeService,
		markdown.ServiceOptions{Projects: hostCfg.Projects},
	)
	if err != nil {
		log.Fatal("Failed to create markdown service:", err)
	}
	markdownService.WithWorkspaceResolver(
		markdown.NewDBWorkspaceResolver(dbService.Queries, basePath),
	).WithLayoutPreferenceService(layoutPrefsService)

	fmt.Printf("Markdown service initialized with comment service\n")

	// Conditional Temporal startup
	var temporalManager *temporalmgr.Manager
	var goWorker *agentworker.Worker
	cnTemporal := os.Getenv("CN_TEMPORAL")
	if cnTemporal == "1" || cnTemporal == "true" {
		temporalAddr := os.Getenv("TEMPORAL_ADDRESS")
		if temporalAddr == "" {
			temporalAddr = "localhost:7233"
		}
		mgr, err := temporalmgr.NewManager(temporalAddr)
		if err != nil {
			log.Printf("Warning: failed to connect to Temporal: %v", err)
		} else {
			temporalManager = mgr
			defer temporalManager.Close()
			fmt.Printf("Temporal connected at %s\n", temporalAddr)

			goWorker = agentworker.New(mgr.Client())
			goWorker.RegisterWorkflow(conversationworkflow.RunTurnWorkflow)
			goWorker.RegisterWorkflow(workspaces.StartWorkspaceWorkflow)
			goWorker.RegisterWorkflow(workspaces.StopWorkspaceWorkflow)
			goWorker.RegisterWorkflow(workspaces.RestartWorkspaceWorkflow)
			if workspaceManager != nil {
				workspaceManager.SetLifecycleStarter(
					workspaces.NewTemporalLifecycleStarter(temporalManager),
				)
				goWorker.RegisterActivity(
					&workspaces.WorkspaceLifecycleActivities{Manager: workspaceManager},
				)
			}
		}
	}

	agentChatDefaultDir := agentChatDefaultDir(cfg)
	if useHostConfig && strings.TrimSpace(cfg.AgentChatDefaultDir) == "" {
		configuredDefaultDir, err := server.DefaultWorkingDir(hostCfg.Projects)
		if err != nil {
			log.Fatal(err)
		}
		if strings.TrimSpace(configuredDefaultDir) != "" {
			agentChatDefaultDir = configuredDefaultDir
		}
	}
	log.Printf("Agent chat default directory: %s", agentChatDefaultDir)

	agentChatNotifier := agentchat.NewNotifier()
	agentChatProjectName := ""
	if useHostConfig {
		agentChatProjectName = hostCfg.Projects.DefaultRepo
	}
	agentChatService, err := agentchat.NewServiceWithOptions(
		dbService.DB(),
		dbService.Queries,
		agentChatNotifier,
		temporalManager,
		themeService,
		agentchat.ServiceOptions{
			ProjectRoot:             cfg.RepoPath,
			ProjectName:             agentChatProjectName,
			DefaultCwd:              agentChatDefaultDir,
			ThoughtsRoot:            basePath,
			DetailCollapseLineLimit: cfg.AgentChatDetailCollapseLineLimit,
			CallbackBaseURL:         agentChatCallbackBaseURL(cfg),
		},
	)
	if err != nil {
		log.Fatal("Failed to create agent chat service:", err)
	}
	markdownService.WithChatWorkspaceResolver(
		markdown.NewDBChatWorkspaceCandidateResolver(
			dbService.Queries,
			basePath,
			agentChatService,
		),
	).WithEmbeddedChatRenderer(agentChatService)
	agentChatService.SetImplWorkspaceDiscoveryConfig(
		workspaces.ImplWorkspaceDiscoveryConfig{
			MainCheckoutPath: workspaceDiscovery.MainCheckoutPath,
			ParentDir:        workspaceParentDir,
			Domain:           cfg.WorkspaceDomain,
			MetadataDirName:  workspaceDiscovery.MetadataDirName,
			CheckoutPrefixes: workspaceDiscovery.CheckoutPrefixes,
			MainCheckoutName: workspaceDiscovery.MainCheckoutName,
			ModuleMarker:     workspaceDiscovery.ModuleMarker,
			PackageSubdir:    workspaceDiscovery.PackageSubdir,
		},
	)
	agentChatService.SetWorkspaceRuntimeConfig(
		workspaceManagerURL,
		cfg.WorkspaceRestartToken,
	)
	if workspaceManager != nil {
		agentChatService.SetDevWorkspaceManager(workspaceManager)
	}
	agentChatService.StartLiveFlushLoop(runtimeCtx)
	if err := validateInternalAgentChatConfig(cfg); err != nil {
		log.Fatal(err)
	}
	agentChatHandler := agentchat.NewHandler(
		agentChatService,
		themeService,
		agentchat.HandlerOptions{
			InternalToken:         cfg.InternalAgentChatToken,
			InternalAllowLoopback: cfg.InternalAgentChatAllowLoopback,
		},
	).WithLayoutPreferenceService(layoutPrefsService)
	if goWorker != nil {
		goWorker.RegisterWorkflow(agentchat.SyncWorkspacesWorkflow)
		goWorker.RegisterWorkflow(agentchat.PlanWorkspaceDiscoveryWorkflow)
		goWorker.RegisterActivity(agentChatService.FailConversationRunAfterActivityError)
		goWorker.RegisterActivity(&agentchat.WorkspaceSyncActivities{
			Syncer: agentChatService.WorkspaceSyncer(),
		})
		goWorker.RegisterActivity(&agentchat.PlanWorkspaceDiscoveryActivities{
			Syncer: agentChatService.PlanWorkspaceDiscoverySyncer(),
		})
	}
	if temporalManager != nil {
		input := agentChatService.WorkspaceSyncInput()
		if err := agentchat.EnsureSyncWorkspacesSchedule(
			runtimeCtx,
			temporalManager.Client(),
			input,
		); err != nil {
			log.Printf(
				"Warning: failed to ensure Agent Chat workspace sync schedule: %v",
				err,
			)
		} else {
			log.Printf(
				"Agent Chat workspace sync schedule ensured for %s",
				input.ProjectInstanceKey,
			)
		}
	} else {
		log.Printf(
			"Agent Chat workspace sync degraded: Temporal unavailable or disabled; workspace data remains DB-only until sync runs",
		)
	}
	if goWorker != nil {
		go func() {
			if err := goWorker.Run(runtimeCtx); err != nil {
				log.Printf("Go worker error: %v", err)
			}
		}()
	}

	// Mount Connect RPC service for comments
	commentsPath, commentsRPCHandler := commentsv1connect.NewCommentsServiceHandler(
		commentService,
	)
	e.Any(commentsPath+"*", echo.WrapHandler(commentsRPCHandler))

	// Static files. Resolve relative to the runtime package when the host
	// binary is launched from a wrapper checkout with a different working dir.
	staticRoot := resolveStaticRoot()
	e.Static("/static", staticRoot)
	e.Static("/css", filepath.Join(staticRoot, "css"))
	e.Static("/js", filepath.Join(staticRoot, "js"))
	e.Static("/img", filepath.Join(staticRoot, "img"))
	e.File("/manifest.json", filepath.Join(staticRoot, "manifest.json"))

	// Setup webhook service (only when secret is configured)
	if cfg.WebhookSecret != "" {
		webhookService := webhook.NewService(
			cfg.WebhookSecret,
			cfg.RepoPath,
			cfg.RebuildScript,
		)
		webhookHandler := webhook.NewHandler(webhookService)
		webhookHandler.RegisterRoutes(e)
		fmt.Printf("Webhook endpoint registered at /api/webhook/github\n")
	} else {
		fmt.Printf("WEBHOOK_SECRET not set! webhook endpoint disabled\n")
	}

	// Auth routes (HTTP)
	e.GET("/login", authService.HandleLoginPage)
	e.GET("/auth/google", authService.HandleGoogleLogin)
	e.GET("/auth/callback", authService.HandleOAuthCallback)
	e.GET("/logout", authService.HandleLogout)
	auth.RegisterPlaywrightAuthRoutes(e, authService, auth.PlaywrightAuthConfig{
		Enabled:         cfg.PlaywrightAuthEnabled,
		Email:           cfg.PlaywrightAuthEmail,
		Token:           cfg.PlaywrightAuthToken,
		PublicHostToken: strings.TrimSpace(cfg.WorkspaceDomain) != "",
		WorkspaceDomain: cfg.WorkspaceDomain,
	})

	// Mount Connect RPC service for auth
	path, handler := authv1connect.NewAuthServiceHandler(authService)
	e.Any(path+"*", echo.WrapHandler(handler))

	// Create auth middleware
	authMiddleware := authmw.AuthMiddleware(authService)
	var workspaceHandler *workspaces.Handler
	if cfg.WorkspaceMode != "standalone" || workspaceManager != nil {
		workspaceHandler = workspaces.NewHandler(
			workspaceManager,
			workspaceManagerURL,
			cfg.WorkspaceSlug,
			workspaces.WithDevAuth(authService, handoffSigner),
			workspaces.WithRestartAPI(cfg.WorkspaceRestartToken, cfg.RepoPath),
			workspaces.WithPlanWorkspaces(dbService.Queries),
			workspaces.WithImplWorkspaces(dbService.Queries),
			workspaces.WithWorkspaceSyncRefresh(func(ctx context.Context) error {
				input := agentChatService.WorkspaceSyncInput()
				if temporalManager == nil {
					result, err := agentChatService.WorkspaceSyncer().Sync(ctx, input)
					if err != nil {
						return err
					}
					log.Printf(
						"workspace_sync_refresh_complete mode=direct plan_upserted=%d plan_archived=%d impl_upserted=%d impl_repaired_env=%d impl_cleaned_up=%d impl_merged=%d changed=%t",
						result.Plan.Upserted,
						result.Plan.Archived,
						result.Impl.Upserted,
						result.Impl.RepairedEnv,
						result.Impl.CleanedUp,
						result.Impl.Merged,
						result.Changed,
					)
					return nil
				}
				run, err := temporalManager.Client().ExecuteWorkflow(
					ctx,
					temporalclient.StartWorkflowOptions{
						ID: agentchat.SyncWorkspacesWorkflowID(
							input.ProjectInstanceKey,
						) + ":manual:" + strconv.FormatInt(
							time.Now().UnixNano(),
							10,
						),
						TaskQueue: temporalmgr.GoTaskQueue,
					},
					agentchat.SyncWorkspacesWorkflow,
					input,
				)
				if err != nil {
					return err
				}
				var result agentchat.SyncWorkspacesResult
				if err := run.Get(ctx, &result); err != nil {
					return err
				}
				log.Printf(
					"workspace_sync_refresh_complete mode=temporal plan_upserted=%d plan_archived=%d impl_upserted=%d impl_repaired_env=%d impl_cleaned_up=%d impl_merged=%d changed=%t",
					result.Plan.Upserted,
					result.Plan.Archived,
					result.Impl.Upserted,
					result.Impl.RepairedEnv,
					result.Impl.CleanedUp,
					result.Impl.Merged,
					result.Changed,
				)
				return nil
			}),
		)
		if workspaceManager != nil {
			workspaceHandler.RegisterRoutes(e, authMiddleware)
		}
		if workspaceManager != nil && strings.TrimSpace(cfg.WorkspaceRestartToken) != "" {
			workspaceHandler.RegisterInternalRestartRoute(e)
			workspaceHandler.RegisterInternalVerificationRoutes(e, workspaces.NewVerifier(
				workspaceManager,
				cfg.ListenAddress,
				workspaces.NewMemoryVerifyRunStore(),
				workspaces.NewFileLogTailer(),
				workspaces.NewSystemLocalProber(),
			))
		}
		if handoffSigner != nil {
			workspaceHandler.RegisterDevAuthRoute(e)
		}
	}

	temporalUIProxy, err := newPrefixStrippingProxy(cfg.TemporalUIBaseURL, "/temporal")
	if err != nil {
		log.Fatal("Failed to create Temporal UI proxy:", err)
	}
	// Protected API routes (require auth)
	apiGroup := e.Group("/api")
	apiGroup.Use(authMiddleware)
	apiGroup.POST("/syntax-theme", themeService.HandleSyntaxThemeChange)
	apiGroup.POST("/theme", themeService.HandleThemeChange)
	layoutprefs.RegisterRoutes(apiGroup, layoutPrefsService)
	commentHandler.RegisterRoutes(apiGroup)

	// AgentChat entry routes. `/agent-chat` is the canonical route namespace;
	// `/` redirects there as app-root convenience.
	registerAgentChatEntryRoutes(e, authMiddleware, agentChatHandler, markdownService)

	// Protected form routes - require authentication
	formsGroup := e.Group("/forms")
	formsGroup.Use(authMiddleware)
	formsGroup.POST("/comments", commentService.HandleCommentForm)
	formsGroup.POST("/comments/show", commentService.HandleShowCommentForm)
	formsGroup.POST("/comments/expand", commentService.HandleExpandSectionComments)
	formsGroup.POST("/comments/cancel", commentService.HandleCancelCommentForm)
	formsGroup.POST("/replies", commentService.HandleReplyForm)
	formsGroup.POST("/resolve", commentService.HandleResolveComment)

	// Protected routes - require authentication
	thoughtsGroup := e.Group("/thoughts")
	thoughtsGroup.Use(authMiddleware)

	// Handle all paths under /thoughts/*
	thoughtsGroup.GET("", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/thoughts/")
	})
	thoughtsGroup.POST(
		"/actions/open-comments",
		markdownService.HandleOpenCommentsInPlace,
	)
	thoughtsGroup.POST("/actions/select-comment", markdownService.HandleSelectComment)
	thoughtsGroup.POST("/actions/open-chat", markdownService.OpenChatForDocument)
	thoughtsGroup.POST("/chat/open", markdownService.OpenChatForDocument)
	thoughtsGroup.POST("/chat/select", markdownService.SelectChatWorkspaceCandidate)
	thoughtsGroup.POST("/chat/freeform/send", agentChatHandler.SendEmbeddedFreeformPrompt)
	thoughtsGroup.POST(
		"/chat/freeform/resume",
		agentChatHandler.ResumeEmbeddedFreeformThread,
	)
	thoughtsGroup.GET("/chat/:workspace_id", agentChatHandler.HandleWorkspacePage)
	thoughtsGroup.GET(
		"/chat/:workspace_id/thread/:thread_id",
		agentChatHandler.HandleWorkspacePage,
	)
	thoughtsGroup.GET(
		"/chat/:workspace_id/stream",
		agentChatHandler.StreamEmbeddedWorkspace,
	)
	thoughtsGroup.POST(
		"/chat/:workspace_id/send",
		agentChatHandler.SendEmbeddedWorkspacePrompt,
	)
	thoughtsGroup.POST(
		"/chat/:workspace_id/thread/:thread_id/resume",
		agentChatHandler.ResumeEmbeddedWorkspaceThread,
	)
	thoughtsGroup.POST(
		"/chat/:workspace_id/attach-doc",
		agentChatHandler.AttachCurrentDocToEmbeddedChat,
	)
	thoughtsGroup.GET("/*", markdownService.ServeMarkdown)

	// Temporal UI routes (auth-protected)
	e.Any("/temporal", temporalUIProxy, authMiddleware)
	e.Any("/temporal/*", temporalUIProxy, authMiddleware)

	// Storybook routes (auth-protected)
	storybookHandler := storybook.NewHandler(themeService)
	storybookGroup := e.Group("/storybook")
	storybookGroup.Use(authMiddleware)
	storybookHandler.RegisterRoutes(storybookGroup)

	// Internal Agent Chat endpoints (from TS worker — no auth, localhost only)
	e.POST("/internal/agent-chat/events", agentChatHandler.HandleInternalRunEvent)
	e.GET("/internal/agent-chat/snapshots", agentChatHandler.HandleInternalRunSnapshot)
	e.POST(
		"/internal/agent-chat/import-session",
		agentChatHandler.HandleInternalPiSessionImport,
	)

	// System health dashboard routes (auth-protected)
	systemService := system.NewService(dbService.DB())
	sysGroup := e.Group("/system")
	sysGroup.Use(authMiddleware)
	sysGroup.GET("", func(c echo.Context) error {
		return systemService.HandleDashboard(c, themeService)
	})
	sysGroup.GET("/stream", systemService.HandleStream)
	sysGroup.GET("/health", systemService.HandleHealthJSON)
	sysGroup.POST("/history", systemService.HandleHistory)
	sysGroup.POST("/history/snapshot", systemService.HandleSnapshotDetail)

	// Start the server
	publicURL := strings.TrimRight(cfg.PublicBaseURL, "/")
	if publicURL == "" {
		publicURL = "http://localhost:4200"
	}
	fmt.Printf("Starting server on %s\n", listenAddress)
	fmt.Println("Agent Chat available at " + publicURL + "/")
	fmt.Println("Docs available at " + publicURL + "/thoughts/")

	serverErrCh := make(chan error, 1)
	go func() {
		if err := e.Start(listenAddress); !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	select {
	case err := <-serverErrCh:
		log.Fatal(err)
	case <-runtimeCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
