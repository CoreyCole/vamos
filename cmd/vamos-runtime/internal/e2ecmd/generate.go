package e2ecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/e2e/generate"
	"github.com/CoreyCole/vamos/pkg/e2e/steps"
	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

type GenerateConfig struct {
	StoryDir     string
	GeneratedDir string
	PackageName  string
	Check        bool
	Stdout       io.Writer
}

func NewGenerateCommand() *cobra.Command {
	cfg := GenerateConfig{
		StoryDir:     filepath.Join("docs", "features"),
		GeneratedDir: filepath.Join("pkg", "e2e", "generated"),
		PackageName:  "generated",
	}
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Playwright-Go tests from stories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunGenerate(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.StoryDir, "story-dir", cfg.StoryDir, "directory containing *.story.md files")
	cmd.Flags().StringVar(&cfg.GeneratedDir, "generated-dir", cfg.GeneratedDir, "directory for generated Go tests")
	cmd.Flags().StringVar(&cfg.PackageName, "package", cfg.PackageName, "generated Go package name")
	cmd.Flags().BoolVar(&cfg.Check, "check", cfg.Check, "check generated files are fresh without writing")
	return cmd
}

func RunGenerate(ctx context.Context, cfg GenerateConfig) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	features, err := story.ParseDir(cfg.StoryDir, story.ParseOptions{Strict: true})
	if err != nil {
		return err
	}
	opts := generate.Options{
		StoryDir:    cfg.StoryDir,
		OutputDir:   cfg.GeneratedDir,
		PackageName: cfg.PackageName,
		StepCatalog: steps.DefaultCatalog(),
	}
	if cfg.Check {
		return generate.CheckFresh(features, opts)
	}
	result, err := generate.Generate(features, opts)
	if err != nil {
		return err
	}
	if err := generate.Write(result); err != nil {
		return err
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	_, err = fmt.Fprintf(stdout, "generated %d e2e files\n", len(result.Files))
	return err
}
