package vamos

import (
	"os"
	"path/filepath"
	"strings"

	oldruntime "github.com/CoreyCole/vamos/pkg/e2e/runtime"
	oldselectors "github.com/CoreyCole/vamos/pkg/e2e/selectors"
	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
)

// bridgeContext keeps legacy Vamos-only step helpers compiling until they move to
// typed DatastarUI steps. New app runtime/auth/preflight/fixtures use the direct
// DatastarUI facade in this package.
func bridgeContext(ctx *duiruntime.Context) (*oldruntime.Context, func()) {
	legacy := legacyStepConfig(ctx.Config)
	oldCtx := &oldruntime.Context{
		Config:     legacy,
		Playwright: ctx.Playwright,
		Browser:    ctx.Browser,
		Page:       ctx.Page,
		Console:    oldruntime.NewConsoleMonitor(ctx.Page),
		Artifacts:  ctx.Artifacts,
		Selectors:  oldselectors.DefaultCatalog(),
		Fixture:    fixtureFromMemory(ctx),
		Memory:     copyMemory(ctx.Memory),
	}
	return oldCtx, func() {
		for key, value := range oldCtx.Memory {
			ctx.Memory[key] = value
		}
		storeFixture(ctx, oldCtx.Fixture)
	}
}

func legacyStepConfig(cfg duiruntime.Config) oldruntime.Config {
	ws, _ := ReadWorkspaceEnv(cfg.RepoRoot)
	thoughtsRoot := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
	if thoughtsRoot == "" {
		thoughtsRoot = filepath.Join(cfg.RepoRoot, "thoughts")
	}
	return oldruntime.Config{
		RepoRoot:     cfg.RepoRoot,
		PackageRoot:  cfg.RepoRoot,
		BaseURL:      cfg.BaseURL,
		AuthToken:    strings.TrimSpace(os.Getenv("VAMOS_E2E_AUTH_TOKEN")),
		ArtifactsDir: cfg.ArtifactsDir,
		ThoughtsRoot: thoughtsRoot,
		Workspace: oldruntime.WorkspaceIdentity{
			Slug:         ws.Slug,
			CheckoutPath: ws.CheckoutPath,
			DBPath:       ws.DBPath,
			ManagerURL:   ws.ManagerURL,
		},
		Headless: cfg.Headless,
	}
}

func copyMemory(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
