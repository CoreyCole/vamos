package agentchat

import (
	"strings"

	"github.com/CoreyCole/vamos/server/services/commentui"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

func BuildDocDocument(state DocPaneState) (markdown.WorkbenchDocument, bool) {
	if !state.Selected.Exists || strings.TrimSpace(state.Selected.RelativePath) == "" ||
		strings.TrimSpace(state.Selected.HTML) == "" {
		return markdown.WorkbenchDocument{}, false
	}
	commentArgs := commentui.CommentableMarkdownArgs{
		Surface:      commentui.CommentSurfaceDoc,
		IDPrefix:     docTargetSlug(state.WorkspaceID, state.Selected.RelativePath),
		WorkspaceID:  state.WorkspaceID,
		DocPath:      state.Selected.RelativePath,
		HTML:         state.Selected.HTML,
		Sections:     commentSectionsFromDoc(state.Selected.Sections),
		Comments:     docCommentThreads(state.Comments),
		Routes:       docCommentRoutes(state.WorkspaceID),
		HiddenFields: map[string]string{"doc_rel_path": state.Selected.RelativePath},
		SelectionSignals: commentui.SelectionSignalArgs{
			Prefix: docSelectionSignalPrefix(
				state.WorkspaceID,
				state.Selected.RelativePath,
			),
			ExcludeSelector: "#comment-sidebar, [data-comment-target=true], [id^=comment-target-]",
			ShowRoute:       "/agent-chat/" + state.WorkspaceID + "/docs/comments/show",
			HiddenFields: map[string]string{
				"doc_rel_path": state.Selected.RelativePath,
			},
			ContainerID: "agent-chat-doc-scroll-region",
		},
	}
	title := strings.TrimSpace(state.Selected.DisplayName)
	if title == "" {
		title = state.Selected.RelativePath
	}
	return markdown.WorkbenchDocument{
		Path:        state.Selected.RelativePath,
		Title:       title,
		CurrentPath: state.Selected.RelativePath,
		CommentUI:   commentArgs,
	}, true
}

// BuildArtifactDocument is kept as a test/backward-compatible alias while the
// public workbench surface uses Doc terminology.
func BuildArtifactDocument(state ArtifactPaneState) (markdown.WorkbenchDocument, bool) {
	return BuildDocDocument(state)
}
