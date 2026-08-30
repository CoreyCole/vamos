package markdown

import (
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

const documentCopySourceID = "document-copy-source"

func BuildDocumentWorkbenchActions(pageArgs *PageArgs) templ.Component {
	if pageArgs == nil {
		return nil
	}
	actions := make([]workbench.OverflowAction, 0, 2)
	if pageArgs.ViewerArgs.RawMarkdown != "" {
		actions = append(actions, DocumentCopyAction())
	}
	if pageArgs.ViewerArgs.CommentMode != CommentModeNone {
		actions = append(actions, DocumentCommentAction(pageArgs))
	}
	if len(actions) == 0 {
		return nil
	}
	overflow := workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Document actions",
		Groups: []workbench.OverflowActionGroup{{
			Label:   "Document",
			Actions: actions,
		}},
	})
	return documentWorkbenchActions(pageArgs.ViewerArgs.RawMarkdown, overflow)
}

func DocumentCopyAction() workbench.OverflowAction {
	return workbench.OverflowAction{
		Label:        "Copy document",
		Description:  "Copy source to clipboard",
		Kind:         workbench.OverflowActionButton,
		ClientAction: "navigator.clipboard.writeText(document.getElementById('" + documentCopySourceID + "').content.textContent); el.closest('details')?.removeAttribute('open')",
	}
}

func DocumentCommentAction(pageArgs *PageArgs) workbench.OverflowAction {
	heading := "Document"
	if pageArgs != nil {
		if title := strings.TrimSpace(
			DocumentTitle(pageArgs.FilePath, pageArgs.ViewerArgs.Frontmatter),
		); title != "" {
			heading = title
		}
	}
	fields := map[string]string{
		"section_hint":          "document",
		"heading_hint":          heading,
		"comment_target_chrome": string(commentui.CommentTargetChromePatchOnly),
		"selected_text":         "",
		"workbench_v2":          "1",
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
