package agentchat

import (
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/commentui"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

func docTargetSlug(workspaceID, docRelPath string) string {
	return commentui.SafeCommentTargetSlug("doc", workspaceID, docRelPath)
}

//nolint:unparam // Section IDs are runtime-derived; focused tests currently pass one fixture section.
func docSectionTargetID(workspaceID, docRelPath, sectionID string) string {
	return commentui.TargetID(docTargetSlug(workspaceID, docRelPath), sectionID)
}

func docDocumentTargetID(workspaceID, docRelPath string) string {
	return commentui.TargetID(
		docTargetSlug(workspaceID, docRelPath),
		string(WorkspaceDocCommentTargetDocument),
	)
}

func docSelectionSignalPrefix(workspaceID, docRelPath string) string {
	return commentui.SafeSelectionSignalPrefix("doc", workspaceID, docRelPath)
}

func docCommentRoutes(workspaceID string) commentui.CommentRoutes {
	base := "/agent-chat/" + workspaceID + "/docs/comments"
	return commentui.CommentRoutes{
		Show:   base + "/show",
		Create: base,
		Cancel: base + "/cancel",
		Expand: base + "/expand",
		Reply: func(commentID string) string {
			return base + "/" + commentID + "/replies"
		},
		Resolve: func(commentID string) string {
			return base + "/" + commentID + "/resolve"
		},
		Reopen: func(commentID string) string {
			return base + "/" + commentID + "/reopen"
		},
	}
}

func docCommentThreads(
	comments []WorkspaceDocCommentView,
) []commentui.CommentThreadView {
	sources := make([]commentui.ThreadSource, 0, len(comments))
	for _, view := range comments {
		sectionID := string(WorkspaceDocCommentTargetDocument)
		if view.Comment.SectionHint.Valid &&
			strings.TrimSpace(view.Comment.SectionHint.String) != "" {
			sectionID = view.Comment.SectionHint.String
		}
		heading := ""
		if view.Comment.HeadingHint.Valid {
			heading = view.Comment.HeadingHint.String
		}
		replies := make([]commentui.ReplySource, 0, len(view.Replies))
		for _, reply := range view.Replies {
			replies = append(replies, commentui.ReplySource{
				AuthorEmail: reply.UserEmail,
				ActorLabel:  reply.ActorType,
				CreatedAt:   reply.CreatedAt,
				Body:        reply.ReplyText,
			})
		}
		sources = append(sources, commentui.ThreadSource{
			ID:           view.Comment.ID,
			AuthorEmail:  view.Comment.UserEmail,
			ActorLabel:   "user",
			CreatedAt:    view.Comment.CreatedAt,
			Body:         view.Comment.CommentText,
			SelectedText: view.Comment.SelectedText,
			SectionID:    sectionID,
			HeadingHint:  heading,
			Resolved:     view.Comment.Resolved,
			Replies:      replies,
			HiddenFields: map[string]string{
				"doc_rel_path": docUIPath(view.Comment.DocPath),
				"section_hint": sectionID,
				"heading_hint": heading,
			},
		})
	}
	return commentui.BuildThreadViews(sources)
}

func commentSectionsFromDoc(
	sections []markdown.Section,
) []commentui.CommentSectionView {
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

func docUIPath(path string) string {
	path = strings.TrimSpace(path)
	return strings.TrimPrefix(path, "thoughts/")
}

func docCommentTargetView(
	args DocSectionCommentsArgs,
) commentui.CommentTargetView {
	sectionID := strings.TrimSpace(args.SectionID)
	if sectionID == "" {
		sectionID = string(WorkspaceDocCommentTargetDocument)
	}
	docPath := docUIPath(firstNonEmpty(args.DocRelPath, args.DocPath))
	prefix := docTargetSlug(args.WorkspaceID, docPath)
	return commentui.BuildTargetView(commentui.TargetInput{
		Surface:      commentui.CommentSurfaceDoc,
		IDPrefix:     prefix,
		DocPath:      docPath,
		SectionID:    sectionID,
		HeadingHint:  args.HeadingHint,
		UserEmail:    args.UserEmail,
		Threads:      docCommentThreads(args.Comments),
		Routes:       docCommentRoutes(args.WorkspaceID),
		HiddenFields: map[string]string{"doc_rel_path": docPath},
	})
}

func docCommentFormView(
	args WorkspaceDocCommentFormArgs,
	target commentui.CommentTargetView,
) commentui.CommentFormView {
	id := args.ID
	if id == "" {
		id = "doc-comment-form-" + target.SignalKey
	}
	return commentui.CommentFormView{
		ID:           id,
		Target:       target,
		SelectedText: args.SelectedText,
		Error:        args.Error,
	}
}

func commentsForDocSection(
	comments []WorkspaceDocCommentView,
	sectionID, headingHint string,
) []WorkspaceDocCommentView {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID == "" {
		sectionID = string(WorkspaceDocCommentTargetDocument)
	}
	out := make([]WorkspaceDocCommentView, 0)
	for _, view := range comments {
		commentSection := string(WorkspaceDocCommentTargetDocument)
		if view.Comment.SectionHint.Valid &&
			strings.TrimSpace(view.Comment.SectionHint.String) != "" {
			commentSection = view.Comment.SectionHint.String
		}
		commentHeading := ""
		if view.Comment.HeadingHint.Valid {
			commentHeading = view.Comment.HeadingHint.String
		}
		if commentSection == sectionID ||
			(headingHint != "" && commentHeading == headingHint) {
			out = append(out, view)
		}
	}
	return out
}

func docCommentTargetRefreshFromComment(
	comment db.WorkspaceDocComment,
) WorkspaceDocCommentTargetRefresh {
	section := string(WorkspaceDocCommentTargetDocument)
	if comment.SectionHint.Valid && strings.TrimSpace(comment.SectionHint.String) != "" {
		section = comment.SectionHint.String
	}
	heading := ""
	if comment.HeadingHint.Valid {
		heading = comment.HeadingHint.String
	}
	return WorkspaceDocCommentTargetRefresh{
		WorkspaceID: comment.WorkspaceID.String,
		DocPath:     comment.DocPath,
		SectionHint: section,
		HeadingHint: heading,
	}
}

type docCommentFormReader interface {
	FormValue(key string) string
}

func docCommentTargetRefreshFromForm(
	workspaceID string,
	form docCommentFormReader,
) WorkspaceDocCommentTargetRefresh {
	section := strings.TrimSpace(form.FormValue("section_hint"))
	if section == "" {
		section = string(WorkspaceDocCommentTargetDocument)
	}
	docPath := strings.TrimSpace(form.FormValue("doc_rel_path"))
	if docPath == "" {
		docPath = strings.TrimSpace(form.FormValue("artifact_rel_path"))
	}
	return WorkspaceDocCommentTargetRefresh{
		WorkspaceID: workspaceID,
		DocRelPath:  docPath,
		DocPath:     docPath,
		SectionHint: section,
		HeadingHint: strings.TrimSpace(form.FormValue("heading_hint")),
	}
}
