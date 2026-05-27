package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

const ThoughtsChatContext = "chat"

type EmbeddedChatSelectionScope string

const (
	EmbeddedChatSelectionScopeGlobal    EmbeddedChatSelectionScope = "global"
	EmbeddedChatSelectionScopeFreeform  EmbeddedChatSelectionScope = "freeform"
	EmbeddedChatSelectionScopeWorkspace EmbeddedChatSelectionScope = "workspace"
)

type EmbeddedChatURLState struct {
	DocPath          string
	Context          string
	WorkspaceID      string
	ThreadID         string
	RunID            string
	WorkspaceContext markdown.DocumentWorkspaceContext
}

type EmbeddedChatSelection struct {
	WorkspaceID string
	ThreadID    string
	RunID       string
	ExplicitURL bool
	Scope       EmbeddedChatSelectionScope
}

type EmbeddedChatPanelArgs struct {
	DocPath            string
	WorkspaceID        string
	ThreadID           string
	RunID              string
	Transcript         TranscriptPaneState
	HasThread          bool
	PendingAttachments []AttachedPath
	StreamURL          string
	ComposerAction     string
	Cwd                string
	ThreadMetadata     ThreadMetadataView
}

type EmbeddedChatPatchInput struct {
	UserEmail   string
	DocPath     string
	WorkspaceID string
	ThreadID    string
	RunID       string
	AttachDoc   bool
}

type EmbeddedFreeformPanelArgs struct {
	ThreadID       string
	RunID          string
	Transcript     TranscriptPaneState
	HasThread      bool
	StreamURL      string
	ComposerAction string
	Cwd            string
	ThreadMetadata ThreadMetadataView
}

func ParseEmbeddedChatURL(c echo.Context) EmbeddedChatURLState {
	docPath := strings.TrimSpace(c.Param("*"))
	if docPath == "" {
		docPath = strings.TrimSpace(c.QueryParam("doc_path"))
	}
	docPath = markdown.CanonicalThoughtsDocPathLoose(docPath)
	return EmbeddedChatURLState{
		DocPath:     docPath,
		Context:     strings.TrimSpace(c.QueryParam("context")),
		WorkspaceID: strings.TrimSpace(c.QueryParam("chat_workspace")),
		ThreadID:    strings.TrimSpace(c.QueryParam("thread")),
		RunID:       strings.TrimSpace(c.QueryParam("run")),
	}
}

func BuildThoughtsChatDocURL(state EmbeddedChatURLState) string {
	values := url.Values{}
	values.Set("context", ThoughtsChatContext)
	threadID := strings.TrimSpace(state.ThreadID)
	if threadID == "" {
		if workspaceID := strings.TrimSpace(state.WorkspaceID); workspaceID != "" {
			values.Set("chat_workspace", workspaceID)
		}
	}
	if threadID != "" {
		values.Set("thread", threadID)
	}
	if runID := strings.TrimSpace(state.RunID); runID != "" {
		values.Set("run", runID)
	}
	return thoughtsDocRedirectURL(
		markdown.CanonicalThoughtsDocPathLoose(state.DocPath),
		values,
	)
}

func (s *Service) GetLastEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
) (EmbeddedChatSelection, error) {
	return s.lastEmbeddedChatSelectionForScope(
		ctx,
		userEmail,
		EmbeddedChatSelectionScopeGlobal,
	)
}

func (s *Service) LastWorkspaceEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
	workspaceID string,
) (EmbeddedChatSelection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return EmbeddedChatSelection{}, nil
	}
	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return EmbeddedChatSelection{}, err
	}
	if threadID := strings.TrimSpace(workspace.SelectedThreadID.String); threadID != "" {
		return EmbeddedChatSelection{
			WorkspaceID: workspace.ID,
			ThreadID:    threadID,
			Scope:       EmbeddedChatSelectionScopeWorkspace,
		}, nil
	}
	if strings.TrimSpace(workspace.CurrentSessionID.String) != "" {
		return EmbeddedChatSelection{
			WorkspaceID: workspace.ID,
			Scope:       EmbeddedChatSelectionScopeWorkspace,
		}, nil
	}
	row, err := s.queries.GetUserChatSelection(ctx, db.GetUserChatSelectionParams{
		UserEmail: strings.TrimSpace(userEmail),
		Scope:     string(EmbeddedChatSelectionScopeWorkspace),
		ScopeID:   workspace.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddedChatSelection{
				WorkspaceID: workspace.ID,
				Scope:       EmbeddedChatSelectionScopeWorkspace,
			}, nil
		}
		return EmbeddedChatSelection{}, err
	}
	return embeddedChatSelectionFromRow(row, EmbeddedChatSelectionScopeWorkspace), nil
}

func (s *Service) LastFreeformEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
) (EmbeddedChatSelection, error) {
	for _, scope := range []EmbeddedChatSelectionScope{
		EmbeddedChatSelectionScopeFreeform,
		EmbeddedChatSelectionScopeGlobal,
	} {
		selection, err := s.lastEmbeddedChatSelectionForScope(ctx, userEmail, scope)
		if err != nil {
			return EmbeddedChatSelection{}, err
		}
		if strings.TrimSpace(selection.WorkspaceID) == "" {
			continue
		}
		workspace, err := s.GetWorkspaceForUserOrTrustedImport(
			ctx,
			userEmail,
			selection.WorkspaceID,
		)
		if err != nil {
			return EmbeddedChatSelection{}, err
		}
		if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) == WorkspaceWorkflowFreeform {
			selection.Scope = scope
			return selection, nil
		}
	}
	threads, err := s.queries.ListAgentThreads(ctx, db.ListAgentThreadsParams{
		UserEmail: strings.TrimSpace(userEmail),
		Limit:     50,
	})
	if err != nil {
		return EmbeddedChatSelection{}, err
	}
	for _, thread := range threads {
		_, err := s.queries.GetPrimaryWorkspaceForThread(ctx, db.GetPrimaryWorkspaceForThreadParams{
			ThreadID:  thread.ID,
			UserEmail: thread.UserEmail,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddedChatSelection{
				ThreadID: thread.ID,
				Scope:    EmbeddedChatSelectionScopeFreeform,
			}, nil
		}
		if err != nil {
			return EmbeddedChatSelection{}, err
		}
	}
	return EmbeddedChatSelection{}, nil
}

func (s *Service) lastEmbeddedChatSelectionForScope(
	ctx context.Context,
	userEmail string,
	scope EmbeddedChatSelectionScope,
) (EmbeddedChatSelection, error) {
	userEmail = strings.TrimSpace(userEmail)
	if userEmail == "" {
		return EmbeddedChatSelection{}, nil
	}
	row, err := s.queries.GetLatestUserChatSelectionByScope(
		ctx,
		db.GetLatestUserChatSelectionByScopeParams{
			UserEmail: userEmail,
			Scope:     string(scope),
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddedChatSelection{}, nil
		}
		return EmbeddedChatSelection{}, err
	}
	return embeddedChatSelectionFromRow(row, scope), nil
}

func embeddedChatSelectionFromRow(
	row db.UserChatSelection,
	scope EmbeddedChatSelectionScope,
) EmbeddedChatSelection {
	return EmbeddedChatSelection{
		WorkspaceID: strings.TrimSpace(row.WorkspaceID),
		ThreadID:    strings.TrimSpace(row.ThreadID.String),
		RunID:       strings.TrimSpace(row.RunID.String),
		Scope:       scope,
	}
}

func (s *Service) PersistEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
	selection EmbeddedChatSelection,
) error {
	userEmail = strings.TrimSpace(userEmail)
	workspaceID := strings.TrimSpace(selection.WorkspaceID)
	if userEmail == "" || workspaceID == "" {
		return nil
	}
	scope := selection.Scope
	if scope == "" {
		scope = EmbeddedChatSelectionScopeGlobal
	}
	scopeID := ""
	if scope == EmbeddedChatSelectionScopeWorkspace {
		scopeID = workspaceID
	}
	_, err := s.queries.UpsertUserChatSelection(ctx, db.UpsertUserChatSelectionParams{
		UserEmail:   userEmail,
		Scope:       string(scope),
		ScopeID:     scopeID,
		WorkspaceID: workspaceID,
		ThreadID:    nullString(strings.TrimSpace(selection.ThreadID)),
		RunID:       nullString(strings.TrimSpace(selection.RunID)),
	})
	return err
}

func (s *Service) validateEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
	selection EmbeddedChatSelection,
) (EmbeddedChatSelection, error) {
	selection.WorkspaceID = strings.TrimSpace(selection.WorkspaceID)
	selection.ThreadID = strings.TrimSpace(selection.ThreadID)
	selection.RunID = strings.TrimSpace(selection.RunID)
	if selection.WorkspaceID == "" {
		return selection, nil
	}
	if _, err := s.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		selection.WorkspaceID,
	); err != nil {
		return EmbeddedChatSelection{}, err
	}
	if selection.ThreadID != "" {
		if _, err := s.sharedWorkspaceThread(
			ctx,
			selection.WorkspaceID,
			selection.ThreadID,
		); err != nil {
			if errors.Is(err, ErrThreadWorkspaceMismatch) {
				log.Printf(
					"workspace_error source=agentchat severity=warn workspace_id=%q thread_id=%q message=%q",
					selection.WorkspaceID,
					selection.ThreadID,
					err.Error(),
				)
				selection.ThreadID = ""
				selection.RunID = ""
				return selection, nil
			}
			return EmbeddedChatSelection{}, err
		}
	}
	return selection, nil
}

func (s *Service) ResolveEmbeddedChatSelection(
	ctx context.Context,
	userEmail string,
	state EmbeddedChatURLState,
) (EmbeddedChatSelection, error) {
	explicit := strings.TrimSpace(state.WorkspaceID) != "" ||
		strings.TrimSpace(state.ThreadID) != "" ||
		strings.TrimSpace(state.RunID) != ""
	if explicit {
		selection := EmbeddedChatSelection{
			WorkspaceID: strings.TrimSpace(state.WorkspaceID),
			ThreadID:    strings.TrimSpace(state.ThreadID),
			RunID:       strings.TrimSpace(state.RunID),
			ExplicitURL: true,
			Scope:       EmbeddedChatSelectionScopeGlobal,
		}
		return s.validateEmbeddedChatSelection(ctx, userEmail, selection)
	}
	if strings.TrimSpace(state.Context) != ThoughtsChatContext {
		return EmbeddedChatSelection{}, nil
	}
	if workspaceID := strings.TrimSpace(state.WorkspaceContext.WorkspaceID); workspaceID != "" {
		selection, err := s.LastWorkspaceEmbeddedChatSelection(ctx, userEmail, workspaceID)
		if err != nil {
			return EmbeddedChatSelection{}, err
		}
		return s.validateEmbeddedChatSelection(ctx, userEmail, selection)
	}
	selection, err := s.LastFreeformEmbeddedChatSelection(ctx, userEmail)
	if err != nil {
		return EmbeddedChatSelection{}, err
	}
	if strings.TrimSpace(selection.WorkspaceID) == "" {
		return EmbeddedChatSelection{}, nil
	}
	return s.validateEmbeddedChatSelection(ctx, userEmail, selection)
}

func (s *Service) RenderEmbeddedChatPanel(
	ctx context.Context,
	request markdown.EmbeddedChatRenderRequest,
) (templ.Component, markdown.EmbeddedChatURLReplacement, error) {
	state := EmbeddedChatURLState{
		DocPath:          request.DocPath,
		Context:          request.Context,
		WorkspaceID:      request.WorkspaceID,
		ThreadID:         request.ThreadID,
		RunID:            request.RunID,
		WorkspaceContext: request.WorkspaceContext,
	}
	selection, err := s.ResolveEmbeddedChatSelection(ctx, request.UserEmail, state)
	if err != nil {
		return nil, markdown.EmbeddedChatURLReplacement{}, err
	}
	if selection.WorkspaceID == "" && (selection.ThreadID != "" || selection.RunID != "") {
		args, err := s.BuildEmbeddedFreeformPanelArgs(
			ctx,
			request.UserEmail,
			selection.ThreadID,
			selection.RunID,
		)
		if err != nil {
			return nil, markdown.EmbeddedChatURLReplacement{}, err
		}
		return EmbeddedFreeformRightRailContent(args), markdown.EmbeddedChatURLReplacement{}, nil
	}
	if selection.WorkspaceID == "" {
		return EmbeddedFreeformRightRailContent(
			EmbeddedFreeformPanelArgs{
				ComposerAction: "@post('/thoughts/chat/freeform/send', {contentType: 'form'})",
				Cwd:            s.defaultCwd,
			},
		), markdown.EmbeddedChatURLReplacement{}, nil
	}
	workspace, err := s.GetWorkspaceForUserOrTrustedImport(
		ctx,
		request.UserEmail,
		selection.WorkspaceID,
	)
	if err != nil {
		return nil, markdown.EmbeddedChatURLReplacement{}, err
	}
	if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) == WorkspaceWorkflowFreeform {
		args, err := s.BuildEmbeddedFreeformPanelArgs(
			ctx,
			request.UserEmail,
			selection.ThreadID,
			selection.RunID,
		)
		if err != nil {
			return nil, markdown.EmbeddedChatURLReplacement{}, err
		}
		replacement := markdown.EmbeddedChatURLReplacement{}
		if !selection.ExplicitURL &&
			strings.TrimSpace(request.Context) == ThoughtsChatContext {
			replacement.URL = BuildThoughtsChatDocURL(EmbeddedChatURLState{
				DocPath:     request.DocPath,
				WorkspaceID: selection.WorkspaceID,
				ThreadID:    args.ThreadID,
				RunID:       args.RunID,
			})
		}
		return EmbeddedFreeformRightRailContent(args), replacement, nil
	}
	args, err := s.BuildEmbeddedChatPanelArgs(ctx, EmbeddedChatPatchInput{
		UserEmail:   request.UserEmail,
		DocPath:     request.DocPath,
		WorkspaceID: selection.WorkspaceID,
		ThreadID:    selection.ThreadID,
		RunID:       selection.RunID,
		AttachDoc:   request.AttachDoc,
	})
	if err != nil {
		return nil, markdown.EmbeddedChatURLReplacement{}, err
	}
	replacement := markdown.EmbeddedChatURLReplacement{}
	if !selection.ExplicitURL &&
		strings.TrimSpace(request.Context) == ThoughtsChatContext {
		replacement.URL = BuildThoughtsChatDocURL(EmbeddedChatURLState{
			DocPath:     request.DocPath,
			WorkspaceID: selection.WorkspaceID,
			ThreadID:    selection.ThreadID,
			RunID:       selection.RunID,
		})
	}
	return EmbeddedChatRightRailContent(args), replacement, nil
}

func (s *Service) BuildEmbeddedFreeformPanelArgs(
	ctx context.Context,
	userEmail string,
	threadID string,
	runID string,
) (EmbeddedFreeformPanelArgs, error) {
	args, err := s.BuildPageArgs(ctx, userEmail, threadID, runID, "", s.defaultCwd)
	if err != nil {
		return EmbeddedFreeformPanelArgs{}, err
	}
	resolvedThreadID := getThreadID(args.CurrentThread)
	resolvedRunID := getRunID(args.ActiveRun)
	composerAction := "@post('/thoughts/chat/freeform/send', {contentType: 'form'})"
	if resolvedThreadID != "" {
		composerAction = "@post('" + thoughtsThreadChatAction(resolvedThreadID, "resume") + "', {contentType: 'form'})"
	}
	streamURL := ""
	if resolvedThreadID != "" {
		values := url.Values{}
		if resolvedRunID != "" {
			values.Set("run", resolvedRunID)
		}
		values.Set("since", "0")
		streamURL = thoughtsThreadChatAction(resolvedThreadID, "stream") + "?" + values.Encode()
	}
	return EmbeddedFreeformPanelArgs{
		ThreadID:       resolvedThreadID,
		RunID:          resolvedRunID,
		Transcript:     args.Transcript,
		HasThread:      args.CurrentThread != nil,
		StreamURL:      streamURL,
		ComposerAction: composerAction,
		Cwd:            args.Cwd,
		ThreadMetadata: args.ThreadMetadata,
	}, nil
}

func embeddedFreeformModeLabel(metadata ThreadMetadataView) string {
	if metadata.Primary != nil {
		return "Workspace-backed thread"
	}
	return "Freeform chat"
}

func embeddedWorkspaceModeLabel(metadata ThreadMetadataView) string {
	if metadata.Primary != nil {
		return "Workspace-backed thread"
	}
	return "Workspace chat"
}

func attachedPathContains(paths []AttachedPath, docPath string) bool {
	docPath = markdown.CanonicalThoughtsDocPathLoose(docPath)
	for _, attached := range paths {
		if markdown.CanonicalThoughtsDocPathLoose(attached.Path) == docPath {
			return true
		}
	}
	return false
}

func (s *Service) BuildEmbeddedChatPanelArgs(
	ctx context.Context,
	input EmbeddedChatPatchInput,
) (EmbeddedChatPanelArgs, error) {
	input.DocPath = markdown.CanonicalThoughtsDocPathLoose(input.DocPath)
	pageArgs, err := s.BuildWorkspacePageArgs(ctx, BuildWorkspacePageInput{
		UserEmail:   input.UserEmail,
		WorkspaceID: strings.TrimSpace(input.WorkspaceID),
		ThreadID:    strings.TrimSpace(input.ThreadID),
		RunID:       strings.TrimSpace(input.RunID),
		DocRelPath:  input.DocPath,
		DocPath:     input.DocPath,
	})
	if err != nil {
		return EmbeddedChatPanelArgs{}, err
	}
	threadID := ""
	if pageArgs.Projection.SelectedThread != nil {
		threadID = pageArgs.Projection.SelectedThread.ID
	}
	runID := strings.TrimSpace(input.RunID)
	if pageArgs.Projection.ActiveRun != nil {
		runID = pageArgs.Projection.ActiveRun.ID
	}
	attachments := []AttachedPath{}
	if input.AttachDoc && input.DocPath != "" {
		attachmentPath := "thoughts/" + strings.TrimPrefix(input.DocPath, "thoughts/")
		attachments = []AttachedPath{
			{Path: attachmentPath, Basename: filepath.Base(input.DocPath)},
		}
	}
	threadMetadata := ThreadMetadataView{}
	if pageArgs.Projection.SelectedThread != nil {
		workspaceContext, err := s.GetThreadWorkspaceContext(
			ctx,
			input.UserEmail,
			pageArgs.Projection.SelectedThread.ID,
		)
		if err != nil {
			return EmbeddedChatPanelArgs{}, err
		}
		threadMetadata = s.BuildThreadMetadataView(ctx, workspaceContext, pageArgs.Projection.Workspace.Cwd.String)
	}
	return EmbeddedChatPanelArgs{
		DocPath:            input.DocPath,
		WorkspaceID:        pageArgs.WorkspaceID,
		ThreadID:           threadID,
		RunID:              runID,
		Transcript:         pageArgs.Projection.Transcript,
		HasThread:          threadID != "",
		PendingAttachments: attachments,
		ThreadMetadata:     threadMetadata,
		StreamURL: embeddedWorkspaceStreamURL(
			pageArgs.WorkspaceID,
			threadID,
			runID,
			input.DocPath,
			pageArgs.Cursor,
		),
		ComposerAction: embeddedWorkspaceSendAction(
			pageArgs.WorkspaceID,
			threadID,
			threadID != "",
		),
		Cwd: pageArgs.Projection.Workspace.Cwd.String,
	}, nil
}
