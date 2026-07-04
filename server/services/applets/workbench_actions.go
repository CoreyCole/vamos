package applets

import (
	"context"
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

type AppletCommentReader interface {
	GetCommentsForFileInternal(ctx context.Context, filePath string) (*comments.GetCommentsResponse, error)
}

func BuildAppletWorkbenchActions(applet AppletContext, process appletruntime.AppletProcessState) templ.Component {
	actions := make([]workbench.OverflowAction, 0, 4)
	if action, ok := AppletCommentAction(applet); ok {
		actions = append(actions, action)
	}
	actions = append(actions, AppletLifecycleActions(applet, process)...)
	if len(actions) == 0 {
		return nil
	}
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Applet actions",
		Groups: []workbench.OverflowActionGroup{{
			Label:   "Applet",
			Actions: actions,
		}},
	})
}

func AppletCommentUI(ctx context.Context, applet AppletContext, userEmail string, reader AppletCommentReader) (commentui.CommentableMarkdownArgs, bool, error) {
	if !supportsAppletComments(applet) {
		return commentui.CommentableMarkdownArgs{}, false, nil
	}
	threads := []commentui.CommentThreadView{}
	if reader != nil {
		resp, err := reader.GetCommentsForFileInternal(ctx, applet.IdentityPath)
		if err != nil {
			return commentui.CommentableMarkdownArgs{}, false, err
		}
		if resp != nil {
			threads = appletCommentThreads(resp.Comments)
		}
	}
	return commentui.CommentableMarkdownArgs{
		Surface:   commentui.CommentSurfaceThoughts,
		IDPrefix:  commentui.SafeCommentTargetSlug("thoughts", applet.IdentityPath),
		DocPath:   applet.IdentityPath,
		Comments:  threads,
		UserEmail: strings.TrimSpace(userEmail),
		Routes: commentui.CommentRoutes{
			Show:   "/forms/comments/show",
			Create: "/forms/comments",
			Cancel: "/forms/comments/cancel",
			Expand: "/forms/comments/expand",
			Reply: func(string) string {
				return "/forms/replies"
			},
			Resolve: func(string) string {
				return "/forms/resolve"
			},
		},
		HiddenFields: map[string]string{"doc_path": applet.IdentityPath},
	}, true, nil
}

func AppletCommentAction(applet AppletContext) (workbench.OverflowAction, bool) {
	if !supportsAppletComments(applet) {
		return workbench.OverflowAction{}, false
	}
	heading := strings.TrimSpace(applet.Manifest.Title)
	if heading == "" {
		heading = "Applet"
	}
	return workbench.OverflowAction{
		Label:       "Comment",
		Description: "Add whole-applet comment",
		Kind:        workbench.OverflowActionForm,
		FormAction:  "/forms/comments/show",
		FormMethod:  "post",
		SubmitMode:  workbench.OverflowActionSubmitDatastar,
		HiddenFields: map[string]string{
			"doc_path":              applet.IdentityPath,
			"section_hint":          "document",
			"heading_hint":          heading,
			"comment_target_chrome": string(commentui.CommentTargetChromePatchOnly),
			"selected_text":         "",
		},
	}, true
}

func AppletLifecycleActions(applet AppletContext, process appletruntime.AppletProcessState) []workbench.OverflowAction {
	startLabel := "Restart"
	if process.Status == "" || process.Status == appletruntime.ProcessStatusStopped {
		startLabel = "Start"
	}
	actions := []workbench.OverflowAction{{
		Label:      startLabel,
		Kind:       workbench.OverflowActionForm,
		FormAction: "/forms/applets/" + applet.RuntimeKey + "/restart",
		FormMethod: "post",
		SubmitMode: workbench.OverflowActionSubmitNative,
		HiddenFields: map[string]string{
			"identity_path": applet.IdentityPath,
		},
	}}
	if process.Status == appletruntime.ProcessStatusHealthy || process.Status == appletruntime.ProcessStatusUnhealthy {
		actions = append(actions, workbench.OverflowAction{
			Label:      "Stop",
			Kind:       workbench.OverflowActionForm,
			FormAction: "/forms/applets/" + applet.RuntimeKey + "/stop",
			FormMethod: "post",
			SubmitMode: workbench.OverflowActionSubmitNative,
			HiddenFields: map[string]string{
				"identity_path": applet.IdentityPath,
			},
		})
	}
	actions = append(actions, workbench.OverflowAction{
		Label:  "Open in new tab",
		Kind:   workbench.OverflowActionLink,
		Href:   applet.IFrameSrc,
		Target: "_blank",
		Rel:    "noopener",
	})
	return actions
}

func supportsAppletComments(applet AppletContext) bool {
	return strings.HasPrefix(strings.TrimSpace(applet.IdentityPath), "thoughts/")
}

func appletCommentThreads(items []comments.CommentWithReplies) []commentui.CommentThreadView {
	sources := make([]commentui.ThreadSource, 0, len(items))
	for _, item := range items {
		sectionID := item.Comment.SectionHint.String
		if !item.Comment.SectionHint.Valid || strings.TrimSpace(sectionID) == "" {
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
