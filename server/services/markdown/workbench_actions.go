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
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Document actions",
		Groups: []workbench.OverflowActionGroup{{
			Label:   "Document",
			Actions: actions,
		}},
	})
}

func DocumentCopyAction() workbench.OverflowAction {
	return workbench.OverflowAction{
		Label:        "Copy document",
		Description:  "Copy source to clipboard",
		Kind:         workbench.OverflowActionButton,
		ClientAction: "navigator.clipboard.writeText(document.getElementById('" + documentCopySourceID + "').content.textContent); el.closest('[data-overflow-menu]')?.style.setProperty('display','none')",
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

func BuildThreadArtifactHeaderActions(
	pageArgs *PageArgs,
	docPath, chatHref string,
	showThoughts bool,
) templ.Component {
	docPath = strings.TrimSpace(docPath)
	chatHref = strings.TrimSpace(chatHref)
	groups := make([]workbench.OverflowActionGroup, 0, 2)
	pathActions := make([]workbench.OverflowAction, 0, 3)
	if docPath != "" {
		if showThoughts {
			pathActions = append(pathActions, workbench.OverflowAction{
				Label: "Thoughts",
				Kind:  workbench.OverflowActionLink,
				Href:  ThoughtsDocURL(docPath, ""),
			})
		}
		pathActions = append(pathActions, DocumentCopyPathAction(docPath))
	}
	if chatHref != "" {
		pathActions = append(pathActions, workbench.OverflowAction{
			Label: "chat about this plan",
			Kind:  workbench.OverflowActionLink,
			Href:  chatHref,
		})
	}
	if len(pathActions) > 0 {
		groups = append(groups, workbench.OverflowActionGroup{Actions: pathActions})
	}
	docActions := make([]workbench.OverflowAction, 0, 2)
	if pageArgs != nil {
		if pageArgs.ViewerArgs.RawMarkdown != "" {
			docActions = append(docActions, DocumentCopyAction())
		}
		if pageArgs.ViewerArgs.CommentMode != CommentModeNone {
			docActions = append(docActions, DocumentCommentAction(pageArgs))
		}
	}
	if len(docActions) > 0 {
		groups = append(groups, workbench.OverflowActionGroup{
			Label:   "Document",
			Actions: docActions,
		})
	}
	if len(groups) == 0 {
		return nil
	}
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label:  "Artifact actions",
		Groups: groups,
	})
}

func DocumentCopyPathAction(docPath string) workbench.OverflowAction {
	path := "thoughts/" + strings.TrimSpace(docPath)
	escaped := strings.ReplaceAll(path, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return workbench.OverflowAction{
		Label:        "Copy path",
		Description:  "Copy path and attach in chat",
		Kind:         workbench.OverflowActionButton,
		ClientAction: "const p='" + escaped + "'; navigator.clipboard?.writeText(p); const input = document.getElementById('agent-chat-composer-input'); if (input) { input.value += (input.value ? '\n' : '') + p; input.dispatchEvent(new Event('input', {bubbles: true})); } el.closest('[data-overflow-menu]')?.style.setProperty('display','none')",
	}
}
