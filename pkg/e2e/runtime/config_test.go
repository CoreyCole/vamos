package runtime

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("VAMOS_BASE_URL", "")
	t.Setenv("VAMOS_E2E_ARTIFACTS_DIR", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFromEnv(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.BaseURL, "http://localhost:4200"; got != want {
		t.Fatalf("BaseURL=%q want %q", got, want)
	}
	if !strings.HasSuffix(
		cfg.ArtifactsDir,
		".e2e-runs",
	) {
		t.Fatalf("ArtifactsDir=%q missing expected suffix", cfg.ArtifactsDir)
	}
	if err := cfg.ValidateBrowserConfig(); err != nil {
		t.Fatalf("ValidateBrowserConfig() error = %v", err)
	}
}

func TestLoadConfigFromEnvParsesCommaSeparatedViewports(t *testing.T) {
	t.Setenv("VAMOS_E2E_VIEWPORTS", "mobile, desktop-half,desktop-full")
	cfg, err := LoadConfigFromEnv(".")
	if err != nil {
		t.Fatal(err)
	}
	want := []ViewportClass{ViewportMobile, ViewportDesktopHalf, ViewportDesktopFull}
	if !reflect.DeepEqual(cfg.Viewports, want) {
		t.Fatalf("Viewports=%v want %v", cfg.Viewports, want)
	}
}

func TestDefaultVerifyViewports(t *testing.T) {
	want := []ViewportClass{ViewportMobile, ViewportDesktopHalf, ViewportDesktopFull}
	if got := DefaultVerifyViewports(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultVerifyViewports()=%v want %v", got, want)
	}
}

func TestBuildAuthURL(t *testing.T) {
	got, err := BuildAuthURL(
		Config{BaseURL: "http://example.test/", AuthToken: "secret"},
		"/next",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"http://example.test/internal/playwright-auth", "redirect=%2Fnext", "token=secret"} {
		if !strings.Contains(got, want) {
			t.Fatalf("auth URL %q missing %q", got, want)
		}
	}
}
