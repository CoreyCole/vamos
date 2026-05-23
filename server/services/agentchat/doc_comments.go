package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

func (s *Service) BuildWorkspaceDocCommentProjection(
	ctx context.Context,
	workspace db.Workspace,
	relPath string,
	includeResolved bool,
) ([]WorkspaceDocCommentView, error) {
	rel, err := ValidateWorkspaceRelPath(workspace.RootDocPath, relPath)
	if err != nil {
		return nil, err
	}
	documentPath, err := DocPathFromRoot(workspace.RootDocPath, rel)
	if err != nil {
		return nil, err
	}
	includeResolvedParam := int64(0)
	if includeResolved {
		includeResolvedParam = 1
	}
	rows, err := s.queries.ListDocumentComments(
		ctx,
		db.ListDocumentCommentsParams{
			DocPath:         documentPath,
			IncludeResolved: includeResolvedParam,
		},
	)
	if err != nil {
		return nil, err
	}
	views, err := s.hydrateWorkspaceDocCommentReplies(ctx, s.queries, rows)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(
		filepath.Join(workspace.RootDocPath, filepath.FromSlash(rel)),
	)
	if err != nil {
		if os.IsNotExist(err) {
			return AnchorWorkspaceDocComments(nil, nil, views), nil
		}
		return nil, fmt.Errorf("read doc for comments: %w", err)
	}
	sections := s.renderer.RenderToSections(content)
	return AnchorWorkspaceDocComments(content, sections, views), nil
}

func (s *Service) ListWorkspaceDocComments(
	ctx context.Context,
	userEmail, workspaceID, relPath string,
	includeResolved bool,
) ([]WorkspaceDocCommentView, error) {
	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.BuildWorkspaceDocCommentProjection(ctx, workspace, relPath, includeResolved)
}

func (s *Service) LoadWorkspaceDocCommentTarget(
	ctx context.Context,
	userEmail string,
	refresh WorkspaceDocCommentTargetRefresh,
) (DocSectionCommentsArgs, error) {
	workspace, err := s.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		refresh.WorkspaceID,
	)
	if err != nil {
		return DocSectionCommentsArgs{}, err
	}
	rel, err := workspaceRelPathFromCommentInput(
		workspace.RootDocPath,
		refresh.DocRelPath,
		refresh.DocPath,
	)
	if err != nil {
		return DocSectionCommentsArgs{}, err
	}
	documentPath, err := DocPathFromRoot(workspace.RootDocPath, rel)
	if err != nil {
		return DocSectionCommentsArgs{}, err
	}
	comments, err := s.BuildWorkspaceDocCommentProjection(ctx, workspace, rel, true)
	if err != nil {
		return DocSectionCommentsArgs{}, err
	}
	section := strings.TrimSpace(refresh.SectionHint)
	if section == "" {
		section = "document"
	}
	return DocSectionCommentsArgs{
		WorkspaceID: workspace.ID,
		DocRelPath:  rel,
		DocPath:     documentPath,
		SectionID:   section,
		HeadingHint: refresh.HeadingHint,
		UserEmail:   userEmail,
		Comments: commentsForDocSection(
			comments,
			section,
			refresh.HeadingHint,
		),
	}, nil
}

func (s *Service) ListAgentFacingWorkspaceDocComments(
	ctx context.Context,
	userEmail, workspaceID, relPath string,
	includeResolved bool,
) ([]WorkspaceDocCommentView, error) {
	return s.ListWorkspaceDocComments(
		ctx,
		userEmail,
		workspaceID,
		relPath,
		includeResolved,
	)
}

func (s *Service) GetAgentFacingWorkspaceDocComment(
	ctx context.Context,
	userEmail, workspaceID, commentID string,
) (WorkspaceDocCommentView, error) {
	if _, err := s.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		workspaceID,
	); err != nil {
		return WorkspaceDocCommentView{}, err
	}
	comment, err := s.queries.GetDocumentComment(ctx, strings.TrimSpace(commentID))
	if err != nil {
		return WorkspaceDocCommentView{}, err
	}
	views, err := s.hydrateWorkspaceDocCommentReplies(
		ctx,
		s.queries,
		[]db.WorkspaceDocComment{comment},
	)
	if err != nil {
		return WorkspaceDocCommentView{}, err
	}
	if len(views) == 0 {
		return WorkspaceDocCommentView{}, sql.ErrNoRows
	}
	return views[0], nil
}

func workspaceRelPathFromCommentInput(
	rootPath, docRelPath, documentPath string,
) (string, error) {
	if strings.TrimSpace(docRelPath) != "" {
		return ValidateWorkspaceRelPath(rootPath, docRelPath)
	}
	documentPath = strings.TrimSpace(documentPath)
	if documentPath == "" {
		return "", errors.New("doc path is required")
	}
	if !strings.HasPrefix(filepath.ToSlash(filepath.Clean(documentPath)), "thoughts/") {
		return ValidateWorkspaceRelPath(rootPath, documentPath)
	}
	return RelPathFromDocPath(rootPath, documentPath)
}

func (s *Service) CreateWorkspaceDocComment(
	ctx context.Context,
	input CreateWorkspaceDocCommentInput,
) (db.WorkspaceDocComment, error) {
	workspace, err := s.queries.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return db.WorkspaceDocComment{}, err
	}
	rel, err := workspaceRelPathFromCommentInput(
		workspace.RootDocPath,
		input.DocRelPath,
		input.DocPath,
	)
	if err != nil {
		return db.WorkspaceDocComment{}, err
	}
	commentText := strings.TrimSpace(input.CommentText)
	if commentText == "" {
		return db.WorkspaceDocComment{}, errors.New("comment_text is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.WorkspaceDocComment{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	commentID := uuid.NewString()
	documentPath, err := DocPathFromRoot(workspace.RootDocPath, rel)
	if err != nil {
		return db.WorkspaceDocComment{}, err
	}
	comment, err := q.CreateDocumentComment(ctx, db.CreateDocumentCommentParams{
		ID:            commentID,
		WorkspaceRoot: workspace.RootDocPath,
		WorkspaceID:   nullString(workspace.ID),
		DocPath:       documentPath,
		UserEmail:     strings.TrimSpace(input.UserEmail),
		CommentText:   commentText,
		SelectedText:  strings.TrimSpace(input.Anchor.SelectedText),
		SectionHint:   nullString(input.Anchor.SectionHint),
		HeadingHint:   nullString(input.Anchor.HeadingHint),
		StartLine:     int64(input.Anchor.StartLine),
		StartColumn:   int64(input.Anchor.StartColumn),
		EndLine:       int64(input.Anchor.EndLine),
		EndColumn:     int64(input.Anchor.EndColumn),
	})
	if err != nil {
		return db.WorkspaceDocComment{}, err
	}
	if _, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   "comment_created",
		ActorEmail:  input.UserEmail,
		ActorType:   "user",
		DocPath:     documentPath,
		CommentID:   comment.ID,
		EventKey:    "comment_created:" + comment.ID,
	}); err != nil {
		return db.WorkspaceDocComment{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.WorkspaceDocComment{}, err
	}
	s.notifyWorkspaceDocCommentChanged(workspace.ID)
	return comment, nil
}

func (s *Service) ReplyToWorkspaceDocComment(
	ctx context.Context,
	input ReplyWorkspaceDocCommentInput,
) (db.WorkspaceDocCommentReply, error) {
	replyText := strings.TrimSpace(input.ReplyText)
	if replyText == "" {
		return db.WorkspaceDocCommentReply{}, errors.New("reply_text is required")
	}
	actorType := strings.TrimSpace(input.ActorType)
	if actorType == "" {
		actorType = "user"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.WorkspaceDocCommentReply{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	comment, err := q.GetDocumentComment(ctx, strings.TrimSpace(input.CommentID))
	if err != nil {
		return db.WorkspaceDocCommentReply{}, err
	}

	replyEventKey := ""
	if requestID := strings.TrimSpace(input.RequestID); requestID != "" {
		replyEventKey = "comment_replied:" + comment.ID + ":" + requestID
		if _, err := q.GetWorkspaceEventByKey(ctx, db.GetWorkspaceEventByKeyParams{
			WorkspaceID: input.WorkspaceID,
			EventKey:    nullString(replyEventKey),
		}); err == nil {
			return db.WorkspaceDocCommentReply{}, tx.Commit()
		} else if !errors.Is(err, sql.ErrNoRows) {
			return db.WorkspaceDocCommentReply{}, err
		}
	}

	reply, err := q.CreateDocumentCommentReply(
		ctx,
		db.CreateDocumentCommentReplyParams{
			ID:        uuid.NewString(),
			CommentID: comment.ID,
			UserEmail: strings.TrimSpace(input.UserEmail),
			ActorType: actorType,
			ReplyText: replyText,
		},
	)
	if err != nil {
		return db.WorkspaceDocCommentReply{}, err
	}
	if replyEventKey == "" {
		replyEventKey = "comment_replied:" + reply.ID
	}
	if _, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: input.WorkspaceID,
		EventType:   "comment_replied",
		ActorEmail:  input.UserEmail,
		ActorType:   actorType,
		DocPath:     comment.DocPath,
		CommentID:   comment.ID,
		EventKey:    replyEventKey,
	}); err != nil {
		return db.WorkspaceDocCommentReply{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.WorkspaceDocCommentReply{}, err
	}
	s.notifyWorkspaceDocCommentChanged(input.WorkspaceID, "comment_replied")
	return reply, nil
}

func (s *Service) ResolveWorkspaceDocComment(
	ctx context.Context,
	userEmail, workspaceID, commentID, actorEmail, actorType string,
) error {
	return s.ResolveWorkspaceDocCommentWithRequest(
		ctx,
		userEmail,
		workspaceID,
		commentID,
		actorEmail,
		actorType,
		"",
	)
}

func (s *Service) ResolveWorkspaceDocCommentWithRequest(
	ctx context.Context,
	userEmail, workspaceID, commentID, actorEmail, actorType, requestID string,
) error {
	_ = userEmail
	return s.setWorkspaceDocCommentResolved(ctx, CommentActionRequest{
		WorkspaceID: workspaceID,
		CommentID:   commentID,
		ActorEmail:  actorEmail,
		ActorType:   actorType,
		RequestID:   requestID,
		Resolved:    true,
		EventType:   "comment_resolved",
	})
}

func (s *Service) ReopenWorkspaceDocComment(
	ctx context.Context,
	userEmail, workspaceID, commentID, actorEmail, actorType string,
) error {
	return s.ReopenWorkspaceDocCommentWithRequest(
		ctx,
		userEmail,
		workspaceID,
		commentID,
		actorEmail,
		actorType,
		"",
	)
}

func (s *Service) ReopenWorkspaceDocCommentWithRequest(
	ctx context.Context,
	userEmail, workspaceID, commentID, actorEmail, actorType, requestID string,
) error {
	_ = userEmail
	return s.setWorkspaceDocCommentResolved(ctx, CommentActionRequest{
		WorkspaceID: workspaceID,
		CommentID:   commentID,
		ActorEmail:  actorEmail,
		ActorType:   actorType,
		RequestID:   requestID,
		Resolved:    false,
		EventType:   "comment_reopened",
	})
}

func (s *Service) AgentReplyAndMaybeResolveWorkspaceDocComment(
	ctx context.Context,
	input AgentCommentActionInput,
) error {
	if input.Resolve && !input.DocUpdated && !input.ArtifactUpdated &&
		!input.NoChangeDecision {
		return errors.New("agent resolve requires doc_updated or no_change_decision")
	}
	if strings.TrimSpace(input.ReplyText) == "" {
		return errors.New("reply_text is required")
	}
	requestID := strings.TrimSpace(input.RequestID)
	replyRequestID := requestID
	if replyRequestID != "" {
		replyRequestID += ":reply"
	}
	if _, err := s.ReplyToWorkspaceDocComment(ctx, ReplyWorkspaceDocCommentInput{
		WorkspaceID: input.WorkspaceID,
		CommentID:   input.CommentID,
		UserEmail:   input.UserEmail,
		ActorType:   "agent",
		ReplyText:   input.ReplyText,
		RequestID:   replyRequestID,
	}); err != nil {
		return err
	}
	if !input.Resolve {
		return nil
	}
	payload, err := json.Marshal(map[string]bool{
		"doc_updated":        input.DocUpdated,
		"no_change_decision": input.NoChangeDecision,
	})
	if err != nil {
		return err
	}
	return s.setWorkspaceDocCommentResolved(ctx, CommentActionRequest{
		WorkspaceID: input.WorkspaceID,
		CommentID:   input.CommentID,
		ActorEmail:  input.UserEmail,
		ActorType:   "agent",
		RequestID:   requestID,
		Resolved:    true,
		EventType:   "comment_resolved",
		PayloadJSON: string(payload),
	})
}

func commentActionEventKey(eventType, commentID, requestID string) string {
	eventType = strings.TrimSpace(eventType)
	commentID = strings.TrimSpace(commentID)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return eventType + ":" + commentID + ":" + requestID
}

func (s *Service) setWorkspaceDocCommentResolved(
	ctx context.Context,
	input CommentActionRequest,
) error {
	actorType := strings.TrimSpace(input.ActorType)
	if actorType == "" {
		actorType = "user"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)
	comment, err := q.GetDocumentComment(ctx, strings.TrimSpace(input.CommentID))
	if err != nil {
		return err
	}
	if comment.Resolved == input.Resolved {
		return tx.Commit()
	}
	if input.Resolved {
		err = q.ResolveDocumentComment(ctx, db.ResolveDocumentCommentParams{
			ID:                comment.ID,
			ResolvedBy:        nullString(input.ActorEmail),
			ResolvedActorType: nullString(actorType),
		})
	} else {
		err = q.ReopenDocumentComment(ctx, comment.ID)
	}
	if err != nil {
		return err
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		if input.Resolved {
			eventType = "comment_resolved"
		} else {
			eventType = "comment_reopened"
		}
	}
	if _, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: input.WorkspaceID,
		EventType:   eventType,
		ActorEmail:  input.ActorEmail,
		ActorType:   actorType,
		DocPath:     comment.DocPath,
		CommentID:   comment.ID,
		PayloadJSON: input.PayloadJSON,
		EventKey:    commentActionEventKey(eventType, comment.ID, input.RequestID),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notifyWorkspaceDocCommentChanged(input.WorkspaceID, eventType)
	return nil
}

func (s *Service) hydrateWorkspaceDocCommentReplies(
	ctx context.Context,
	q *db.Queries,
	comments []db.WorkspaceDocComment,
) ([]WorkspaceDocCommentView, error) {
	views := make([]WorkspaceDocCommentView, 0, len(comments))
	for _, comment := range comments {
		replies, err := q.ListDocumentCommentReplies(ctx, comment.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, WorkspaceDocCommentView{
			Comment:     comment,
			Replies:     replies,
			AnchorState: "unanchored",
			MatchIndex:  -1,
		})
	}
	return views, nil
}

func AnchorWorkspaceDocComments(
	markdownText []byte,
	sections []markdown.Section,
	comments []WorkspaceDocCommentView,
) []WorkspaceDocCommentView {
	_ = sections
	text := string(markdownText)
	for i := range comments {
		quote := strings.TrimSpace(comments[i].Comment.SelectedText)
		if quote == "" {
			comments[i].AnchorState = "unanchored"
			comments[i].MatchIndex = -1
			continue
		}
		idx := strings.Index(text, quote)
		if idx >= 0 {
			comments[i].AnchorState = "inline"
			comments[i].MatchIndex = idx
		} else {
			comments[i].AnchorState = "stale"
			comments[i].MatchIndex = -1
		}
	}
	sort.SliceStable(comments, func(i, j int) bool {
		left := docCommentSortRank(comments[i])
		right := docCommentSortRank(comments[j])
		if left != right {
			return left < right
		}
		if comments[i].MatchIndex != comments[j].MatchIndex {
			return comments[i].MatchIndex < comments[j].MatchIndex
		}
		return comments[i].Comment.CreatedAt.Before(comments[j].Comment.CreatedAt)
	})
	return comments
}

const docCommentSortRankInline = 2

func docCommentSortRank(view WorkspaceDocCommentView) int {
	switch view.AnchorState {
	case "stale":
		return 0
	case "unanchored":
		return 1
	default:
		return docCommentSortRankInline
	}
}

func (s *Service) notifyWorkspaceDocCommentChanged(
	workspaceID string,
	eventType ...string,
) {
	typ := "comment_created"
	if len(eventType) > 0 && strings.TrimSpace(eventType[0]) != "" {
		typ = strings.TrimSpace(eventType[0])
	}
	s.NotifyWorkspaceForEvent(db.WorkspaceEvent{
		WorkspaceID: strings.TrimSpace(workspaceID),
		EventType:   typ,
	})
}
