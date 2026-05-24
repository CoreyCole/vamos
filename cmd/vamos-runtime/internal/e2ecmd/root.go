package e2ecmd

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "e2e", Short: "Story-driven E2E workflows"}
	cmd.AddCommand(NewCheckCommand())
	cmd.AddCommand(NewGenerateCommand())
	cmd.AddCommand(NewRunCommand())
	cmd.AddCommand(NewReviewCommand())
	cmd.AddCommand(NewFixCommand())
	cmd.AddCommand(NewGoldensCommand())
	return cmd
}

func NewReviewCommand() *cobra.Command {
	cfg := ReviewConfig{Baseline: "main"}
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Run semantic visual review",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunReview(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.RunDir, "run", cfg.RunDir, "E2E run directory")
	cmd.Flags().StringVar(&cfg.Baseline, "baseline", cfg.Baseline, "baseline ref for semantic goldens")
	cmd.Flags().StringVar(&cfg.PlanDir, "plan-dir", cfg.PlanDir, "QRSPI plan directory for review artifact")
	return cmd
}

func NewFixCommand() *cobra.Command {
	cfg := FixConfig{}
	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Plan bounded E2E repairs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunFix(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.RunDir, "run", cfg.RunDir, "E2E run directory")
	cmd.Flags().BoolVar(&cfg.Apply, "apply", cfg.Apply, "apply bounded repair plan")
	return cmd
}

func NewGoldensCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "goldens", Short: "Manage semantic visual goldens"}
	cmd.AddCommand(newGoldensCaptureCommand())
	cmd.AddCommand(newGoldensAcceptCommand())
	return cmd
}

func newGoldensCaptureCommand() *cobra.Command {
	cfg := GoldensConfig{GoldenRoot: "pkg/e2e/goldens"}
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture golden candidates from a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunGoldensCapture(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.RunDir, "run", cfg.RunDir, "E2E run directory")
	cmd.Flags().StringVar(&cfg.GoldenRoot, "golden-root", cfg.GoldenRoot, "semantic golden screenshot root")
	return cmd
}

func newGoldensAcceptCommand() *cobra.Command {
	cfg := GoldensConfig{GoldenRoot: "pkg/e2e/goldens"}
	cmd := &cobra.Command{
		Use:   "accept",
		Short: "Accept golden candidates with human approval",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunGoldensAccept(cmd.Context(), cfg)
		},
	}
	cmd.Flags().StringVar(&cfg.RunDir, "run", cfg.RunDir, "E2E run directory")
	cmd.Flags().StringVar(&cfg.GoldenRoot, "golden-root", cfg.GoldenRoot, "semantic golden screenshot root")
	cmd.Flags().BoolVar(&cfg.HumanApproved, "human-approved", cfg.HumanApproved, "confirm human approval for accepting goldens")
	return cmd
}
