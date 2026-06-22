package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) AdvanceQRSPIWorkflow(ctx context.Context, userEmail, sourceThreadID string) (db.AgentThread, error) {
	userEmail = strings.TrimSpace(userEmail)
	sourceThreadID = strings.TrimSpace(sourceThreadID)
	if userEmail == "" || sourceThreadID == "" {
		return db.AgentThread{}, fmt.Errorf("user email and source thread id are required")
	}
	sourceThread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{ID: sourceThreadID, UserEmail: userEmail})
	if err != nil {
		return db.AgentThread{}, err
	}
	primary, ok, err := s.ResolvePrimaryWorkspaceForThread(ctx, userEmail, sourceThread.ID)
	if err != nil {
		return db.AgentThread{}, err
	}
	if !ok {
		return db.AgentThread{}, fmt.Errorf("primary workspace not found")
	}
	if _, err := s.AdvanceWorkflowHumanGate(ctx, primary.ID, userEmail); err != nil {
		return db.AgentThread{}, err
	}
	return sourceThread, nil
}

type CreateThreadFromWorkspaceInput struct {
	UserEmail         string
	SourceThreadID    string
	TargetWorkspaceID string
	TargetKind        NewThreadTargetKind
	ContextXML        string
}

func (s *Service) CreateThreadFromWorkspace(ctx context.Context, input CreateThreadFromWorkspaceInput) (db.AgentThread, error) {
	userEmail := strings.TrimSpace(input.UserEmail)
	sourceThreadID := strings.TrimSpace(input.SourceThreadID)
	if userEmail == "" || sourceThreadID == "" {
		return db.AgentThread{}, fmt.Errorf("user email and source thread id are required")
	}
	sourceThread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{ID: sourceThreadID, UserEmail: userEmail})
	if err != nil {
		return db.AgentThread{}, err
	}
	kind := input.TargetKind
	if kind == "" {
		kind = NewThreadTargetPrimary
	}
	var selectedWorkspace db.Workspace
	if kind != NewThreadTargetFreeform {
		workspace, err := s.resolveThreadNewTargetWorkspace(ctx, userEmail, sourceThread.ID, strings.TrimSpace(input.TargetWorkspaceID), kind)
		if err != nil {
			return db.AgentThread{}, err
		}
		selectedWorkspace = workspace
	}
	cwd := sourceThread.Cwd
	if kind != NewThreadTargetFreeform {
		cwd = firstNonEmpty(selectedWorkspace.Cwd.String, selectedWorkspace.RootDocPath, sourceThread.Cwd)
	}
	title := truncateTitle("New thread from " + sourceThread.Title)
	if kind == NewThreadTargetFreeform {
		title = truncateTitle("Freeform from " + sourceThread.Title)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.AgentThread{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	thread := db.AgentThread{}
	entryID := ""
	if kind != NewThreadTargetFreeform {
		entryID = uuid.NewString()
	}
	thread, err = q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         userEmail,
		Title:             title,
		Cwd:               cwd,
		LineageID:         uuid.NewString(),
		HeadEntryID:       sql.NullString{String: entryID, Valid: entryID != ""},
		ParentThreadID:    sql.NullString{String: sourceThread.ID, Valid: true},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		return db.AgentThread{}, err
	}
	if kind != NewThreadTargetFreeform {
		contextText := firstNonEmpty(strings.TrimSpace(input.ContextXML), qrspiContextThreadSwitch(sourceThread.ID, selectedWorkspace.ID))
		payloadBytes, err := json.Marshal(map[string]any{
			"type":      "message",
			"id":        uuid.NewString(),
			"parentId":  nil,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"message": map[string]any{
				"role":    "assistant",
				"content": contextText,
			},
		})
		if err != nil {
			return db.AgentThread{}, err
		}
		if err := q.CreateAgentEntry(ctx, db.CreateAgentEntryParams{
			LineageID:        thread.LineageID,
			EntryID:          entryID,
			ParentEntryID:    sql.NullString{},
			EntryType:        "message",
			OriginOrder:      0,
			PayloadJson:      string(payloadBytes),
			OriginThreadID:   thread.ID,
			OriginRunID:      sql.NullString{},
			OriginSessionID:  sql.NullString{},
			SessionTimestamp: time.Now().UTC(),
		}); err != nil {
			return db.AgentThread{}, err
		}
		if err := q.UpsertThreadWorkspaceAssociation(ctx, db.UpsertThreadWorkspaceAssociationParams{
			ThreadID:    thread.ID,
			WorkspaceID: selectedWorkspace.ID,
			IsPrimary:   1,
			Role:        string(ThreadWorkspaceRolePrimary),
			AdoptedFrom: "thread_new_target",
		}); err != nil {
			return db.AgentThread{}, err
		}
		if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{ID: selectedWorkspace.ID, SelectedThreadID: nullString(thread.ID)}); err != nil {
			return db.AgentThread{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return db.AgentThread{}, err
	}
	return thread, nil
}

func (s *Service) resolveThreadNewTargetWorkspace(ctx context.Context, userEmail, sourceThreadID, workspaceID string, kind NewThreadTargetKind) (db.Workspace, error) {
	if kind == NewThreadTargetPrimary && workspaceID == "" {
		workspace, ok, err := s.ResolvePrimaryWorkspaceForThread(ctx, userEmail, sourceThreadID)
		if err != nil {
			return db.Workspace{}, err
		}
		if !ok {
			return db.Workspace{}, fmt.Errorf("primary workspace not found")
		}
		return workspace, nil
	}
	if workspaceID == "" {
		return db.Workspace{}, fmt.Errorf("workspace id is required")
	}
	context, err := s.GetThreadWorkspaceContext(ctx, userEmail, sourceThreadID)
	if err != nil {
		return db.Workspace{}, err
	}
	if context.Primary != nil && context.Primary.ID == workspaceID {
		return *context.Primary, nil
	}
	for _, workspace := range context.Related {
		if workspace.ID == workspaceID {
			return workspace, nil
		}
	}
	return db.Workspace{}, fmt.Errorf("workspace is not associated with source thread")
}

func qrspiContextThreadSwitch(sourceThreadID, workspaceID string) string {
	return strings.TrimSpace(fmt.Sprintf(`<qrspi-context>
  <type>workspace-switch</type>
  <sourceThread>%s</sourceThread>
  <workspace>%s</workspace>
  <instruction>Continue from this workspace state; do not treat this entry as workflow completion.</instruction>
</qrspi-context>`, sourceThreadID, workspaceID))
}

func (s *Service) latestAssistantTextForThread(ctx context.Context, thread db.AgentThread) (string, error) {
	if !thread.HeadEntryID.Valid || strings.TrimSpace(thread.HeadEntryID.String) == "" {
		return "", fmt.Errorf("thread has no head entry")
	}
	entries, err := s.queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{LineageID: thread.LineageID, HeadEntryID: thread.HeadEntryID.String})
	if err != nil {
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if text := assistantTextFromPayload(entries[i].PayloadJson); strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
	}
	return "", fmt.Errorf("thread has no assistant message")
}

func extractFirstQRSPIResultYAML(text string) string {
	yamlText, err := qrspi.ExtractQRSPIResultYAML(text)
	if err != nil {
		return ""
	}
	return yamlText
}

func qrspiImplementationWorkspaceFromText(resultText string) string {
	parsed, err := (qrspi.QRSPIResultParser{}).Parse(resultText, wruntime.ParseContext{})
	if err != nil {
		return ""
	}
	result, ok := parsed.(qrspi.Result)
	if !ok {
		return ""
	}
	path := qrspi.QRSPIResultWorkspaceMetadata(result).ImplementationWorkspace
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}

func nextQRSPIThreadContext(sourceThreadID, workspaceID, cwd, resultYAML string) string {
	return strings.TrimSpace(fmt.Sprintf(`Continuing QRSPI from previous thread %s.
Primary workspace: %s
Pi cwd: %s

Prior result YAML:
%s`, sourceThreadID, workspaceID, cwd, resultYAML))
}
