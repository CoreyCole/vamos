package workflows

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type ExternalWorkflowResultInput struct {
	WorkspaceID string
	ThreadID    string
	HeadEntryID string
	State       wruntime.State
	Result      wruntime.WorkflowResult
}

func (s *Service) ApplyExternalWorkflowResult(
	ctx context.Context,
	input ExternalWorkflowResultInput,
) (wruntime.TransitionDecision, bool, error) {
	if s == nil || s.Store == nil || s.Definitions == nil {
		return wruntime.TransitionDecision{}, false, nil
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	threadID := strings.TrimSpace(input.ThreadID)
	if workspaceID == "" || threadID == "" {
		return wruntime.TransitionDecision{}, false, fmt.Errorf("workspace id and thread id are required")
	}
	state := input.State
	result := input.Result
	if state.LastResult != nil && sameWorkflowResultSnapshot(*state.LastResult, result) {
		return wruntime.TransitionDecision{State: state}, false, nil
	}

	def, ok := s.Definitions.Get(wruntime.WorkflowID(strings.TrimSpace(state.Type)))
	if !ok {
		return wruntime.TransitionDecision{}, false, fmt.Errorf("workflow definition %q is not registered", state.Type)
	}
	state, ok = alignExternalResultState(state, result)
	if !ok {
		return wruntime.TransitionDecision{State: state}, false, nil
	}
	applied, err := semantic.Apply(ctx, semantic.ApplyInput{
		Definition:     def,
		WorkflowResult: &result,
		Context: semantic.Context{
			WorkflowType:      wruntime.WorkflowID(state.Type),
			State:             state,
			ExpectedNodeID:    result.SourceNodeID,
			Source:            semantic.SourceExternalImport,
			ImplementationCwd: strings.TrimSpace(state.ExecutionCwd),
		},
	})
	if err != nil {
		return wruntime.TransitionDecision{State: state}, false, err
	}
	decision, err := s.prepareQRSPIApplyDecision(ctx, workspaceID, state, applied)
	if err != nil {
		return wruntime.TransitionDecision{}, false, err
	}
	run := db.AgentRun{
		ThreadID:    threadID,
		WorkspaceID: nullString(workspaceID),
	}
	decision.Events = append(decision.Events, wruntime.Event{
		Type:   "workflow_imported_terminal_result",
		NodeID: applied.WorkflowResult.SourceNodeID,
	})
	if err := s.persistQRSPIApplyResult(ctx, workspaceID, run, applied.WorkflowResult, decision); err != nil {
		return wruntime.TransitionDecision{}, false, err
	}
	if err := s.startQRSPIApplyNext(ctx, workspaceID, threadID, run, def, decision); err != nil {
		return decision, true, err
	}
	return decision, true, nil
}

func sameWorkflowResultSnapshot(snapshot wruntime.WorkflowResultSnapshot, result wruntime.WorkflowResult) bool {
	return snapshot.SourceNodeID == result.SourceNodeID &&
		strings.TrimSpace(snapshot.Status) == string(result.Status) &&
		strings.TrimSpace(snapshot.PrimaryArtifact) == strings.TrimSpace(result.PrimaryArtifact) &&
		strings.TrimSpace(snapshot.DisplayNext) == strings.TrimSpace(result.DisplayNext) &&
		snapshot.Outcome == result.Outcome
}

func alignExternalResultState(
	state wruntime.State,
	result wruntime.WorkflowResult,
) (wruntime.State, bool) {
	if state.CurrentNodeID == result.SourceNodeID {
		return state, true
	}
	if state.PendingNextNodeID == result.SourceNodeID &&
		(state.Status == wruntime.WorkspaceStatusIdle ||
			state.Status == wruntime.WorkspaceStatusWaitingHuman) {
		state.CurrentNodeID = result.SourceNodeID
		state.Status = wruntime.WorkspaceStatusIdle
		state.HumanGate = nil
		return state, true
	}
	return state, false
}
