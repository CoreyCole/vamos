package e2ecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	duiappconfig "github.com/coreycole/datastarui/e2e/appconfig"

	"github.com/CoreyCole/vamos/pkg/e2e/artifacts"
	"github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

type RunConfig struct {
	Story        string
	Scenario     string
	Viewport     string
	BaseURL      string
	ArtifactsDir string
	PlanDir      string
	NoRestart    bool
	ConfigPath   string
}

func RunE2E(ctx context.Context, cfg RunConfig) error {
	appCfg, err := loadAppConfig(cfg)
	if err != nil {
		return err
	}
	if cfg.BaseURL != "" {
		appCfg.BaseURL = cfg.BaseURL
	}

	viewportEnv := selectedViewportEnv(cfg)
	artifactRoot := appCfg.ResolvePath(appCfg.ArtifactsDir)
	if cfg.ArtifactsDir != "" {
		artifactRoot = cfg.ArtifactsDir
	}
	manifest, err := artifacts.NewRun(artifactRoot)
	if err != nil {
		return err
	}
	runDir := artifacts.RunDir(artifactRoot, manifest)

	runCfg, err := runtime.LoadConfigFromEnv(appCfg.RootDir)
	if err != nil {
		return err
	}
	runCfg.BaseURL = strings.TrimRight(appCfg.BaseURL, "/")
	runCfg.ArtifactsDir = runDir

	if err := setRunEnvironment(appCfg, runCfg, viewportEnv, runDir); err != nil {
		return err
	}

	if ShouldPreflight(cfg, appCfg) {
		if err := runtime.PreflightWorkspace(ctx, runCfg); err != nil {
			return err
		}
	}
	if serverCommand := BuildServerCommand(cfg, appCfg); len(serverCommand) > 0 {
		build := exec.CommandContext(ctx, serverCommand[0], serverCommand[1:]...)
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		build.Dir = appCfg.RootDir
		if err := build.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(serverCommand, " "), err)
		}
	}

	args := BuildGoTestArgs(cfg, appCfg)
	if err := ensureSelectedTestsExist(ctx, args, appCfg.RootDir); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = appCfg.RootDir
	cmd.Env = append(os.Environ(),
		"E2E_CONFIG="+appCfg.ConfigPath,
		"E2E_BASE_URL="+runCfg.BaseURL,
		"E2E_ARTIFACTS_DIR="+runDir,
		"E2E_CAPTURE_SUCCESS=1",
		"E2E_VIEWPORTS="+viewportEnv,
		"E2E_RUN_BROWSER=1",
		"VAMOS_BASE_URL="+runCfg.BaseURL,
		"VAMOS_E2E_ARTIFACTS_DIR="+runDir,
		"VAMOS_E2E_CAPTURE_SUCCESS=1",
		"VAMOS_E2E_VIEWPORTS="+viewportEnv,
		"VAMOS_E2E_RUN_BROWSER=1",
	)
	runErr := cmd.Run()
	manifest.Command = "go " + strings.Join(args, " ")
	manifest.BaseURL = runCfg.BaseURL
	manifest.ViewportFilter = viewportEnv
	manifest.RepoCommit = currentGitCommit(ctx, appCfg.RootDir)
	manifest.Stories = appendSelectedStory(manifest.Stories, cfg.Story)
	collected, collectErr := artifacts.CollectRunArtifacts(runDir)
	if collectErr != nil {
		return collectErr
	}
	manifest.Artifacts = collected.Entries
	manifest.Screenshots = collected.Screenshots
	manifest.HTMLSnapshots = collected.HTMLSnapshots
	manifest.Traces = collected.Traces
	if runErr != nil {
		failures := []artifacts.Failure{
			{
				Story:    cfg.Story,
				Scenario: cfg.Scenario,
				Viewport: cfg.Viewport,
				Error:    runErr.Error(),
				ArtifactPaths: append(
					append([]string{}, manifest.Screenshots...),
					manifest.HTMLSnapshots...),
			},
		}
		failuresPath, err := artifacts.WriteFailures(artifactRoot, manifest, failures)
		if err != nil {
			return err
		}
		manifest.FailuresPath = failuresPath
		_, _ = artifacts.WriteMarkdownReport(artifactRoot, manifest, failures)
	}
	if _, err := artifacts.WriteStaticIndex(manifest, runDir, artifacts.StaticIndexOptions{}); err != nil {
		return err
	}
	if _, err := artifacts.WriteManifest(artifactRoot, manifest); err != nil {
		return err
	}
	if cfg.PlanDir != "" {
		bundle, err := artifacts.ExportPlanBundle(
			ctx,
			manifest,
			artifacts.PlanBundleOptions{
				PlanDir:      cfg.PlanDir,
				RunDir:       runDir,
				Command:      manifest.Command,
				IncludeHTML:  true,
				IncludeTrace: true,
			},
		)
		if err != nil {
			return err
		}
		manifest.PlanBundlePath = bundle.IndexPath
		if _, err := artifacts.WriteManifest(artifactRoot, manifest); err != nil {
			return err
		}
	}
	if runErr != nil {
		return fmt.Errorf("go %v: %w", args, runErr)
	}
	return nil
}

func loadAppConfig(cfg RunConfig) (duiappconfig.Config, error) {
	path := cfg.ConfigPath
	if path == "" {
		path = os.Getenv("E2E_CONFIG")
	}
	return duiappconfig.Load(path, ".")
}

func setRunEnvironment(appCfg duiappconfig.Config, runCfg runtime.Config, viewportEnv, runDir string) error {
	values := map[string]string{
		"E2E_CONFIG":                appCfg.ConfigPath,
		"E2E_BASE_URL":              runCfg.BaseURL,
		"E2E_ARTIFACTS_DIR":         runDir,
		"E2E_CAPTURE_SUCCESS":       "1",
		"E2E_VIEWPORTS":             viewportEnv,
		"E2E_RUN_BROWSER":           "1",
		"VAMOS_BASE_URL":            runCfg.BaseURL,
		"VAMOS_E2E_ARTIFACTS_DIR":   runDir,
		"VAMOS_E2E_CAPTURE_SUCCESS": "1",
		"VAMOS_E2E_VIEWPORTS":       viewportEnv,
		"VAMOS_E2E_RUN_BROWSER":     "1",
	}
	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func ShouldPreflight(_ RunConfig, appCfg duiappconfig.Config) bool {
	return appCfg.Preflight.Mode == "vamos_workspace"
}

func BuildServerCommand(cfg RunConfig, appCfg duiappconfig.Config) []string {
	if cfg.NoRestart {
		return nil
	}
	if appCfg.Server.SkipWhenBaseURLSet && (cfg.BaseURL != "" || os.Getenv("E2E_BASE_URL") != "") {
		return nil
	}
	command := strings.TrimSpace(appCfg.Server.Command)
	if command == "" {
		command = "just build"
	}
	return strings.Fields(command)
}

func verifyViewportEnvValue() string {
	viewports := runtime.DefaultVerifyViewports()
	parts := make([]string, 0, len(viewports))
	for _, viewport := range viewports {
		parts = append(parts, string(viewport))
	}
	return strings.Join(parts, ",")
}

func selectedViewportEnv(cfg RunConfig) string {
	if viewport := strings.TrimSpace(cfg.Viewport); viewport != "" {
		return viewport
	}
	return verifyViewportEnvValue()
}

func BuildGoTestArgs(cfg RunConfig, appCfg duiappconfig.Config) []string {
	runPackage := strings.TrimSpace(appCfg.RunPackage)
	if runPackage == "" {
		runPackage = "./pkg/e2e/generated"
	}
	args := []string{"test", runPackage}
	if cfg.Story != "" || cfg.Scenario != "" {
		pattern := slugToTestFragment(cfg.Story)
		if cfg.Scenario != "" {
			pattern += ".*" + slugToTestFragment(cfg.Scenario)
		}
		args = append(args, "-run", pattern)
	}
	return args
}

func ensureSelectedTestsExist(ctx context.Context, args []string, dir string) error {
	pattern := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-run" {
			pattern = args[i+1]
			break
		}
	}
	if pattern == "" {
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("go test package argument missing")
	}
	listArgs := []string{"test", args[1], "-list", pattern}
	listCmd := exec.CommandContext(ctx, "go", listArgs...)
	listCmd.Dir = repoRootForCommand(dir)
	out, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go %v: %w\n%s", listArgs, err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Test") {
			return nil
		}
	}
	return fmt.Errorf("no E2E tests matched -run %q", pattern)
}

func currentGitCommit(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoRootForCommand(dir)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func repoRootForCommand(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return cwd
		}
		abs = parent
	}
}

func slugToTestFragment(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '/' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func appendSelectedStory(stories []string, story string) []string {
	if story == "" {
		return stories
	}
	return append(stories, story)
}
