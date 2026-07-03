package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/server"
)

const EnvConfigPath = "VAMOS_CONFIG"

const (
	defaultWebhookForwardTimeout = 15 * time.Second
	githubPushEvent              = "push"
)

type FileConfig struct {
	App        server.AppConfig       `yaml:"app"`
	Runtime    server.RuntimeConfig   `yaml:"runtime"`
	Auth       server.AuthConfig      `yaml:"auth"`
	Web        server.WebConfig       `yaml:"web"`
	Projects   ProjectsConfigFile     `yaml:"projects"`
	Workspaces server.WorkspaceConfig `yaml:"workspaces"`
	Deploy     server.DeployConfig    `yaml:"deploy"`
}

type ProjectsConfigFile struct {
	DefaultRepo             string                    `yaml:"default_repo"`
	DefaultCheckout         string                    `yaml:"default_checkout"`
	DefaultBaselineCheckout string                    `yaml:"default_baseline_checkout"`
	Repos                   map[string]RepoConfigFile `yaml:"repos"`
}

type RepoConfigFile struct {
	GitHubURL        string                        `yaml:"github_url"`
	DefaultBranch    string                        `yaml:"default_branch"`
	DefaultCheckout  string                        `yaml:"default_checkout"`
	BaselineCheckout string                        `yaml:"baseline_checkout"`
	Checkouts        map[string]CheckoutConfigFile `yaml:"checkouts"`
}

type CheckoutConfigFile struct {
	RootPath          string              `yaml:"root_path"`
	Purpose           string              `yaml:"purpose"`
	Role              server.CheckoutRole `yaml:"role"`
	MustBeClean       bool                `yaml:"must_be_clean"`
	MustBeLatest      bool                `yaml:"must_be_latest"`
	WebhookSyncBranch string              `yaml:"webhook_sync_branch"`
}

type LoadFileConfigOptions struct {
	Path         string
	AllowMissing bool
}

func DefaultUserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agents", "config.yml"), nil
}

func LoadFileConfig(opts LoadFileConfigOptions) (FileConfig, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		var err error
		path, err = DefaultUserConfigPath()
		if err != nil {
			return FileConfig{}, err
		}
	}
	expanded, err := ExpandPath(path)
	if err != nil {
		return FileConfig{}, err
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		if opts.AllowMissing && errors.Is(err, os.ErrNotExist) {
			return FileConfig{}, nil
		}
		return FileConfig{}, err
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse %s: %w", expanded, err)
	}
	return expandFileConfigPaths(cfg)
}

func LoadFromEnvAndFile() (server.HostConfig, error) {
	cfgPath := os.Getenv(EnvConfigPath)
	fileCfg, err := LoadFileConfig(
		LoadFileConfigOptions{Path: cfgPath, AllowMissing: true},
	)
	if err != nil {
		return server.HostConfig{}, err
	}
	cfg := fileCfg.HostConfig()
	overlayEnv(&cfg)

	// During the migration slice, only reject legacy process env when the new
	// loader is explicitly selected. Existing cn-agents service env continues to
	// boot until the final hard cutover slice removes this gate.
	if strings.TrimSpace(cfgPath) != "" {
		if err := RejectLegacyConfig(os.Environ()); err != nil {
			return server.HostConfig{}, err
		}
	}
	return ValidateHostConfig(cfg)
}

func (fc FileConfig) HostConfig() server.HostConfig {
	repos := make(map[string]server.RepoConfig, len(fc.Projects.Repos))
	for name, repo := range fc.Projects.Repos {
		checkouts := make(map[string]server.CheckoutConfig, len(repo.Checkouts))
		for checkoutName, checkout := range repo.Checkouts {
			checkouts[checkoutName] = server.CheckoutConfig(checkout)
		}
		repos[name] = server.RepoConfig{
			GitHubURL:        repo.GitHubURL,
			DefaultBranch:    repo.DefaultBranch,
			DefaultCheckout:  repo.DefaultCheckout,
			BaselineCheckout: repo.BaselineCheckout,
			Checkouts:        checkouts,
		}
	}
	return server.HostConfig{
		App:     fc.App,
		Runtime: fc.Runtime,
		Auth:    fc.Auth,
		Web:     fc.Web,
		Projects: server.ProjectsConfig{
			DefaultRepo:             fc.Projects.DefaultRepo,
			DefaultCheckout:         fc.Projects.DefaultCheckout,
			DefaultBaselineCheckout: fc.Projects.DefaultBaselineCheckout,
			Repos:                   repos,
		},
		Workspaces: fc.Workspaces,
		Deploy:     fc.Deploy,
	}
}

func ValidateHostConfig(cfg server.HostConfig) (server.HostConfig, error) {
	var err error
	if cfg.App.Name == "" {
		cfg.App.Name = "Vamos"
	}
	if cfg.App.AccountLabel == "" {
		cfg.App.AccountLabel = "your account"
	}
	if cfg.Runtime.StateDir == "" {
		cfg.Runtime.StateDir, err = DefaultStateDir()
		if err != nil {
			return cfg, err
		}
	}
	if cfg.Runtime.StateDir, err = ExpandStatePath(
		"",
		"runtime.state_dir",
		cfg.Runtime.StateDir,
	); err != nil {
		return cfg, err
	}
	if cfg.Runtime.DatabasePath == "" {
		cfg.Runtime.DatabasePath = filepath.Join(cfg.Runtime.StateDir, "agents.db")
	}
	if cfg.Runtime.DatabasePath, err = ExpandStatePath(
		cfg.Runtime.StateDir,
		"runtime.database_path",
		cfg.Runtime.DatabasePath,
	); err != nil {
		return cfg, err
	}
	if cfg.Runtime.ThoughtsRepo, err = ExpandOptionalHostPath(
		"runtime.thoughts_repo",
		cfg.Runtime.ThoughtsRepo,
	); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.Runtime.ThoughtsRoot) == "" &&
		strings.TrimSpace(cfg.Runtime.ThoughtsRepo) != "" {
		cfg.Runtime.ThoughtsRoot = filepath.Join(cfg.Runtime.ThoughtsRepo, "thoughts")
	}
	if cfg.Runtime.ThoughtsRoot, err = ExpandOptionalHostPath(
		"runtime.thoughts_root",
		cfg.Runtime.ThoughtsRoot,
	); err != nil {
		return cfg, err
	}
	if cfg.Auth.GoogleCredentialsFile, err = ExpandOptionalHostPath(
		"auth.google_credentials_file",
		cfg.Auth.GoogleCredentialsFile,
	); err != nil {
		return cfg, err
	}
	if cfg.Workspaces.ParentDir, err = ExpandOptionalHostPath(
		"workspaces.parent_dir",
		cfg.Workspaces.ParentDir,
	); err != nil {
		return cfg, err
	}
	if cfg.Workspaces.MainCheckoutPath, err = ExpandOptionalHostPath(
		"workspaces.main_checkout_path",
		cfg.Workspaces.MainCheckoutPath,
	); err != nil {
		return cfg, err
	}
	if cfg.Workspaces.StateDir != "" {
		if cfg.Workspaces.StateDir, err = ExpandStatePath(
			cfg.Runtime.StateDir,
			"workspaces.state_dir",
			cfg.Workspaces.StateDir,
		); err != nil {
			return cfg, err
		}
	}
	for slug, checkout := range cfg.Workspaces.ConfiguredCheckouts {
		checkout.RootPath, err = ExpandOptionalHostPath(
			"workspaces.configured_checkouts."+slug+".root_path",
			checkout.RootPath,
		)
		if err != nil {
			return cfg, err
		}
		if err := validateCheckoutRole("workspaces.configured_checkouts."+slug+".role", checkout.Role); err != nil {
			return cfg, err
		}
		cfg.Workspaces.ConfiguredCheckouts[slug] = checkout
	}
	if cfg.Workspaces.MetadataDirName == "" {
		cfg.Workspaces.MetadataDirName = ".vamos"
	}
	if cfg.Workspaces.ModuleMarker == "" {
		cfg.Workspaces.ModuleMarker = "go.mod"
	}
	if cfg.Deploy.WebServiceName == "" {
		cfg.Deploy.WebServiceName = "vamos"
	}
	if cfg.Deploy.TSWorkerServiceName == "" {
		cfg.Deploy.TSWorkerServiceName = "vamos-ts-worker"
	}
	for i, forward := range cfg.Deploy.WebhookForwards {
		normalized, err := normalizeWebhookForwardConfig(i, forward)
		if err != nil {
			return cfg, err
		}
		cfg.Deploy.WebhookForwards[i] = normalized
	}
	if cfg.Web.ListenAddress == "" {
		cfg.Web.ListenAddress = ":4200"
	}
	if cfg.Web.PublicBaseURL == "" {
		cfg.Web.PublicBaseURL = "http://localhost:4200"
	}
	if cfg.Web.TemporalUIBaseURL == "" {
		cfg.Web.TemporalUIBaseURL = "http://127.0.0.1:8233"
	}
	for repoName, repo := range cfg.Projects.Repos {
		for checkoutName, checkout := range repo.Checkouts {
			checkout.RootPath, err = ExpandHostPath(
				"projects.repos."+repoName+".checkouts."+checkoutName+".root_path",
				checkout.RootPath,
			)
			if err != nil {
				return cfg, err
			}
			if err := validateCheckoutRole("projects.repos."+repoName+".checkouts."+checkoutName+".role", checkout.Role); err != nil {
				return cfg, err
			}
			repo.Checkouts[checkoutName] = checkout
		}
		cfg.Projects.Repos[repoName] = repo
	}
	return cfg, nil
}

func normalizeWebhookForwardConfig(
	index int,
	in server.WebhookForwardConfig,
) (server.WebhookForwardConfig, error) {
	path := fmt.Sprintf("deploy.webhook_forwards.%d", index)
	in.URL = strings.TrimSpace(in.URL)
	if in.URL == "" {
		return in, fmt.Errorf("%s.url is required", path)
	}
	parsed, err := url.Parse(in.URL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return in, fmt.Errorf("%s.url must be an absolute HTTP(S) URL", path)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return in, fmt.Errorf("%s.url must use http or https; got %q", path, parsed.Scheme)
	}

	if len(in.Events) == 0 {
		in.Events = []string{githubPushEvent}
	}
	for i, event := range in.Events {
		event = strings.ToLower(strings.TrimSpace(event))
		if event != githubPushEvent {
			return in, fmt.Errorf("%s.events.%d only supports push; got %q", path, i, in.Events[i])
		}
		in.Events[i] = event
	}

	if strings.TrimSpace(in.Timeout) == "" {
		in.Timeout = defaultWebhookForwardTimeout.String()
	}
	if _, err := time.ParseDuration(in.Timeout); err != nil {
		return in, fmt.Errorf("%s.timeout must be a duration: %w", path, err)
	}

	if in.BestEffort == nil {
		bestEffort := true
		in.BestEffort = &bestEffort
	}
	return in, nil
}

func validateCheckoutRole(path string, role server.CheckoutRole) error {
	switch role {
	case "", server.CheckoutRoleMain, server.CheckoutRoleStage:
		return nil
	default:
		return fmt.Errorf("%s role must be main or stage; got %q", path, role)
	}
}

func RejectLegacyConfig(environ []string) error {
	legacy := []string{"CN_AGENTS_", "REPO_PATH=", "MARKDOWN_BASE_PATH="}
	for _, item := range environ {
		for _, prefix := range legacy {
			if strings.HasPrefix(item, prefix) {
				return fmt.Errorf(
					"legacy cn-agents config %q is not supported after Vamos cutover; use VAMOS_* or YAML",
					strings.SplitN(item, "=", 2)[0],
				)
			}
		}
	}
	return nil
}

func expandFileConfigPaths(cfg FileConfig) (FileConfig, error) {
	for i, repo := range cfg.Deploy.WebhookRepos {
		if strings.TrimSpace(repo.RepoPath) != "" {
			expanded, err := ExpandPath(repo.RepoPath)
			if err != nil {
				return FileConfig{}, fmt.Errorf("deploy.webhook_repos.%d.repo_path: %w", i, err)
			}
			repo.RepoPath = expanded
		}
		if strings.TrimSpace(repo.RebuildScript) != "" {
			expanded, err := ExpandPath(repo.RebuildScript)
			if err != nil {
				return FileConfig{}, fmt.Errorf("deploy.webhook_repos.%d.rebuild_script: %w", i, err)
			}
			repo.RebuildScript = expanded
		}
		cfg.Deploy.WebhookRepos[i] = repo
	}
	for repoName, repo := range cfg.Projects.Repos {
		for checkoutName, checkout := range repo.Checkouts {
			root, err := ExpandPath(checkout.RootPath)
			if err != nil {
				return FileConfig{}, fmt.Errorf(
					"repos.%s.checkouts.%s.root_path: %w",
					repoName,
					checkoutName,
					err,
				)
			}
			checkout.RootPath = root
			repo.Checkouts[checkoutName] = checkout
		}
		cfg.Projects.Repos[repoName] = repo
	}
	return cfg, nil
}

func overlayEnv(cfg *server.HostConfig) {
	overlayString(&cfg.Runtime.ThoughtsRepo, "VAMOS_THOUGHTS_REPO")
	overlayString(&cfg.Runtime.ThoughtsRoot, "VAMOS_THOUGHTS_ROOT")
	overlayString(&cfg.Runtime.DatabasePath, "VAMOS_DATABASE_PATH")
	overlayString(&cfg.Auth.GoogleCredentialsFile, "GOOGLE_CREDENTIALS_FILE")
	overlayList(&cfg.Auth.AllowedDomains, "AUTH_ALLOWED_DOMAINS")
	overlayList(&cfg.Auth.WhitelistedEmails, "AUTH_WHITELISTED_EMAILS")
	overlayString(&cfg.Deploy.WebhookSecret, "WEBHOOK_SECRET")
	overlayString(&cfg.Auth.InternalToken, "VAMOS_INTERNAL_TOKEN")
	overlayString(&cfg.Auth.DevAuthSigningKey, "VAMOS_DEV_AUTH_SIGNING_KEY")
	overlayString(&cfg.Auth.DevAuthVerifyKey, "VAMOS_DEV_AUTH_VERIFY_KEY")
	overlayBool(&cfg.Auth.PlaywrightEnabled, "VAMOS_PLAYWRIGHT_AUTH_ENABLED")
	overlayString(&cfg.Auth.PlaywrightEmail, "VAMOS_PLAYWRIGHT_AUTH_EMAIL")
	overlayString(&cfg.Auth.PlaywrightToken, "VAMOS_PLAYWRIGHT_AUTH_TOKEN")
	overlayString(&cfg.Web.ListenAddress, "VAMOS_LISTEN_ADDRESS")
	overlayString(&cfg.Web.PublicBaseURL, "VAMOS_PUBLIC_BASE_URL")
	overlayString(&cfg.Workspaces.Mode, "VAMOS_WORKSPACE_MODE")
	overlayString(&cfg.Workspaces.Domain, "VAMOS_WORKSPACE_DOMAIN")
	overlayString(&cfg.Workspaces.ParentDir, "VAMOS_WORKSPACE_PARENT_DIR")
	overlayString(&cfg.Workspaces.StateDir, "VAMOS_WORKSPACE_STATE_DIR")
	overlayString(&cfg.Workspaces.MetadataDirName, "VAMOS_WORKSPACE_METADATA_DIR")
	overlayString(&cfg.Workspaces.Slug, "VAMOS_WORKSPACE_SLUG")
	overlayString(&cfg.Workspaces.ManagerURL, "VAMOS_WORKSPACE_MANAGER_URL")
	overlayString(&cfg.Workspaces.RestartToken, "VAMOS_WORKSPACE_RESTART_TOKEN")
}

func overlayString(dst *string, envName string) {
	if value, ok := os.LookupEnv(envName); ok {
		*dst = value
	}
}

func overlayList(dst *[]string, envName string) {
	value, ok := os.LookupEnv(envName)
	if !ok {
		return
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			items = append(items, item)
		}
	}
	*dst = items
}

func overlayBool(dst *bool, envName string) {
	value, ok := os.LookupEnv(envName)
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		*dst = true
	case "0", "false", "no", "off", "":
		*dst = false
	}
}
