package comments

import (
	"github.com/CoreyCole/vamos/pkg/db"
)

// CreateCommentRequest represents a comment creation request
type CreateCommentRequest struct {
	FilePath     string `json:"file_path"`
	CommentText  string `json:"comment_text"`
	SelectedText string `json:"selected_text"`
	StartLine    int    `json:"start_line"`
	StartColumn  int    `json:"start_column"`
	EndLine      int    `json:"end_line"`
	EndColumn    int    `json:"end_column"`
	SectionID    string `json:"section_id"` // Section identifier for layout
	HeadingHint  string `json:"heading_hint"`
}

// CreateReplyRequest represents a reply creation request
type CreateReplyRequest struct {
	CommentID string `json:"comment_id"`
	ReplyText string `json:"reply_text"`
}

// CommentWithReplies represents a comment and its replies
type CommentWithReplies struct {
	Comment db.WorkspaceDocComment        `json:"comment"`
	Replies []db.WorkspaceDocCommentReply `json:"replies"`
}

// GetCommentsResponse represents the response for getting comments
type GetCommentsResponse struct {
	Comments        []CommentWithReplies `json:"comments"`
	GitCommit       string               `json:"git_commit"`
	GitHubPermalink string               `json:"github_permalink"`
}

// GroupBySectionID groups comments by their section_id
func (r *GetCommentsResponse) GroupBySectionID() map[string][]CommentWithReplies {
	grouped := make(map[string][]CommentWithReplies)
	for _, comment := range r.Comments {
		sectionID := comment.Comment.SectionHint.String
		if !comment.Comment.SectionHint.Valid || sectionID == "" {
			sectionID = "unknown"
		}
		grouped[sectionID] = append(grouped[sectionID], comment)
	}
	return grouped
}
