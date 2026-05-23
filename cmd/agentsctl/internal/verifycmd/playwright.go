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

type browserCommandRunner func(ctx context.Context, script string, args []string, outPath string) error

var runBrowserCommand browserCommandRunner = runNodeBrowserCommand

func RunBrowserVerify(
	ctx context.Context,
	cfg BrowserVerifyConfig,
) (ClientVerifyStep, error) {
	step := ClientVerifyStep{
		Name:       "browser",
		Layer:      workspaces.VerificationLayerBrowser,
		Status:     statusPassed,
		OutputPath: filepath.Join(cfg.ReportDir, "playwright-output.txt"),
	}
	if cfg.ExpectStopped {
		step.Name = "browser-unavailable-after-stop"
	}
	if cfg.AuthToken == "" {
		err := errors.New(
			"playwright auth token is required via --playwright-auth-token or CN_AGENTS_PLAYWRIGHT_AUTH_TOKEN",
		)
		step.Status = statusFailed
		step.Error = err.Error()
		return step, err
	}
	script, err := playwrightScriptPath()
	if err != nil {
		step.Status = statusFailed
		step.Error = err.Error()
		return step, err
	}
	args := []string{
		"--base-url", cfg.BaseURL,
		"--domain", cfg.Domain,
		"--slug", cfg.Slug,
		"--token", cfg.AuthToken,
		"--report", cfg.ReportDir,
	}
	if cfg.ExpectStopped {
		args = append(args, "--expect-stopped")
	}
	runCtx := ctx
	cancel := func() {}
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	defer cancel()
	if err := runBrowserCommand(runCtx, script, args, step.OutputPath); err != nil {
		step.Status = statusFailed
		step.Error = err.Error()
		return step, err
	}
	return step, nil
}

func runNodeBrowserCommand(
	ctx context.Context,
	script string,
	args []string,
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
	cmdArgs := append([]string{script}, args...)
	cmd := exec.CommandContext(ctx, "node", cmdArgs...)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("playwright verifier failed: %w (see %s)", err, outPath)
	}
	return nil
}

func playwrightScriptPath() (string, error) {
	const scriptRel = "workspace-verify-playwright.mjs"
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidates := []string{
			filepath.Join(dir, "scripts", scriptRel),
			filepath.Join(dir, "pkg", "agents", "scripts", scriptRel),
		}
		for _, candidate := range candidates {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", errors.New("workspace-verify-playwright.mjs not found")
}
