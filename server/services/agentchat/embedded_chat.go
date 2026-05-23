package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

const ThoughtsChatContext = "chat"

type EmbeddedChatURLState struct {
	DocPath     string
	Context     string
	WorkspaceID string
	ThreadID    string
	RunID       string
}

type EmbeddedChatSelection struct {
	WorkspaceID string
	ThreadID    string
	RunID       string
	ExplicitURL bool
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
	if workspaceID := strings.TrimSpace(state.WorkspaceID); workspaceID != "" {
		values.Set("chat_workspace", workspaceID)
	}
	if threadID := strings.TrimSpace(state.ThreadID); threadID != "" {
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
	userEmail = strings.TrimSpace(userEmail)
	if userEmail == "" {
		return EmbeddedChatSelection{}, nil
	}
	row, err := s.queries.GetUserChatSelection(ctx, userEmail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EmbeddedChatSelection{}, nil
		}
		return EmbeddedChatSelection{}, err
	}
	return EmbeddedChatSelection{
		WorkspaceID: strings.TrimSpace(row.WorkspaceID),
		ThreadID:    strings.TrimSpace(row.ThreadID.String),
		RunID:       strings.TrimSpace(row.RunID.String),
	}, nil
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
	_, err := s.queries.UpsertUserChatSelection(ctx, db.UpsertUserChatSelectionParams{
		UserEmail:   userEmail,
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
		}
		return s.validateEmbeddedChatSelection(ctx, userEmail, selection)
	}
	if strings.TrimSpace(state.Context) != ThoughtsChatContext {
		return EmbeddedChatSelection{}, nil
	}
	selection, err := s.GetLastEmbeddedChatSelection(ctx, userEmail)
	if err != nil || strings.TrimSpace(selection.WorkspaceID) == "" {
		return selection, err
	}
	selection.ExplicitURL = false
	return s.validateEmbeddedChatSelection(ctx, userEmail, selection)
}

func (s *Service) RenderEmbeddedChatPanel(
	ctx context.Context,
	request markdown.EmbeddedChatRenderRequest,
) (templ.Component, markdown.EmbeddedChatURLReplacement, error) {
	state := EmbeddedChatURLState{
		DocPath:     request.DocPath,
		Context:     request.Context,
		WorkspaceID: request.WorkspaceID,
		ThreadID:    request.ThreadID,
		RunID:       request.RunID,
	}
	selection, err := s.ResolveEmbeddedChatSelection(ctx, request.UserEmail, state)
	if err != nil {
		return nil, markdown.EmbeddedChatURLReplacement{}, err
	}
	if selection.WorkspaceID == "" {
		return EmbeddedFreeformRightRailContent(
			EmbeddedFreeformPanelArgs{
				ComposerAction: "@post('/thoughts/chat/freeform/send', {contentType: 'form'})",
				Cwd:            s.defaultCwd,
			},
		), markdown.EmbeddedChatURLReplacement{}, nil
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
		composerAction = "@post('/thoughts/chat/freeform/resume', {contentType: 'form'})"
	}
	streamURL := ""
	if resolvedThreadID != "" {
		values := url.Values{}
		values.Set("thread", resolvedThreadID)
		if resolvedRunID != "" {
			values.Set("run", resolvedRunID)
		}
		values.Set("since", "0")
		streamURL = "/agent-chat/stream?" + values.Encode()
	}
	return EmbeddedFreeformPanelArgs{
		ThreadID:       resolvedThreadID,
		RunID:          resolvedRunID,
		Transcript:     args.Transcript,
		HasThread:      args.CurrentThread != nil,
		StreamURL:      streamURL,
		ComposerAction: composerAction,
		Cwd:            args.Cwd,
	}, nil
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
	return EmbeddedChatPanelArgs{
		DocPath:            input.DocPath,
		WorkspaceID:        pageArgs.WorkspaceID,
		ThreadID:           threadID,
		RunID:              runID,
		Transcript:         pageArgs.Projection.Transcript,
		HasThread:          threadID != "",
		PendingAttachments: attachments,
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
