package hermescmd

import "github.com/spf13/cobra"

func NewCommand() *cobra.Command {
	return newCommand(
		defaultRunner,
		defaultStartWaitProcessFactory,
		defaultNotifierFactory,
	)
}

func newCommand(
	run commandRunner,
	processes StartWaitProcessFactory,
	notifiers NotifierFactory,
) *cobra.Command {
	cmd := &cobra.Command{Use: "hermes", Short: "Hermes gateway and Pi worker utilities"}
	cmd.AddCommand(
		newPiCommand(
			run,
			managedCommandDependencies{
				processFactory:  processes,
				notifierFactory: notifiers,
			},
		),
		newSetupCommand(),
		newDoctorCommand(),
		newCleanupOpaqueSettlementSchedulesCommand(defaultCleanupTemporalDialer),
	)
	return cmd
}
