package verifycmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type BrowserVerifyConfig struct {
	BaseURL       string
	Domain        string
	Slug          string
	AuthToken     string
	ReportDir     string
	ExpectStopped bool
	Timeout       time.Duration
}

type browserCommandRunner func(ctx context.Context, cfg BrowserVerifyConfig, story, outPath string) error

var runBrowserCommand browserCommandRunner = runDatastarUIBrowserCommand

func RunBrowserVerify(
	ctx context.Context,
	cfg BrowserVerifyConfig,
) (ClientVerifyStep, error) {
	step := ClientVerifyStep{
		Name:       "browser",
		Layer:      workspaces.VerificationLayerBrowser,
		Status:     statusPassed,
		OutputPath: filepath.Join(cfg.ReportDir, "datastarui-e2e-output.txt"),
	}
	story := "workspace-public-switch"
	if cfg.ExpectStopped {
		step.Name = "browser-unavailable-after-stop"
		story = "workspace-public-unavailable"
	}
	if cfg.AuthToken == "" {
		err := errors.New(
			"playwright auth token is required via --playwright-auth-token or VAMOS_PLAYWRIGHT_AUTH_TOKEN",
		)
		step.Status = statusFailed
		step.Error = err.Error()
		return step, err
	}
	runCtx := ctx
	cancel := func() {}
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	defer cancel()
	if err := runBrowserCommand(runCtx, cfg, story, step.OutputPath); err != nil {
		step.Status = statusFailed
		step.Error = err.Error()
		return step, err
	}
	return step, nil
}

func runDatastarUIBrowserCommand(
	ctx context.Context,
	cfg BrowserVerifyConfig,
	story string,
	outPath string,
) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	repoRoot, err := findVamosRepoRoot()
	if err != nil {
		return err
	}
	artifactsDir := filepath.Join(cfg.ReportDir, "datastarui-e2e-runs")
	cmd := exec.CommandContext(
		ctx,
		filepath.Join(repoRoot, "..", "datastarui", "scripts", "datastarui.sh"),
		"e2e",
		"run",
		"--config",
		filepath.Join(repoRoot, "datastarui-e2e.yml"),
		"--base-url",
		cfg.BaseURL,
		"--no-restart",
		"--story",
		story,
		"--viewport",
		"desktop-full",
		"--artifacts-dir",
		artifactsDir,
	)
	cmd.Dir = repoRoot
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = append(os.Environ(),
		"VAMOS_E2E_AUTH_TOKEN="+cfg.AuthToken,
		"VAMOS_E2E_WORKSPACE_SLUG="+cfg.Slug,
		"VAMOS_E2E_WORKSPACE_DOMAIN="+cfg.Domain,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("datastarui e2e verifier failed: %w (see %s)", err, outPath)
	}
	return nil
}

func findVamosRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "datastarui-e2e.yml")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", errors.New("vamos repo root with datastarui-e2e.yml not found")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
