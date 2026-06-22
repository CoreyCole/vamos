package agentchat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
)

const (
	defaultQRSPIProjectionMaxResults  = 50
	defaultQRSPIProjectionMaxDuration = 10 * time.Second
)

type QRSPIExternalResultSource struct {
	WorkspaceID    string
	ThreadID       string
	HeadEntryID    string
	SessionID      string
	SessionPath    string
	ResultJSON     string
	SourceEventID  string
	WorkflowNodeID string
	SelectThread   bool
}

func (s *Service) ApplyQRSPIProjections(
	ctx context.Context,
	input QRSPIProjectionApplyInput,
) (QRSPIProjectionApplyResult, error) {
	if s == nil || s.queries == nil {
		return QRSPIProjectionApplyResult{}, errors.New("qrspi projection applier requires service queries")
	}
	input = normalizeQRSPIProjectionApplyInput(input)
	rows, err := s.queries.ListPendingQRSPIProjections(ctx, int64(input.MaxResults))
	if err != nil {
		return QRSPIProjectionApplyResult{}, err
	}
	var result QRSPIProjectionApplyResult
	started := time.Now()
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if time.Since(started) >= input.MaxDuration {
			break
		}
		workspace, err := s.workspaceForQRSPIProjection(ctx, row)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if markErr := s.markQRSPIProjectionSkipped(ctx, row.ID, "workspace not found for plan_dir"); markErr != nil {
					return result, markErr
				}
				result.Skipped++
				continue
			}
			return result, err
		}
		if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) != WorkspaceWorkflowQRSPI {
			if markErr := s.markQRSPIProjectionSkipped(ctx, row.ID, "workspace is not QRSPI"); markErr != nil {
				return result, markErr
			}
			result.Skipped++
			continue
		}
		threadID, err := s.ensureQRSPIProjectionThread(ctx, workspace, row)
		if err != nil {
			return result, err
		}
		if s.qrspiProjectionBeforeApplyForTest != nil {
			if err := s.qrspiProjectionBeforeApplyForTest(); err != nil {
				if isSQLiteBusyError(err) {
					return result, err
				}
				if markErr := s.markQRSPIProjectionFailed(ctx, row.ID, err.Error()); markErr != nil {
					return result, markErr
				}
				result.Failed++
				continue
			}
		}
		applied, err := s.ApplyQRSPIExternalResult(ctx, QRSPIExternalResultSource{
			WorkspaceID:    workspace.ID,
			ThreadID:       threadID,
			HeadEntryID:    projectionHeadEntryID(row),
			SessionID:      row.SessionID.String,
			SessionPath:    row.SessionArtifactPath.String,
			ResultJSON:     row.ResultJson,
			SourceEventID:  row.SourceEventID,
			WorkflowNodeID: projectionWorkflowNodeID(row),
			SelectThread:   false,
		})
		if err != nil {
			if isSQLiteBusyError(err) {
				return result, err
			}
			if markErr := s.markQRSPIProjectionFailed(ctx, row.ID, err.Error()); markErr != nil {
				return result, markErr
			}
			result.Failed++
			continue
		}
		if err := s.queries.MarkQRSPIProjectionApplied(ctx, row.ID); err != nil {
			return result, err
		}
		if applied {
			result.Applied++
		} else {
			result.Skipped++
		}
	}
	result.Changed = result.Applied > 0 || result.Skipped > 0 || result.Failed > 0
	return result, nil
}

func normalizeQRSPIProjectionApplyInput(input QRSPIProjectionApplyInput) QRSPIProjectionApplyInput {
	if input.MaxResults <= 0 {
		input.MaxResults = defaultQRSPIProjectionMaxResults
	}
	if input.MaxDuration <= 0 {
		input.MaxDuration = defaultQRSPIProjectionMaxDuration
	}
	return input
}

func (s *Service) workspaceForQRSPIProjection(
	ctx context.Context,
	row db.QrspiSessionProjection,
) (db.Workspace, error) {
	planDir := strings.TrimSpace(row.PlanDir)
	if planDir == "" {
		return db.Workspace{}, sql.ErrNoRows
	}
	if canonical, ok := s.canonicalPlanDirFromSource(planDir); ok {
		if workspace, err := s.queries.FindWorkspaceByRootDocPath(ctx, canonical); err == nil {
			return workspace, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return db.Workspace{}, err
		}
	}
	return s.queries.FindWorkspaceByRootDocPath(ctx, filepath.Clean(planDir))
}

func (s *Service) ApplyQRSPIExternalResult(
	ctx context.Context,
	source QRSPIExternalResultSource,
) (bool, error) {
	workspaceID := strings.TrimSpace(source.WorkspaceID)
	threadID := strings.TrimSpace(source.ThreadID)
	if workspaceID == "" || threadID == "" {
		return false, nil
	}
	workspace, err := s.queries.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) != WorkspaceWorkflowQRSPI {
		return false, nil
	}
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter == nil || adapter.Definitions == nil || adapter.Store == nil {
		return false, nil
	}
	state, err := loadQRSPIWorkspaceStateOrInitial(ctx, adapter, workspaceID)
	if err != nil {
		return false, err
	}
	currentDef, ok := adapter.Definitions.Get(wruntime.WorkflowID(strings.TrimSpace(state.Type)))
	if !ok {
		return false, nil
	}
	workflowResult, err := parseQRSPIExternalWorkflowResult(currentDef, state, source)
	if err != nil {
		return false, err
	}
	_, applied, err := adapter.ApplyExternalWorkflowResult(
		ctx,
		agentchatworkflows.ExternalWorkflowResultInput{
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			HeadEntryID: strings.TrimSpace(source.HeadEntryID),
			State:       state,
			Result:      workflowResult,
		},
	)
	if err != nil {
		return false, err
	}
	if applied && source.SelectThread {
		_ = s.queries.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspaceID,
			SelectedThreadID: nullString(threadID),
		})
	}
	return applied, nil
}

func parseQRSPIExternalWorkflowResult(
	def wruntime.Definition,
	state wruntime.State,
	source QRSPIExternalResultSource,
) (wruntime.WorkflowResult, error) {
	parseCtx := wruntime.ParseContext{
		WorkflowType:   strings.TrimSpace(state.Type),
		ExpectedNodeID: wruntime.NodeID(strings.TrimSpace(source.WorkflowNodeID)),
		ThreadID:       strings.TrimSpace(source.ThreadID),
		SessionID:      strings.TrimSpace(source.SessionID),
		HeadEntryID:    strings.TrimSpace(source.HeadEntryID),
		SessionPath:    strings.TrimSpace(source.SessionPath),
	}
	payload := strings.TrimSpace(source.ResultJSON)
	if payload == "" {
		return wruntime.WorkflowResult{}, errors.New("qrspi projection result_json is empty")
	}
	if embedded := embeddedQRSPIResultText(payload); strings.TrimSpace(embedded) != "" {
		payload = embedded
	}
	parsed, err := def.ResultParser.Parse(payload, parseCtx)
	if err == nil {
		return def.ResultConverter.ToWorkflowResult(parsed, parseCtx)
	}
	if direct, directErr := decodeQRSPIResultJSON(payload); directErr == nil {
		return def.ResultConverter.ToWorkflowResult(direct, parseCtx)
	}
	return wruntime.WorkflowResult{}, err
}

func embeddedQRSPIResultText(payload string) string {
	var meta TerminalMetadataQRSPI
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		return ""
	}
	for _, candidate := range []string{meta.ResultYAML, meta.RawResult} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	if len(meta.ResultJSON) > 0 && json.Valid(meta.ResultJSON) {
		return string(meta.ResultJSON)
	}
	return ""
}

func decodeQRSPIResultJSON(payload string) (qrspi.Result, error) {
	var envelope struct {
		Result qrspi.Result `json:"qrspi_result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && strings.TrimSpace(envelope.Result.Stage) != "" {
		return validateDirectQRSPIResult(envelope.Result)
	}
	var direct qrspi.Result
	if err := json.Unmarshal([]byte(payload), &direct); err != nil {
		return qrspi.Result{}, err
	}
	return validateDirectQRSPIResult(direct)
}

func validateDirectQRSPIResult(result qrspi.Result) (qrspi.Result, error) {
	result.Stage = strings.TrimSpace(result.Stage)
	result.Status = strings.TrimSpace(result.Status)
	result.Outcome = strings.TrimSpace(result.Outcome)
	result.Artifact = strings.TrimSpace(result.Artifact)
	if result.Stage == "" {
		return qrspi.Result{}, errors.New("qrspi result stage is required")
	}
	if result.Status == "" {
		return qrspi.Result{}, errors.New("qrspi result status is required")
	}
	if result.Status == string(wruntime.StatusComplete) && result.Outcome == "" {
		return qrspi.Result{}, errors.New("qrspi result outcome is required when status is complete")
	}
	if result.Summary.TextContent() == "" {
		return qrspi.Result{}, errors.New("qrspi result summary is required")
	}
	return result, nil
}

func (s *Service) ensureQRSPIProjectionThread(
	ctx context.Context,
	workspace db.Workspace,
	row db.QrspiSessionProjection,
) (string, error) {
	threadID := metadataProjectionThreadID(row.SourceEventID)
	if _, err := s.queries.GetAgentThread(ctx, threadID); err == nil {
		return threadID, s.queries.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{
			ID:          threadID,
			WorkspaceID: nullString(workspace.ID),
		})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	cwd := strings.TrimSpace(workspace.RootDocPath)
	if workspace.Cwd.Valid && strings.TrimSpace(workspace.Cwd.String) != "" {
		cwd = strings.TrimSpace(workspace.Cwd.String)
	}
	_, err := s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                threadID,
		UserEmail:         strings.TrimSpace(workspace.UserEmail),
		Title:             "Pi metadata result",
		Cwd:               cwd,
		LineageID:         metadataProjectionLineageID(row.SourceEventID),
		ProjectID:         "",
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil && !isUniqueConstraintError(err) {
		return "", err
	}
	if err := s.queries.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{
		ID:          threadID,
		WorkspaceID: nullString(workspace.ID),
	}); err != nil {
		return "", err
	}
	return threadID, nil
}

func metadataProjectionThreadID(sourceEventID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sourceEventID)))
	return "pi-metadata-" + hex.EncodeToString(sum[:])[:32]
}

func metadataProjectionLineageID(sourceEventID string) string {
	return "pi-metadata-lineage-" + strings.TrimPrefix(metadataProjectionThreadID(sourceEventID), "pi-metadata-")
}

func projectionHeadEntryID(row db.QrspiSessionProjection) string {
	if strings.TrimSpace(row.SourceEventID) != "" {
		return "pi-metadata:" + strings.TrimSpace(row.SourceEventID)
	}
	return strings.TrimSpace(row.ID)
}

func projectionWorkflowNodeID(row db.QrspiSessionProjection) string {
	if row.WorkflowNodeID.Valid && strings.TrimSpace(row.WorkflowNodeID.String) != "" {
		return row.WorkflowNodeID.String
	}
	if row.Stage.Valid {
		return row.Stage.String
	}
	return ""
}

func (s *Service) markQRSPIProjectionFailed(ctx context.Context, id, message string) error {
	return s.queries.MarkQRSPIProjectionFailed(ctx, db.MarkQRSPIProjectionFailedParams{
		ID:        id,
		LastError: nullString(message),
	})
}

func (s *Service) markQRSPIProjectionSkipped(ctx context.Context, id, message string) error {
	return s.queries.MarkQRSPIProjectionSkipped(ctx, db.MarkQRSPIProjectionSkippedParams{
		ID:        id,
		LastError: nullString(message),
	})
}

func loadQRSPIWorkspaceStateOrInitial(
	ctx context.Context,
	adapter *agentchatworkflows.Service,
	workspaceID string,
) (wruntime.State, error) {
	state, err := adapter.Store.LoadWorkspaceState(ctx, workspaceID)
	if err == nil {
		return state, nil
	}
	def, ok := adapter.Definitions.Get(qrspi.AgentChatWorkflowType)
	if !ok {
		return wruntime.State{}, errors.New("qrspi workflow definition is not registered")
	}
	policy, policyErr := json.Marshal(qrspi.DefaultPolicy())
	if policyErr != nil {
		return wruntime.State{}, policyErr
	}
	return wruntime.InitialState(def, policy)
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "database is locked")
}
