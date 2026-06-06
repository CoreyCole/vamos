package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
	servercfg "github.com/CoreyCole/vamos/server"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

func (s *Service) StartCLIChatRun(
	ctx context.Context,
	actor serverauth.MachineAPIActor,
	resolution servercfg.ProjectCheckoutResolution,
	req ChatStartRequest,
	publicBaseURL string,
) (ChatRunRef, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return ChatRunRef{}, fmt.Errorf("prompt is required")
	}
	workspace, err := s.createCLIProjectWorkspace(ctx, actor, resolution, prompt)
	if err != nil {
		return ChatRunRef{}, err
	}
	thread, run, _, err := s.startWorkspaceThreadWithCWD(
		ctx,
		workspace.ID,
		actor.ActorEmail,
		prompt,
		resolution.RootPath,
	)
	if err != nil {
		return ChatRunRef{}, err
	}
	chatSessionID := s.chatSessionIDForRun(ctx, *run)
	if strings.TrimSpace(chatSessionID) == "" && workspace.CurrentSessionID.Valid {
		chatSessionID = workspace.CurrentSessionID.String
	}
	lastSeq := int64(0)
	if strings.TrimSpace(chatSessionID) != "" {
		if projection, err := s.chatSessions.Snapshot(ctx, chatSessionID); err == nil {
			lastSeq = projection.LastSeq
		}
	}
	return ChatRunRef{
		ProjectID:     resolution.ProjectID,
		WorkspaceID:   workspace.ID,
		ThreadID:      thread.ID,
		RunID:         run.ID,
		ChatSessionID: chatSessionID,
		WebURL:        absoluteChatURL(publicBaseURL, thread.ID, run.ID),
		CWD:           resolution.RootPath,
		EventAfter:    lastSeq,
	}, nil
}

func (s *Service) SteerCLIChatThread(
	ctx context.Context,
	actor serverauth.MachineAPIActor,
	req ChatSteerRequest,
	publicBaseURL string,
) (ChatRunRef, ChatSteerDisposition, error) {
	threadID := strings.TrimSpace(req.ThreadID)
	prompt := strings.TrimSpace(req.Prompt)
	if threadID == "" {
		return ChatRunRef{}, ChatSteerDisposition{}, fmt.Errorf("thread_id is required")
	}
	if prompt == "" {
		return ChatRunRef{}, ChatSteerDisposition{}, fmt.Errorf("prompt is required")
	}

	workspace, ok, err := s.ResolvePrimaryWorkspaceForThread(ctx, actor.ActorEmail, threadID)
	if err != nil {
		return ChatRunRef{}, ChatSteerDisposition{}, err
	}
	if !ok {
		return ChatRunRef{}, ChatSteerDisposition{}, sql.ErrNoRows
	}

	disposition := s.chatSteerDisposition(ctx, workspace, threadID, publicBaseURL)
	latestRun, err := s.queries.GetLatestAgentRunByWorkspaceThread(ctx, db.GetLatestAgentRunByWorkspaceThreadParams{
		WorkspaceID: nullString(workspace.ID),
		ThreadID:    threadID,
	})
	if err == nil && isActiveRunStatus(latestRun.Status) {
		ref := ChatRunRef{
			WorkspaceID: workspace.ID,
			ThreadID:    threadID,
			RunID:       latestRun.ID,
			WebURL:      absoluteChatURL(publicBaseURL, threadID, latestRun.ID),
			CWD:         strings.TrimSpace(workspace.Cwd.String),
		}
		if sessionID := s.chatSessionIDForRun(ctx, latestRun); sessionID != "" {
			ref.ChatSessionID = sessionID
		}
		disposition.Reason = "run_in_progress"
		return ref, disposition, ErrThreadRunInProgress
	}

	thread, run, _, err := s.ResumeWorkspaceThread(ctx, workspace.ID, actor.ActorEmail, threadID, prompt)
	if err != nil {
		if errors.Is(err, ErrThreadRunInProgress) {
			disposition.Reason = "run_in_progress"
		}
		return ChatRunRef{}, disposition, err
	}
	chatSessionID := s.chatSessionIDForRun(ctx, *run)
	lastSeq := int64(0)
	if strings.TrimSpace(chatSessionID) != "" {
		if projection, err := s.chatSessions.Snapshot(ctx, chatSessionID); err == nil {
			lastSeq = projection.LastSeq
		}
	}
	ref := ChatRunRef{
		WorkspaceID:   workspace.ID,
		ThreadID:      thread.ID,
		RunID:         run.ID,
		ChatSessionID: chatSessionID,
		WebURL:        absoluteChatURL(publicBaseURL, thread.ID, run.ID),
		CWD:           thread.Cwd,
		EventAfter:    lastSeq,
	}
	return ref, disposition, nil
}

func (s *Service) chatSteerDisposition(ctx context.Context, workspace db.Workspace, threadID, publicBaseURL string) ChatSteerDisposition {
	latestThreadID := strings.TrimSpace(workspace.SelectedThreadID.String)
	if latestThreadID == "" {
		latestThreadID = threadID
	}
	latestURL := ""
	if latestThreadID != "" {
		latestRunID := ""
		if run, err := s.queries.GetLatestAgentRunByWorkspaceThread(ctx, db.GetLatestAgentRunByWorkspaceThreadParams{
			WorkspaceID: nullString(workspace.ID),
			ThreadID:    latestThreadID,
		}); err == nil {
			latestRunID = run.ID
		}
		latestURL = absoluteChatURL(publicBaseURL, latestThreadID, latestRunID)
	}
	return ChatSteerDisposition{
		InfluencesLatest: latestThreadID == "" || latestThreadID == threadID,
		LatestThreadID:   latestThreadID,
		LatestWebURL:     latestURL,
	}
}

func isActiveRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "complete", "completed", "failed", "canceled", "cancelled":
		return false
	default:
		return true
	}
}

func (s *Service) createCLIProjectWorkspace(
	ctx context.Context,
	actor serverauth.MachineAPIActor,
	resolution servercfg.ProjectCheckoutResolution,
	prompt string,
) (db.Workspace, error) {
	root, err := s.createCLIProjectWorkspaceRoot(actor.ActorEmail, resolution.ProjectID)
	if err != nil {
		return db.Workspace{}, err
	}
	return s.CreateWorkspace(ctx, WorkspaceCreateInput{
		UserEmail:    actor.ActorEmail,
		Title:        truncateTitle(firstNonEmptyCLIString(resolution.ProjectID, prompt)),
		RootDocPath:  root,
		Cwd:          resolution.RootPath,
		WorkflowType: WorkspaceWorkflowFreeform,
		Source:       WorkspaceSourceTerminal,
	})
}

func (s *Service) createCLIProjectWorkspaceRoot(actorEmail, projectID string) (string, error) {
	root := strings.TrimSpace(s.thoughtsRoot)
	if root == "" {
		root = filepath.Join(s.resolveCwd(""), "thoughts")
	}
	dir := filepath.Join(
		root,
		sanitizeWorkspacePathSegment(actorEmail),
		"cli-chat",
		sanitizeWorkspacePathSegment(projectID),
		uuid.NewString(),
	)
	return dir, ensureWorkspaceRoot(dir, projectID)
}

func ensureWorkspaceRoot(dir, projectID string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "AGENTS.md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = fmt.Fprintf(file, "# CLI Agent Chat Workspace\n\nProject: `%s`\n", strings.TrimSpace(projectID))
	return err
}

func (s *Service) startWorkspaceThreadWithCWD(
	ctx context.Context,
	workspaceID, userEmail, prompt, cwd string,
	attachments ...[]AttachedPath,
) (*db.AgentThread, *db.AgentRun, *db.AgentSession, error) {
	if s.temporal == nil {
		return nil, nil, nil, fmt.Errorf("temporal not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, nil, fmt.Errorf("prompt is required")
	}

	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}
	threadCwd := strings.TrimSpace(cwd)
	if threadCwd == "" {
		threadCwd = s.workspaceThreadCwd(workspace)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         userEmail,
		Title:             truncateTitle(prompt),
		Cwd:               threadCwd,
		LineageID:         uuid.NewString(),
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := q.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{
		ID:          thread.ID,
		WorkspaceID: nullString(workspace.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	session, err := s.createWebAgentSession(ctx, q, workspace, thread)
	if err != nil {
		return nil, nil, nil, err
	}
	run, err := s.createRunForSession(
		ctx,
		q,
		workspace,
		session,
		thread,
		conversation.RunTriggerSend,
		prompt,
		sql.NullString{},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chatSession, err := ensureWorkspaceChatSessionTx(
		ctx,
		q,
		workspace,
		userEmail,
		run.WorkflowID,
		run.WorkflowNodeID.String,
		int(run.WorkflowAttempt),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := appendPromptAndRunStartedSessionEventsTx(ctx, q, chatSession, userEmail, thread, run, prompt); err != nil {
		return nil, nil, nil, err
	}
	if err := s.appendRunAttachments(ctx, q, run.ID, thread.ID, flattenAttachedPaths(attachments)); err != nil {
		return nil, nil, nil, err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
		ID:               workspace.ID,
		SelectedThreadID: nullString(thread.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	event, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   "thread_created",
		ActorEmail:  userEmail,
		ActorType:   "user",
		ThreadID:    thread.ID,
		SessionID:   session.ID,
		RunID:       run.ID,
		EventKey:    "thread_created:" + thread.ID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}

	s.NotifyWorkspaceForEvent(event)
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, &session, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, &session, err
	}
	return &thread, startedRun, &session, nil
}

func absoluteChatURL(publicBaseURL, threadID, runID string) string {
	rel := threadThoughtsChatURL(threadID, runID)
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return rel
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rel
	}
	return base + rel
}

func firstNonEmptyCLIString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "Agent Chat"
}
