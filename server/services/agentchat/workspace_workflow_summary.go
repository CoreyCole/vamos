package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func (s *Service) SummaryForPlanDir(ctx context.Context, planDir string) (workspaces.WorkspaceWorkflowSummary, bool, error) {
	canonical, ok := s.canonicalPlanDirFromSource(planDir)
	if !ok {
		return workspaces.WorkspaceWorkflowSummary{}, false, nil
	}
	row, err := s.queries.FindWorkspaceByRootDocPath(ctx, canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return workspaces.WorkspaceWorkflowSummary{}, false, nil
	}
	if err != nil {
		return workspaces.WorkspaceWorkflowSummary{}, false, err
	}
	return workflowSummaryFromWorkspace(row), true, nil
}

func workflowSummaryFromWorkspace(row db.Workspace) workspaces.WorkspaceWorkflowSummary {
	summary := workspaces.WorkspaceWorkflowSummary{
		WorkflowType: strings.TrimSpace(row.WorkflowType),
		Stage:        "unknown",
		Status:       "unknown",
	}
	if summary.WorkflowType == "" {
		summary.WorkflowType = string(WorkspaceWorkflowFreeform)
	}
	if !row.WorkflowStateJson.Valid || strings.TrimSpace(row.WorkflowStateJson.String) == "" {
		return summary
	}
	raw := []byte(row.WorkflowStateJson.String)
	var state wruntime.State
	if err := json.Unmarshal(raw, &state); err == nil && strings.TrimSpace(state.Type) != "" {
		summary.WorkflowType = strings.TrimSpace(state.Type)
		summary.Stage = string(state.CurrentNodeID)
		if summary.Stage == "" {
			summary.Stage = "unknown"
		}
		summary.Status = string(state.Status)
		if summary.Status == "" {
			summary.Status = "unknown"
		}
		summary.WaitingHuman = state.Status == wruntime.WorkspaceStatusWaitingHuman || state.HumanGate != nil
		if state.PendingNextNodeID != "" {
			summary.NextStep = string(state.PendingNextNodeID)
		}
		if state.HumanGate != nil && summary.NextStep == "" {
			summary.NextStep = string(state.HumanGate.To)
		}
		if state.LastResult != nil {
			summary.PrimaryArtifact = state.LastResult.PrimaryArtifact
			summary.Outcome = string(state.LastResult.Outcome)
			if summary.NextStep == "" {
				summary.NextStep = state.LastResult.DisplayNext
			}
		}
		return summary
	}

	var probe struct {
		WorkflowType    string `json:"type"`
		WorkflowTypeAlt string `json:"workflow_type"`
		Stage           string `json:"stage"`
		CurrentStage    string `json:"currentStage"`
		CurrentStep     string `json:"current_step"`
		CurrentStepAlt  string `json:"currentStep"`
		Status          string `json:"status"`
		Outcome         string `json:"outcome"`
		WaitingHuman    bool   `json:"waitingHuman"`
		NextStep        string `json:"nextStep"`
		PrimaryArtifact string `json:"primaryArtifact"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return summary
	}
	summary.WorkflowType = firstNonEmpty(probe.WorkflowType, probe.WorkflowTypeAlt, summary.WorkflowType)
	summary.Stage = firstNonEmpty(probe.Stage, probe.CurrentStage, probe.CurrentStep, probe.CurrentStepAlt, summary.Stage)
	summary.Status = firstNonEmpty(probe.Status, summary.Status)
	summary.Outcome = strings.TrimSpace(probe.Outcome)
	summary.WaitingHuman = probe.WaitingHuman
	summary.NextStep = strings.TrimSpace(probe.NextStep)
	summary.PrimaryArtifact = strings.TrimSpace(probe.PrimaryArtifact)
	return summary
}
