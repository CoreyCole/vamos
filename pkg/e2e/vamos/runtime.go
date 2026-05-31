package vamos

import (
	"context"

	"github.com/playwright-community/playwright-go"

	oldruntime "github.com/CoreyCole/vamos/pkg/e2e/runtime"
	oldselectors "github.com/CoreyCole/vamos/pkg/e2e/selectors"
	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
)

type Runtime struct{}

func init() {
	duiruntime.SetDefaultAppRuntime(Runtime{})
}

func (Runtime) Authenticate(ctx context.Context, page playwright.Page, cfg duiruntime.Config, email string) error {
	legacy, err := legacyConfig(cfg)
	if err != nil {
		return err
	}
	return oldruntime.Authenticate(ctx, page, legacy, email)
}

func (Runtime) Preflight(ctx context.Context, cfg duiruntime.Config) error {
	legacy, err := legacyConfig(cfg)
	if err != nil {
		return err
	}
	return oldruntime.PreflightWorkspace(ctx, legacy)
}

func legacyConfig(cfg duiruntime.Config) (oldruntime.Config, error) {
	legacy, err := oldruntime.LoadConfigFromEnv(cfg.RepoRoot)
	if err != nil {
		return oldruntime.Config{}, err
	}
	legacy.BaseURL = cfg.BaseURL
	legacy.ArtifactsDir = cfg.ArtifactsDir
	return legacy, nil
}

func bridgeContext(ctx *duiruntime.Context) (*oldruntime.Context, func()) {
	legacy, err := legacyConfig(ctx.Config)
	if err != nil {
		return &oldruntime.Context{}, func() {}
	}
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

func copyMemory(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
