package workflows

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) OnRunComplete(
	ctx context.Context,
	result conversation.RunResult,
) error {
	if s == nil || s.Store == nil || s.Definitions == nil {
		return nil
	}

	run, err := s.Store.LoadRun(ctx, result.RunID)
	if err != nil {
		return err
	}
	workspaceID := strings.TrimSpace(run.WorkspaceID.String)
	nodeID := strings.TrimSpace(run.WorkflowNodeID.String)
	if !run.WorkspaceID.Valid || workspaceID == "" || !run.WorkflowNodeID.Valid ||
		nodeID == "" {
		return nil
	}
	if run.WorkflowResultJson.Valid &&
		strings.TrimSpace(run.WorkflowResultJson.String) != "" {
		return nil
	}

	state, err := s.Store.LoadWorkspaceState(ctx, workspaceID)
	if err != nil {
		return err
	}
	def, ok := s.Definitions.Get(wruntime.WorkflowID(strings.TrimSpace(state.Type)))
	if !ok {
		return fmt.Errorf("workflow definition %q is not registered", state.Type)
	}

	headEntryID := strings.TrimSpace(result.HeadEntryID)
	if headEntryID == "" && run.ResultHeadEntryID.Valid {
		headEntryID = run.ResultHeadEntryID.String
	}
	output, err := s.Store.FinalAssistantText(ctx, run.ThreadID, headEntryID)
	if err != nil {
		return err
	}
	result.HeadEntryID = headEntryID
	parseCtx := parseContext(state, run, result, nodeID)
	parsed, err := def.ResultParser.Parse(output, parseCtx)
	if err != nil {
		return s.handleInvalidResult(ctx, def, state, run, err)
	}
	workflowResult, err := def.ResultConverter.ToWorkflowResult(parsed, parseCtx)
	if err != nil {
		return s.handleInvalidResult(ctx, def, state, run, err)
	}
	if err := validateWorkflowArtifacts(
		ctx,
		s.Store,
		workspaceID,
		workflowResult,
	); err != nil {
		return err
	}
	applied, err := s.applyQRSPICompletion(ctx, def, state, run, parseCtx, parsed)
	if err != nil {
		return err
	}
	decision, err := s.prepareQRSPIApplyDecision(ctx, workspaceID, state, applied)
	if err != nil {
		return err
	}
	if err := s.persistQRSPIApplyResult(ctx, workspaceID, run, applied.WorkflowResult, decision); err != nil {
		return err
	}
	if err := s.startQRSPIApplyNext(ctx, workspaceID, run.ThreadID, run, def, decision); err != nil {
		return err
	}
	return nil
}

func (s *Service) applyQRSPICompletion(
	ctx context.Context,
	def wruntime.Definition,
	state wruntime.State,
	run db.AgentRun,
	parseCtx wruntime.ParseContext,
	parsed any,
) (semantic.ApplyResult, error) {
	qrspiParsed, ok := parsed.(qrspi.Result)
	if !ok {
		return semantic.ApplyResult{}, fmt.Errorf("expected qrspi Result, got %T", parsed)
	}
	return semantic.Apply(ctx, semantic.ApplyInput{
		Definition:   def,
		ParsedResult: &qrspiParsed,
		ParseContext: parseCtx,
		Context: semantic.Context{
			WorkflowType:      wruntime.WorkflowID(state.Type),
			State:             state,
			ExpectedNodeID:    parseCtx.ExpectedNodeID,
			Source:            semantic.SourceRunCompletion,
			ImplementationCwd: strings.TrimSpace(state.ExecutionCwd),
			RunID:             run.ID,
		},
	})
}

func (s *Service) prepareQRSPIApplyDecision(
	ctx context.Context,
	workspaceID string,
	state wruntime.State,
	applied semantic.ApplyResult,
) (wruntime.TransitionDecision, error) {
	decision := applied.Decision
	result := applied.WorkflowResult
	if wruntime.WorkflowID(state.Type) != qrspi.AgentChatWorkflowType {
		return decision, nil
	}
	planningCwd := ""
	var err error
	if result.SourceNodeID == qrspi.NodeWorkspace {
		planningCwd, err = s.Store.WorkspacePlanningCwd(ctx, workspaceID)
		if err != nil {
			return decision, err
		}
		decision.State, err = applyQRSPIWorkspaceResult(decision.State, result, planningCwd)
		if err != nil {
			return decision, err
		}
	}
	decision = maybeExitImplementationFollowup(state, decision, result)
	decision, err = maybeEnterImplementationFollowup(decision, result)
	if err != nil {
		return decision, err
	}
	return decision, nil
}

func (s *Service) persistQRSPIApplyResult(
	ctx context.Context,
	workspaceID string,
	run db.AgentRun,
	result wruntime.WorkflowResult,
	decision wruntime.TransitionDecision,
) error {
	if strings.TrimSpace(run.ID) != "" {
		if err := s.Store.SaveRunResult(ctx, run.ID, result); err != nil {
			return err
		}
	}
	if err := s.Store.SaveWorkspaceState(ctx, workspaceID, decision.State); err != nil {
		return err
	}
	return s.Store.AppendWorkflowEvents(ctx, workspaceID, run, decision.Events)
}

func (s *Service) startQRSPIApplyNext(
	ctx context.Context,
	workspaceID string,
	threadID string,
	run db.AgentRun,
	def wruntime.Definition,
	decision wruntime.TransitionDecision,
) error {
	if !decision.StartNext {
		return nil
	}
	nextRunID, startErr := s.startNodeRunWithSQLiteBusyRetry(ctx, def, decision.State, StartNodeRunInput{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		NodeID:      decision.NextNodeID,
		Attempt:     decision.State.Attempts[decision.NextNodeID] + 1,
		Cwd:         effectiveNodeCwd(decision.State, decision.NextNodeID),
	})
	if startErr != nil {
		_ = s.Store.AppendWorkflowEvents(ctx, workspaceID, run, []wruntime.Event{{
			Type:    "workflow_next_start_failed",
			NodeID:  decision.NextNodeID,
			Message: startErr.Error(),
		}})
		return startErr
	}
	if strings.TrimSpace(nextRunID) != "" {
		_ = s.Store.AppendWorkflowEvents(ctx, workspaceID, db.AgentRun{
			ID:          nextRunID,
			WorkspaceID: sql.NullString{String: workspaceID, Valid: true},
		}, []wruntime.Event{{
			Type:    "workflow_next_started",
			NodeID:  decision.NextNodeID,
			Message: nextRunID,
		}})
	}
	return nil
}

func (s *Service) startNodeRunWithSQLiteBusyRetry(
	ctx context.Context,
	def wruntime.Definition,
	state wruntime.State,
	input StartNodeRunInput,
) (string, error) {
	const attempts = 4
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var runID string
		runID, err = s.startNodeRun(ctx, def, state, input)
		if err == nil || !isSQLiteBusyError(err) || attempt == attempts {
			return runID, err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
		}
	}
	return "", err
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "database is locked")
}

func (s *Service) startNodeRun(
	ctx context.Context,
	def wruntime.Definition,
	state wruntime.State,
	input StartNodeRunInput,
) (string, error) {
	if s == nil || s.Runner == nil {
		return "", nil
	}
	if strings.TrimSpace(input.Prompt) == "" {
		node, ok := def.Nodes[input.NodeID]
		if !ok {
			return "", fmt.Errorf("workflow node %q is not registered", input.NodeID)
		}
		if node.Kind == wruntime.NodeKindDone {
			input.Prompt = "Complete the workflow and emit the final workflow result."
		} else {
			prompt, err := RenderNodePrompt(ctx, def, node, state)
			if err != nil {
				return "", err
			}
			input.Prompt = prompt
		}
	}
	return s.Runner.StartNodeRun(ctx, input)
}

func parseContext(
	state wruntime.State,
	run db.AgentRun,
	result conversation.RunResult,
	nodeID string,
) wruntime.ParseContext {
	sessionID := ""
	if run.SessionID.Valid {
		sessionID = run.SessionID.String
	}
	return wruntime.ParseContext{
		WorkflowType:   state.Type,
		ExpectedNodeID: wruntime.NodeID(nodeID),
		RunID:          run.ID,
		ThreadID:       run.ThreadID,
		SessionID:      sessionID,
		HeadEntryID:    result.HeadEntryID,
		SessionPath:    result.SessionPath,
	}
}

type artifactExistenceStore interface {
	ArtifactExists(ctx context.Context, workspaceID, relPath string) (bool, error)
}

func validateWorkflowArtifacts(
	ctx context.Context,
	store artifactExistenceStore,
	workspaceID string,
	result wruntime.WorkflowResult,
) error {
	seen := map[string]bool{}
	paths := make([]string, 0, 1+len(result.Artifacts))
	if path := strings.TrimSpace(result.PrimaryArtifact); path != "" {
		paths = append(paths, path)
	}
	for _, artifact := range result.Artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path != "" {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		exists, err := store.ArtifactExists(ctx, workspaceID, path)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("workflow result artifact %q not found", path)
		}
	}
	return nil
}
