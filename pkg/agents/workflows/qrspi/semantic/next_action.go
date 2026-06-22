package semantic

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

type NextActionKind string

const (
	NextActionStartNext        NextActionKind = "start_next"
	NextActionWaitHuman        NextActionKind = "wait_human"
	NextActionContinuePending  NextActionKind = "continue_pending"
	NextActionBlocked          NextActionKind = "blocked"
	NextActionError            NextActionKind = "error"
	NextActionInvalidRetry     NextActionKind = "invalid_retry"
	NextActionInvalidExhausted NextActionKind = "invalid_exhausted"
	NextActionManualRecovery   NextActionKind = "manual_recovery_needed"
	NextActionDone             NextActionKind = "done"
)

type NextAction struct {
	Kind            NextActionKind         `json:"kind"`
	Severity        string                 `json:"severity,omitempty"`
	CurrentNodeID   wruntime.NodeID        `json:"current_node_id,omitempty"`
	Status          wruntime.ResultStatus  `json:"status,omitempty"`
	Outcome         wruntime.ResultOutcome `json:"outcome,omitempty"`
	PrimaryArtifact string                 `json:"primary_artifact,omitempty"`
	NextNodeID      wruntime.NodeID        `json:"next_node_id,omitempty"`
	HumanGateLabel  string                 `json:"human_gate_label,omitempty"`
	RecoveryReason  string                 `json:"recovery_reason,omitempty"`
	Evidence        []string               `json:"evidence,omitempty"`
}

func ProjectNextAction(result wruntime.WorkflowResult, decision wruntime.TransitionDecision, effects []Effect) NextAction {
	action := NextAction{
		CurrentNodeID:   result.SourceNodeID,
		Status:          result.Status,
		Outcome:         result.Outcome,
		PrimaryArtifact: result.PrimaryArtifact,
		NextNodeID:      decision.NextNodeID,
		HumanGateLabel:  decision.StopReason,
		RecoveryReason:  decision.StopReason,
	}
	switch result.Status {
	case wruntime.StatusBlocked:
		action.Kind = NextActionBlocked
		action.Severity = "warning"
		return action
	case wruntime.StatusError:
		action.Kind = NextActionError
		action.Severity = "error"
		return action
	case wruntime.StatusNeedsHuman:
		action.Kind = NextActionWaitHuman
		action.Severity = "info"
		return action
	}
	if decision.WaitingHuman {
		action.Kind = NextActionWaitHuman
		action.Severity = "info"
		return action
	}
	if result.Status == wruntime.StatusHandoff && decision.NextNodeID != "" {
		action.Kind = NextActionContinuePending
		action.Severity = "info"
		return action
	}
	if decision.StartNext {
		action.Kind = NextActionStartNext
		action.Severity = "info"
		return action
	}
	if decision.NextNodeID != "" {
		action.Kind = NextActionContinuePending
		action.Severity = "info"
		return action
	}
	action.Kind = NextActionDone
	action.Severity = "success"
	return action
}
