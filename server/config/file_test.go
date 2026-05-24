package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server"
)

func TestExpandPathHomeRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ExpandPath("~/workspace")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}
	want := filepath.Join(home, "workspace")
	if got != want {
		t.Fatalf("ExpandPath() = %q, want %q", got, want)
	}
}

func TestLoadFileConfigAllowsMissing(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFileConfig(LoadFileConfigOptions{
		Path:         filepath.Join(t.TempDir(), "missing.yml"),
		AllowMissing: true,
	})
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if cfg.Projects.DefaultRepo != "" || len(cfg.Projects.Repos) != 0 {
		t.Fatalf("LoadFileConfig() = %+v, want zero config", cfg)
	}
}

func TestLoadFileConfigRejectsMissingWhenRequired(t *testing.T) {
	t.Parallel()

	_, err := LoadFileConfig(LoadFileConfigOptions{
		Path:         filepath.Join(t.TempDir(), "missing.yml"),
		AllowMissing: false,
	})
	if err == nil {
		t.Fatal("LoadFileConfig() error = nil, want missing-file error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("LoadFileConfig() error = %v, want not-exist error", err)
	}
}

func TestLoadFileConfigLoadsFullYAMLAndExpandsPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`app:
  name: Vamos
  account_label: company account
runtime:
  thoughts_repo: ~/host
  thoughts_root: ~/host/thoughts
  state_dir: ~/.local/state/vamos
auth:
  google_credentials_file: ~/client-secret.json
web:
  listen_address: :4200
projects:
  default_repo: vamos
  default_checkout: local
  repos:
    vamos:
      github_url: https://github.com/CoreyCole/vamos
      default_branch: main
      default_checkout: local
      baseline_checkout: vamos-main
      checkouts:
        local:
          root_path: ~/cn/chestnut-flake/vamos
          purpose: working
        vamos-main:
          root_path: ~/cn/chestnut-flake/vamos-main
          purpose: baseline
          must_be_clean: true
          must_be_latest: true
          webhook_sync_branch: main
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fileCfg, err := LoadFileConfig(LoadFileConfigOptions{Path: path})
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	cfg, err := ValidateHostConfig(fileCfg.HostConfig())
	if err != nil {
		t.Fatalf("ValidateHostConfig() error = %v", err)
	}
	wantThoughtsRoot := filepath.Join(home, "host", "thoughts")
	if cfg.Runtime.ThoughtsRoot != wantThoughtsRoot {
		t.Fatalf("ThoughtsRoot = %q, want %q", cfg.Runtime.ThoughtsRoot, wantThoughtsRoot)
	}
	wantCredentials := filepath.Join(home, "client-secret.json")
	if cfg.Auth.GoogleCredentialsFile != wantCredentials {
		t.Fatalf(
			"GoogleCredentialsFile = %q, want %q",
			cfg.Auth.GoogleCredentialsFile,
			wantCredentials,
		)
	}
	gotCheckout := cfg.Projects.Repos["vamos"].Checkouts["vamos-main"]
	wantCheckout := filepath.Join(home, "cn", "chestnut-flake", "vamos-main")
	if gotCheckout.RootPath != wantCheckout {
		t.Fatalf("baseline RootPath = %q, want %q", gotCheckout.RootPath, wantCheckout)
	}
	if gotCheckout.WebhookSyncBranch != "main" || !gotCheckout.MustBeClean ||
		!gotCheckout.MustBeLatest {
		t.Fatalf("baseline checkout = %+v, want webhook/clean/latest", gotCheckout)
	}
}

func TestValidateHostConfigDefaultsStateAndDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := ValidateHostConfig(serverlessHostConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("ValidateHostConfig() error = %v", err)
	}
	wantState := filepath.Join(home, ".local", "state", "vamos")
	if cfg.Runtime.StateDir != wantState {
		t.Fatalf("StateDir = %q, want %q", cfg.Runtime.StateDir, wantState)
	}
	wantDB := filepath.Join(wantState, "agents.db")
	if cfg.Runtime.DatabasePath != wantDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.Runtime.DatabasePath, wantDB)
	}
	if cfg.Workspaces.MetadataDirName != ".vamos" {
		t.Fatalf("MetadataDirName = %q, want .vamos", cfg.Workspaces.MetadataDirName)
	}
	if cfg.Deploy.WebServiceName != "vamos" ||
		cfg.Deploy.TSWorkerServiceName != "vamos-ts-worker" {
		t.Fatalf("Deploy defaults = %+v", cfg.Deploy)
	}
}

func TestValidateHostConfigDefaultsThoughtsRootFromRepo(t *testing.T) {
	root := t.TempDir()
	cfg := serverlessHostConfig(root)
	cfg.Runtime.ThoughtsRoot = ""
	validated, err := ValidateHostConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateHostConfig() error = %v", err)
	}
	want := filepath.Join(root, "host", "thoughts")
	if validated.Runtime.ThoughtsRoot != want {
		t.Fatalf("ThoughtsRoot = %q, want %q", validated.Runtime.ThoughtsRoot, want)
	}
}

func TestNormalHostConfigDoesNotRequireVamosSourceCheckout(t *testing.T) {
	cfg := serverlessHostConfig(t.TempDir())
	cfg.Workspaces.MainCheckoutPath = ""
	if _, err := ValidateHostConfig(cfg); err != nil {
		t.Fatalf("ValidateHostConfig() error = %v", err)
	}
}

func TestLoadFromEnvAndFileOverlaysWorkspaceChildEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`app:
  name: Vamos
runtime:
  thoughts_repo: ~/host
  thoughts_root: ~/host/thoughts
  state_dir: ~/.local/state/vamos
auth:
  dev_auth_signing_key: signing-key
web:
  listen_address: :4200
  public_base_url: https://main.example.test
workspaces:
  mode: manager
  domain: example.test
  slug: main
  manager_url: https://main.example.test
  restart_token: manager-token
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(EnvConfigPath, path)
	t.Setenv("VAMOS_WORKSPACE_MODE", "child")
	t.Setenv("VAMOS_WORKSPACE_SLUG", "work")
	t.Setenv("VAMOS_WORKSPACE_MANAGER_URL", "https://main.example.test")
	t.Setenv("VAMOS_WORKSPACE_RESTART_TOKEN", "child-token")
	t.Setenv("VAMOS_DEV_AUTH_VERIFY_KEY", "verify-key")

	cfg, err := LoadFromEnvAndFile()
	if err != nil {
		t.Fatalf("LoadFromEnvAndFile() error = %v", err)
	}
	if cfg.Workspaces.Mode != "child" || cfg.Workspaces.Slug != "work" ||
		cfg.Workspaces.RestartToken != "child-token" ||
		cfg.Auth.DevAuthVerifyKey != "verify-key" {
		t.Fatalf("workspace env overlay failed: %+v auth=%+v", cfg.Workspaces, cfg.Auth)
	}
}

func TestRejectLegacyConfig(t *testing.T) {
	for _, env := range []string{
		"CN_AGENTS_LISTEN_ADDRESS=:4200",
		"CN_AGENTS_WORKSPACE_MODE=manager",
		"REPO_PATH=/tmp/cn-agents",
		"MARKDOWN_BASE_PATH=/tmp/cn-agents/thoughts",
	} {
		err := RejectLegacyConfig([]string{env})
		if err == nil {
			t.Fatalf("RejectLegacyConfig(%q) error = nil, want error", env)
		}
		key := strings.SplitN(env, "=", 2)[0]
		if !strings.Contains(err.Error(), "Vamos") ||
			!strings.Contains(err.Error(), key) {
			t.Fatalf("RejectLegacyConfig(%q) error = %v, want Vamos %s message", env, err, key)
		}
	}
}

func TestValidateHostConfigRejectsRelativeThoughtsRepo(t *testing.T) {
	cfg := serverlessHostConfig(t.TempDir())
	cfg.Runtime.ThoughtsRepo = "cn-agents"
	_, err := ValidateHostConfig(cfg)
	if err == nil {
		t.Fatal("ValidateHostConfig() error = nil, want relative path error")
	}
	if !strings.Contains(err.Error(), "runtime.thoughts_repo") {
		t.Fatalf("ValidateHostConfig() error = %v, want thoughts repo context", err)
	}
}

func TestBaselineCheckoutDevelopBranchAccepted(t *testing.T) {
	root := t.TempDir()
	cfg := serverlessHostConfig(root)
	cfg.Projects.Repos = map[string]server.RepoConfig{}
	cfg.Projects.Repos["monorepo"] = serverRepoForTest{
		GitHubURL:        "https://github.com/premiumlabs/monorepo",
		DefaultBranch:    "develop",
		DefaultCheckout:  "local",
		BaselineCheckout: "monorepo-main",
		Checkouts: map[string]string{
			"local":         filepath.Join(root, "monorepo"),
			"monorepo-main": filepath.Join(root, "monorepo-main"),
		},
		WebhookBranch: "develop",
	}.serverRepo()
	if _, err := ValidateHostConfig(cfg); err != nil {
		t.Fatalf("ValidateHostConfig() error = %v", err)
	}
}

func TestLoadFileConfigReportsMalformedYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("repos: [not valid"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFileConfig(LoadFileConfigOptions{Path: path})
	if err == nil {
		t.Fatal("LoadFileConfig() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse ") {
		t.Fatalf("LoadFileConfig() error = %v, want parse context", err)
	}
}

func serverlessHostConfig(root string) server.HostConfig {
	return server.HostConfig{
		Runtime: server.RuntimeConfig{
			ThoughtsRepo: filepath.Join(root, "host"),
			ThoughtsRoot: filepath.Join(root, "host", "thoughts"),
		},
	}
}

type serverRepoForTest struct {
	GitHubURL        string
	DefaultBranch    string
	DefaultCheckout  string
	BaselineCheckout string
	Checkouts        map[string]string
	WebhookBranch    string
}

func (r serverRepoForTest) serverRepo() server.RepoConfig {
	checkouts := make(map[string]server.CheckoutConfig, len(r.Checkouts))
	for name, root := range r.Checkouts {
		checkout := server.CheckoutConfig{RootPath: root}
		if name == r.BaselineCheckout {
			checkout.Purpose = "baseline"
			checkout.MustBeClean = true
			checkout.MustBeLatest = true
			checkout.WebhookSyncBranch = r.WebhookBranch
		}
		checkouts[name] = checkout
	}
	return server.RepoConfig{
		GitHubURL:        r.GitHubURL,
		DefaultBranch:    r.DefaultBranch,
		DefaultCheckout:  r.DefaultCheckout,
		BaselineCheckout: r.BaselineCheckout,
		Checkouts:        checkouts,
	}
}

type serverRepoForTestMap map[string]serverRepoForTest

func (m serverRepoForTestMap) asServerRepos() map[string]server.RepoConfig {
	repos := make(map[string]server.RepoConfig, len(m))
	for name, repo := range m {
		repos[name] = repo.serverRepo()
	}
	return repos
}
