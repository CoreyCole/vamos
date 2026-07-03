package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Options contains host-provided runtime configuration for a Vamos server.
type Options struct {
	Config HostConfig
}

// HostConfig is the reusable, white-label configuration boundary between a
// private host repository and the Vamos server runtime.
type HostConfig struct {
	App        AppConfig
	Runtime    RuntimeConfig
	Auth       AuthConfig
	Web        WebConfig
	Projects   ProjectsConfig
	Workspaces WorkspaceConfig
	Deploy     DeployConfig
}

type AppConfig struct {
	Name         string `yaml:"name"`
	AccountLabel string `yaml:"account_label"`
}

type RuntimeConfig struct {
	ThoughtsRepo string `yaml:"thoughts_repo"`
	ThoughtsRoot string `yaml:"thoughts_root"`
	StateDir     string `yaml:"state_dir"`
	DatabasePath string `yaml:"database_path"`
}

func (cfg RuntimeConfig) ResolveThoughtsRepo() (string, error) {
	return cleanRequiredAbsHostPath("runtime.thoughts_repo", cfg.ThoughtsRepo)
}

func (cfg RuntimeConfig) ResolveThoughtsRoot() (string, error) {
	root := strings.TrimSpace(cfg.ThoughtsRoot)
	if root == "" && strings.TrimSpace(cfg.ThoughtsRepo) != "" {
		root = filepath.Join(cfg.ThoughtsRepo, "thoughts")
	}
	return cleanRequiredAbsHostPath("runtime.thoughts_root", root)
}

func cleanRequiredAbsHostPath(name, value string) (string, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if clean == "~" || strings.HasPrefix(clean, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if clean == "~" {
			return home, nil
		}
		clean = filepath.Join(home, strings.TrimPrefix(clean, "~/"))
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("%s must be absolute or ~/ relative; got %q", name, value)
	}
	return filepath.Clean(clean), nil
}

type AuthConfig struct {
	GoogleCredentialsFile string   `yaml:"google_credentials_file"`
	AllowedDomains        []string `yaml:"allowed_domains"`
	WhitelistedEmails     []string `yaml:"whitelisted_emails"`
	DevAuthSigningKey     string   `yaml:"dev_auth_signing_key"`
	DevAuthVerifyKey      string   `yaml:"dev_auth_verify_key"`
	PlaywrightEnabled     bool     `yaml:"playwright_enabled"`
	PlaywrightEmail       string   `yaml:"playwright_email"`
	PlaywrightToken       string   `yaml:"playwright_token"`
	InternalToken         string   `yaml:"internal_token"`
	InternalAllowLoopback bool     `yaml:"internal_allow_loopback"`
}

type WebConfig struct {
	ListenAddress           string   `yaml:"listen_address"`
	PublicBaseURL           string   `yaml:"public_base_url"`
	InternalCallbackBaseURL string   `yaml:"internal_callback_base_url"`
	CORSAllowedOrigins      []string `yaml:"cors_allowed_origins"`
	TemporalUIBaseURL       string   `yaml:"temporal_ui_base_url"`
}

type ProjectsConfig struct {
	DefaultRepo             string                `yaml:"default_repo"`
	DefaultCheckout         string                `yaml:"default_checkout"`
	DefaultBaselineCheckout string                `yaml:"default_baseline_checkout"`
	Repos                   map[string]RepoConfig `yaml:"repos"`
}

type RepoConfig struct {
	GitHubURL        string                    `yaml:"github_url"`
	DefaultBranch    string                    `yaml:"default_branch"`
	DefaultCheckout  string                    `yaml:"default_checkout"`
	BaselineCheckout string                    `yaml:"baseline_checkout"`
	Checkouts        map[string]CheckoutConfig `yaml:"checkouts"`
}

type CheckoutRole string

const (
	CheckoutRoleMain  CheckoutRole = "main"
	CheckoutRoleStage CheckoutRole = "stage"
)

type CheckoutConfig struct {
	RootPath          string       `yaml:"root_path"`
	Purpose           string       `yaml:"purpose"`
	Role              CheckoutRole `yaml:"role"`
	MustBeClean       bool         `yaml:"must_be_clean"`
	MustBeLatest      bool         `yaml:"must_be_latest"`
	WebhookSyncBranch string       `yaml:"webhook_sync_branch"`
}

type WorkspaceConfig struct {
	Mode                string                             `yaml:"mode"`
	Domain              string                             `yaml:"domain"`
	ParentDir           string                             `yaml:"parent_dir"`
	StateDir            string                             `yaml:"state_dir"`
	MetadataDirName     string                             `yaml:"metadata_dir_name"`
	CheckoutPrefixes    []string                           `yaml:"checkout_prefixes"`
	MainCheckoutName    string                             `yaml:"main_checkout_name"`
	MainCheckoutPath    string                             `yaml:"main_checkout_path"`
	ModuleMarker        string                             `yaml:"module_marker"`
	PackageSubdir       string                             `yaml:"package_subdir"`
	ManagerURL          string                             `yaml:"manager_url"`
	RestartToken        string                             `yaml:"restart_token"`
	Slug                string                             `yaml:"slug"`
	ConfiguredCheckouts map[string]WorkspaceCheckoutConfig `yaml:"configured_checkouts"`
	ProcessCommands     WorkspaceProcessCommands           `yaml:"process_commands"`
}

type WorkspaceCheckoutConfig struct {
	RootPath    string       `yaml:"root_path"`
	DisplayName string       `yaml:"display_name"`
	IsMain      bool         `yaml:"is_main"`
	Role        CheckoutRole `yaml:"role"`
	ProjectID   string       `yaml:"project_id"`
}

type WorkspaceProcessCommands struct {
	Web      []string `yaml:"web"`
	Temporal []string `yaml:"temporal"`
	TSWorker []string `yaml:"ts_worker"`
}

type DeployConfig struct {
	WebServiceName      string                 `yaml:"web_service_name"`
	TSWorkerServiceName string                 `yaml:"ts_worker_service_name"`
	RebuildScript       string                 `yaml:"rebuild_script"`
	WebhookSecret       string                 `yaml:"webhook_secret"`
	WebhookRepos        []WebhookRepoConfig    `yaml:"webhook_repos"`
	WebhookForwards     []WebhookForwardConfig `yaml:"webhook_forwards"`
	ThoughtsBaseURL     string                 `yaml:"thoughts_base_url"`
	GitHubBaseURL       string                 `yaml:"github_base_url"`
}

type WebhookRepoConfig struct {
	GitHubRepo    string `yaml:"github_repo"`
	RepoPath      string `yaml:"repo_path"`
	RebuildScript string `yaml:"rebuild_script"`
	SyncThoughts  *bool  `yaml:"sync_thoughts"`
}

type WebhookForwardConfig struct {
	URL         string   `yaml:"url"`
	GitHubRepos []string `yaml:"github_repos"`
	Events      []string `yaml:"events"`
	Secret      string   `yaml:"secret"`
	Timeout     string   `yaml:"timeout"`
	BestEffort  *bool    `yaml:"best_effort"`
}

func MustRun(opts Options) {
	if err := Run(context.Background(), opts); err != nil {
		log.Fatal(err)
	}
}

func Run(ctx context.Context, opts Options) error {
	_ = ctx
	_, err := normalizeHostConfig(opts.Config)
	return err
}

func normalizeHostConfig(cfg HostConfig) (HostConfig, error) {
	return cfg, nil
}
