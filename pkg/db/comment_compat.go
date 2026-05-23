package db

import (
	"context"
	"database/sql"
)

type GetWorkspaceDocCommentForWorkspaceParams struct {
	ID          string         `json:"id"`
	WorkspaceID sql.NullString `json:"workspace_id"`
}

func (q *Queries) GetWorkspaceDocCommentForWorkspace(
	ctx context.Context,
	arg GetWorkspaceDocCommentForWorkspaceParams,
) (WorkspaceDocComment, error) {
	comment, err := q.GetDocumentComment(ctx, arg.ID)
	if err != nil {
		return WorkspaceDocComment{}, err
	}
	if arg.WorkspaceID.Valid && comment.WorkspaceID.Valid &&
		comment.WorkspaceID.String != arg.WorkspaceID.String {
		return WorkspaceDocComment{}, sql.ErrNoRows
	}
	return comment, nil
}

func (q *Queries) ListWorkspaceDocCommentReplies(
	ctx context.Context,
	commentID string,
) ([]WorkspaceDocCommentReply, error) {
	return q.ListDocumentCommentReplies(ctx, commentID)
}
