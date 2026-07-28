package hermescmd

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "hermes", Short: "Hermes gateway and Pi worker utilities"}
	cmd.AddCommand(newPiCommand(defaultRunner), newSetupCommand(), newDoctorCommand())
	return cmd
}
