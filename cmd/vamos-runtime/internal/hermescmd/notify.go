package hermescmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const (
	notificationFormatText = "text"
	notificationFormatJSON = "json"
)

func newNotifyCommand(notifierFactory NotifierFactory) *cobra.Command {
	var planDir, piSessionID, messageID, configPath, format string
	cmd := &cobra.Command{
		Use:          "notify --plan <absolute-plan-dir> --pi-session <id> --message-id <id>",
		SilenceUsage: true,
		Short:        "Retry notification for one exact immutable settlement",
		Long:         "Retry notification for one exact immutable settlement. Admission does not prove manager execution or reverse delivery to a child.",
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !filepath.IsAbs(planDir) {
				return errors.New("plan must be an absolute directory")
			}
			if err := sessioningress.ValidatePiSessionID(piSessionID); err != nil {
				return fmt.Errorf("validate Pi session ID: %w", err)
			}
			if err := sessioningress.ValidateMessageID(messageID); err != nil {
				return fmt.Errorf("validate message ID: %w", err)
			}
			if notifierFactory == nil {
				return errors.New("notifier factory is required")
			}
			if format != notificationFormatText && format != notificationFormatJSON {
				return errors.New("format must be text or json")
			}

			sessionDirectory, err := os.Open(filepath.Join(
				filepath.Clean(planDir), ".vamos", "sessions", "pi", piSessionID,
			))
			if err != nil {
				return errors.New("open exact Pi session directory")
			}
			defer sessionDirectory.Close()
			evidence, err := LoadExactSettlementEvidence(
				sessionDirectory,
				piSessionID,
				messageID,
				currentOwnerUID(),
			)
			if err != nil {
				return fmt.Errorf("load exact settlement evidence: %w", err)
			}
			config, err := readParentClientConfig(configPath)
			if err != nil {
				return err
			}
			notifier, err := notifierFactory(evidence.HermesSessionID, config)
			if err != nil {
				return fmt.Errorf("construct settlement notifier: %w", err)
			}
			result := notifier.Notify(cmd.Context(), enqueueRequestFromEvidence(evidence))
			report := recoveryNotificationReport(messageID, result)
			if err := writeNotificationReport(cmd, format, report); err != nil {
				return err
			}
			if !result.Admission {
				return fmt.Errorf(
					"settlement notification was not admitted: %s",
					result.Code,
				)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&planDir, "plan", "", "absolute plan directory")
	cmd.Flags().StringVar(&piSessionID, "pi-session", "", "exact Pi session ID")
	cmd.Flags().StringVar(&messageID, "message-id", "", "exact settlement message ID")
	cmd.Flags().StringVar(&configPath, "config", "", "host-local Hermes config path")
	cmd.Flags().StringVar(
		&format,
		"format",
		notificationFormatText,
		"text or json",
	)
	_ = cmd.MarkFlagRequired("plan")
	_ = cmd.MarkFlagRequired("pi-session")
	_ = cmd.MarkFlagRequired("message-id")

	return cmd
}

func writeNotificationReport(
	cmd *cobra.Command,
	format string,
	report NotificationReport,
) error {
	if format == notificationFormatJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
	}
	status := report.NotificationStatus
	_, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"child_exit: %s\nsettlement_publication: %s\nnotification_admission: %t\nnotification_code: %s\nnotification_detail: %s\nnotification_retryable: %t\nnotification_uncertain: %t\nevent_index: 0\nmessage_id: %s\nmanager_execution: not_observed\nreverse_child_delivery: not_observed\n",
		status.ChildExit,
		status.SettlementPublication,
		status.NotificationAdmission,
		status.NotificationCode,
		status.NotificationDetail,
		status.NotificationRetryable,
		status.NotificationUncertain,
		report.Events[0].MessageID,
	)

	return err
}
