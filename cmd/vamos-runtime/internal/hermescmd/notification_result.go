package hermescmd

import "github.com/CoreyCole/vamos/pkg/hermes/sessioningress"

//nolint:tagliatelle // Recovery output uses the frozen operator-facing snake_case field names.
type NotificationStatus struct {
	ChildExit             string `json:"child_exit"`
	SettlementPublication string `json:"settlement_publication"`
	NotificationAdmission bool   `json:"notification_admission"`
	NotificationCode      string `json:"notification_code"`
	NotificationDetail    string `json:"notification_detail"`
	NotificationRetryable bool   `json:"notification_retryable"`
	NotificationUncertain bool   `json:"notification_uncertain"`
}

//nolint:tagliatelle // Recovery event identity uses the frozen operator-facing field names.
type NotificationEvent struct {
	NotificationStatus
	MessageID  string `json:"message_id"`
	EventIndex int    `json:"event_index"`
}

type NotificationReport struct {
	NotificationStatus
	Events []NotificationEvent `json:"events"`
}

func recoveryNotificationReport(
	messageID string,
	result sessioningress.NotifyResult,
) NotificationReport {
	status := NotificationStatus{
		ChildExit:             "not_applicable",
		SettlementPublication: "loaded_immutable_evidence",
		NotificationAdmission: result.Admission,
		NotificationCode:      result.Code,
		NotificationDetail:    result.Detail,
		NotificationRetryable: result.Retryable,
		NotificationUncertain: result.Uncertain,
	}

	return NotificationReport{
		NotificationStatus: status,
		Events: []NotificationEvent{{
			NotificationStatus: status,
			MessageID:          messageID,
			EventIndex:         0,
		}},
	}
}
