package markdown

import (
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

func BuildDocumentWorkbenchActions(pageArgs *PageArgs) templ.Component {
	if pageArgs == nil || pageArgs.ViewerArgs.CommentMode == CommentModeNone {
		return nil
	}
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Document actions",
		Groups: []workbench.OverflowActionGroup{{
			Label:   "Document",
			Actions: []workbench.OverflowAction{DocumentCommentAction(pageArgs)},
		}},
	})
}

func DocumentCommentAction(pageArgs *PageArgs) workbench.OverflowAction {
	heading := "Document"
	if pageArgs != nil {
		if title := strings.TrimSpace(DocumentTitle(pageArgs.FilePath, pageArgs.ViewerArgs.Frontmatter)); title != "" {
			heading = title
		}
	}
	fields := map[string]string{
		"section_hint":          "document",
		"heading_hint":          heading,
		"comment_target_chrome": string(commentui.CommentTargetChromePatchOnly),
		"selected_text":         "",
	}
	if pageArgs != nil {
		fields["doc_path"] = pageArgs.FilePath
	}
	return workbench.OverflowAction{
		Label:        "Comment",
		Description:  "Add whole-document comment",
		Kind:         workbench.OverflowActionForm,
		FormAction:   "/forms/comments/show",
		FormMethod:   "post",
		SubmitMode:   workbench.OverflowActionSubmitDatastar,
		HiddenFields: fields,
	}
}
