package workflows

import (
	"context"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) handleInvalidResult(
	ctx context.Context,
	def wruntime.Definition,
	state wruntime.State,
	run db.AgentRun,
	parseErr error,
) error {
	workspaceID := strings.TrimSpace(run.WorkspaceID.String)
	if state.Attempts == nil {
		state.Attempts = map[wruntime.NodeID]int{}
	}
	nodeID := state.CurrentNodeID
	if strings.TrimSpace(run.WorkflowNodeID.String) != "" {
		nodeID = wruntime.NodeID(strings.TrimSpace(run.WorkflowNodeID.String))
	}
	attempts := state.Attempts[nodeID]
	if int(run.WorkflowAttempt) > attempts {
		attempts = int(run.WorkflowAttempt)
	}
	limit := qrspi.ParsePolicy(state.Policy).InvalidResultRetryLimit

	if attempts < limit {
		nextAttempt := attempts + 1
		state.Attempts[nodeID] = nextAttempt
		nodeState := state.Nodes[nodeID]
		nodeState.Attempts = nextAttempt
		nodeState.Status = wruntime.NodeStatusRunning
		nodeState.LastRunID = run.ID
		state.Nodes[nodeID] = nodeState
		state.Status = wruntime.WorkspaceStatusRunning
		if err := s.Store.SaveWorkspaceState(ctx, workspaceID, state); err != nil {
			return err
		}
		if err := s.Store.AppendWorkflowEvents(ctx, workspaceID, run, []wruntime.Event{{
			Type:    "workflow_invalid_result_retry",
			NodeID:  nodeID,
			Message: parseErr.Error(),
		}}); err != nil {
			return err
		}
		if s.Runner == nil {
			return nil
		}
		_, err := s.Runner.StartNodeRun(ctx, StartNodeRunInput{
			WorkspaceID: workspaceID,
			ThreadID:    run.ThreadID,
			NodeID:      nodeID,
			Prompt:      def.ResultParser.CorrectionPrompt(parseErr, nextAttempt),
			Attempt:     nextAttempt,
		})
		return err
	}

	state.Status = wruntime.WorkspaceStatusError
	nodeState := state.Nodes[nodeID]
	nodeState.Status = wruntime.NodeStatusError
	nodeState.Attempts = attempts
	nodeState.LastRunID = run.ID
	state.Nodes[nodeID] = nodeState
	if err := s.Store.SaveWorkspaceState(ctx, workspaceID, state); err != nil {
		return err
	}
	return s.Store.AppendWorkflowEvents(ctx, workspaceID, db.AgentRun{
		ID:          run.ID,
		WorkspaceID: run.WorkspaceID,
		ThreadID:    run.ThreadID,
		SessionID:   run.SessionID,
	}, []wruntime.Event{{
		Type:    "workflow_invalid_result_exhausted",
		NodeID:  nodeID,
		Message: parseErr.Error(),
	}})
}
