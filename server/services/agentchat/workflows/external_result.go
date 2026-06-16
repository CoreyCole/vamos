package workflows

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
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
	if wruntime.WorkflowID(state.Type) == qrspi.AgentChatWorkflowType {
		if err := qrspi.ValidateOutcomeArtifacts(result); err != nil {
			return wruntime.TransitionDecision{}, false, err
		}
		planningCwd := ""
		var err error
		if result.SourceNodeID == qrspi.NodeWorkspace {
			planningCwd, err = s.Store.WorkspacePlanningCwd(ctx, workspaceID)
			if err != nil {
				return wruntime.TransitionDecision{}, false, err
			}
		}
		state, err = applyQRSPIWorkspaceResult(state, result, planningCwd)
		if err != nil {
			return wruntime.TransitionDecision{}, false, err
		}
	}

	decision, err := wruntime.DecideTransition(def, state, result)
	if err != nil {
		return wruntime.TransitionDecision{State: state}, false, nil
	}
	if wruntime.WorkflowID(state.Type) == qrspi.AgentChatWorkflowType {
		decision = maybeExitImplementationFollowup(state, decision, result)
		decision, err = maybeEnterImplementationFollowup(decision, result)
		if err != nil {
			return wruntime.TransitionDecision{}, false, err
		}
		if warning := displayNextCompatibilityWarning(decision, result); warning != "" {
			decision.Events = append(decision.Events, wruntime.Event{
				Type:    "workflow_display_next_mismatch",
				NodeID:  result.SourceNodeID,
				Message: warning,
			})
		}
	}
	if err := s.Store.SaveWorkspaceState(ctx, workspaceID, decision.State); err != nil {
		return wruntime.TransitionDecision{}, false, err
	}
	run := db.AgentRun{
		ThreadID:    threadID,
		WorkspaceID: nullString(workspaceID),
	}
	events := append(decision.Events, wruntime.Event{
		Type:   "workflow_imported_terminal_result",
		NodeID: result.SourceNodeID,
	})
	if err := s.Store.AppendWorkflowEvents(ctx, workspaceID, run, events); err != nil {
		return wruntime.TransitionDecision{}, false, err
	}
	if decision.StartNext {
		_, err = s.startNodeRunWithSQLiteBusyRetry(ctx, def, decision.State, StartNodeRunInput{
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			NodeID:      decision.NextNodeID,
			Attempt:     decision.State.Attempts[decision.NextNodeID] + 1,
			Cwd:         effectiveNodeCwd(decision.State, decision.NextNodeID),
		})
		if err != nil {
			return decision, true, err
		}
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
