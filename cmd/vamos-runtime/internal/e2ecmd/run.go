package e2ecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
}

func RunE2E(ctx context.Context, cfg RunConfig) error {
	if cfg.BaseURL != "" {
		if err := os.Setenv("VAMOS_BASE_URL", cfg.BaseURL); err != nil {
			return err
		}
	}
	if cfg.Viewport != "" {
		if err := os.Setenv("VAMOS_E2E_VIEWPORTS", cfg.Viewport); err != nil {
			return err
		}
	}
	if os.Getenv("VAMOS_BASE_URL") != "" {
		if err := os.Setenv("VAMOS_E2E_RUN_BROWSER", "1"); err != nil {
			return err
		}
	}

	runCfg, err := runtime.LoadConfigFromEnv(".")
	if err != nil {
		return err
	}
	artifactRoot := runCfg.ArtifactsDir
	if cfg.ArtifactsDir != "" {
		artifactRoot = cfg.ArtifactsDir
	}
	manifest, err := artifacts.NewRun(artifactRoot)
	if err != nil {
		return err
	}
	runDir := artifacts.RunDir(artifactRoot, manifest)
	if err := os.Setenv("VAMOS_E2E_ARTIFACTS_DIR", runDir); err != nil {
		return err
	}
	runCfg.ArtifactsDir = runDir

	if ShouldPreflight(cfg) {
		if err := runtime.PreflightWorkspace(ctx, runCfg); err != nil {
			return err
		}
	}
	if !cfg.NoRestart {
		build := exec.CommandContext(ctx, "just", "build")
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		build.Dir = "."
		if err := build.Run(); err != nil {
			return fmt.Errorf("just build: %w", err)
		}
	}

	args := BuildGoTestArgs(cfg)
	if err := ensureSelectedTestsExist(ctx, args); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"VAMOS_BASE_URL="+runCfg.BaseURL,
		"VAMOS_E2E_ARTIFACTS_DIR="+runDir,
		"VAMOS_E2E_CAPTURE_SUCCESS=1",
	)
	if cfg.Viewport != "" {
		cmd.Env = append(cmd.Env, "VAMOS_E2E_VIEWPORTS="+cfg.Viewport)
	}
	runErr := cmd.Run()
	manifest.Stories = appendSelectedStory(manifest.Stories, cfg.Story)
	manifest.Screenshots = findFiles(runDir, ".png")
	manifest.HTMLSnapshots = findFiles(runDir, ".html")
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
				Command:      "go " + strings.Join(args, " "),
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

func ShouldPreflight(RunConfig) bool { return true }

func BuildGoTestArgs(cfg RunConfig) []string {
	args := []string{"test", "./pkg/e2e/generated"}
	if cfg.Story != "" || cfg.Scenario != "" {
		pattern := slugToTestFragment(cfg.Story)
		if cfg.Scenario != "" {
			pattern += ".*" + slugToTestFragment(cfg.Scenario)
		}
		args = append(args, "-run", pattern)
	}
	return args
}

func ensureSelectedTestsExist(ctx context.Context, args []string) error {
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
	listArgs := []string{"test", "./pkg/e2e/generated", "-list", pattern}
	listCmd := exec.CommandContext(ctx, "go", listArgs...)
	listCmd.Dir = repoRootForCommand(".")
	out, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go %v: %w\n%s", listArgs, err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Test") {
			return nil
		}
	}
	return fmt.Errorf("no generated E2E tests matched -run %q", pattern)
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

func findFiles(root, ext string) []string {
	out := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out
}

func appendSelectedStory(stories []string, story string) []string {
	if story == "" {
		return stories
	}
	return append(stories, story)
}
