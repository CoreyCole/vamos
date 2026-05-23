package ctlcmd

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/ctl/verifycmd"
	"github.com/CoreyCole/vamos/pkg/ctl/workspacecmd"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "ctl", Short: "Workspace and operational controls"}
	cmd.AddCommand(newWorkspaceCommand())
	cmd.AddCommand(newVerifyCommand())
	return cmd
}

func newWorkspaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "workspace <status|logs|doctor|restart|register-current>",
		Short:              "Manage the current Vamos workspace checkout",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsHelp(args) {
				return cmd.Help()
			}
			return workspacecmd.Main(args)
		},
	}
}

func newVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "verify", Short: "Verify managed runtime surfaces"}
	cmd.AddCommand(&cobra.Command{
		Use:                "workspaces",
		Short:              "Verify workspace manager and child workspace routing",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsHelp(args) {
				return cmd.Help()
			}
			return verifycmd.Main(args)
		},
	})
	return cmd
}

func wantsHelp(args []string) bool {
	return slices.Contains(args, "--help") || slices.Contains(args, "-h")
}
