package vamos

import (
	"context"

	"github.com/playwright-community/playwright-go"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
)

func App() duiruntime.App {
	return duiruntime.App{
		Name:         "vamos",
		Authenticate: Authenticate,
		Preflight:    Preflight,
	}
}

func init() {
	duiruntime.SetDefaultAppRuntime(appRuntime{app: App()})
}

type appRuntime struct{ app duiruntime.App }

func (r appRuntime) Authenticate(ctx context.Context, page playwright.Page, cfg duiruntime.Config, email string) error {
	return r.app.Authenticate(ctx, page, cfg, email)
}

func (r appRuntime) Preflight(ctx context.Context, cfg duiruntime.Config) error {
	return r.app.Preflight(ctx, cfg)
}
