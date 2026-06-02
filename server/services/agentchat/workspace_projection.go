package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
)

var ErrThreadWorkspaceMismatch = errors.New("thread does not belong to workspace")

func (s *Service) BuildWorkspacePageArgs(
	ctx context.Context,
	input BuildWorkspacePageInput,
) (*WorkspacePageArgs, error) {
	threadID := strings.TrimSpace(input.ThreadID)
	workspace, trustedThread, trusted, err := s.workspacePageContext(ctx, input, threadID)
	if err != nil {
		return nil, err
	}

	currentChatSessionID := ""
	if workspace.CurrentSessionID.Valid {
		currentChatSessionID = strings.TrimSpace(workspace.CurrentSessionID.String)
	}
	activeChatPath, _ := s.chatSessions.ActivePath(
		ctx,
		workspace.ID,
		currentChatSessionID,
	)

	projection := WorkspaceProjection{
		Workspace:            workspace,
		CurrentChatSessionID: currentChatSessionID,
		ActiveChatPath:       activeChatPath,
		Header: WorkspaceHeaderState{
			WorkspaceID:      workspace.ID,
			Title:            workspace.Title,
			RootDocPath:      workspace.RootDocPath,
			WorkflowLabel:    workspace.WorkflowType,
			SelectedThreadID: threadID,
		},
		Transcript: TranscriptPaneState{
			Stable: []TranscriptMessage{},
			Live:   LiveTranscriptView{Items: []TranscriptMessage{}},
			Policy: s.defaultTranscriptRenderPolicy(),
		},
	}

	if currentChatSessionID != "" {
		if snapshot, err := s.chatSessions.Snapshot(
			ctx,
			currentChatSessionID,
		); err == nil {
			projection.ChatTree = snapshot.Tree
			projection.Transcript.Stable = s.transcriptFromChatProjection(snapshot)
		}
	}

	if threadID != "" {
		thread := trustedThread
		if !trusted {
			var err error
			thread, err = s.sharedWorkspaceThread(ctx, workspace.ID, threadID)
			if err != nil {
				return nil, err
			}
		}
		projection.SelectedThread = &thread
		stable := projection.Transcript.Stable
		if len(stable) == 0 {
			var err error
			stable, err = s.buildStableTranscript(ctx, thread)
			if err != nil {
				return nil, err
			}
		}
		metadata, err := s.buildTranscriptMetadata(ctx, thread)
		if err != nil {
			return nil, err
		}
		projection.Header.ModelLabel = metadata.ModelLabel
		projection.Header.ThinkingLabel = metadata.ThinkingLabel
		live, cursor := s.buildLiveTranscript(thread.ID)
		projection.Transcript = TranscriptPaneState{
			Cursor: cursor,
			Stable: stable,
			Live:   live,
			Policy: s.defaultTranscriptRenderPolicy(),
		}
		projection.ActiveRun = s.lookupWorkspaceRun(
			ctx,
			workspace.ID,
			thread.ID,
			input.RunID,
		)
		projection.Header.HasActiveRun = projection.ActiveRun != nil &&
			projection.ActiveRun.Status == "running"
	}

	selectedArtifact := strings.TrimSpace(input.DocRelPath)
	if selectedArtifact == "" && workspace.SelectedDocPath.Valid {
		selectedArtifact = workspace.SelectedDocPath.String
	}
	activeRunID := ""
	if projection.ActiveRun != nil {
		activeRunID = projection.ActiveRun.ID
	}
	artifacts, err := s.buildWorkspaceDocPane(
		ctx,
		workspace,
		activeRunID,
		selectedArtifact,
		!trusted,
	)
	if err != nil {
		return nil, err
	}
	projection.Docs = artifacts
	projection.Sidebar = s.BuildWorkspaceSidebarState(
		ctx,
		input.UserEmail,
		workspace.ID,
		threadID,
	)
	projection.PlanSidebar, err = s.BuildPlanSidebarState(ctx, PlanSidebarInput{
		UserEmail:         input.UserEmail,
		ActiveWorkspaceID: workspace.ID,
		ActiveThreadID:    threadID,
		ActivePlanDir:     workspace.RootDocPath,
	})
	if err != nil {
		return nil, err
	}
	projection.PlanSidebar.TargetID = "agent-chat-workspace-sidebar"
	projection.SidebarProjection, _ = s.BuildWorkspaceSidebarProjection(ctx, SidebarInput{
		UserEmail:         input.UserEmail,
		ActiveWorkspaceID: workspace.ID,
	})
	projection.Minimap, _ = s.BuildPlanMinimap(
		ctx,
		input.UserEmail,
		workspace.ID,
		workspace.RootDocPath,
		true,
		100,
	)
	projection.Workflow, _ = s.BuildWorkspaceWorkflowState(ctx, workspace)
	projection.Workflow.ThreadID = threadID
	if projection.Workflow.LastResultCard != nil {
		projection.Workflow.LastResultCard.ThreadID = threadID
	}
	projection.Transcript.Stable = attachQRSPIWorkflowCardToLatestAssistantMessage(
		projection.Transcript.Stable,
		projection.Workflow.LastResultCard,
	)
	projection.Log, _ = s.BuildWorkspaceLogState(ctx, workspace.ID, 20)
	projection.Header.WorkflowLabel = workspaceWorkflowLabel(projection.Workflow)
	if sessions, err := s.BuildPlanSessionState(
		ctx,
		input.UserEmail,
		workspace.ID,
		workspace.RootDocPath,
		threadID,
		true,
	); err == nil {
		projection.Sessions = sessions
	}

	return &WorkspacePageArgs{
		UserEmail:   input.UserEmail,
		WorkspaceID: workspace.ID,
		Cursor:      s.CurrentCursor(workspace.ID),
		Projection:  projection,
	}, nil
}

func (s *Service) transcriptFromChatProjection(proj chatsession.ChatProjection) []TranscriptMessage {
	messages := make([]TranscriptMessage, 0, len(proj.Messages)+len(proj.Tools)+len(proj.Artifacts))
	for _, msg := range proj.Messages {
		domID := strings.TrimSpace(msg.ID)
		if domID == "" {
			domID = fmt.Sprintf("chat-%d", msg.Seq)
		}
		entryID := domID
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "assistant"
		}
		item := s.newBubbleTranscriptMessage(domID, entryID, role, msg.Content, role == "assistant")
		item.ChatSessionID = proj.SessionID
		item.ChatNodeID = domID
		item.ChatEventSeq = msg.Seq
		messages = append(messages, item)
	}
	for _, tool := range proj.Tools {
		title := firstNonEmptyString(tool.Name, "tool")
		body := strings.TrimSpace(tool.Summary)
		if body == "" {
			body = tool.Status
		}
		item := s.newDetailTranscriptMessage(
			firstNonEmptyString(tool.ID, fmt.Sprintf("tool-%d", tool.Seq)),
			tool.ID,
			title,
			body,
			tool.Status == "failed",
			true,
		)
		item.ToolCallID = tool.ID
		item.ChatSessionID = proj.SessionID
		item.ChatNodeID = tool.ID
		item.ChatEventSeq = tool.Seq
		messages = append(messages, item)
	}
	for _, artifact := range proj.Artifacts {
		title := "file " + artifact.Kind
		item := s.newDetailTranscriptMessage(
			firstNonEmptyString(artifact.Path, fmt.Sprintf("artifact-%d", artifact.Seq)),
			artifact.Path,
			title,
			artifact.Path,
			false,
			false,
		)
		item.HeaderCode = artifact.Path
		item.ChatSessionID = proj.SessionID
		item.ChatNodeID = artifact.Path
		item.ChatEventSeq = artifact.Seq
		messages = append(messages, item)
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].ChatEventSeq < messages[j].ChatEventSeq
	})
	return messages
}

func (s *Service) workspacePageContext(
	ctx context.Context,
	input BuildWorkspacePageInput,
	threadID string,
) (db.Workspace, db.AgentThread, bool, error) {
	workspace, err := s.queries.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err == nil {
		shared := strings.TrimSpace(
			workspace.UserEmail,
		) != strings.TrimSpace(
			input.UserEmail,
		)
		if shared && strings.TrimSpace(threadID) != "" {
			thread, threadErr := s.sharedWorkspaceThread(ctx, workspace.ID, threadID)
			if threadErr != nil {
				return db.Workspace{}, db.AgentThread{}, false, threadErr
			}
			return workspace, thread, true, nil
		}
		return workspace, db.AgentThread{}, shared, nil
	}
	if strings.TrimSpace(threadID) == "" {
		return db.Workspace{}, db.AgentThread{}, false, err
	}

	trustedWorkspace, trustedThread, trustedErr := s.trustedImportedWorkspaceThread(
		ctx,
		input.WorkspaceID,
		threadID,
	)
	if trustedErr != nil {
		return db.Workspace{}, db.AgentThread{}, false, err
	}
	return trustedWorkspace, trustedThread, true, nil
}

func (s *Service) sharedWorkspaceThread(
	ctx context.Context,
	workspaceID string,
	threadID string,
) (db.AgentThread, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	thread, err := s.queries.GetAgentThread(ctx, strings.TrimSpace(threadID))
	if err != nil {
		return db.AgentThread{}, err
	}
	ok, err := s.threadHasWorkspaceAssociation(ctx, thread.ID, workspaceID)
	if err != nil {
		return db.AgentThread{}, err
	}
	if !ok {
		return db.AgentThread{}, ErrThreadWorkspaceMismatch
	}
	return thread, nil
}

func (s *Service) trustedImportedWorkspaceThread(
	ctx context.Context,
	workspaceID string,
	threadID string,
) (db.Workspace, db.AgentThread, error) {
	workspace, err := s.queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	thread, err := s.sharedWorkspaceThread(ctx, workspace.ID, threadID)
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}

	sessions, err := s.queries.ListAgentSessionsByWorkspace(ctx, nullString(workspace.ID))
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if !hasImportedSessionForThread(sessions, thread.ID) {
		return db.Workspace{}, db.AgentThread{}, errors.New(
			"workspace thread is not an imported pi session",
		)
	}
	return workspace, thread, nil
}

func (s *Service) trustedImportedWorkspace(
	ctx context.Context,
	workspaceID string,
) (db.Workspace, error) {
	workspace, err := s.queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return db.Workspace{}, err
	}
	sessions, err := s.queries.ListAgentSessionsByWorkspace(ctx, nullString(workspace.ID))
	if err != nil {
		return db.Workspace{}, err
	}
	for _, session := range sessions {
		if isTrustedImportedSessionStatus(session.Status) {
			return workspace, nil
		}
	}
	return db.Workspace{}, errors.New("workspace is not an imported pi session")
}

func hasImportedSessionForThread(sessions []db.AgentSession, threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	for _, session := range sessions {
		if !session.ThreadID.Valid ||
			strings.TrimSpace(session.ThreadID.String) != threadID {
			continue
		}
		if isTrustedImportedSessionStatus(session.Status) {
			return true
		}
	}
	return false
}

func isTrustedImportedSessionStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "imported", "diverged":
		return true
	default:
		return false
	}
}

func (s *Service) RedirectURLForImportedThread(
	ctx context.Context,
	workspaceID string,
	threadID string,
) (string, error) {
	_, thread, err := s.trustedImportedWorkspaceThread(ctx, workspaceID, threadID)
	if err != nil {
		return "", err
	}
	return threadHref(thread.ID), nil
}

func (s *Service) BuildWorkspaceLiveTranscriptState(
	ctx context.Context,
	input BuildWorkspacePageInput,
) (TranscriptPaneState, string, error) {
	threadID := strings.TrimSpace(input.ThreadID)
	workspace, trustedThread, trusted, err := s.workspacePageContext(ctx, input, threadID)
	if err != nil {
		return TranscriptPaneState{}, "", err
	}
	if threadID == "" && workspace.SelectedThreadID.Valid {
		threadID = workspace.SelectedThreadID.String
	}
	if threadID == "" {
		return TranscriptPaneState{
			Stable: []TranscriptMessage{},
			Live:   LiveTranscriptView{Items: []TranscriptMessage{}},
			Policy: s.defaultTranscriptRenderPolicy(),
		}, "", nil
	}

	thread := trustedThread
	if !trusted {
		thread, err = s.sharedWorkspaceThread(ctx, workspace.ID, threadID)
		if err != nil {
			return TranscriptPaneState{}, "", err
		}
	}

	live, cursor := s.buildLiveTranscript(thread.ID)
	return TranscriptPaneState{
		Cursor: cursor,
		Stable: []TranscriptMessage{},
		Live:   live,
		Policy: s.defaultTranscriptRenderPolicy(),
	}, thread.ID, nil
}

const (
	workspaceSidebarWorkspacesLimit = 200
	workspaceSidebarFallbackLabel   = "Workspace"
)

func (s *Service) BuildWorkspaceSidebarState(
	ctx context.Context,
	userEmail string,
	workspaceID string,
	selectedThreadID string,
) WorkspaceSidebarState {
	_ = userEmail
	state := WorkspaceSidebarState{
		WorkspaceID:      workspaceID,
		SelectedThreadID: selectedThreadID,
		HasSelection:     strings.TrimSpace(selectedThreadID) != "",
	}
	workspaces, err := s.queries.ListWorkspaces(ctx, workspaceSidebarWorkspacesLimit)
	if err != nil {
		return state
	}

	groups := make([]ThreadSidebarGroup, 0, len(workspaces))
	for _, workspace := range workspaces {
		threads, err := s.queries.ListAgentThreadsByWorkspace(
			ctx,
			workspace.ID,
		)
		if err != nil {
			threads = []db.AgentThread{}
		}
		group := ThreadSidebarGroup{
			Key:           "workspace:" + workspace.ID,
			KindLabel:     workspaceWorkflowSidebarLabel(workspace.WorkflowType),
			Label:         workspace.Title,
			Timestamp:     workspaceSidebarArtifactLabel(workspace),
			ThreadCount:   len(threads),
			IsActive:      workspace.ID == workspaceID,
			WorkspaceHref: thoughtsWorkspaceHref(workspace.RootDocPath, workspace.ID),
			Threads:       []ThreadSidebarThread{},
		}
		if group.IsActive {
			state.ActiveGroupKey = group.Key
		}
		if thread, ok := selectVisibleWorkspaceThread(
			workspace,
			threads,
			workspaceID,
			selectedThreadID,
		); ok {
			group.Threads = append(
				group.Threads,
				s.workspaceThreadSidebarRow(
					workspace,
					thread,
					workspaceID,
					selectedThreadID,
				),
			)
		}
		groups = append(groups, group)
	}
	state.Groups = groups
	return state
}

func selectVisibleWorkspaceThread(
	workspace db.Workspace,
	threads []db.AgentThread,
	activeWorkspaceID string,
	selectedThreadID string,
) (db.AgentThread, bool) {
	if len(threads) == 0 {
		return db.AgentThread{}, false
	}
	if workspace.ID == strings.TrimSpace(activeWorkspaceID) {
		selectedThreadID = strings.TrimSpace(selectedThreadID)
		if selectedThreadID != "" {
			for _, thread := range threads {
				if thread.ID == selectedThreadID {
					return thread, true
				}
			}
		}
	}
	return threads[0], true
}

func (s *Service) workspaceThreadSidebarRow(
	workspace db.Workspace,
	thread db.AgentThread,
	activeWorkspaceID string,
	selectedThreadID string,
) ThreadSidebarThread {
	return ThreadSidebarThread{
		ID:       thread.ID,
		Href:     workspaceThreadHrefForWorkspace(workspace, thread.ID),
		Title:    thread.Title,
		CwdLabel: s.threadSidebarCwdLabel(thread.Cwd, workspace.RootDocPath),
		IsActive: workspace.ID == strings.TrimSpace(activeWorkspaceID) &&
			thread.ID == strings.TrimSpace(selectedThreadID),
	}
}

func thoughtsWorkspaceHref(rootDocPath, workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	values := url.Values{}
	if workspaceID != "" {
		values.Set("chat_workspace", workspaceID)
	}
	return thoughtsDocRedirectURL(rootDocPath, values)
}

func threadHref(threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	values := url.Values{"thread": []string{threadID}}
	return "/agent-chat?" + values.Encode()
}

func workspaceThreadHrefForWorkspace(_ db.Workspace, threadID string) string {
	return threadHref(threadID)
}

func workspaceThreadHref(workspaceID, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if strings.TrimSpace(workspaceID) == "" || threadID == "" {
		return ""
	}
	return BuildThoughtsChatDocURL(EmbeddedChatURLState{ThreadID: threadID})
}

type WorkspaceSessionSummary struct {
	Header          PiSessionHeader
	FirstPromptText string
	ImportStats     PiSessionImportStats
	ThreadTitle     string
}

func (s *Service) BuildWorkspaceSessionState(
	ctx context.Context,
	workspaceID string,
	selectedThreadID string,
) (WorkspaceSessionState, error) {
	sessions, err := s.queries.ListAgentSessionsByWorkspace(ctx, nullString(workspaceID))
	if err != nil {
		return WorkspaceSessionState{}, err
	}
	return s.workspaceSessionStateFromRows(
		ctx,
		sessions,
		selectedThreadID,
		"",
		false,
	), nil
}

func (s *Service) BuildPlanSessionState(
	ctx context.Context,
	userEmail string,
	workspaceID string,
	planDir string,
	selectedThreadID string,
	includeDescendants bool,
) (WorkspaceSessionState, error) {
	canonical, ok := s.canonicalPlanDirFromSource(planDir)
	if !ok {
		return s.BuildWorkspaceSessionState(ctx, workspaceID, selectedThreadID)
	}

	workspaceID = strings.TrimSpace(workspaceID)
	userEmail = strings.TrimSpace(userEmail)
	var sessions []db.AgentSession
	var err error
	if includeDescendants {
		sessions, err = s.queries.ListAgentSessionsByPlanDirPrefix(
			ctx,
			db.ListAgentSessionsByPlanDirPrefixParams{
				UserEmail:     nullString(userEmail),
				WorkspaceID:   workspaceID,
				PlanDir:       nullString(canonical),
				PlanDirPrefix: nullString(canonical + string(filepath.Separator) + "%"),
			},
		)
	} else {
		sessions, err = s.queries.ListAgentSessionsByPlanDir(
			ctx,
			db.ListAgentSessionsByPlanDirParams{
				UserEmail:   nullString(userEmail),
				WorkspaceID: workspaceID,
				PlanDir:     nullString(canonical),
			},
		)
	}
	if err != nil {
		return WorkspaceSessionState{}, err
	}
	return s.workspaceSessionStateFromRows(
		ctx,
		sessions,
		selectedThreadID,
		canonical,
		includeDescendants,
	), nil
}

func (s *Service) workspaceSessionStateFromRows(
	ctx context.Context,
	sessions []db.AgentSession,
	selectedThreadID string,
	planDir string,
	includeDescendants bool,
) WorkspaceSessionState {
	state := WorkspaceSessionState{
		PlanDir:            planDir,
		PlanLabel:          planSessionStateLabel(planDir),
		IncludeDescendants: includeDescendants,
		History:            make([]WorkspaceSessionHistoryItem, 0, len(sessions)),
		Sessions:           sessions,
	}
	for _, session := range sessions {
		summary := s.workspaceSessionSummary(ctx, session)
		sessionPlanDir := planDir
		if strings.TrimSpace(sessionPlanDir) == "" && session.WorkspaceID.Valid {
			if workspace, err := s.queries.GetWorkspace(
				ctx,
				session.WorkspaceID.String,
			); err == nil {
				sessionPlanDir = workspace.RootDocPath
			}
		}
		state.History = append(
			state.History,
			s.workspaceSessionHistoryItem(
				session,
				selectedThreadID,
				sessionPlanDir,
				summary,
			),
		)
	}
	return state
}

func (s *Service) BuildPlanMinimap(
	ctx context.Context,
	userEmail string,
	workspaceID string,
	planDir string,
	includeDescendants bool,
	limit int64,
) (WorkspaceMinimapState, error) {
	_ = userEmail
	_ = planDir
	_ = includeDescendants
	return s.BuildWorkspaceMinimap(ctx, workspaceID, limit)
}

func planSessionStateLabel(planDir string) string {
	planDir = strings.TrimSpace(planDir)
	if planDir == "" {
		return ""
	}
	label, _ := formatSidebarGroupDisplay(filepath.Base(filepath.Clean(planDir)))
	if strings.TrimSpace(label) != "" {
		return label
	}
	return filepath.Base(filepath.Clean(planDir))
}

func (s *Service) workspaceSessionSummary(
	ctx context.Context,
	session db.AgentSession,
) WorkspaceSessionSummary {
	metadata := parseSessionImportMetadata(session.MetadataJson.String)
	summary := WorkspaceSessionSummary{
		Header:          metadata.Header,
		FirstPromptText: strings.TrimSpace(metadata.FirstUserText),
		ImportStats:     metadata.Stats,
	}
	if session.ThreadID.Valid && strings.TrimSpace(session.ThreadID.String) != "" {
		if thread, err := s.queries.GetAgentThread(
			ctx,
			session.ThreadID.String,
		); err == nil {
			summary.ThreadTitle = thread.Title
			if summary.FirstPromptText == "" {
				summary.FirstPromptText = firstUserPromptFromThread(
					ctx,
					s.queries,
					thread,
				)
			}
		}
	}
	return summary
}

func parseSessionImportMetadata(raw string) sessionImportMetadata {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sessionImportMetadata{}
	}
	var metadata sessionImportMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err == nil {
		if metadata.Inference.Status != "" || metadata.Stats.LinesRead != 0 ||
			metadata.Stats.EntriesRead != 0 || metadata.Stats.EntriesImported != 0 ||
			metadata.Header.ID != "" || metadata.FirstUserText != "" {
			return metadata
		}
	}
	var inference WorkspaceInferenceResult
	if err := json.Unmarshal(
		[]byte(raw),
		&inference,
	); err == nil &&
		inference.Status != "" {
		metadata.Inference = inference
	}
	return metadata
}

func firstUserPromptFromThread(
	ctx context.Context,
	queries *db.Queries,
	thread db.AgentThread,
) string {
	if !thread.HeadEntryID.Valid || strings.TrimSpace(thread.HeadEntryID.String) == "" {
		return ""
	}
	rows, err := queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: thread.HeadEntryID.String,
	})
	if err != nil {
		return ""
	}
	for _, row := range rows {
		var envelope struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(row.PayloadJson), &envelope); err != nil {
			continue
		}
		if envelope.Type == "message" &&
			strings.TrimSpace(envelope.Message.Role) == "user" {
			return strings.TrimSpace(extractContentText(envelope.Message.Content))
		}
	}
	return ""
}

func (s *Service) workspaceSessionHistoryItem(
	session db.AgentSession,
	selectedThreadID string,
	planDir string,
	summary WorkspaceSessionSummary,
) WorkspaceSessionHistoryItem {
	threadID := ""
	if session.ThreadID.Valid {
		threadID = strings.TrimSpace(session.ThreadID.String)
	}
	title := strings.TrimSpace(summary.Header.ID)
	if title == "" {
		title = strings.TrimSpace(summary.ThreadTitle)
	}
	if title == "" {
		title = strings.TrimSpace(session.SessionID.String)
	}
	if title == "" {
		title = session.ID
	}
	return WorkspaceSessionHistoryItem{
		ID:       session.ID,
		ThreadID: threadID,
		ThreadHref: s.workspaceSessionThreadHref(
			session.WorkspaceID.String,
			threadID,
			planDir,
		),
		Title:              title,
		Status:             strings.TrimSpace(session.Status),
		SourceLabel:        strings.TrimSpace(session.Source),
		CwdLabel:           strings.TrimSpace(session.Cwd.String),
		SessionPathLabel:   strings.TrimSpace(session.SessionPath.String),
		InferredPlanLabel:  strings.TrimSpace(session.InferredPlanDir.String),
		FirstPromptExcerpt: truncateSessionHistoryText(summary.FirstPromptText, 160),
		FirstCommandLabel:  firstCommandFromPrompt(summary.FirstPromptText),
		ImportStatsLabel:   importStatsLabel(summary.ImportStats),
		ErrorLabel:         strings.TrimSpace(session.LastError.String),
		UpdatedAtLabel:     formatWorkspaceEventTime(session.UpdatedAt),
		IsCurrentThread: threadID != "" &&
			threadID == strings.TrimSpace(selectedThreadID),
	}
}

func (s *Service) workspaceSessionThreadHref(
	_, threadID, _ string,
) string {
	return threadHref(threadID)
}

func firstCommandFromPrompt(prompt string) string {
	known := map[string]struct{}{
		"q-question":  {},
		"q-research":  {},
		"q-design":    {},
		"q-outline":   {},
		"q-plan":      {},
		"q-implement": {},
		"q-review":    {},
		"q-resume":    {},
	}
	for _, field := range strings.Fields(prompt) {
		token := strings.Trim(field, "`'\".,;:()[]{}<>")
		if strings.HasPrefix(token, "/") && len(token) > 1 {
			return token
		}
		if strings.HasPrefix(token, "@") && len(token) > 1 {
			return token
		}
		if _, ok := known[strings.ToLower(token)]; ok {
			return token
		}
	}
	return ""
}

func importStatsLabel(stats PiSessionImportStats) string {
	parts := []string{}
	if stats.EntriesImported > 0 {
		parts = append(parts, fmt.Sprintf("%d imported", stats.EntriesImported))
	}
	if stats.EntriesSkipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", stats.EntriesSkipped))
	}
	if len(parts) == 0 && stats.EntriesRead > 0 {
		parts = append(parts, fmt.Sprintf("%d read", stats.EntriesRead))
	}
	return strings.Join(parts, " · ")
}

func truncateSessionHistoryText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit-1]) + "…"
}

func workspaceWorkflowSidebarLabel(workflowType string) string {
	switch WorkspaceWorkflowType(strings.TrimSpace(workflowType)) {
	case WorkspaceWorkflowQRSPI:
		return "QRSPI"
	case WorkspaceWorkflowFreeform:
		return "Freeform"
	default:
		if strings.TrimSpace(workflowType) != "" {
			return strings.TrimSpace(workflowType)
		}
		return workspaceSidebarFallbackLabel
	}
}

func workspaceSidebarArtifactLabel(workspace db.Workspace) string {
	artifactRoot := strings.TrimSpace(workspace.RootDocPath)
	if artifactRoot == "" {
		return ""
	}
	return filepath.Base(artifactRoot)
}

func (s *Service) lookupWorkspaceRun(
	ctx context.Context,
	workspaceID string,
	threadID string,
	runID string,
) *db.AgentRun {
	workspaceKey := nullString(workspaceID)
	if strings.TrimSpace(runID) != "" {
		run, err := s.queries.GetAgentRunForWorkspace(
			ctx,
			db.GetAgentRunForWorkspaceParams{ID: runID, WorkspaceID: workspaceKey},
		)
		if err == nil && run.ThreadID == threadID {
			return &run
		}
		if run, err := s.queries.GetAgentRun(ctx, runID); err == nil &&
			run.ThreadID == threadID {
			return &run
		}
	}
	if run, err := s.queries.GetLatestAgentRunByWorkspaceThread(
		ctx,
		db.GetLatestAgentRunByWorkspaceThreadParams{
			WorkspaceID: workspaceKey,
			ThreadID:    threadID,
		},
	); err == nil {
		return &run
	}
	return nil
}

func (s *Service) BuildWorkspaceDocPane(
	ctx context.Context,
	workspace db.Workspace,
	activeRunID string,
	selectedPath string,
) (WorkspaceDocPaneState, error) {
	return s.buildWorkspaceDocPane(ctx, workspace, activeRunID, selectedPath, true)
}

func (s *Service) buildWorkspaceDocPane(
	ctx context.Context,
	workspace db.Workspace,
	activeRunID string,
	selectedPath string,
	persistSelection bool,
) (WorkspaceDocPaneState, error) {
	root := strings.TrimSpace(workspace.RootDocPath)
	if root == "" {
		return WorkspaceDocPaneState{ActiveRunID: activeRunID}, nil
	}
	resolvedRoot, err := ValidateWorkspaceRootDocPath(
		root,
		s.thoughtsRoot,
		workspace.UserEmail,
	)
	if err != nil {
		return WorkspaceDocPaneState{}, err
	}

	parentRoot := resolvedRoot
	workspace.RootDocPath = parentRoot
	if _, err := s.SyncWorkspaceDocInventory(ctx, workspace); err != nil {
		return WorkspaceDocPaneState{}, err
	}
	rows, err := s.queries.ListWorkspaceDocs(ctx, workspace.ID)
	if err != nil {
		return WorkspaceDocPaneState{}, err
	}
	parentFiles := workspaceArtifactRelPaths(parentRoot, rows)
	root = parentRoot
	files := parentFiles

	selectedRel := ""
	if strings.TrimSpace(selectedPath) != "" {
		if cleanRel, err := ValidateWorkspaceRelPath(
			parentRoot,
			selectedPath,
		); err == nil &&
			containsDocPath(parentFiles, cleanRel) {
			selectedRel = cleanRel
		}
	}
	if selectedRel == "" && workspace.SelectedDocPath.Valid {
		if cleanRel, err := ValidateWorkspaceRelPath(
			parentRoot,
			workspace.SelectedDocPath.String,
		); err == nil &&
			containsDocPath(parentFiles, cleanRel) {
			selectedRel = cleanRel
		}
	}
	if selectedRel == "" {
		selectedRel = defaultDocPath(parentFiles)
	}

	selectedAbs := ""
	if selectedRel != "" {
		selectedAbs = filepath.Join(parentRoot, filepath.FromSlash(selectedRel))
		if focusedRoot := focusedRootDocPath(
			parentRoot,
			selectedAbs,
		); focusedRoot != "" &&
			!sameFilesystemPath(focusedRoot, parentRoot) {
			root = focusedRoot
			files, err = listRenderableDocs(root)
			if err != nil {
				return WorkspaceDocPaneState{}, err
			}
			selectedRel = relativeDocPath(root, selectedAbs)
			if selectedRel == "" {
				selectedRel = defaultDocPath(files)
			}
		}
	}

	persistedRel := selectedRel
	if !sameFilesystemPath(root, parentRoot) && selectedRel != "" {
		if rel, err := filepath.Rel(
			parentRoot,
			filepath.Join(root, filepath.FromSlash(selectedRel)),
		); err == nil {
			persistedRel = filepath.ToSlash(rel)
		}
	}
	if persistSelection && persistedRel != "" &&
		(!workspace.SelectedDocPath.Valid || workspace.SelectedDocPath.String != persistedRel) {
		_ = s.queries.UpdateWorkspaceSelectedDoc(
			ctx,
			db.UpdateWorkspaceSelectedDocParams{
				ID:              workspace.ID,
				SelectedDocPath: nullString(persistedRel),
			},
		)
	}

	lineage, _ := DiscoverPlanNodes(parentRoot)
	s.linkPlanLineage(ctx, &lineage)
	state := WorkspaceDocPaneState{
		WorkspaceID: workspace.ID,
		ActiveRunID: activeRunID,
		RootDocPath: root,
		RootLabel:   docRootLabel(root),
		WorkingDir: s.buildWorkingDirectoryState(
			parentRoot,
			docResetPath(parentRoot, root),
		),
		PlanLineage: &lineage,
	}
	selected, err := s.renderDoc(root, selectedRel)
	if err != nil {
		return WorkspaceDocPaneState{}, err
	}
	state.Tree = buildDocTree(files, selectedRel, root)
	state.Selected = selected
	if selected.Exists && selected.RelativePath != "" {
		commentWorkspace := workspace
		commentWorkspace.RootDocPath = root
		comments, err := s.BuildWorkspaceDocCommentProjection(
			ctx,
			commentWorkspace,
			selected.RelativePath,
			true,
		)
		if err != nil {
			return WorkspaceDocPaneState{}, err
		}
		state.Comments = comments
	}
	return state, nil
}

func (s *Service) BuildWorkspaceMinimap(
	ctx context.Context,
	workspaceID string,
	limit int64,
) (WorkspaceMinimapState, error) {
	if limit <= 0 {
		limit = 100
	}
	events, err := s.queries.ListWorkspaceEvents(ctx, db.ListWorkspaceEventsParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		return WorkspaceMinimapState{}, err
	}
	state := WorkspaceMinimapState{Events: make([]WorkspaceMinimapEvent, 0, len(events))}
	for _, event := range events {
		state.Events = append(state.Events, workspaceMinimapEvent(event))
	}
	return state, nil
}

func workspaceMinimapEvent(event db.WorkspaceEvent) WorkspaceMinimapEvent {
	category, label := workspaceEventCategoryAndLabel(event)
	return WorkspaceMinimapEvent{
		ID:        event.ID,
		Type:      event.EventType,
		Category:  category,
		Label:     label,
		ThreadID:  event.ThreadID.String,
		SessionID: event.SessionID.String,
		RunID:     event.RunID.String,
		DocPath:   event.DocPath.String,
		CommentID: event.CommentID.String,
		CreatedAt: event.CreatedAt,
	}
}

func (s *Service) BuildWorkspaceLogState(
	ctx context.Context,
	workspaceID string,
	limit int64,
) (WorkspaceLogState, error) {
	if limit <= 0 {
		limit = 20
	}
	events, err := s.queries.ListRecentWorkspaceLogEvents(
		ctx,
		db.ListRecentWorkspaceLogEventsParams{
			WorkspaceID: workspaceID,
			Limit:       limit,
		},
	)
	if err != nil {
		return WorkspaceLogState{}, err
	}
	workspace, _ := s.queries.GetWorkspace(ctx, workspaceID)
	state := WorkspaceLogState{Events: make([]WorkspaceLogItem, 0, len(events))}
	for _, event := range events {
		state.Events = append(state.Events, workspaceLogItem(event, workspace))
	}
	return state, nil
}

func workspaceLogEventTypes() []string {
	return []string{
		"artifact_created",
		"artifact_updated",
		"artifact_deleted",
		"session_imported",
		"session_import_diverged",
		"session_sync_failed",
		"run_started",
		"run_completed",
		"run_failed",
	}
}

func includeWorkspaceLogEvent(eventType string) bool {
	for _, included := range workspaceLogEventTypes() {
		if eventType == included {
			return true
		}
	}
	return false
}

func workspaceLogItem(event db.WorkspaceEvent, workspace db.Workspace) WorkspaceLogItem {
	category, label := workspaceEventCategoryAndLabel(event)
	threadID := strings.TrimSpace(event.ThreadID.String)
	return WorkspaceLogItem{
		ID:             event.ID,
		Type:           event.EventType,
		Category:       category,
		Label:          label,
		Detail:         workspaceLogDetail(event),
		ThreadID:       threadID,
		ThreadHref:     workspaceLogThreadHref(event, workspace, threadID),
		SessionID:      strings.TrimSpace(event.SessionID.String),
		RunID:          strings.TrimSpace(event.RunID.String),
		DocPath:        strings.TrimSpace(event.DocPath.String),
		CreatedAtLabel: formatWorkspaceEventTime(event.CreatedAt),
	}
}

func workspaceLogThreadHref(
	_ db.WorkspaceEvent,
	_ db.Workspace,
	threadID string,
) string {
	return threadHref(threadID)
}

func workspaceLogDetail(event db.WorkspaceEvent) string {
	if relPath := strings.TrimSpace(event.DocPath.String); relPath != "" {
		return relPath
	}
	if sessionID := strings.TrimSpace(event.SessionID.String); sessionID != "" {
		return "session " + sessionID
	}
	if runID := strings.TrimSpace(event.RunID.String); runID != "" {
		return "run " + runID
	}
	return ""
}

func workspaceEventCategoryAndLabel(event db.WorkspaceEvent) (string, string) {
	path := event.DocPath.String
	switch event.EventType {
	case "thread_created":
		return "thread", "Thread created"
	case "thread_selected":
		return "thread", "Thread selected"
	case "thread_forked":
		return "thread", "Thread forked"
	case "thread_attached":
		return "thread", "Thread attached"
	case "session_imported":
		return "session", "Session imported"
	case "session_import_diverged":
		return "session", "Session diverged"
	case "session_sync_failed":
		return "session", "Session sync failed"
	case "run_started":
		return "run", "Run started"
	case "run_checkpointed":
		return "run", "Run checkpointed"
	case "run_completed":
		return "run", "Run completed"
	case "run_failed":
		return "run", "Run failed"
	case "artifact_created":
		return "artifact", labelWithPath("Artifact created", path)
	case "artifact_updated":
		return "artifact", labelWithPath("Artifact updated", path)
	case "artifact_deleted":
		return "artifact", labelWithPath("Artifact deleted", path)
	case "artifact_selected":
		return "artifact", labelWithPath("Artifact selected", path)
	case "comment_created":
		return "comment", labelWithPath("Comment created", path)
	case "comment_replied":
		return "comment", labelWithPath("Comment replied", path)
	case "comment_resolved":
		return "comment", labelWithPath("Comment resolved", path)
	case "comment_reopened":
		return "comment", labelWithPath("Comment reopened", path)
	case "workflow_stage_changed":
		return "workflow", workflowEventLabel(event, "Workflow stage changed")
	case "workflow_policy_updated":
		return "workflow", "Workflow policy updated"
	case "workflow_review_waiting":
		return "workflow", workflowEventLabel(event, "Review waiting")
	case "workflow_approved":
		return "workflow", workflowEventLabel(event, "Workflow approved")
	case "workflow_rejected":
		return "workflow", workflowEventLabel(event, "Workflow rejected")
	default:
		return "workspace", humanizeWorkspaceEventType(event.EventType)
	}
}

func labelWithPath(label, relPath string) string {
	if strings.TrimSpace(relPath) == "" {
		return label
	}
	return label + ": " + relPath
}

func workflowEventLabel(event db.WorkspaceEvent, fallback string) string {
	if !event.PayloadJson.Valid || strings.TrimSpace(event.PayloadJson.String) == "" {
		return fallback
	}
	var payload struct {
		CurrentStep string `json:"current_step"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal([]byte(event.PayloadJson.String), &payload); err != nil {
		return fallback
	}
	if payload.CurrentStep == "" {
		return fallback
	}
	if payload.Status == "" {
		return "Workflow: " + payload.CurrentStep
	}
	return fmt.Sprintf("Workflow: %s (%s)", payload.CurrentStep, payload.Status)
}

func humanizeWorkspaceEventType(eventType string) string {
	eventType = strings.TrimSpace(strings.ReplaceAll(eventType, "_", " "))
	if eventType == "" {
		return "Workspace event"
	}
	return strings.ToUpper(eventType[:1]) + eventType[1:]
}

func (s *Service) BuildWorkspaceWorkflowState(
	ctx context.Context,
	workspace db.Workspace,
) (WorkspaceWorkflowState, error) {
	_ = ctx
	workflowType := WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType))
	if workflowType == "" {
		workflowType = WorkspaceWorkflowFreeform
	}
	state := WorkspaceWorkflowState{
		WorkspaceID: workspace.ID,
		Type:        workflowType,
		CurrentStep: "Freeform",
		Status:      "active",
	}
	if workflowType == WorkspaceWorkflowQRSPI {
		state.CurrentStep = "question"
		state.Status = "pending"
		state.Mermaid = defaultQRSPIWorkflowMermaid()
		if def, ok := s.workflowDefinition(wruntime.WorkflowID(workflowType)); ok {
			state.Mermaid = mermaidForDefinition(def)
		}
	}
	if !workspace.WorkflowStateJson.Valid ||
		strings.TrimSpace(workspace.WorkflowStateJson.String) == "" {
		return state, nil
	}
	raw := []byte(workspace.WorkflowStateJson.String)
	var runtimeState wruntime.State
	if err := json.Unmarshal(raw, &runtimeState); err == nil &&
		strings.TrimSpace(runtimeState.Type) != "" && runtimeState.CurrentNodeID != "" {
		state.Type = WorkspaceWorkflowType(runtimeState.Type)
		state.CurrentStep = string(runtimeState.CurrentNodeID)
		state.Status = string(runtimeState.Status)
		state.WaitingHuman = runtimeState.Status == wruntime.WorkspaceStatusWaitingHuman
		if runtimeState.HumanGate != nil {
			state.ReviewGate = string(runtimeState.HumanGate.To)
			state.HumanGateReason = runtimeState.HumanGate.Reason
		}
		if runtimeState.LastResult != nil {
			state.LastResultSummary = runtimeState.LastResult.Summary
			state.PrimaryArtifact = runtimeState.LastResult.PrimaryArtifact
			state.NextDisplay = runtimeState.LastResult.DisplayNext
		}
		state.BypassedNodes = bypassedWorkflowNodes(runtimeState)
		if state.Type == WorkspaceWorkflowQRSPI {
			policy, err := ProjectWorkspaceWorkflowPolicy(runtimeState)
			if err != nil {
				return WorkspaceWorkflowState{}, err
			}
			state.Policy = policy
			state.ActiveCwd = ProjectWorkspaceCwd(runtimeState, workspace)
		}
		if def, ok := s.workflowDefinition(wruntime.WorkflowID(runtimeState.Type)); ok {
			state.Mermaid = mermaidForDefinition(def)
			state.RuntimeNextStep = RuntimeNextNodeLabel(def, runtimeState)
			if runtimeState.LastResult != nil && state.Type == WorkspaceWorkflowQRSPI {
				result := wruntime.WorkflowResult{
					WorkflowType: runtimeState.Type,
					SourceNodeID: runtimeState.LastResult.SourceNodeID,
					Status: wruntime.ResultStatus(
						runtimeState.LastResult.Status,
					),
					Summary:         runtimeState.LastResult.Summary,
					PrimaryArtifact: runtimeState.LastResult.PrimaryArtifact,
					Artifacts:       runtimeState.LastResult.Artifacts,
					DisplayNext:     runtimeState.LastResult.DisplayNext,
					Workspace:       runtimeState.LastResult.Workspace,
					Outcome:         runtimeState.LastResult.Outcome,
					Raw:             runtimeState.LastResult.Raw,
				}
				card, err := ProjectQRSPIWorkflowCard(
					runtimeState,
					result,
					state.Policy,
					state.ActiveCwd,
					state.RuntimeNextStep,
					workspace.ID,
					strings.TrimSpace(workspace.SelectedThreadID.String),
				)
				if err != nil {
					return WorkspaceWorkflowState{}, err
				}
				state.LastResultCard = card
			}
		}
		state.Metadata = raw
		return state, nil
	}

	var stored struct {
		Type           WorkspaceWorkflowType `json:"type"`
		CurrentStep    string                `json:"current_step"`
		CurrentStepAlt string                `json:"currentStep"`
		Status         string                `json:"status"`
		ReviewGate     string                `json:"review_gate"`
		ReviewGateAlt  string                `json:"reviewGate"`
		Mermaid        string                `json:"mermaid"`
		Metadata       json.RawMessage       `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return WorkspaceWorkflowState{}, fmt.Errorf(
			"parse workspace workflow state: %w",
			err,
		)
	}
	if stored.Type != "" {
		state.Type = stored.Type
	}
	if stored.CurrentStep != "" {
		state.CurrentStep = stored.CurrentStep
	} else if stored.CurrentStepAlt != "" {
		state.CurrentStep = stored.CurrentStepAlt
	}
	if stored.Status != "" {
		state.Status = stored.Status
	}
	if stored.ReviewGate != "" {
		state.ReviewGate = stored.ReviewGate
	} else if stored.ReviewGateAlt != "" {
		state.ReviewGate = stored.ReviewGateAlt
	}
	if stored.Mermaid != "" {
		state.Mermaid = stored.Mermaid
	}
	if len(stored.Metadata) > 0 && string(stored.Metadata) != "null" {
		state.Metadata = stored.Metadata
	}
	return state, nil
}

func ProjectWorkspaceCwd(
	state wruntime.State,
	workspace db.Workspace,
) WorkspaceCwdProjection {
	executionCwd := strings.TrimSpace(state.ExecutionCwd)
	current := state.CurrentNodeID
	needsImplementation := current == qrspi.NodeImplement ||
		current == qrspi.NodeReviewImplementation ||
		current == qrspi.NodeHumanReviewImplementation ||
		current == qrspi.NodeDone
	if needsImplementation {
		if executionCwd == "" {
			return WorkspaceCwdProjection{
				Scope:       "implementation_workspace",
				Blocked:     true,
				BlockReason: "Implementation workspace is missing; run /q-workspace before implementation.",
			}
		}
		return WorkspaceCwdProjection{
			Path:  executionCwd,
			Label: filepath.Base(filepath.Clean(executionCwd)),
			Scope: "implementation_workspace",
		}
	}
	planning := ""
	if workspace.Cwd.Valid {
		planning = strings.TrimSpace(workspace.Cwd.String)
	}
	if planning == "" {
		planning = strings.TrimSpace(workspace.RootDocPath)
	}
	return WorkspaceCwdProjection{
		Path:  planning,
		Label: filepath.Base(filepath.Clean(planning)),
		Scope: "planning_checkout",
	}
}

func (s *Service) workflowDefinition(
	workflowID wruntime.WorkflowID,
) (wruntime.Definition, bool) {
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter == nil || adapter.Definitions == nil {
		return wruntime.Definition{}, false
	}
	return adapter.Definitions.Get(workflowID)
}

func bypassedWorkflowNodes(state wruntime.State) []string {
	bypassed := make([]string, 0)
	for id, node := range state.Nodes {
		if node.Status == wruntime.NodeStatusBypassed {
			bypassed = append(bypassed, string(id))
		}
	}
	sort.Strings(bypassed)
	return bypassed
}

func mermaidForDefinition(def wruntime.Definition) string {
	lines := make([]string, 0, 1+len(def.Edges))
	lines = append(lines, "flowchart LR")
	for _, edge := range def.Edges {
		from := def.Nodes[edge.From]
		to := def.Nodes[edge.To]
		lines = append(lines, fmt.Sprintf(
			"  %s[%s] --> %s[%s]",
			edge.From,
			mermaidNodeLabel(from),
			edge.To,
			mermaidNodeLabel(to),
		))
	}
	return strings.Join(lines, "\n")
}

func mermaidNodeLabel(node wruntime.Node) string {
	label := strings.TrimSpace(node.DisplayName)
	if label == "" {
		label = string(node.ID)
	}
	return strings.ReplaceAll(label, "\"", "'")
}

func workspaceArtifactHref(workspaceID, artifactRelPath string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	artifactRelPath = strings.TrimSpace(artifactRelPath)
	if workspaceID == "" || artifactRelPath == "" {
		return ""
	}
	values := url.Values{
		"artifact":       []string{artifactRelPath},
		"chat_workspace": []string{workspaceID},
	}
	return "/thoughts/?" + values.Encode()
}

func workspaceWorkflowLabel(state WorkspaceWorkflowState) string {
	if state.CurrentStep == "" && state.Status == "" {
		return string(state.Type)
	}
	parts := []string{string(state.Type)}
	if state.CurrentStep != "" {
		parts = append(parts, state.CurrentStep)
	}
	if state.Status != "" {
		parts = append(parts, state.Status)
	}
	return strings.Join(parts, " · ")
}

func defaultQRSPIWorkflowMermaid() string {
	return strings.Join([]string{
		"flowchart LR",
		"  question[Question] --> research[Research]",
		"  research --> design[Design]",
		"  design --> outline[Outline]",
		"  outline --> plan[Plan]",
		"  plan --> implement[Implement]",
		"  implement --> review[Review]",
	}, "\n")
}

func (s *Service) UpdateWorkspaceWorkflowState(
	ctx context.Context,
	workspaceID string,
	state WorkspaceWorkflowState,
) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	workflowType := state.Type
	if workflowType == "" {
		workflowType = WorkspaceWorkflowFreeform
	}
	payload := struct {
		Type        WorkspaceWorkflowType `json:"type"`
		CurrentStep string                `json:"current_step,omitempty"`
		Status      string                `json:"status,omitempty"`
		ReviewGate  string                `json:"review_gate,omitempty"`
		Mermaid     string                `json:"mermaid,omitempty"`
		Metadata    json.RawMessage       `json:"metadata,omitempty"`
	}{
		Type:        workflowType,
		CurrentStep: strings.TrimSpace(state.CurrentStep),
		Status:      strings.TrimSpace(state.Status),
		ReviewGate:  strings.TrimSpace(state.ReviewGate),
		Mermaid:     strings.TrimSpace(state.Mermaid),
		Metadata:    state.Metadata,
	}
	workflowJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal workspace workflow state: %w", err)
	}
	if err := s.queries.UpdateWorkspaceWorkflowState(
		ctx,
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspaceID,
			WorkflowType:      string(workflowType),
			WorkflowStateJson: nullString(string(workflowJSON)),
		},
	); err != nil {
		return err
	}
	eventKey := "workflow:" + payload.CurrentStep + ":" + payload.Status
	if strings.Trim(eventKey, ":") == "workflow" {
		eventKey = ""
	}
	event, err := s.AppendWorkspaceEvent(ctx, s.queries, AppendWorkspaceEventInput{
		WorkspaceID: workspaceID,
		EventType:   "workflow_stage_changed",
		ActorType:   "system",
		PayloadJSON: string(workflowJSON),
		EventKey:    eventKey,
	})
	if err != nil {
		return err
	}
	s.NotifyWorkspaceForEvent(event)
	return nil
}

func (s *Service) RedirectURLForThread(
	ctx context.Context,
	userEmail, threadID string,
) (string, error) {
	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        strings.TrimSpace(threadID),
		UserEmail: strings.TrimSpace(userEmail),
	})
	if err != nil {
		return "", err
	}
	return threadHref(thread.ID), nil
}
