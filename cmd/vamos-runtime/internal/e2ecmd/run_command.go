package e2ecmd

import (
	"context"

	"github.com/spf13/cobra"
)

func NewRunCommand() *cobra.Command {
	cfg := RunConfig{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run generated E2E tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return RunE2E(ctx, cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.Story, "story", "", "story slug to run")
	cmd.Flags().StringVar(&cfg.Scenario, "scenario", "", "scenario slug to run")
	cmd.Flags().StringVar(&cfg.Viewport, "viewport", "", "viewport class or comma-separated viewport classes to run")
	cmd.Flags().StringVar(&cfg.BaseURL, "base-url", "", "base URL for browser E2E")
	cmd.Flags().StringVar(&cfg.ArtifactsDir, "artifacts-dir", "", "directory for run artifacts")
	cmd.Flags().StringVar(&cfg.PlanDir, "plan-dir", "", "QRSPI plan dir for run index artifacts")
	cmd.Flags().BoolVar(&cfg.NoRestart, "no-restart", false, "skip just build restart before running")
	return cmd
}
