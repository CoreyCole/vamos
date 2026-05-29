package runtime

import (
	"fmt"
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
	Console    *ConsoleMonitor
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

func newPageOptionsForViewport(viewport Viewport) playwright.BrowserNewPageOptions {
	return playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{
			Width:  viewport.Width,
			Height: viewport.Height,
		},
		ExtraHttpHeaders: map[string]string{
			"X-Vamos-Viewport-Class": string(viewport.Class),
		},
	}
}

func RunScenario(t *testing.T, featureSlug, scenarioSlug string, fn ScenarioFunc) {
	runScenarioAcrossConfiguredViewports(t, featureSlug, scenarioSlug, fn)
}

func RunScenarioWithViewport(
	t *testing.T,
	featureSlug, scenarioSlug string,
	viewport ViewportClass,
	fn ScenarioFunc,
) {
	t.Helper()
	cfg, err := loadScenarioConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := runScenarioOnce(t, cfg, featureSlug, scenarioSlug, viewport, fn); err != nil {
		t.Fatal(err)
	}
}

func runScenarioAcrossConfiguredViewports(
	t *testing.T,
	featureSlug, scenarioSlug string,
	fn ScenarioFunc,
) {
	t.Helper()
	cfg, err := loadScenarioConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	viewports, err := scenarioViewports(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, viewport := range viewports {
		viewport := viewport
		t.Run(string(viewport), func(t *testing.T) {
			t.Helper()
			if err := runScenarioOnce(t, cfg, featureSlug, scenarioSlug, viewport, fn); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func loadScenarioConfig(t testing.TB) (Config, error) {
	t.Helper()
	if os.Getenv("VAMOS_BASE_URL") == "" && os.Getenv("VAMOS_E2E_RUN_BROWSER") != "1" {
		t.Skip("VAMOS_BASE_URL is required for browser E2E")
	}
	cfg, err := LoadConfigFromEnv(".")
	if err != nil {
		return Config{}, err
	}
	if err := cfg.ValidateBrowserConfig(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func scenarioViewports(cfg Config, explicit ViewportClass) ([]ViewportClass, error) {
	if explicit != "" {
		if _, err := ResolveViewports([]string{string(explicit)}); err != nil {
			return nil, err
		}
		return []ViewportClass{explicit}, nil
	}
	if len(cfg.Viewports) == 0 {
		return []ViewportClass{ViewportDesktopFull}, nil
	}
	names := make([]string, 0, len(cfg.Viewports))
	for _, viewport := range cfg.Viewports {
		names = append(names, string(viewport))
	}
	if _, err := ResolveViewports(names); err != nil {
		return nil, err
	}
	return append([]ViewportClass{}, cfg.Viewports...), nil
}

func runScenarioOnce(
	t testing.TB,
	cfg Config,
	featureSlug, scenarioSlug string,
	viewport ViewportClass,
	fn ScenarioFunc,
) error {
	t.Helper()

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("start playwright: %w", err)
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
		return fmt.Errorf("launch chromium: %w", err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			t.Fatalf("close browser: %v", err)
		}
	}()

	viewports, err := ResolveViewports([]string{string(viewport)})
	if err != nil {
		return err
	}
	page, err := browser.NewPage(newPageOptionsForViewport(viewports[0]))
	if err != nil {
		return fmt.Errorf("new page: %w", err)
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
		return fmt.Errorf("artifact sink: %w", err)
	}
	ctx := &Context{
		Config:     cfg,
		Playwright: pw,
		Browser:    browser,
		Page:       page,
		Console:    NewConsoleMonitor(page),
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
	return nil
}
