package markdown

import (
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/pkg/datastarui/components/toast"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

const documentCopySourceID = "document-copy-source"

func closeOverflowMenuExpr() string {
	return "el.closest('[data-overflow-menu]')?.style.setProperty('display','none')"
}

// clipboardWriteTextThenToast runs writeText then shows the root clipboard_success toast on success only.
func clipboardWriteTextThenToast(jsTextExpr string) string {
	show := toast.ShowToastExpr("clipboard_success", 2000)
	return "navigator.clipboard.writeText(" + jsTextExpr + ").then(() => { " + show + " }); " + closeOverflowMenuExpr()
}

func thoughtsClipboardPath(docPath string) string {
	docPath = strings.TrimSpace(docPath)
	if docPath == "" {
		return "thoughts/"
	}
	if strings.HasPrefix(docPath, "thoughts/") {
		return docPath
	}
	return "thoughts/" + docPath
}

func escapeJSSingleQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

func ShareArtifactAction(docPath string) workbench.OverflowAction {
	path := thoughtsClipboardPath(docPath)
	return workbench.OverflowAction{
		Label:        "Share artifact",
		Kind:         workbench.OverflowActionButton,
		ClientAction: clipboardWriteTextThenToast("'" + escapeJSSingleQuoted(path) + "'"),
	}
}

func ShareChatAction() workbench.OverflowAction {
	return workbench.OverflowAction{
		Label:        "Share chat",
		Kind:         workbench.OverflowActionButton,
		ClientAction: clipboardWriteTextThenToast("location.href.replace(/#.*$/, '')"),
	}
}

func BuildDocumentWorkbenchActions(pageArgs *PageArgs) templ.Component {
	if pageArgs == nil {
		return nil
	}
	actions := make([]workbench.OverflowAction, 0, 3)
	actions = append(actions, ShareArtifactAction(pageArgs.FilePath))
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
		Label: "Copy document contents",
		Kind:  workbench.OverflowActionButton,
		ClientAction: clipboardWriteTextThenToast(
			"document.getElementById('" + documentCopySourceID + "').content.textContent",
		),
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

// threadArtifactOverflowActions builds path-header Artifact items (doc shell).
// includePlanChat controls "Chat about this plan" (skip when already in that plan room).
// No Share chat — that belongs on the chat-header overflow only.
func threadArtifactOverflowActions(
	pageArgs *PageArgs,
	docPath string,
	includePlanChat bool,
	thoughtsBasePath string,
) []workbench.OverflowAction {
	docPath = strings.TrimSpace(docPath)
	actions := make([]workbench.OverflowAction, 0, 5)
	actions = append(actions, workbench.ArtifactReloadOverflowAction())
	if docPath != "" {
		actions = append(actions, ShareArtifactAction(docPath))
	}
	if includePlanChat {
		chatHref := thoughtsChatHref(thoughtsBasePath, docPath)
		if chatHref == "" {
			chatHref = thoughtsChatHref("", docPath)
		}
		if chatHref != "" {
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
	return BuildThreadArtifactHeaderActionsWithBase(pageArgs, docPath, "")
}

func BuildThreadArtifactHeaderActionsWithBase(
	pageArgs *PageArgs,
	docPath string,
	thoughtsBasePath string,
) templ.Component {
	actions := threadArtifactOverflowActions(pageArgs, docPath, true, thoughtsBasePath)
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

// BuildChatHeaderOverflow is the single desktop chat-header ⋯ menu: flat Share +
// optional document/plan actions (no SHARE/ARTIFACT section headers). Pass
// includePlanChat=false when already in that plan room so "Chat about this plan"
// is omitted. Copy path is folded into Share artifact (no composer append).
func BuildChatHeaderOverflow(
	pageArgs *PageArgs,
	docPath string,
	includePlanChat bool,
) templ.Component {
	docPath = strings.TrimSpace(docPath)
	actions := make([]workbench.OverflowAction, 0, 6)
	if docPath != "" {
		actions = append(actions, ShareArtifactAction(docPath))
	}
	actions = append(actions, ShareChatAction())
	if pageArgs != nil {
		if pageArgs.ViewerArgs.RawMarkdown != "" {
			actions = append(actions, DocumentCopyAction())
		}
		if pageArgs.ViewerArgs.CommentMode != CommentModeNone {
			actions = append(actions, DocumentCommentAction(pageArgs))
		}
	}
	if includePlanChat {
		if chatHref := thoughtsChatHref("", docPath); chatHref != "" {
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
	return workbench.OverflowActions(workbench.OverflowActionsArgs{
		Label: "Share",
		Groups: []workbench.OverflowActionGroup{{
			Actions: actions,
		}},
	})
}

// DocumentCopyPathAction remains for callers that still need a path-only copy.
// Share menus use ShareArtifactAction instead (no composer append).
func DocumentCopyPathAction(docPath string) workbench.OverflowAction {
	path := thoughtsClipboardPath(docPath)
	return workbench.OverflowAction{
		Label:       "Copy path",
		Description: "Copy path and attach in chat",
		Kind:        workbench.OverflowActionButton,
		ClientAction: "const p='" + escapeJSSingleQuoted(
			path,
		) + "'; navigator.clipboard?.writeText(p); const input = document.getElementById('agent-chat-composer-input'); if (input) { input.value += (input.value ? '\\n' : '') + p; input.dispatchEvent(new Event('input', {bubbles: true})); } " + closeOverflowMenuExpr(),
	}
}
