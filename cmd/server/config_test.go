package main

import (
	"testing"

	server "github.com/CoreyCole/vamos/server"
)

func TestApplyVamosEnvOverridesPrefersRuntimeWorkspaceEnv(t *testing.T) {
	t.Setenv("VAMOS_LISTEN_ADDRESS", "127.0.0.1:0")
	t.Setenv("VAMOS_PUBLIC_BASE_URL", "https://feature.workspaces.test")
	t.Setenv("VAMOS_INTERNAL_CALLBACK_BASE_URL", "http://127.0.0.1:1234")
	t.Setenv("VAMOS_WORKSPACE_MODE", "child")
	t.Setenv("VAMOS_WORKSPACE_SLUG", "feature")
	t.Setenv("VAMOS_WORKSPACE_MANAGER_URL", "https://main.workspaces.test")
	t.Setenv("VAMOS_WORKSPACE_RESTART_TOKEN", "restart-token")
	t.Setenv("VAMOS_DEV_AUTH_VERIFY_KEY", "verify-key")
	t.Setenv("VAMOS_THOUGHTS_REPO", "/repo")
	t.Setenv("VAMOS_THOUGHTS_ROOT", "/repo/thoughts")
	t.Setenv("VAMOS_DATABASE_PATH", "/repo/.vamos/state/agents.db")
	t.Setenv("VAMOS_INTERNAL_TOKEN", "internal-token")
	t.Setenv("VAMOS_DEFAULT_CWD", "/repo/workspace")
	t.Setenv("VAMOS_PLAYWRIGHT_AUTH_ENABLED", "true")
	t.Setenv("VAMOS_PLAYWRIGHT_AUTH_EMAIL", "operator@example.com")
	t.Setenv("VAMOS_PLAYWRIGHT_AUTH_TOKEN", "playwright-token")

	cfg := applyVamosEnvOverrides(Config{
		ListenAddress:          ":4200",
		PublicBaseURL:          "https://stale.example.test",
		WorkspaceMode:          "manager",
		WorkspaceSlug:          "work",
		RepoPath:               "/stale/repo",
		MarkdownBasePath:       "/stale/thoughts",
		InternalAgentChatToken: "stale-token",
		AgentChatDefaultDir:    "/stale/cwd",
	})

	if cfg.ListenAddress != "127.0.0.1:0" {
		t.Fatalf("ListenAddress = %q", cfg.ListenAddress)
	}
	if cfg.PublicBaseURL != "https://feature.workspaces.test" {
		t.Fatalf("PublicBaseURL = %q", cfg.PublicBaseURL)
	}
	if cfg.InternalCallbackBaseURL != "http://127.0.0.1:1234" {
		t.Fatalf("InternalCallbackBaseURL = %q", cfg.InternalCallbackBaseURL)
	}
	if cfg.WorkspaceMode != "child" || cfg.WorkspaceSlug != "feature" {
		t.Fatalf("workspace mode/slug = %q/%q", cfg.WorkspaceMode, cfg.WorkspaceSlug)
	}
	if cfg.WorkspaceManagerURL != "https://main.workspaces.test" || cfg.WorkspaceRestartToken != "restart-token" {
		t.Fatalf("workspace manager/restart = %q/%q", cfg.WorkspaceManagerURL, cfg.WorkspaceRestartToken)
	}
	if cfg.DevAuthVerifyKey != "verify-key" {
		t.Fatalf("DevAuthVerifyKey = %q", cfg.DevAuthVerifyKey)
	}
	if cfg.RepoPath != "/repo" || cfg.MarkdownBasePath != "/repo/thoughts" {
		t.Fatalf("repo/thoughts = %q/%q", cfg.RepoPath, cfg.MarkdownBasePath)
	}
	if cfg.DatabasePath != "/repo/.vamos/state/agents.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.InternalAgentChatToken != "internal-token" {
		t.Fatalf("InternalAgentChatToken = %q", cfg.InternalAgentChatToken)
	}
	if cfg.AgentChatDefaultDir != "/repo/workspace" {
		t.Fatalf("AgentChatDefaultDir = %q", cfg.AgentChatDefaultDir)
	}
	if !cfg.PlaywrightAuthEnabled || cfg.PlaywrightAuthEmail != "operator@example.com" || cfg.PlaywrightAuthToken != "playwright-token" {
		t.Fatalf("playwright cfg = %#v", cfg)
	}
}

func TestVamosEnvOverridesHostConfigCallbackDefault(t *testing.T) {
	t.Setenv("VAMOS_INTERNAL_CALLBACK_BASE_URL", "http://127.0.0.1:1234")

	cfg := applyHostConfigToLegacyConfig(Config{}, server.HostConfig{
		Web: server.WebConfig{InternalCallbackBaseURL: "http://localhost:4200"},
	})
	cfg = applyVamosEnvOverrides(cfg)

	if cfg.InternalCallbackBaseURL != "http://127.0.0.1:1234" {
		t.Fatalf("InternalCallbackBaseURL = %q", cfg.InternalCallbackBaseURL)
	}
}

func TestApplyVamosEnvOverridesInvalidBoolKeepsLegacyValue(t *testing.T) {
	t.Setenv("VAMOS_PLAYWRIGHT_AUTH_ENABLED", "not-a-bool")

	cfg := applyVamosEnvOverrides(Config{PlaywrightAuthEnabled: true})

	if !cfg.PlaywrightAuthEnabled {
		t.Fatal("invalid VAMOS_PLAYWRIGHT_AUTH_ENABLED should keep existing value")
	}
}
