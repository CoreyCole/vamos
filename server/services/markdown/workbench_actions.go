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

func shareOverflowActions() []workbench.OverflowAction {
	closeMenu := "el.closest('[data-overflow-menu]')?.style.setProperty('display','none')"
	return []workbench.OverflowAction{
		{
			Label:        "Share artifact",
			Kind:         workbench.OverflowActionButton,
			ClientAction: closeMenu,
		},
		{
			Label:        "Share chat",
			Kind:         workbench.OverflowActionButton,
			ClientAction: closeMenu,
		},
	}
}

// threadArtifactOverflowActions builds path-header / chat-header Artifact items.
// includePlanChat controls "Chat about this plan" (skip when already in that plan room).
func threadArtifactOverflowActions(
	pageArgs *PageArgs,
	docPath string,
	includePlanChat bool,
) []workbench.OverflowAction {
	docPath = strings.TrimSpace(docPath)
	actions := make([]workbench.OverflowAction, 0, 4)
	if docPath != "" {
		actions = append(actions, DocumentCopyPathAction(docPath))
	}
	if includePlanChat {
		if chatHref := planLeadChatHref(docPath); chatHref != "" {
			chat := workbench.OverflowAction{
				Label: "Chat about this plan",
				Kind:  workbench.OverflowActionLink,
				Href:  chatHref,
			}
			if name := planLeadRoomID(docPath); name != "" {
				chat.Description = name
			}
			actions = append(actions, chat)
		}
	}
	if pageArgs != nil {
		if pageArgs.ViewerArgs.RawMarkdown != "" {
			actions = append(actions, DocumentCopyAction())
		}
		if pageArgs.ViewerArgs.CommentMode != CommentModeNone {
			actions = append(actions, DocumentCommentAction(pageArgs))
		}
	}
	return actions
}

func BuildThreadArtifactHeaderActions(
	pageArgs *PageArgs,
	docPath string,
) templ.Component {
	actions := threadArtifactOverflowActions(pageArgs, docPath, true)
	if len(actions) == 0 {
		return nil
	}
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Artifact actions",
		Groups: []workbench.OverflowActionGroup{{
			Actions: actions,
		}},
	})
}

// BuildChatHeaderOverflow is the single desktop chat-header ⋯ menu: Share stubs
// always, plus optional Artifact group (Copy path / document / Comment). Pass
// includePlanChat=false when already in that plan room so "Chat about this plan"
// is omitted.
func BuildChatHeaderOverflow(
	pageArgs *PageArgs,
	docPath string,
	includePlanChat bool,
) templ.Component {
	groups := []workbench.OverflowActionGroup{{
		Label:   "Share",
		Actions: shareOverflowActions(),
	}}
	if artifact := threadArtifactOverflowActions(pageArgs, docPath, includePlanChat); len(artifact) > 0 {
		groups = append(groups, workbench.OverflowActionGroup{
			Label:   "Artifact",
			Actions: artifact,
		})
	}
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label:  "Share",
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
		ClientAction: "const p='" + escaped + "'; navigator.clipboard?.writeText(p); const input = document.getElementById('agent-chat-composer-input'); if (input) { input.value += (input.value ? '\\n' : '') + p; input.dispatchEvent(new Event('input', {bubbles: true})); } el.closest('[data-overflow-menu]')?.style.setProperty('display','none')",
	}
}
