package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	conversationworkflow "github.com/CoreyCole/vamos/pkg/agents/workflows/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

type EnqueueThreadMailInput struct {
	ThreadID       string         `json:"thread_id"`
	OpID           string         `json:"op_id"`
	SpeakerAgentID string         `json:"speaker_agent_id"`
	FromKind       string         `json:"from_kind"`
	FromAgentID    string         `json:"from_agent_id"`
	FromUserEmail  string         `json:"from_user_email"`
	Body           string         `json:"body"`
	Attachments    []AttachedPath `json:"attachments,omitempty"`
}

type EnqueueThreadMailResult struct {
	Duplicate  bool
	WorkflowID string
}

func (s *Service) EnqueueThreadMail(
	ctx context.Context,
	in EnqueueThreadMailInput,
) (EnqueueThreadMailResult, error) {
	if s.temporal == nil {
		return EnqueueThreadMailResult{}, fmt.Errorf("temporal not configured")
	}
	threadID := strings.TrimSpace(in.ThreadID)
	opID := strings.TrimSpace(in.OpID)
	body := strings.TrimSpace(in.Body)
	fromKind := strings.TrimSpace(in.FromKind)
	if threadID == "" {
		return EnqueueThreadMailResult{}, fmt.Errorf("thread_id is required")
	}
	if opID == "" {
		return EnqueueThreadMailResult{}, fmt.Errorf("op_id is required")
	}
	if body == "" {
		return EnqueueThreadMailResult{}, fmt.Errorf("body is required")
	}
	if fromKind == "" {
		fromKind = EnqueueFromUser
	}

	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return EnqueueThreadMailResult{}, err
	}
	if err := GuardEnqueueDestination(thread, EnqueueMail{
		FromKind:    fromKind,
		FromAgentID: in.FromAgentID,
	}); err != nil {
		return EnqueueThreadMailResult{}, err
	}

	speakerID := strings.TrimSpace(in.SpeakerAgentID)
	if speakerID == "" {
		speakerID = speakerAgentIDForMail(thread, fromKind, in.FromAgentID)
	}

	rows, err := s.queries.InsertAgentThreadOp(ctx, db.InsertAgentThreadOpParams{
		ThreadID: threadID,
		OpID:     opID,
		SpeakerAgentSlug: sql.NullString{
			String: speakerID,
			Valid:  speakerID != "",
		},
		FromKind: fromKind,
		FromAgentSlug: sql.NullString{
			String: strings.TrimSpace(in.FromAgentID),
			Valid:  strings.TrimSpace(in.FromAgentID) != "",
		},
		FromUserEmail: strings.TrimSpace(in.FromUserEmail),
		Body:          body,
	})
	if err != nil {
		return EnqueueThreadMailResult{}, err
	}
	workflowID := conversation.ThreadWorkflowID(threadID)
	if rows == 0 {
		return EnqueueThreadMailResult{Duplicate: true, WorkflowID: workflowID}, nil
	}

	mail := conversation.ThreadMail{
		ThreadID:       threadID,
		OpID:           opID,
		SpeakerAgentID: speakerID,
		FromKind:       fromKind,
		FromAgentID:    strings.TrimSpace(in.FromAgentID),
		FromUserEmail:  strings.TrimSpace(in.FromUserEmail),
		Body:           body,
		Attachments:    attachmentPaths(in.Attachments),
	}
	if _, err := s.temporal.SignalWithStartWorkflow(
		ctx,
		workflowID,
		conversation.ThreadMailSignal,
		mail,
		conversationworkflow.ThreadInboxWorkflow,
		conversation.ThreadWorkflowInput{ThreadID: threadID},
	); err != nil {
		_ = s.queries.DeleteAgentThreadOp(ctx, db.DeleteAgentThreadOpParams{
			ThreadID: threadID,
			OpID:     opID,
		})
		return EnqueueThreadMailResult{}, err
	}
	return EnqueueThreadMailResult{WorkflowID: workflowID}, nil
}

func speakerAgentIDForMail(thread db.AgentThread, fromKind, fromAgentID string) string {
	if strings.TrimSpace(fromKind) == EnqueueFromAgent {
		return strings.TrimSpace(fromAgentID)
	}
	return strings.TrimSpace(thread.AgentSlug.String)
}

func (s *Service) PrepareThreadTurn(
	ctx context.Context,
	mail conversation.ThreadMail,
) (conversation.RunInput, error) {
	thread, err := s.queries.GetAgentThread(ctx, strings.TrimSpace(mail.ThreadID))
	if err != nil {
		return conversation.RunInput{}, err
	}
	speakerID := strings.TrimSpace(mail.SpeakerAgentID)
	if speakerID == "" {
		speakerID = speakerAgentIDForMail(thread, mail.FromKind, mail.FromAgentID)
	}
	run, err := s.createQueuedRun(ctx, s.queries, thread, mail, speakerID)
	if err != nil {
		return conversation.RunInput{}, err
	}
	if mail.FromKind != EnqueueFromAgent {
		if err := s.seedPendingUserPrompt(thread, run); err != nil {
			return conversation.RunInput{}, err
		}
	}
	prepared, err := s.buildRunInput(ctx, thread, run)
	if err != nil {
		return conversation.RunInput{}, err
	}
	return prepared.Input, nil
}

func threadTurnRunID(threadID, opID string) string {
	return uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte(strings.TrimSpace(threadID)+"\x00"+strings.TrimSpace(opID)),
	).String()
}

func attachmentPaths(paths []AttachedPath) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		path := strings.TrimSpace(p.Path)
		if path == "" {
			continue
		}
		out = append(out, path)
	}
	return out
}

func attachedPathsFromMail(paths []string) []AttachedPath {
	out := make([]AttachedPath, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out = append(out, AttachedPath{Path: path, Basename: pathBase(path)})
	}
	return out
}

func pathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}

func (s *Service) createQueuedRun(
	ctx context.Context,
	q *db.Queries,
	thread db.AgentThread,
	mail conversation.ThreadMail,
	speakerAgentID string,
) (db.AgentRun, error) {
	workspaceID := sql.NullString{}
	workspace, err := q.GetPrimaryWorkspaceForThread(
		ctx,
		db.GetPrimaryWorkspaceForThreadParams{
			ThreadID:  thread.ID,
			UserEmail: thread.UserEmail,
		},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return db.AgentRun{}, err
	}
	if err == nil {
		workspaceID = nullString(workspace.ID)
	}
	runID := threadTurnRunID(thread.ID, mail.OpID)
	workflowID := conversation.ThreadWorkflowID(thread.ID)
	docRoot := thread.Cwd
	run, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		ID:                   runID,
		WorkspaceID:          workspaceID,
		ThreadID:             thread.ID,
		SessionID:            sql.NullString{},
		Trigger:              string(conversation.RunTriggerResume),
		Status:               "running",
		PromptText:           mail.Body,
		RestoreHeadEntryID:   thread.HeadEntryID,
		ResultHeadEntryID:    sql.NullString{},
		WorkflowID:           workflowID,
		TemporalRunID:        sql.NullString{},
		WorkflowNodeID:       sql.NullString{},
		WorkflowAttempt:      0,
		WorkflowResultStatus: sql.NullString{},
		WorkflowResultJson:   sql.NullString{},
		RootDocPath:          docRoot,
		ErrorMessage:         sql.NullString{},
		SpeakerAgentSlug: sql.NullString{
			String: speakerAgentID,
			Valid:  speakerAgentID != "",
		},
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			existing, getErr := q.GetAgentRun(ctx, runID)
			if getErr == nil {
				return existing, nil
			}
			return db.AgentRun{}, ErrThreadRunInProgress
		}
		return db.AgentRun{}, err
	}
	existing, listErr := q.ListAgentRunAttachmentsForRun(ctx, run.ID)
	if listErr == nil && len(existing) == 0 {
		if err := s.appendRunAttachments(
			ctx,
			q,
			run.ID,
			thread.ID,
			attachedPathsFromMail(mail.Attachments),
		); err != nil {
			return db.AgentRun{}, err
		}
	}
	return run, nil
}
