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

func (s *Service) CreateNextQRSPIThread(ctx context.Context, userEmail, sourceThreadID string) (db.AgentThread, error) {
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
	assistantText, err := s.latestAssistantTextForThread(ctx, sourceThread)
	if err != nil {
		return db.AgentThread{}, err
	}
	resultXML := extractFirstQRSPIResultXML(assistantText)
	if resultXML == "" {
		return db.AgentThread{}, fmt.Errorf("latest assistant message has no qrspi result")
	}
	cwd := firstNonEmpty(qrspiImplementationWorkspaceFromText(resultXML), sourceThread.Cwd, primary.Cwd.String, primary.RootDocPath)
	contextText := nextQRSPIThreadContext(sourceThread.ID, primary.ID, cwd, resultXML)
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
	entryID := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.AgentThread{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         userEmail,
		Title:             truncateTitle("Continue " + sourceThread.Title),
		Cwd:               cwd,
		LineageID:         uuid.NewString(),
		HeadEntryID:       sql.NullString{String: entryID, Valid: true},
		ParentThreadID:    sql.NullString{String: sourceThread.ID, Valid: true},
		ForkedFromEntryID: sql.NullString{},
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
		WorkspaceID: primary.ID,
		IsPrimary:   1,
		Role:        string(ThreadWorkspaceRolePrimary),
		AdoptedFrom: "qrspi_next_thread",
	}); err != nil {
		return db.AgentThread{}, err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{ID: primary.ID, SelectedThreadID: nullString(thread.ID)}); err != nil {
		return db.AgentThread{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.AgentThread{}, err
	}
	return thread, nil
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

func extractFirstQRSPIResultXML(text string) string {
	start := strings.Index(text, "<qrspi-result>")
	end := strings.Index(text, "</qrspi-result>")
	if start < 0 || end < start {
		return ""
	}
	end += len("</qrspi-result>")
	return strings.TrimSpace(text[start:end])
}

func qrspiImplementationWorkspaceFromText(xmlText string) string {
	parsed, err := (qrspi.QRSPIXMLParser{}).Parse(xmlText, wruntime.ParseContext{})
	if err != nil {
		return ""
	}
	result, ok := parsed.(qrspi.ResultXML)
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

func nextQRSPIThreadContext(sourceThreadID, workspaceID, cwd, resultXML string) string {
	return strings.TrimSpace(fmt.Sprintf(`Continuing QRSPI from previous thread %s.
Primary workspace: %s
Pi cwd: %s

Prior result XML:
%s`, sourceThreadID, workspaceID, cwd, resultXML))
}
