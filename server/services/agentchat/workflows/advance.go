package workflows

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) AdvanceHumanGate(
	ctx context.Context,
	workspaceID string,
	userEmail string,
) (string, error) {
	if s == nil || s.Store == nil {
		return "", errors.New("workflow service is not configured")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", errors.New("workspace id is required")
	}

	state, err := s.Store.LoadWorkspaceState(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	originalState := state
	isHumanGate := state.Status == wruntime.WorkspaceStatusWaitingHuman && state.HumanGate != nil
	isPendingNode := state.Status == wruntime.WorkspaceStatusIdle && strings.TrimSpace(string(state.PendingNextNodeID)) != ""
	if !isHumanGate && !isPendingNode {
		return "", errors.New("workspace has no pending workflow node")
	}

	next := state.PendingNextNodeID
	if isHumanGate {
		next = state.HumanGate.To
	}
	if strings.TrimSpace(string(next)) == "" {
		return "", errors.New("workspace has no pending workflow node")
	}
	state.CurrentNodeID = next
	state.Status = wruntime.WorkspaceStatusIdle
	state.HumanGate = nil
	state.PendingNextNodeID = next
	if state.Attempts == nil {
		state.Attempts = map[wruntime.NodeID]int{}
	}
	attempt := state.Attempts[next] + 1
	var def wruntime.Definition
	var hasDefinition bool
	if s.Definitions != nil {
		def, hasDefinition = s.Definitions.Get(
			wruntime.WorkflowID(strings.TrimSpace(state.Type)),
		)
		if !hasDefinition {
			return "", errors.New("workflow definition is not registered")
		}
	}

	if err := s.Store.SaveWorkspaceState(ctx, workspaceID, state); err != nil {
		return "", err
	}
	eventType := "workflow_pending_node_started"
	if isHumanGate {
		eventType = "workflow_human_gate_approved"
	}
	if err := s.Store.AppendWorkflowEvents(ctx, workspaceID, db.AgentRun{
		WorkspaceID: sql.NullString{String: workspaceID, Valid: true},
	}, []wruntime.Event{{
		Type:    eventType,
		NodeID:  next,
		Message: strings.TrimSpace(userEmail),
	}}); err != nil {
		return "", err
	}
	input := StartNodeRunInput{
		WorkspaceID: workspaceID,
		NodeID:      next,
		Attempt:     attempt,
		Cwd:         effectiveNodeCwd(state, next),
	}
	var runID string
	if hasDefinition {
		runID, err = s.startNodeRun(ctx, def, state, input)
	} else if s.Runner != nil {
		runID, err = s.Runner.StartNodeRun(ctx, input)
	}
	if err != nil {
		_ = s.Store.SaveWorkspaceState(ctx, workspaceID, originalState)
		return "", err
	}
	return runID, nil
}
