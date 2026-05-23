package markdown

import (
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

func thoughtsCommentThreads(
	items []comments.CommentWithReplies,
) []commentui.CommentThreadView {
	sources := make([]commentui.ThreadSource, 0, len(items))
	for _, item := range items {
		sectionID := item.Comment.SectionHint.String
		if !item.Comment.SectionHint.Valid || sectionID == "" {
			sectionID = "document"
		}
		headingHint := ""
		if item.Comment.HeadingHint.Valid {
			headingHint = item.Comment.HeadingHint.String
		}
		replies := make([]commentui.ReplySource, 0, len(item.Replies))
		for _, reply := range item.Replies {
			replies = append(replies, commentui.ReplySource{
				AuthorEmail: reply.UserEmail,
				CreatedAt:   reply.CreatedAt,
				Body:        reply.ReplyText,
			})
		}
		sources = append(sources, commentui.ThreadSource{
			ID:           item.Comment.ID,
			AuthorEmail:  item.Comment.UserEmail,
			CreatedAt:    item.Comment.CreatedAt,
			Body:         item.Comment.CommentText,
			SelectedText: item.Comment.SelectedText,
			SectionID:    sectionID,
			HeadingHint:  headingHint,
			Resolved:     item.Comment.Resolved,
			Replies:      replies,
			HiddenFields: map[string]string{
				"comment_id": item.Comment.ID,
				"doc_path":   item.Comment.DocPath,
			},
		})
	}
	return commentui.BuildThreadViews(sources)
}

func commentSectionsFromMarkdown(sections []Section) []commentui.CommentSectionView {
	out := make([]commentui.CommentSectionView, 0, len(sections))
	for _, section := range sections {
		out = append(out, commentui.CommentSectionView{
			ID:          section.ID,
			HeadingHTML: section.HeadingHTML,
			BodyHTML:    section.BodyHTML,
			HTMLContent: section.HTMLContent,
			LineStart:   section.LineStart,
			LineEnd:     section.LineEnd,
			Title:       section.Title,
		})
	}
	return out
}

func commentFrontmatterFromMarkdown(fm *Frontmatter) *commentui.CommentFrontmatterView {
	if fm == nil {
		return nil
	}
	return &commentui.CommentFrontmatterView{
		Date:          fm.Date,
		Researcher:    fm.Researcher,
		GitCommit:     fm.GitCommit,
		Branch:        fm.Branch,
		Repository:    fm.Repository,
		Topic:         fm.Topic,
		Tags:          fm.Tags,
		Status:        fm.Status,
		LastUpdated:   fm.LastUpdated,
		LastUpdatedBy: fm.LastUpdatedBy,
	}
}
