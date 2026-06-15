package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkflowPolicyPreset string

const (
	WorkflowPolicyPresetDiscuss   WorkflowPolicyPreset = "discuss"
	WorkflowPolicyPresetGuided    WorkflowPolicyPreset = "guided"
	WorkflowPolicyPresetAutopilot WorkflowPolicyPreset = "autopilot"
	WorkflowPolicyPresetFastDraft WorkflowPolicyPreset = "fast_draft"
)

const (
	legacyWorkflowPolicyPresetManual   WorkflowPolicyPreset = "manual"
	legacyWorkflowPolicyPresetAssisted WorkflowPolicyPreset = "assisted"
	WorkflowPolicyPresetManual         WorkflowPolicyPreset = WorkflowPolicyPresetGuided
	WorkflowPolicyPresetAssisted       WorkflowPolicyPreset = WorkflowPolicyPresetAutopilot
)

type WorkspaceWorkflowPolicyProjection struct {
	AdvanceMode             qrspi.AdvanceMode
	AutoMode                bool
	EnablePlanReviews       bool
	InvalidResultRetryLimit int
	Preset                  WorkflowPolicyPreset
	ModeLabel               string
	ReviewLabel             string
	RetryLabel              string
	TimingCopy              string
	Editable                bool
	AdvancedJSON            string
}

type UpdateWorkspaceWorkflowPolicyInput struct {
	WorkspaceID             string
	UserEmail               string
	AdvanceMode             qrspi.AdvanceMode
	AutoMode                bool
	EnablePlanReviews       bool
	InvalidResultRetryLimit int
}

func PolicyForPreset(preset WorkflowPolicyPreset) qrspi.Policy {
	switch preset {
	case WorkflowPolicyPresetDiscuss:
		return qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeDiscuss,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		}
	case WorkflowPolicyPresetAutopilot:
		return qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeAutopilot,
			AutoMode:                true,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		}
	case WorkflowPolicyPresetFastDraft:
		return qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeAutopilot,
			AutoMode:                true,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 1,
		}
	default:
		return qrspi.DefaultPolicy()
	}
}

func parseWorkflowPolicyPreset(value string) (WorkflowPolicyPreset, error) {
	switch WorkflowPolicyPreset(strings.TrimSpace(value)) {
	case "", WorkflowPolicyPresetGuided, legacyWorkflowPolicyPresetManual:
		return WorkflowPolicyPresetGuided, nil
	case WorkflowPolicyPresetDiscuss:
		return WorkflowPolicyPresetDiscuss, nil
	case WorkflowPolicyPresetAutopilot, legacyWorkflowPolicyPresetAssisted:
		return WorkflowPolicyPresetAutopilot, nil
	case WorkflowPolicyPresetFastDraft:
		return WorkflowPolicyPresetFastDraft, nil
	default:
		return "", fmt.Errorf("unknown workflow policy preset %q", value)
	}
}

func marshalWorkflowPolicy(policy qrspi.Policy) (json.RawMessage, error) {
	if err := qrspi.ValidateConfig(policy); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func ProjectWorkspaceWorkflowPolicy(
	state wruntime.State,
) (WorkspaceWorkflowPolicyProjection, error) {
	policy := qrspi.ParsePolicy(state.Policy)
	if err := qrspi.ValidateConfig(policy); err != nil {
		return WorkspaceWorkflowPolicyProjection{}, err
	}
	editable, copy := policyTimingCopy(state.Status)
	advanced, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return WorkspaceWorkflowPolicyProjection{}, err
	}
	return WorkspaceWorkflowPolicyProjection{
		AdvanceMode:             policy.EffectiveAdvanceMode(),
		AutoMode:                policy.IsAutoMode(),
		EnablePlanReviews:       policy.EnablePlanReviews,
		InvalidResultRetryLimit: policy.InvalidResultRetryLimit,
		Preset:                  presetForPolicy(policy),
		ModeLabel:               policyModeLabel(policy),
		ReviewLabel:             policyReviewLabel(policy),
		RetryLabel: fmt.Sprintf(
			"Retries %d",
			policy.InvalidResultRetryLimit,
		),
		TimingCopy:   copy,
		Editable:     editable,
		AdvancedJSON: string(advanced),
	}, nil
}

func presetForPolicy(policy qrspi.Policy) WorkflowPolicyPreset {
	switch {
	case policy.EffectiveAdvanceMode() == qrspi.AdvanceModeDiscuss && policy.EnablePlanReviews && policy.InvalidResultRetryLimit == 1:
		return WorkflowPolicyPresetDiscuss
	case policy.EffectiveAdvanceMode() == qrspi.AdvanceModeGuided && policy.EnablePlanReviews && policy.InvalidResultRetryLimit == 1:
		return WorkflowPolicyPresetGuided
	case policy.EffectiveAdvanceMode() == qrspi.AdvanceModeAutopilot && policy.EnablePlanReviews && policy.InvalidResultRetryLimit == 1:
		return WorkflowPolicyPresetAutopilot
	case policy.EffectiveAdvanceMode() == qrspi.AdvanceModeAutopilot && !policy.EnablePlanReviews && policy.InvalidResultRetryLimit == 1:
		return WorkflowPolicyPresetFastDraft
	default:
		return ""
	}
}

func policyModeLabel(policy qrspi.Policy) string {
	switch policy.EffectiveAdvanceMode() {
	case qrspi.AdvanceModeDiscuss:
		return "Discuss: pause after valid YAML"
	case qrspi.AdvanceModeAutopilot:
		return "Autopilot: auto-continue safe gates"
	default:
		return "Guided: continue graph-safe steps"
	}
}

func policyReviewLabel(policy qrspi.Policy) string {
	if policy.EnablePlanReviews {
		return "Planning reviews on"
	}
	return "Fast draft: planning reviews skipped"
}

func policyTimingCopy(status wruntime.WorkspaceStatus) (bool, string) {
	switch status {
	case wruntime.WorkspaceStatusRunning:
		return true, "Applies after the current agent run finishes."
	case wruntime.WorkspaceStatusWaitingHuman:
		return true, "Current gate target is already selected; changes affect later steps after you proceed."
	case wruntime.WorkspaceStatusBlocked, wruntime.WorkspaceStatusError:
		return true, "Saved, but does not resume automatically."
	case wruntime.WorkspaceStatusDone:
		return false, "Workflow is done; config is read-only."
	default:
		return true, "Changes apply to future transitions."
	}
}

func (s *Service) UpdateWorkspaceWorkflowPolicy(
	ctx context.Context,
	input UpdateWorkspaceWorkflowPolicyInput,
) (WorkspaceWorkflowState, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return WorkspaceWorkflowState{}, errors.New("workspace id is required")
	}
	workspace, err := s.GetWorkspaceForUser(
		ctx,
		strings.TrimSpace(input.UserEmail),
		workspaceID,
	)
	if err != nil {
		return WorkspaceWorkflowState{}, err
	}
	if !isQRSPIWorkflowType(WorkspaceWorkflowType(workspace.WorkflowType)) {
		return WorkspaceWorkflowState{}, errors.New(
			"workflow policy is only supported for QRSPI workspaces",
		)
	}
	if !workspace.WorkflowStateJson.Valid ||
		strings.TrimSpace(workspace.WorkflowStateJson.String) == "" {
		return WorkspaceWorkflowState{}, errors.New(
			"workspace has no runtime workflow state",
		)
	}

	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(workspace.WorkflowStateJson.String),
		&state,
	); err != nil {
		return WorkspaceWorkflowState{}, fmt.Errorf(
			"parse runtime workflow state: %w",
			err,
		)
	}
	if state.Status == wruntime.WorkspaceStatusDone {
		return WorkspaceWorkflowState{}, errors.New(
			"workflow is done; policy is read-only",
		)
	}

	advanceMode := input.AdvanceMode
	if advanceMode == "" {
		if input.AutoMode {
			advanceMode = qrspi.AdvanceModeAutopilot
		} else {
			advanceMode = qrspi.AdvanceModeGuided
		}
	}
	policy := qrspi.Policy{
		AdvanceMode:             advanceMode,
		AutoMode:                advanceMode == qrspi.AdvanceModeAutopilot,
		EnablePlanReviews:       input.EnablePlanReviews,
		InvalidResultRetryLimit: input.InvalidResultRetryLimit,
	}
	encoded, err := marshalWorkflowPolicy(policy)
	if err != nil {
		return WorkspaceWorkflowState{}, err
	}
	state.Policy = encoded

	def, ok := s.workflowDefinition(wruntime.WorkflowID(state.Type))
	if !ok {
		return WorkspaceWorkflowState{}, errors.New(
			"workflow definition is not registered",
		)
	}
	if err := wruntime.ValidateState(def, state); err != nil {
		return WorkspaceWorkflowState{}, err
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return WorkspaceWorkflowState{}, err
	}
	if err := s.queries.UpdateWorkspaceWorkflowState(
		ctx,
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      state.Type,
			WorkflowStateJson: nullString(string(stateJSON)),
		},
	); err != nil {
		return WorkspaceWorkflowState{}, err
	}

	eventPayload := struct {
		AdvanceMode             string `json:"advanceMode"`
		AutoMode                bool   `json:"autoMode"`
		EnablePlanReviews       bool   `json:"enablePlanReviews"`
		InvalidResultRetryLimit int    `json:"invalidResultRetryLimit"`
		Status                  string `json:"status"`
	}{
		AdvanceMode:             string(policy.EffectiveAdvanceMode()),
		AutoMode:                policy.IsAutoMode(),
		EnablePlanReviews:       policy.EnablePlanReviews,
		InvalidResultRetryLimit: policy.InvalidResultRetryLimit,
		Status:                  string(state.Status),
	}
	payloadJSON, err := json.Marshal(eventPayload)
	if err != nil {
		return WorkspaceWorkflowState{}, err
	}
	event, err := s.AppendWorkspaceEvent(ctx, s.queries, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   "workflow_policy_updated",
		ActorEmail:  input.UserEmail,
		ActorType:   "user",
		PayloadJSON: string(payloadJSON),
		EventKey: fmt.Sprintf(
			"workflow_policy_updated:%s:%s",
			workspace.ID,
			time.Now().UTC().Format(time.RFC3339Nano),
		),
	})
	if err != nil {
		return WorkspaceWorkflowState{}, err
	}
	s.NotifyWorkspaceForEvent(event)

	updated := workspace
	updated.WorkflowStateJson = nullString(string(stateJSON))
	return s.BuildWorkspaceWorkflowState(ctx, updated)
}
