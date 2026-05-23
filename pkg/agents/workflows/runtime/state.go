package runtime

import (
	"encoding/json"
	"fmt"
)

type NodeStatus string

const (
	NodeStatusPending  NodeStatus = "pending"
	NodeStatusRunning  NodeStatus = "running"
	NodeStatusComplete NodeStatus = "complete"
	NodeStatusBypassed NodeStatus = "bypassed"
	NodeStatusBlocked  NodeStatus = "blocked"
	NodeStatusError    NodeStatus = "error"
)

type WorkspaceStatus string

const (
	WorkspaceStatusIdle         WorkspaceStatus = "idle"
	WorkspaceStatusRunning      WorkspaceStatus = "running"
	WorkspaceStatusWaitingHuman WorkspaceStatus = "waiting_human"
	WorkspaceStatusComplete     WorkspaceStatus = "complete"
	WorkspaceStatusBlocked      WorkspaceStatus = "blocked"
	WorkspaceStatusError        WorkspaceStatus = "error"
	WorkspaceStatusDone         WorkspaceStatus = "done"
)

type State struct {
	Type              string                  `json:"type"`
	Version           string                  `json:"version"`
	CurrentNodeID     NodeID                  `json:"current_node_id"`
	Status            WorkspaceStatus         `json:"status"`
	Policy            json.RawMessage         `json:"policy"`
	ExecutionCwd      string                  `json:"executionCwd,omitempty"`
	Attempts          map[NodeID]int          `json:"attempts"`
	Nodes             map[NodeID]NodeState    `json:"nodes,omitempty"`
	LastResult        *WorkflowResultSnapshot `json:"last_result,omitempty"`
	HumanGate         *HumanGateState         `json:"human_gate,omitempty"`
	PendingNextNodeID NodeID                  `json:"pending_next_node_id,omitempty"`
	Followups         []FollowupContext       `json:"followups,omitempty"`
}

type NodeState struct {
	Status       NodeStatus `json:"status"`
	Attempts     int        `json:"attempts"`
	LastRunID    string     `json:"last_run_id,omitempty"`
	LastArtifact string     `json:"last_artifact,omitempty"`
	BypassReason string     `json:"bypass_reason,omitempty"`
}

type WorkflowResultSnapshot struct {
	SourceNodeID    NodeID        `json:"source_node_id"`
	Status          string        `json:"status"`
	Summary         string        `json:"summary"`
	PrimaryArtifact string        `json:"primary_artifact,omitempty"`
	Artifacts       []ArtifactRef `json:"artifacts,omitempty"`
	DisplayNext     string        `json:"display_next,omitempty"`
	Workspace       string        `json:"workspace,omitempty"`
	Outcome         ResultOutcome `json:"outcome,omitempty"`
}

type HumanGateState struct {
	From                NodeID `json:"from"`
	To                  NodeID `json:"to"`
	Reason              string `json:"reason"`
	ReviewContextNodeID NodeID `json:"review_context_node_id,omitempty"`
	ReviewArtifact      string `json:"review_artifact,omitempty"`
}

type FollowupContext struct {
	ParentPlanDir      string `json:"parent_plan_dir"`
	FollowupPlanDir    string `json:"followup_plan_dir"`
	ParentReviewNodeID NodeID `json:"parent_review_node_id"`
	ParentReviewPath   string `json:"parent_review_path"`
}

func InitialState(def Definition, policy json.RawMessage) (State, error) {
	if err := ValidateDefinition(def); err != nil {
		return State{}, err
	}
	if len(policy) == 0 || string(policy) == "null" {
		policy = def.PolicySpec.Defaults
	}
	if def.PolicySpec.Validate != nil {
		if err := def.PolicySpec.Validate(policy); err != nil {
			return State{}, err
		}
	}

	state := State{
		Type:          string(def.ID),
		Version:       def.Version,
		CurrentNodeID: def.Start,
		Status:        WorkspaceStatusIdle,
		Policy:        policy,
		Attempts:      map[NodeID]int{},
		Nodes:         map[NodeID]NodeState{},
	}
	for id := range def.Nodes {
		state.Nodes[id] = NodeState{Status: NodeStatusPending}
	}
	return state, nil
}

func ValidateState(def Definition, state State) error {
	if state.Type != string(def.ID) {
		return fmt.Errorf(
			"state type %q does not match definition %q",
			state.Type,
			def.ID,
		)
	}
	if _, ok := def.Nodes[state.CurrentNodeID]; !ok {
		return fmt.Errorf("current node %q is not in definition", state.CurrentNodeID)
	}
	if state.Attempts == nil {
		return fmt.Errorf("attempts is required")
	}
	if state.Nodes == nil {
		return fmt.Errorf("nodes is required")
	}
	if def.PolicySpec.Validate != nil {
		if err := def.PolicySpec.Validate(state.Policy); err != nil {
			return err
		}
	}
	return nil
}
