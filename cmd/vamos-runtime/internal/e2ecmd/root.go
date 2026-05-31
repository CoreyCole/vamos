package e2ecmd

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "e2e", Short: "Vamos-specific E2E repair policy workflow"}
	cmd.AddCommand(NewFixCommand())
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
