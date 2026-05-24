package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/CoreyCole/vamos/pkg/e2e/selectors"
)

type Context struct {
	Config     Config
	Playwright *playwright.Playwright
	Browser    playwright.Browser
	Page       playwright.Page
	Artifacts  ArtifactSink
	Selectors  selectors.Catalog
	Fixture    any
	Memory     map[string]string
}

type ScenarioFunc func(t testing.TB, ctx *Context)

type ArtifactSink interface {
	Capture(label string, page playwright.Page) error
}

const scenarioTimeout = 5 * time.Minute

func RunScenario(t *testing.T, featureSlug, scenarioSlug string, fn ScenarioFunc) {
	runScenario(t, featureSlug, scenarioSlug, ViewportDesktopFull, fn)
}

func RunScenarioWithViewport(
	t *testing.T,
	featureSlug, scenarioSlug string,
	viewport ViewportClass,
	fn ScenarioFunc,
) {
	runScenario(t, featureSlug, scenarioSlug, viewport, fn)
}

func runScenario(
	t *testing.T,
	featureSlug, scenarioSlug string,
	viewport ViewportClass,
	fn ScenarioFunc,
) {
	t.Helper()
	if os.Getenv("VAMOS_BASE_URL") == "" && os.Getenv("VAMOS_E2E_RUN_BROWSER") != "1" {
		t.Skip("VAMOS_BASE_URL is required for browser E2E")
	}
	cfg, err := LoadConfigFromEnv(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateBrowserConfig(); err != nil {
		t.Fatal(err)
	}
	if viewport != "" {
		cfg.Viewports = []ViewportClass{viewport}
	}

	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("start playwright: %v", err)
	}
	defer func() {
		if err := pw.Stop(); err != nil {
			t.Fatalf("stop playwright: %v", err)
		}
	}()

	browser, err := pw.Chromium.Launch(
		playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(cfg.Headless)},
	)
	if err != nil {
		t.Fatalf("launch chromium: %v", err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			t.Fatalf("close browser: %v", err)
		}
	}()

	viewports, err := ResolveViewports([]string{string(viewport)})
	if err != nil {
		t.Fatal(err)
	}
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{
			Width:  viewports[0].Width,
			Height: viewports[0].Height,
		},
	})
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	artifactDir := cfg.ArtifactsDir
	if artifactDir != "" {
		artifactDir = filepath.Join(
			artifactDir,
			featureSlug,
			scenarioSlug,
			string(viewport),
		)
	}
	artifactSink, err := NewFileArtifactSink(artifactDir)
	if err != nil {
		t.Fatalf("artifact sink: %v", err)
	}
	ctx := &Context{
		Config:     cfg,
		Playwright: pw,
		Browser:    browser,
		Page:       page,
		Artifacts:  artifactSink,
		Selectors:  selectors.DefaultCatalog(),
		Memory:     map[string]string{},
	}
	defer func() {
		if t.Failed() || os.Getenv("VAMOS_E2E_CAPTURE_SUCCESS") == "1" {
			_ = ctx.Artifacts.Capture("page", ctx.Page)
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(t, ctx)
	}()
	select {
	case <-done:
	case <-time.After(scenarioTimeout):
		t.Fatalf("scenario %s/%s timed out", featureSlug, scenarioSlug)
	}
}
