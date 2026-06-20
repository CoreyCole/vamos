package rootcmd

import (
	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/authcmd"
	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/chatcmd"
	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/ctlcmd"
	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/e2ecmd"
	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/qrspicmd"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vamos",
		Short: "Vamos Agents developer CLI",
		Long:  "Managed CLI for Vamos Agents workspace operations and story E2E workflows.",
	}
	cmd.AddCommand(authcmd.NewCommand())
	cmd.AddCommand(chatcmd.NewCommand())
	cmd.AddCommand(ctlcmd.NewCommand())
	cmd.AddCommand(e2ecmd.NewCommand())
	cmd.AddCommand(qrspicmd.NewCommand())
	return cmd
}
