package e2ecmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "e2e", Short: "Story-driven E2E workflows"}
	cmd.AddCommand(NewCheckCommand())
	cmd.AddCommand(notImplemented("generate", "Generate Playwright-Go tests from stories"))
	cmd.AddCommand(notImplemented("run", "Run generated E2E tests"))
	cmd.AddCommand(notImplemented("review", "Run semantic visual review"))
	cmd.AddCommand(notImplemented("fix", "Plan bounded E2E repairs"))
	cmd.AddCommand(&cobra.Command{Use: "goldens", Short: "Manage semantic visual goldens"})
	return cmd
}

func notImplemented(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("vamos e2e %s not implemented yet", use)
		},
	}
}
