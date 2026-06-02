package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agentchat"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func TestRootUsesAuthenticatedChatWorkbench(t *testing.T) {
	t.Parallel()

	e := echo.New()
	authRan := false
	authStopsBeforeHandler := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authRan = true
			return c.NoContent(http.StatusNoContent)
		}
	}
	registerAgentChatEntryRoutes(e, authStopsBeforeHandler, &agentchat.Handler{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if !authRan {
		t.Fatal("root route did not run auth middleware")
	}
}

func TestAgentChatPageRoutesNotFound(t *testing.T) {
	t.Parallel()

	e := echo.New()
	registerAgentChatEntryRoutes(e, func(next echo.HandlerFunc) echo.HandlerFunc {
		return next
	}, &agentchat.Handler{}, nil)

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "bare retired page", path: "/agent-chat"},
		{name: "slash retired page", path: "/agent-chat/"},
		{name: "workspace page", path: "/agent-chat/foo"},
		{name: "thread page", path: "/agent-chat/foo/thread/bar"},
		{name: "document page", path: "/agent-chat/thoughts/x.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d, want 404", tc.path, rec.Code)
			}
		})
	}
}

func TestDefaultStatePathUsesXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	got, err := defaultStatePath("agents.db")
	if err != nil {
		t.Fatalf("defaultStatePath() error = %v", err)
	}
	want := filepath.Join(stateHome, "cn-agents", "agents.db")
	if got != want {
		t.Fatalf("defaultStatePath() = %q, want %q", got, want)
	}
}

func TestExpandRuntimePathsExpandsHomeRelativePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := expandRuntimePaths(Config{
		DatabasePath:          "~/.local/state/cn-agents/agents.db",
		GoogleCredentialsFile: "~/cn-agents/client_secret.json",
		MarkdownBasePath:      "~/cn-agents/thoughts",
		RepoPath:              "~/cn-agents",
		AgentChatDefaultDir:   "~/cn-agents/thoughts/creative-mode-agent/plans",
		WorkspaceParentDir:    "~/cn",
		WorkspaceStateDir:     "~/.local/state/cn-agents/workspaces",
		ConfigPath:            "~/.config/agents/config.yml",
	})
	if err != nil {
		t.Fatalf("expandRuntimePaths() error = %v", err)
	}
	for name, got := range map[string]string{
		"DatabasePath":          cfg.DatabasePath,
		"GoogleCredentialsFile": cfg.GoogleCredentialsFile,
		"MarkdownBasePath":      cfg.MarkdownBasePath,
		"RepoPath":              cfg.RepoPath,
		"AgentChatDefaultDir":   cfg.AgentChatDefaultDir,
		"WorkspaceParentDir":    cfg.WorkspaceParentDir,
		"WorkspaceStateDir":     cfg.WorkspaceStateDir,
		"ConfigPath":            cfg.ConfigPath,
	} {
		if !strings.HasPrefix(got, home+string(os.PathSeparator)) {
			t.Fatalf("%s = %q, want under home %q", name, got, home)
		}
	}
}

func TestNormalizeListenAddress(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ":4200"},
		{"4301", ":4301"},
		{":4301", ":4301"},
		{"127.0.0.1:4301", "127.0.0.1:4301"},
	}
	for _, tc := range cases {
		got, err := normalizeListenAddress(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeListenAddressRejectsBadValue(t *testing.T) {
	for _, in := range []string{"abc", ":abc", "127.0.0.1:http"} {
		if _, err := normalizeListenAddress(in); err == nil {
			t.Fatalf("normalizeListenAddress(%q) error=nil", in)
		}
	}
}

func TestAgentChatCallbackBaseURL(t *testing.T) {
	got := agentChatCallbackBaseURL(Config{
		InternalCallbackBaseURL: "http://127.0.0.1:4301/",
		PublicBaseURL:           "https://foo.cn-agents.test",
	})
	if got != "http://127.0.0.1:4301" {
		t.Fatalf("got %q", got)
	}
	got = agentChatCallbackBaseURL(Config{PublicBaseURL: "https://foo.cn-agents.test/"})
	if got != "https://foo.cn-agents.test" {
		t.Fatalf("got %q", got)
	}
}

func TestAgentChatDefaultDirPrefersExplicitDir(t *testing.T) {
	got := agentChatDefaultDir(Config{
		RepoPath:            "/repo",
		AgentChatDefaultDir: "/explicit",
	})
	if got != "/explicit" {
		t.Fatalf("agentChatDefaultDir() = %q, want /explicit", got)
	}
}

func TestAgentChatDefaultDirFallsBackToRepoPath(t *testing.T) {
	got := agentChatDefaultDir(Config{RepoPath: "/repo"})
	if got != "/repo" {
		t.Fatalf("agentChatDefaultDir() = %q, want /repo", got)
	}
}

func TestHostFromBaseURL(t *testing.T) {
	got := hostFromBaseURL("https://main.cn-agents.test/workspaces")
	if got != "main.cn-agents.test" {
		t.Fatalf("got %q", got)
	}
}

func TestChildWorkspacesReadOnlyEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "standalone false", cfg: Config{WorkspaceMode: "standalone"}, want: false},
		{name: "manager false", cfg: Config{WorkspaceMode: "manager"}, want: false},
		{name: "child true", cfg: Config{WorkspaceMode: "child"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := childWorkspacesReadOnlyEnabled(tt.cfg); got != tt.want {
				t.Fatalf("childWorkspacesReadOnlyEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkspaceHandlerManagerUsesFixtureForChildWithoutManager(t *testing.T) {
	t.Parallel()

	manager := workspaceHandlerManager(nil, Config{WorkspaceMode: "child", WorkspaceSlug: "feature"})
	if _, ok := manager.(*fixtureLifecycleRegistry); !ok {
		t.Fatalf("workspaceHandlerManager() = %T, want *fixtureLifecycleRegistry", manager)
	}
}

func TestNewFixtureLifecycleRegistryDescribesCurrentChild(t *testing.T) {
	t.Parallel()

	registry := newFixtureLifecycleRegistry(Config{
		WorkspaceSlug: "feature-slug",
		PublicBaseURL: "https://feature.workspaces.test/",
		RepoPath:      "/repo/feature",
	})

	snapshots, err := registry.ListLifecycle(context.Background())
	if err != nil {
		t.Fatalf("ListLifecycle() error = %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("len(snapshots) = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.Workspace.Slug != "feature-slug" || got.Workspace.CheckoutPath != "/repo/feature" || got.Workspace.Host != "feature.workspaces.test" {
		t.Fatalf("snapshot workspace = %+v", got.Workspace)
	}
	if got.DesiredState != workspaces.WorkspaceDesiredRunning || got.ObservedState != workspaces.WorkspaceObservedRunning {
		t.Fatalf("snapshot state = %s/%s", got.DesiredState, got.ObservedState)
	}
}

func TestFixtureLifecycleRegistryRejectsMutations(t *testing.T) {
	t.Parallel()

	registry := newFixtureLifecycleRegistry(Config{WorkspaceSlug: "feature"})
	if _, err := registry.Start(context.Background(), "feature"); !errors.Is(err, errFixtureWorkspacesReadOnly) {
		t.Fatalf("Start() error = %v, want read-only", err)
	}
	if _, err := registry.RequestLifecycle(context.Background(), workspaces.WorkspaceLifecycleRequest{Slug: "feature", Kind: workspaces.WorkspaceTransitionRestart}); !errors.Is(err, errFixtureWorkspacesReadOnly) {
		t.Fatalf("RequestLifecycle() error = %v, want read-only", err)
	}
}

func validSigningKeyForTest(t *testing.T) string {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return base64.RawURLEncoding.EncodeToString(seed)
}

func validVerifyKeyForTest(t *testing.T) string {
	t.Helper()
	signingKey, err := workspaces.ParseHandoffSigningKey(validSigningKeyForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := signingKey.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatal("signing key did not expose Ed25519 public key")
	}
	return workspaces.EncodeHandoffVerifyKey(publicKey)
}

func TestValidateWorkspaceConfig(t *testing.T) {
	if err := validateWorkspaceConfig(Config{WorkspaceMode: "standalone"}); err != nil {
		t.Fatalf("standalone: %v", err)
	}
	if err := validateWorkspaceConfig(Config{WorkspaceMode: "manager"}); err == nil {
		t.Fatal("manager without domain error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{WorkspaceMode: "manager", WorkspaceDomain: "cn-agents.test"},
	); err == nil {
		t.Fatal("manager without signing key error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{
			WorkspaceMode:     "manager",
			WorkspaceDomain:   "cn-agents.test",
			DevAuthSigningKey: "not-base64",
		},
	); err == nil {
		t.Fatal("manager with invalid signing key error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{
			WorkspaceMode:     "manager",
			WorkspaceDomain:   "cn-agents.test",
			DevAuthSigningKey: validSigningKeyForTest(t),
		},
	); err != nil {
		t.Fatalf("manager with signing key: %v", err)
	}
	if err := validateWorkspaceConfig(Config{WorkspaceMode: "child"}); err == nil {
		t.Fatal("child without slug error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{WorkspaceMode: "child", WorkspaceSlug: "feature"},
	); err == nil {
		t.Fatal("child without verify key error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{
			WorkspaceMode:    "child",
			WorkspaceSlug:    "feature",
			DevAuthVerifyKey: "not-base64",
		},
	); err == nil {
		t.Fatal("child with invalid verify key error=nil")
	}
	if err := validateWorkspaceConfig(
		Config{
			WorkspaceMode:    "child",
			WorkspaceSlug:    "feature",
			DevAuthVerifyKey: validVerifyKeyForTest(t),
		},
	); err != nil {
		t.Fatalf("child with verify key: %v", err)
	}
}

func TestExpandRuntimePathsRejectsHostRelativePath(t *testing.T) {
	t.Parallel()

	_, err := expandRuntimePaths(Config{
		DatabasePath:     "agents.db",
		MarkdownBasePath: "thoughts",
	})
	if err == nil {
		t.Fatal("expandRuntimePaths() error = nil, want host-relative path error")
	}
	if !strings.Contains(err.Error(), "MARKDOWN_BASE_PATH must be absolute") {
		t.Fatalf("expandRuntimePaths() error = %v, want MARKDOWN_BASE_PATH context", err)
	}
}

func TestExpandRuntimePathsAllowsMissingOptionalConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := expandRuntimePaths(Config{DatabasePath: "agents.db"})
	if err != nil {
		t.Fatalf("expandRuntimePaths() error = %v", err)
	}
	want := filepath.Join(home, ".config", "agents", "config.yml")
	if cfg.ConfigPath != want {
		t.Fatalf("ConfigPath = %q, want %q", cfg.ConfigPath, want)
	}
}

func TestExpandRuntimePathsResolvesRelativeRebuildScriptUnderModuleCWD(t *testing.T) {
	cwd := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	cfg, err := expandRuntimePaths(Config{
		DatabasePath:  "agents.db",
		ConfigPath:    "~/missing.yml",
		RebuildScript: "scripts/webhook-rebuild.sh",
	})
	if err != nil {
		t.Fatalf("expandRuntimePaths() error = %v", err)
	}
	want := filepath.Join(cwd, "scripts", "webhook-rebuild.sh")
	if cfg.RebuildScript != want {
		t.Fatalf("RebuildScript = %q, want %q", cfg.RebuildScript, want)
	}
}

func TestExpandRuntimePathsRejectsPreCutoverRelativeDatabasePath(t *testing.T) {
	t.Parallel()

	_, err := expandRuntimePaths(Config{DatabasePath: "data/thoughts.db"})
	if err == nil {
		t.Fatal("expandRuntimePaths() error = nil, want pre-cutover DB path error")
	}
	if !strings.Contains(err.Error(), "ambiguous after pkg/agents cwd cutover") {
		t.Fatalf("expandRuntimePaths() error = %v, want cutover guard", err)
	}
}

func TestWorkspaceReleaseHandlerOptionsShowReleaseLanesWithoutTemporalQueue(t *testing.T) {
	t.Parallel()

	_, releaseRegistry, err := workspaces.BuildDefaultReleaseRegistry("stage", "main")
	if err != nil {
		t.Fatalf("BuildDefaultReleaseRegistry() error = %v", err)
	}
	handler := workspaces.NewHandler(
		fakeMainTestLifecycleManager{snapshots: []workspaces.WorkspaceLifecycleSnapshot{
			{Workspace: workspaces.Workspace{Slug: "stage", DisplayName: "Stage", Status: workspaces.StatusRunning, URL: "https://stage.workspaces.test/", Commit: "abcdef1234567890"}},
			{Workspace: workspaces.Workspace{Slug: "main", DisplayName: "Main", Status: workspaces.StatusStopped, IsMain: true, Commit: "1234567890abcdef"}},
		}},
		"https://main.workspaces.test",
		"main",
		workspaceReleaseHandlerOptions(releaseRegistry, nil, nil)...,
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	if err := handler.HandleWorkspacesPage(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleWorkspacesPage() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Release lanes",
		`action="/workspaces/stage/stop"`,
		`action="/workspaces/main/start"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("workspaces page missing %q: %s", want, body)
		}
	}
}

type fakeMainTestLifecycleManager struct {
	snapshots []workspaces.WorkspaceLifecycleSnapshot
}

func (f fakeMainTestLifecycleManager) Refresh(context.Context) error { return nil }

func (f fakeMainTestLifecycleManager) List() []workspaces.Workspace {
	items := make([]workspaces.Workspace, 0, len(f.snapshots))
	for _, snap := range f.snapshots {
		items = append(items, snap.Workspace)
	}
	return items
}

func (f fakeMainTestLifecycleManager) Lookup(slug string) (workspaces.Workspace, bool) {
	for _, snap := range f.snapshots {
		if snap.Workspace.Slug == slug {
			return snap.Workspace, true
		}
	}
	return workspaces.Workspace{}, false
}

func (f fakeMainTestLifecycleManager) LookupHost(string) (workspaces.Workspace, bool) {
	return workspaces.Workspace{}, false
}

func (f fakeMainTestLifecycleManager) Start(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, nil
}

func (f fakeMainTestLifecycleManager) Stop(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, nil
}

func (f fakeMainTestLifecycleManager) Restart(context.Context, string) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, nil
}

func (f fakeMainTestLifecycleManager) RequestLifecycle(context.Context, workspaces.WorkspaceLifecycleRequest) (workspaces.WorkspaceLifecycleSnapshot, error) {
	return workspaces.WorkspaceLifecycleSnapshot{}, nil
}

func (f fakeMainTestLifecycleManager) ListLifecycle(context.Context) ([]workspaces.WorkspaceLifecycleSnapshot, error) {
	return f.snapshots, nil
}

func (f fakeMainTestLifecycleManager) CompleteTransition(context.Context, string, string, workspaces.WorkspaceTransitionResult) error {
	return nil
}
