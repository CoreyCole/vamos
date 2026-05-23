package comments

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	commentsv1 "github.com/CoreyCole/vamos/pkg/proto/comments/v1"
)

// formatNullTime formats a sql.NullTime as RFC3339 string, returns empty string if null
func formatNullTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

// CreateComment implements the Connect RPC handler for creating comments
func (s *Service) CreateComment(
	ctx context.Context,
	req *connect.Request[commentsv1.CreateCommentRequest],
) (*connect.Response[commentsv1.CreateCommentResponse], error) {
	// Validate session from cookie
	userEmail, err := s.validateSessionFromCookie(ctx, req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Create comment using internal service method
	comment, err := s.createCommentInternal(ctx, userEmail, CreateCommentRequest{
		FilePath:     req.Msg.FilePath,
		CommentText:  req.Msg.CommentText,
		SelectedText: req.Msg.SelectedText,
		StartLine:    int(req.Msg.StartLine),
		StartColumn:  int(req.Msg.StartColumn),
		EndLine:      int(req.Msg.EndLine),
		EndColumn:    int(req.Msg.EndColumn),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert to proto response
	return connect.NewResponse(&commentsv1.CreateCommentResponse{
		Comment: &commentsv1.Comment{
			Id:           comment.ID,
			FilePath:     comment.DocPath,
			GitCommit:    s.gitCommit,
			UserEmail:    comment.UserEmail,
			CommentText:  comment.CommentText,
			SelectedText: comment.SelectedText,
			StartLine:    comment.StartLine,
			StartColumn:  comment.StartColumn,
			EndLine:      comment.EndLine,
			EndColumn:    comment.EndColumn,
			CreatedAt:    comment.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    formatNullTime(comment.UpdatedAt),
		},
	}), nil
}

// CreateReply implements the Connect RPC handler for creating replies
func (s *Service) CreateReply(
	ctx context.Context,
	req *connect.Request[commentsv1.CreateReplyRequest],
) (*connect.Response[commentsv1.CreateReplyResponse], error) {
	// Validate session from cookie
	userEmail, err := s.validateSessionFromCookie(ctx, req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Create reply using internal service method
	reply, err := s.createReplyInternal(ctx, userEmail, CreateReplyRequest{
		CommentID: req.Msg.CommentId,
		ReplyText: req.Msg.ReplyText,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert to proto response
	return connect.NewResponse(&commentsv1.CreateReplyResponse{
		Reply: &commentsv1.Reply{
			Id:        reply.ID,
			CommentId: reply.CommentID,
			UserEmail: reply.UserEmail,
			ReplyText: reply.ReplyText,
			CreatedAt: reply.CreatedAt.Format(time.RFC3339),
			UpdatedAt: formatNullTime(reply.UpdatedAt),
		},
	}), nil
}

// GetCommentsForFile implements the Connect RPC handler for retrieving comments
func (s *Service) GetCommentsForFile(
	ctx context.Context,
	req *connect.Request[commentsv1.GetCommentsRequest],
) (*connect.Response[commentsv1.GetCommentsResponse], error) {
	// Get comments using internal service method
	response, err := s.GetCommentsForFileInternal(ctx, req.Msg.FilePath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert to proto response
	protoComments := make([]*commentsv1.CommentWithReplies, len(response.Comments))
	for i, item := range response.Comments {
		// Convert comment
		protoComment := &commentsv1.Comment{
			Id:           item.Comment.ID,
			FilePath:     item.Comment.DocPath,
			GitCommit:    s.gitCommit,
			UserEmail:    item.Comment.UserEmail,
			CommentText:  item.Comment.CommentText,
			SelectedText: item.Comment.SelectedText,
			StartLine:    item.Comment.StartLine,
			StartColumn:  item.Comment.StartColumn,
			EndLine:      item.Comment.EndLine,
			EndColumn:    item.Comment.EndColumn,
			CreatedAt:    item.Comment.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    formatNullTime(item.Comment.UpdatedAt),
		}

		// Convert replies
		protoReplies := make([]*commentsv1.Reply, len(item.Replies))
		for j, reply := range item.Replies {
			protoReplies[j] = &commentsv1.Reply{
				Id:        reply.ID,
				CommentId: reply.CommentID,
				UserEmail: reply.UserEmail,
				ReplyText: reply.ReplyText,
				CreatedAt: reply.CreatedAt.Format(time.RFC3339),
				UpdatedAt: formatNullTime(reply.UpdatedAt),
			}
		}

		protoComments[i] = &commentsv1.CommentWithReplies{
			Comment: protoComment,
			Replies: protoReplies,
		}
	}

	return connect.NewResponse(&commentsv1.GetCommentsResponse{
		Comments: protoComments,
	}), nil
}

// validateSessionFromCookie extracts the session cookie, validates it against
// the database, and returns the authenticated user's email.
func (s *Service) validateSessionFromCookie(
	ctx context.Context,
	headers http.Header,
) (string, error) {
	// Parse cookies from the raw header using net/http
	req := &http.Request{Header: headers}
	cookie, err := req.Cookie("thoughts_session")
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf("no session cookie")
	}

	// Validate session against database
	session, err := s.queries.GetSession(ctx, cookie.Value)
	if err != nil {
		return "", fmt.Errorf("invalid or expired session")
	}

	return session.UserEmail, nil
}
