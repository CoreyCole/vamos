package e2ecmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/e2e/review"
)

type ReviewConfig struct {
	RunDir   string
	Baseline string
	PlanDir  string
}

func RunReview(ctx context.Context, cfg ReviewConfig) error {
	if strings.TrimSpace(cfg.RunDir) == "" {
		return fmt.Errorf("--run is required")
	}
	manifestPath := manifestPathForRun(cfg.RunDir)
	input := review.VisualReviewInput{
		RunManifestPath: manifestPath,
		BaselineRef:     defaultString(cfg.Baseline, "main"),
		BaselineCommit:  gitRev(ctx, defaultString(cfg.Baseline, "main")),
		WorkspaceCommit: gitRev(ctx, "HEAD"),
		PlanDir:         cfg.PlanDir,
		SkillName:       "e2e-image-review",
	}
	result, err := review.RunVisualReview(ctx, input)
	if err != nil {
		return err
	}
	path := filepath.Join(cfg.RunDir, "e2e-visual.md")
	if cfg.PlanDir != "" {
		path = filepath.Join(
			cfg.PlanDir,
			"context",
			"implement",
			"e2e-runs",
			filepath.Base(cfg.RunDir),
			"e2e-visual.md",
		)
	}
	result.ArtifactPath = path
	return review.WriteMarkdown(path, input, result)
}

func manifestPathForRun(runDir string) string {
	for _, name := range []string{"manifest.json", "run.json"} {
		path := filepath.Join(runDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(runDir, "manifest.json")
}

func gitRev(ctx context.Context, rev string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", rev)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
