package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type DBStore struct {
	Queries *db.Queries
}

func NewDBStore(queries *db.Queries) *DBStore {
	return &DBStore{Queries: queries}
}

func (s *DBStore) LoadWorkspaceState(
	ctx context.Context,
	workspaceID string,
) (wruntime.State, error) {
	workspace, err := s.Queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return wruntime.State{}, err
	}
	if !workspace.WorkflowStateJson.Valid ||
		strings.TrimSpace(workspace.WorkflowStateJson.String) == "" {
		return wruntime.State{}, fmt.Errorf(
			"workspace %q has no workflow state",
			workspace.ID,
		)
	}
	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(workspace.WorkflowStateJson.String),
		&state,
	); err != nil {
		return wruntime.State{}, fmt.Errorf("parse workspace workflow state: %w", err)
	}
	return state, nil
}

func (s *DBStore) SaveWorkspaceState(
	ctx context.Context,
	workspaceID string,
	state wruntime.State,
) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal workspace workflow state: %w", err)
	}
	return s.Queries.UpdateWorkspaceWorkflowState(
		ctx,
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                strings.TrimSpace(workspaceID),
			WorkflowType:      state.Type,
			WorkflowStateJson: nullString(string(raw)),
		},
	)
}

func (s *DBStore) LoadRun(ctx context.Context, runID string) (db.AgentRun, error) {
	return s.Queries.GetAgentRun(ctx, strings.TrimSpace(runID))
}

func (s *DBStore) SaveRunResult(
	ctx context.Context,
	runID string,
	result wruntime.WorkflowResult,
) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal workflow result: %w", err)
	}
	return s.Queries.UpdateAgentRunWorkflowResult(
		ctx,
		db.UpdateAgentRunWorkflowResultParams{
			ID:                   strings.TrimSpace(runID),
			WorkflowResultStatus: nullString(string(result.Status)),
			WorkflowResultJson:   nullString(string(raw)),
		},
	)
}

func (s *DBStore) AppendWorkflowEvents(
	ctx context.Context,
	workspaceID string,
	run db.AgentRun,
	events []wruntime.Event,
) error {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal workflow event: %w", err)
		}
		_, err = s.Queries.CreateWorkspaceEvent(ctx, db.CreateWorkspaceEventParams{
			WorkspaceID:  strings.TrimSpace(workspaceID),
			EventType:    strings.TrimSpace(event.Type),
			ActorEmail:   sql.NullString{},
			ActorType:    "system",
			ThreadID:     nullString(run.ThreadID),
			SessionID:    run.SessionID,
			RunID:        nullString(run.ID),
			DocPath: sql.NullString{},
			CommentID:    sql.NullString{},
			PayloadJson:  nullString(string(payload)),
			EventKey: nullString(
				run.ID + ":" + event.Type + ":" + string(event.NodeID),
			),
		})
		if err != nil && !isUniqueConstraintError(err) {
			return err
		}
	}
	return nil
}

func (s *DBStore) ArtifactExists(
	ctx context.Context,
	workspaceID, relPath string,
) (bool, error) {
	workspace, err := s.Queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return false, err
	}
	documentPath, err := documentPathFromRoot(workspace.RootDocPath, relPath)
	if err != nil {
		return false, err
	}
	artifact, err := s.Queries.GetWorkspaceDoc(ctx, db.GetWorkspaceDocParams{
		WorkspaceID:  strings.TrimSpace(workspaceID),
		DocPath: documentPath,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return !artifact.DeletedAt.Valid, nil
}

func (s *DBStore) WorkspacePlanningCwd(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	workspace, err := s.Queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", err
	}
	if workspace.SelectedThreadID.Valid &&
		strings.TrimSpace(workspace.SelectedThreadID.String) != "" {
		thread, err := s.Queries.GetAgentThread(
			ctx,
			strings.TrimSpace(workspace.SelectedThreadID.String),
		)
		if err == nil && strings.TrimSpace(thread.Cwd) != "" {
			return strings.TrimSpace(thread.Cwd), nil
		}
	}
	if workspace.Cwd.Valid {
		return strings.TrimSpace(workspace.Cwd.String), nil
	}
	return "", nil
}

func (s *DBStore) FinalAssistantText(
	ctx context.Context,
	threadID, headEntryID string,
) (string, error) {
	thread, err := s.Queries.GetAgentThread(ctx, strings.TrimSpace(threadID))
	if err != nil {
		return "", err
	}
	headEntryID = strings.TrimSpace(headEntryID)
	if headEntryID == "" && thread.HeadEntryID.Valid {
		headEntryID = thread.HeadEntryID.String
	}
	if headEntryID == "" {
		return "", fmt.Errorf("head entry id is required")
	}
	rows, err := s.Queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: headEntryID,
	})
	if err != nil {
		return "", err
	}
	for i := len(rows) - 1; i >= 0; i-- {
		text := assistantText(rows[i].PayloadJson)
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
	}
	return "", fmt.Errorf("no assistant message found for head entry %q", headEntryID)
}

func assistantText(payload string) string {
	var envelope struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return ""
	}
	if strings.TrimSpace(envelope.Type) != "message" ||
		strings.TrimSpace(envelope.Message.Role) != "assistant" {
		return ""
	}
	return extractContentText(envelope.Message.Content)
}

func extractContentText(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(extractContentText(item)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := value["text"].(string); ok {
			return text
		}
		if contentText, ok := value["content"].(string); ok {
			return contentText
		}
		if nested, ok := value["content"].([]any); ok {
			return extractContentText(nested)
		}
		return ""
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
