package runtime

import (
	"fmt"
	"strings"
)

type Event struct {
	Type    string `json:"type"`
	NodeID  NodeID `json:"node_id,omitempty"`
	Message string `json:"message,omitempty"`
}

type TransitionDecision struct {
	State        State
	NextNodeID   NodeID
	StartNext    bool
	WaitingHuman bool
	StopReason   string
	Events       []Event
}

func DecideTransition(
	def Definition,
	state State,
	result WorkflowResult,
) (TransitionDecision, error) {
	if err := ValidateState(def, state); err != nil {
		return TransitionDecision{}, err
	}
	if result.SourceNodeID != state.CurrentNodeID {
		return TransitionDecision{}, fmt.Errorf(
			"result source %q does not match current node %q",
			result.SourceNodeID,
			state.CurrentNodeID,
		)
	}
	if err := ValidateWorkflowResult(def, state, result); err != nil {
		return TransitionDecision{}, err
	}

	state.LastResult = &WorkflowResultSnapshot{
		SourceNodeID:    result.SourceNodeID,
		Status:          string(result.Status),
		Summary:         result.Summary,
		PrimaryArtifact: result.PrimaryArtifact,
		Artifacts:       result.Artifacts,
		DisplayNext:     result.DisplayNext,
		Workspace:       result.Workspace,
		Outcome:         result.Outcome,
		Raw:             result.Raw,
	}
	nodeState := state.Nodes[result.SourceNodeID]
	nodeState.LastRunID = result.Evidence.RunID
	nodeState.LastArtifact = result.PrimaryArtifact
	state.Nodes[result.SourceNodeID] = nodeState

	switch result.Status {
	case StatusComplete, StatusDone:
		// Continue below.
	case StatusNeedsHuman:
		state.Status = WorkspaceStatusWaitingHuman
		return TransitionDecision{
			State:        state,
			WaitingHuman: true,
			StopReason:   "result requested human input",
			Events: []Event{
				{Type: "workflow_waiting_human", NodeID: result.SourceNodeID},
			},
		}, nil
	case StatusBlocked:
		nodeState.Status = NodeStatusBlocked
		state.Nodes[result.SourceNodeID] = nodeState
		state.Status = WorkspaceStatusBlocked
		return TransitionDecision{
			State:      state,
			StopReason: "result blocked",
			Events:     []Event{{Type: "workflow_blocked", NodeID: result.SourceNodeID}},
		}, nil
	case StatusError:
		nodeState.Status = NodeStatusError
		state.Nodes[result.SourceNodeID] = nodeState
		state.Status = WorkspaceStatusError
		return TransitionDecision{
			State:      state,
			StopReason: "result error",
			Events:     []Event{{Type: "workflow_error", NodeID: result.SourceNodeID}},
		}, nil
	default:
		return TransitionDecision{}, fmt.Errorf(
			"unsupported result status %q",
			result.Status,
		)
	}

	nodeState.Status = NodeStatusComplete
	state.Nodes[result.SourceNodeID] = nodeState
	config, err := decodeTransitionConfig(def, state)
	if err != nil {
		return TransitionDecision{}, err
	}
	next, edge, ok := nextEdge(
		def,
		TransitionContext{Config: config, State: state, Result: result},
		state.CurrentNodeID,
	)
	if !ok {
		state.Status = WorkspaceStatusDone
		state.PendingNextNodeID = ""
		return TransitionDecision{
			State:      state,
			StopReason: "terminal",
			Events:     []Event{{Type: "workflow_done", NodeID: result.SourceNodeID}},
		}, nil
	}
	target := def.Nodes[edge.To]
	if edge.Gate.Human || target.Kind == NodeKindHumanReview {
		reason := edge.Gate.Reason
		if strings.TrimSpace(reason) == "" {
			reason = target.Prompt.Static
		}
		if target.Kind == NodeKindHumanReview && target.AutoApprovable &&
			autoModeEnabled(config) {
			return autoApproveHumanReview(def, state, config, target, reason)
		}
		state.Status = WorkspaceStatusWaitingHuman
		state.HumanGate = &HumanGateState{
			From:                edge.From,
			To:                  edge.To,
			Reason:              reason,
			ReviewContextNodeID: result.SourceNodeID,
			ReviewArtifact:      result.PrimaryArtifact,
		}
		state.PendingNextNodeID = edge.To
		return TransitionDecision{
			State:        state,
			NextNodeID:   edge.To,
			WaitingHuman: true,
			StopReason:   reason,
			Events: []Event{
				{
					Type:    "workflow_waiting_human",
					NodeID:  edge.To,
					Message: reason,
				},
			},
		}, nil
	}

	state.CurrentNodeID = next
	state.Status = WorkspaceStatusIdle
	state.PendingNextNodeID = next
	return TransitionDecision{
		State:      state,
		NextNodeID: next,
		StartNext:  shouldStartNonHumanEdge(config),
		Events:     []Event{{Type: "workflow_node_ready", NodeID: next}},
	}, nil
}

func ValidateWorkflowResult(def Definition, state State, result WorkflowResult) error {
	node, ok := def.Nodes[state.CurrentNodeID]
	if !ok {
		return fmt.Errorf("current node %q is not in definition", state.CurrentNodeID)
	}
	if !node.Contract.AllowsStatus(result.Status) {
		return fmt.Errorf("status %q is not valid for node %q", result.Status, node.ID)
	}
	if result.Status == StatusComplete {
		if len(node.Contract.Outcomes) > 0 &&
			strings.TrimSpace(string(result.Outcome)) == "" {
			return fmt.Errorf("outcome is required for node %q", node.ID)
		}
		if !node.Contract.AllowsOutcome(result.Outcome) {
			return fmt.Errorf(
				"outcome %q is not valid for node %q",
				result.Outcome,
				node.ID,
			)
		}
	}
	if node.Contract.PrimaryArtifactRequired &&
		strings.TrimSpace(result.PrimaryArtifact) == "" {
		return fmt.Errorf("primary artifact is required for node %q", node.ID)
	}
	return nil
}

func ApplyBypassNodes(
	def Definition,
	state State,
	from, to NodeID,
	reason string,
) (State, []Event) {
	if _, ok := def.Nodes[from]; !ok {
		return state, []Event{
			{
				Type:    "workflow_bypass_error",
				NodeID:  from,
				Message: fmt.Sprintf("node %q is not in definition", from),
			},
		}
	}
	if _, ok := def.Nodes[to]; !ok {
		return state, []Event{
			{
				Type:    "workflow_bypass_error",
				NodeID:  to,
				Message: fmt.Sprintf("node %q is not in definition", to),
			},
		}
	}
	nodeState := state.Nodes[from]
	nodeState.Status = NodeStatusBypassed
	nodeState.BypassReason = reason
	state.Nodes[from] = nodeState
	state.CurrentNodeID = to
	state.PendingNextNodeID = to
	state.Status = WorkspaceStatusIdle
	state.HumanGate = nil
	return state, []Event{{Type: "workflow_node_bypassed", NodeID: from, Message: reason}}
}

func decodeTransitionConfig(def Definition, state State) (any, error) {
	if def.PolicySpec.Decode != nil {
		return def.PolicySpec.Decode(state.Policy)
	}
	return nil, nil
}

func autoModeEnabled(config any) bool {
	autoMode, ok := config.(AutoModeConfig)
	return ok && autoMode.IsAutoMode()
}

func shouldStartNonHumanEdge(config any) bool {
	advanceMode, ok := config.(AdvanceModeConfig)
	return !ok || advanceMode.ShouldStartNonHumanEdges()
}

func autoApproveHumanReview(
	def Definition,
	state State,
	config any,
	target Node,
	reason string,
) (TransitionDecision, error) {
	next, _, ok := nextEdge(
		def,
		TransitionContext{
			Config: config,
			State:  state,
			Result: WorkflowResult{
				SourceNodeID: target.ID,
				Status:       StatusComplete,
				Outcome:      OutcomeComplete,
			},
		},
		target.ID,
	)
	if !ok {
		state.Status = WorkspaceStatusDone
		state.PendingNextNodeID = ""
		return TransitionDecision{
			State:      state,
			StopReason: "terminal",
			Events: []Event{
				{
					Type:    "workflow_human_gate_auto_approved",
					NodeID:  target.ID,
					Message: reason,
				},
				{Type: "workflow_done", NodeID: target.ID},
			},
		}, nil
	}

	nodeState := state.Nodes[target.ID]
	nodeState.Status = NodeStatusBypassed
	nodeState.BypassReason = "human gate auto-approved by config"
	state.Nodes[target.ID] = nodeState
	state.CurrentNodeID = next
	state.PendingNextNodeID = next
	state.Status = WorkspaceStatusIdle
	state.HumanGate = nil

	return TransitionDecision{
		State:      state,
		NextNodeID: next,
		StartNext:  true,
		Events: []Event{
			{
				Type:    "workflow_human_gate_auto_approved",
				NodeID:  target.ID,
				Message: reason,
			},
			{Type: "workflow_node_ready", NodeID: next},
		},
	}, nil
}

func nextEdge(def Definition, ctx TransitionContext, from NodeID) (NodeID, Edge, bool) {
	for _, edge := range def.Edges {
		if edge.From != from {
			continue
		}
		if edge.Outcome != "" && edge.Outcome != ctx.Result.Outcome {
			continue
		}
		if edge.Predicate != nil && !edge.Predicate(ctx) {
			continue
		}
		return edge.To, edge, true
	}
	return "", Edge{}, false
}
