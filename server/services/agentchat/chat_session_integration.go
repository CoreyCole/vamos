package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

func ensureWorkspaceChatSessionTx(
	ctx context.Context,
	q *db.Queries,
	workspace db.Workspace,
	actorEmail string,
	workflowID string,
	workflowNodeID string,
	workflowAttempt int,
) (db.ChatSession, error) {
	if workspace.CurrentSessionID.Valid &&
		strings.TrimSpace(workspace.CurrentSessionID.String) != "" {
		session, err := q.GetChatSession(ctx, workspace.CurrentSessionID.String)
		if err == nil {
			return session, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return db.ChatSession{}, err
		}
	}
	session, err := chatsession.CreateSessionWithQueries(
		ctx,
		q,
		chatsession.CreateSessionInput{
			WorkspaceID:     workspace.ID,
			ActorEmail:      firstNonEmptyString(actorEmail, workspace.UserEmail),
			WorkflowID:      workflowID,
			WorkflowNodeID:  workflowNodeID,
			WorkflowAttempt: workflowAttempt,
			TopologyKind:    chatsession.TopologyRoot,
		},
	)
	if err != nil {
		return db.ChatSession{}, err
	}
	if err := q.UpdateWorkspaceCurrentSession(ctx, db.UpdateWorkspaceCurrentSessionParams{
		ID:               workspace.ID,
		CurrentSessionID: sql.NullString{String: session.ID, Valid: true},
		CurrentBranchID: sql.NullString{
			String: session.BranchID,
			Valid:  strings.TrimSpace(session.BranchID) != "",
		},
	}); err != nil {
		return db.ChatSession{}, err
	}
	return session, nil
}

func appendChatSessionEventTx(
	ctx context.Context,
	q *db.Queries,
	input chatsession.AppendEventInput,
) (chatsession.ChatEvent, error) {
	return chatsession.AppendEventWithQueries(ctx, q, input)
}

func appendSemanticEventTx(
	ctx context.Context,
	q *db.Queries,
	input chatsession.AppendEventInput,
) (chatsession.ChatEvent, error) {
	return appendChatSessionEventTx(ctx, q, input)
}

func appendPromptAndRunStartedSessionEventsTx(
	ctx context.Context,
	q *db.Queries,
	chatSession db.ChatSession,
	actorEmail string,
	thread db.AgentThread,
	run db.AgentRun,
	prompt string,
) error {
	messageID := fmt.Sprintf("%s:prompt", run.ID)
	if _, err := appendChatSessionEventTx(ctx, q, chatsession.AppendEventInput{
		SessionID:          chatSession.ID,
		EventType:          chatsession.EventMessageCompleted,
		ActorParticipantID: actorEmail,
		RunID:              run.ID,
		PayloadJSON: marshalSessionPayload(map[string]any{
			"id":        messageID,
			"role":      "user",
			"content":   prompt,
			"actor":     actorEmail,
			"thread_id": thread.ID,
			"run_id":    run.ID,
		}),
	}); err != nil {
		return err
	}
	_, err := appendChatSessionEventTx(ctx, q, chatsession.AppendEventInput{
		SessionID: chatSession.ID,
		EventType: chatsession.EventRunStarted,
		RunID:     run.ID,
		PayloadJSON: marshalSessionPayload(map[string]any{
			"id":                run.ID,
			"run_id":            run.ID,
			"thread_id":         thread.ID,
			"legacy_session_id": runSessionID(run),
			"trigger":           run.Trigger,
			"workflow_id":       run.WorkflowID,
			"workflow_node_id":  run.WorkflowNodeID.String,
			"workflow_attempt":  run.WorkflowAttempt,
		}),
	})
	return err
}

func chatSessionEventSeqForNode(
	ctx context.Context,
	q *db.Queries,
	sessionID string,
	nodeID string,
) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	nodeID = strings.TrimSpace(nodeID)
	events, err := q.ListChatSessionEventsAfter(ctx, db.ListChatSessionEventsAfterParams{
		SessionID: sessionID,
		AfterSeq:  0,
		Limit:     100000,
	})
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, event := range events {
		if event.Seq > latest {
			latest = event.Seq
		}
		if nodeID == "" || strings.TrimSpace(event.PayloadJson) == "" {
			continue
		}
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJson), &payload); err == nil &&
			strings.TrimSpace(payload.ID) == nodeID {
			return event.Seq, nil
		}
	}
	return latest, nil
}

func (s *Service) chatSessionIDForRun(ctx context.Context, run db.AgentRun) string {
	if !run.WorkspaceID.Valid {
		return ""
	}
	workspaceID := strings.TrimSpace(run.WorkspaceID.String)
	if workspaceID == "" {
		return ""
	}
	if current, err := s.queries.GetWorkspace(
		ctx,
		workspaceID,
	); err == nil &&
		current.CurrentSessionID.Valid {
		if sessionID := strings.TrimSpace(
			current.CurrentSessionID.String,
		); sessionID != "" {
			if events, err := s.queries.ListChatSessionEventsAfter(
				ctx,
				db.ListChatSessionEventsAfterParams{
					SessionID: sessionID,
					AfterSeq:  0,
					Limit:     100000,
				},
			); err == nil {
				for _, event := range events {
					if event.RunID.Valid && event.RunID.String == run.ID {
						return sessionID
					}
				}
			}
		}
	}
	sessions, err := s.queries.ListChatSessionsByWorkspace(ctx, workspaceID)
	if err != nil {
		return ""
	}
	for _, session := range sessions {
		events, err := s.queries.ListChatSessionEventsAfter(
			ctx,
			db.ListChatSessionEventsAfterParams{
				SessionID: session.ID,
				AfterSeq:  0,
				Limit:     100000,
			},
		)
		if err != nil {
			continue
		}
		for _, event := range events {
			if event.RunID.Valid && event.RunID.String == run.ID {
				return session.ID
			}
		}
	}
	return ""
}

func SemanticEventForLiveEnvelope(
	sessionID string,
	env conversation.EventEnvelope,
) (chatsession.AppendEventInput, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return chatsession.AppendEventInput{}, false
	}
	switch env.EventType {
	case "message_start", "message_update", "message_end":
		eventType := chatsession.EventMessageCheckpointed
		if env.EventType == "message_start" {
			eventType = chatsession.EventMessageStarted
		} else if env.EventType == "message_end" {
			eventType = chatsession.EventMessageCompleted
		}
		payload, ok := liveMessageSessionPayload(env)
		if !ok {
			return chatsession.AppendEventInput{}, false
		}
		return chatsession.AppendEventInput{
			SessionID:   sessionID,
			EventType:   eventType,
			RunID:       env.RunID,
			PayloadJSON: payload,
		}, true
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		eventType := chatsession.EventToolUpdated
		if env.EventType == "tool_execution_start" {
			eventType = chatsession.EventToolStarted
		} else if env.EventType == "tool_execution_end" {
			eventType = chatsession.EventToolCompleted
		}
		payload, failed, ok := liveToolSessionPayload(env)
		if !ok {
			return chatsession.AppendEventInput{}, false
		}
		if failed {
			eventType = chatsession.EventToolFailed
		}
		return chatsession.AppendEventInput{
			SessionID:   sessionID,
			EventType:   eventType,
			RunID:       env.RunID,
			PayloadJSON: payload,
		}, true
	default:
		return chatsession.AppendEventInput{}, false
	}
}

func liveMessageSessionPayload(env conversation.EventEnvelope) (json.RawMessage, bool) {
	var envelope struct {
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal([]byte(env.PayloadJSON), &envelope); err != nil || len(envelope.Message) == 0 {
		return nil, false
	}
	var message struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	if err := json.Unmarshal(envelope.Message, &message); err != nil {
		return nil, false
	}
	role := strings.TrimSpace(message.Role)
	content := liveContentText(message.Content)
	if role == "" && strings.TrimSpace(content) == "" {
		return nil, false
	}
	return marshalSessionPayload(map[string]any{
		"id":         firstNonEmptyString(env.EventKey, env.RunID+":live-message"),
		"role":       role,
		"content":    content,
		"run_id":     env.RunID,
		"thread_id":  env.ThreadID,
		"session_id": env.SessionID,
		"raw":        jsonRawObject(envelope.Message),
	}), true
}

func liveToolSessionPayload(env conversation.EventEnvelope) (json.RawMessage, bool, bool) {
	var payload struct {
		ToolCallID    string          `json:"toolCallId"`
		ToolName      string          `json:"toolName"`
		Args          json.RawMessage `json:"args"`
		PartialResult json.RawMessage `json:"partialResult"`
		Result        json.RawMessage `json:"result"`
		IsError       bool            `json:"isError"`
	}
	if err := json.Unmarshal([]byte(env.PayloadJSON), &payload); err != nil {
		return nil, false, false
	}
	toolCallID := strings.TrimSpace(payload.ToolCallID)
	toolName := strings.TrimSpace(payload.ToolName)
	if toolCallID == "" && toolName == "" {
		return nil, false, false
	}
	summary := toolName
	if payload.IsError {
		summary = firstNonEmptyString(toolName, "tool") + " failed"
	}
	return marshalSessionPayload(map[string]any{
		"id":           firstNonEmptyString(toolCallID, env.EventKey, env.RunID+":tool"),
		"tool_call_id": toolCallID,
		"tool_name":    toolName,
		"run_id":       env.RunID,
		"thread_id":    env.ThreadID,
		"session_id":   env.SessionID,
		"summary":      summary,
		"args":         jsonRawObject(payload.Args),
		"partial":      jsonRawObject(payload.PartialResult),
		"result":       jsonRawObject(payload.Result),
	}), payload.IsError, true
}

func liveContentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, part := range v {
			if text := liveContentText(part); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if text, ok := v["text"]; ok {
			return liveContentText(text)
		}
		if thinking, ok := v["thinking"]; ok {
			return liveContentText(thinking)
		}
		if content, ok := v["content"]; ok {
			return liveContentText(content)
		}
	}
	return ""
}

func jsonRawObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func (s *Service) recordTemporalWorkerSurface(
	ctx context.Context,
	run db.AgentRun,
	chatSessionID string,
) error {
	chatSessionID = strings.TrimSpace(chatSessionID)
	if chatSessionID == "" {
		return nil
	}
	_, err := s.queries.CreateAgentSurfaceAttachment(
		ctx,
		db.CreateAgentSurfaceAttachmentParams{
			ID:            uuid.NewString(),
			ChatSessionID: chatSessionID,
			RunID:         sql.NullString{String: run.ID, Valid: true},
			SurfaceKind:   "temporal_worker",
			SurfaceID: sql.NullString{
				String: run.WorkflowID,
				Valid:  strings.TrimSpace(run.WorkflowID) != "",
			},
			UserEmail:       sql.NullString{},
			PermissionMode:  string(chatsession.PermissionOwn),
			LastHeartbeatAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		},
	)
	if isUniqueConstraintError(err) {
		return nil
	}
	return err
}

func (s *Service) StartWorkspaceSessionCommand(
	ctx context.Context,
	input chatsession.SubmitCommandInput,
) (*db.AgentRun, error) {
	outcome, err := s.chatSessions.SubmitCommand(ctx, input)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Prompt   string `json:"prompt"`
		ThreadID string `json:"thread_id"`
	}
	_ = json.Unmarshal(input.PayloadJSON, &payload)
	prompt := strings.TrimSpace(payload.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf(
			"message.send command %s missing prompt",
			outcome.CommandID,
		)
	}
	var run *db.AgentRun
	if strings.TrimSpace(payload.ThreadID) == "" {
		_, run, _, err = s.StartWorkspaceThread(
			ctx,
			input.WorkspaceID,
			input.ActorEmail,
			prompt,
		)
	} else {
		_, run, _, err = s.ResumeWorkspaceThread(
			ctx,
			input.WorkspaceID,
			input.ActorEmail,
			payload.ThreadID,
			prompt,
		)
	}
	if err != nil {
		_, _ = s.chatSessions.FailCommand(
			ctx,
			outcome.CommandID,
			marshalSessionPayload(map[string]any{"error": err.Error()}),
		)
		return nil, err
	}
	_, _ = s.chatSessions.ApplyCommand(
		ctx,
		outcome.CommandID,
		marshalSessionPayload(map[string]any{"status": "applied", "run_id": run.ID}),
	)
	return run, nil
}

func SemanticEventForCheckpoint(
	cp conversation.Checkpoint,
) ([]chatsession.AppendEventInput, error) {
	chatSessionID := strings.TrimSpace(cp.ChatSessionID)
	if chatSessionID == "" {
		return nil, nil
	}
	events := make([]chatsession.AppendEventInput, 0, len(cp.NewEntries))
	for _, entry := range cp.NewEntries {
		role, content, ok := semanticMessageFromSnapshotEntry(entry)
		if !ok || role != "assistant" {
			continue
		}
		messageID := strings.TrimSpace(entry.EntryID)
		if messageID == "" {
			messageID = fmt.Sprintf("%s:%d", cp.RunID, entry.OriginOrder)
		}
		events = append(events, chatsession.AppendEventInput{
			SessionID: chatSessionID,
			EventType: chatsession.EventMessageCheckpointed,
			RunID:     cp.RunID,
			PayloadJSON: marshalSessionPayload(map[string]any{
				"id":      messageID,
				"role":    role,
				"content": content,
				"run_id":  cp.RunID,
			}),
		})
	}
	return events, nil
}

func SemanticEventForRunResult(
	result conversation.RunResult,
) chatsession.AppendEventInput {
	return chatsession.AppendEventInput{
		SessionID: result.ChatSessionID,
		EventType: chatsession.EventRunCompleted,
		RunID:     result.RunID,
		PayloadJSON: marshalSessionPayload(map[string]any{
			"id":            result.RunID,
			"run_id":        result.RunID,
			"head_entry_id": result.HeadEntryID,
			"session_path":  result.SessionPath,
			"root_doc_path": result.RootDocPath,
			"summary":       "Run completed",
		}),
	}
}

func SemanticEventForRunFailure(
	failure conversation.RunFailure,
) chatsession.AppendEventInput {
	return chatsession.AppendEventInput{
		SessionID: failure.ChatSessionID,
		EventType: chatsession.EventRunFailed,
		RunID:     failure.RunID,
		PayloadJSON: marshalSessionPayload(map[string]any{
			"id":            failure.RunID,
			"run_id":        failure.RunID,
			"head_entry_id": failure.HeadEntryID,
			"session_path":  failure.SessionPath,
			"root_doc_path": failure.RootDocPath,
			"error":         failure.ErrorMessage,
		}),
	}
}

func semanticMessageFromSnapshotEntry(
	entry conversation.SnapshotEntry,
) (string, string, bool) {
	var payload struct {
		Message json.RawMessage `json:"message"`
		Role    string          `json:"role"`
		Content any             `json:"content"`
	}
	if err := json.Unmarshal([]byte(entry.PayloadJSON), &payload); err != nil {
		return "", "", false
	}
	if len(payload.Message) > 0 {
		var nested struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		}
		if err := json.Unmarshal(payload.Message, &nested); err == nil {
			payload.Role = nested.Role
			payload.Content = nested.Content
		}
	}
	role := strings.TrimSpace(payload.Role)
	content := contentString(payload.Content)
	return role, content, role != "" && strings.TrimSpace(content) != ""
}

func contentString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, part := range v {
			if s := contentString(part); strings.TrimSpace(s) != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if text, ok := v["text"]; ok {
			return contentString(text)
		}
		if content, ok := v["content"]; ok {
			return contentString(content)
		}
	}
	return ""
}

func marshalSessionPayload(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
