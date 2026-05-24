package e2ecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/steps"
	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

type CheckConfig struct {
	StoryDir     string
	GeneratedDir string
	Stdout       io.Writer
}

func NewCheckCommand() *cobra.Command {
	cfg := CheckConfig{StoryDir: filepath.Join("docs", "features"), GeneratedDir: filepath.Join("pkg", "e2e", "generated")}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Parse and validate E2E stories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunCheck(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.StoryDir, "story-dir", cfg.StoryDir, "directory containing *.story.md files")
	cmd.Flags().StringVar(&cfg.GeneratedDir, "generated-dir", cfg.GeneratedDir, "directory for generated Go tests")
	return cmd
}

func RunCheck(ctx context.Context, cfg CheckConfig) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	features, err := story.ParseDir(cfg.StoryDir, story.ParseOptions{Strict: true})
	if err != nil {
		return err
	}
	stepCatalog := steps.DefaultCatalog()
	fixtureRegistry := fixtures.DefaultRegistry()
	for _, feature := range features {
		if err := story.ValidateFeature(feature, stepCatalog, fixtureRegistry); err != nil {
			return err
		}
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	_, err = fmt.Fprintf(stdout, "validated %d story features\n", len(features))
	return err
}
