//go:build !integration || unit
// +build !integration unit

package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

func TestMarkdownPageUsesSharedCommentUI(t *testing.T) {
	args := &PageArgs{
		FilePath:      "thoughts/creative-mode-agent/plans/plan-a/design.md",
		UserEmail:     "user@example.com",
		PageSessionID: "page-session-1",
		ViewerArgs: ViewerArgs{Sections: []Section{{
			ID:          "section-1",
			Title:       "Design",
			HeadingHTML: `<h1 id="design">Design</h1>`,
			BodyHTML:    `<p>Body</p>`,
		}}},
		CommentUI: commentui.CommentableMarkdownArgs{
			Surface:  commentui.CommentSurfaceThoughts,
			IDPrefix: commentui.SafeCommentTargetSlug("thoughts", "design.md"),
			DocPath:  "thoughts/creative-mode-agent/plans/plan-a/design.md",
			Sections: []commentui.CommentSectionView{{
				ID:          "section-1",
				Title:       "Design",
				HeadingHTML: `<h1 id="design">Design</h1>`,
				BodyHTML:    `<p>Body</p>`,
			}},
			Routes: commentui.CommentRoutes{
				Show:   "/forms/comments/show",
				Create: "/forms/comments",
				Cancel: "/forms/comments/cancel",
				Expand: "/forms/comments/expand",
			},
			HiddenFields: map[string]string{
				"file_path": "thoughts/creative-mode-agent/plans/plan-a/design.md",
			},
			SelectionSignals: commentui.SelectionSignalArgs{
				Prefix:    "comment_selection",
				ShowRoute: "/forms/comments/show",
				HiddenFields: map[string]string{
					"file_path": "thoughts/creative-mode-agent/plans/plan-a/design.md",
				},
			},
		},
		WorkspaceContext: DocumentWorkspaceContext{
			WorkspaceID:  "workspace-1",
			RelativePath: "design.md",
			Attached:     true,
		},
	}

	var buf bytes.Buffer
	if err := MarkdownPage(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`data-comment-target="true"`,
		`data-section-id="section-1"`,
		`comment-target-`,
		`comment_selection-inline-comment-trigger`,
		`name="selected_text"`,
		`id="document-surface"`,
		`thoughts-open-chat-result`,
		`Discussions`,
		`id="workbench-root"`,
		`id="thoughts-section-map-region"`,
		`id="thoughts-document-region"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("MarkdownPage render missing %q in %s", want, html)
		}
	}
	for _, removed := range []string{
		`id="comment-sidebar"`,
		`commentSidebarExpanded`,
		`$commentSidebarExpanded`,
		`w-80`,
		`w-10`,
		`id="comment-sidebar" class="w-80 shrink-0 min-h-screen"`,
		`section-comments-target-`,
		`data-effect="if ($comment_selection.top)`,
		`discussions_popover`,
		`This document matches multiple Agent Chat workspaces`,
		`No Agent Chat workspace owns this document yet`,
		`Attached to Agent Chat workspace`,
		`Open chat`,
		`Attach to chat`,
	} {
		if strings.Contains(html, removed) {
			t.Fatalf(
				"MarkdownPage render kept removed discussions chrome %q: %s",
				removed,
				html,
			)
		}
	}
}

//nolint:paralleltest // templ component rendering uses shared Tailwind merge state in tests.
func TestMarkdownWorkbenchContextClosedByDefaultAndCommentsOpen(t *testing.T) {
	args := &PageArgs{
		FilePath:      "thoughts/creative-mode-agent/plans/plan-a/design.md",
		UserEmail:     "user@example.com",
		PageSessionID: "page-session-1",
		ViewerArgs: ViewerArgs{Sections: []Section{{
			ID:          "section-1",
			Title:       "Design",
			HeadingHTML: `<h1 id="design">Design</h1>`,
			BodyHTML:    `<p>Body</p>`,
		}}},
		TableOfContents: []TocItem{{ID: "design", Text: "Design", Level: 1}},
		CommentUI:       minimalCommentUIArgs(),
	}
	build := func(contextMode string) workbench.WorkbenchState {
		view := workbench.WorkbenchViewFocus
		visible := false
		kind := workbench.RegionEmpty
		if contextMode == "comments" {
			view = workbench.WorkbenchViewSplit
			visible = true
			kind = workbench.RegionComments
		}
		state, err := workbench.BuildWorkbenchState(workbench.BuildWorkbenchStateInput{
			UserEmail:   args.UserEmail,
			Page:        workbench.WorkbenchPageThoughts,
			View:        view,
			ContextMode: contextMode,
			ActivePath:  args.FilePath,
			Regions: []workbench.WorkbenchRegion{
				{
					ID:        "thoughts-sections",
					Slot:      workbench.WorkbenchSlotNavigation,
					Kind:      workbench.RegionSections,
					Visible:   true,
					TargetID:  "thoughts-section-map-region",
					Component: SectionMapPanel(BuildSectionMapArgs(args)),
				},
				{
					ID:        "thoughts-document",
					Slot:      workbench.WorkbenchSlotPrimary,
					Kind:      workbench.RegionDocument,
					Visible:   true,
					TargetID:  "thoughts-document-region",
					Component: DocumentPanel(BuildDocumentPanelArgs(args)),
				},
				{
					ID:       "thoughts-context",
					Slot:     workbench.WorkbenchSlotContext,
					Kind:     kind,
					Visible:  visible,
					TargetID: "thoughts-context-region",
					Component: ThoughtsContextPanel(
						ThoughtsContextArgs{
							Mode:      contextMode,
							PageArgs:  args,
							CommentUI: args.CommentUI,
						},
					),
				},
			},
		})
		if err != nil {
			t.Fatalf("BuildWorkbenchState() error = %v", err)
		}
		return state
	}

	var focus bytes.Buffer
	if err := MarkdownWorkbenchPage(
		MarkdownWorkbenchArgs{PageArgs: args, Workbench: build("")},
	).Render(t.Context(), &focus); err != nil {
		t.Fatalf("focus render error = %v", err)
	}
	focusHTML := focus.String()
	if !strings.Contains(focusHTML, `id="thoughts-context-region"`) ||
		!strings.Contains(focusHTML, `data-workbench-region="thoughts-context"`) {
		t.Fatalf("focus render missing context region shell: %s", focusHTML)
	}
	if !strings.Contains(focusHTML, `id="thoughts-context-region"`) ||
		!strings.Contains(focusHTML, `class="hidden`) {
		t.Fatalf("focus render should keep context hidden by default: %s", focusHTML)
	}
	for _, want := range []string{
		`id="` + commentui.CommentsContextPanelID + `"`,
		`id="` + commentui.MobileSectionCommentContentID + `"`,
	} {
		if !strings.Contains(focusHTML, want) {
			t.Fatalf(
				"focus render missing stable comment patch target %q in %s",
				want,
				focusHTML,
			)
		}
	}

	var comments bytes.Buffer
	if err := MarkdownWorkbenchPage(
		MarkdownWorkbenchArgs{PageArgs: args, Workbench: build("comments")},
	).Render(t.Context(), &comments); err != nil {
		t.Fatalf("comments render error = %v", err)
	}
	commentsHTML := comments.String()
	for _, want := range []string{`id="thoughts-context-region"`, `id="comments-context-panel"`, `Comments`} {
		if !strings.Contains(commentsHTML, want) {
			t.Fatalf("comments render missing %q in %s", want, commentsHTML)
		}
	}
}

//nolint:paralleltest // templ component rendering uses shared Tailwind merge state in tests.
func TestMarkdownPageHidesWorkspaceOwnershipText(t *testing.T) {
	contexts := map[string]DocumentWorkspaceContext{
		"attached": {
			WorkspaceID:  "workspace-1",
			RelativePath: "design.md",
			Attached:     true,
		},
		"ambiguous": {Ambiguous: true},
		"no-owner":  {},
	}
	for name, workspaceContext := range contexts {
		name := name
		workspaceContext := workspaceContext
		t.Run(name, func(t *testing.T) {
			args := &PageArgs{
				FilePath:         "thoughts/creative-mode-agent/plans/plan-a/design.md",
				UserEmail:        "user@example.com",
				PageSessionID:    "page-session-1",
				WorkspaceContext: workspaceContext,
				CommentUI:        minimalCommentUIArgs(),
			}

			var buf bytes.Buffer
			if err := MarkdownPage(args).Render(t.Context(), &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			html := buf.String()
			for _, removed := range []string{
				`This document matches multiple Agent Chat workspaces`,
				`No Agent Chat workspace owns this document yet`,
				`Attached to Agent Chat workspace`,
				`Attach to chat`,
			} {
				if strings.Contains(html, removed) {
					t.Fatalf(
						"MarkdownPage render kept ownership text %q: %s",
						removed,
						html,
					)
				}
			}
		})
	}
}

//nolint:paralleltest // templ component rendering uses shared Tailwind merge state in tests.
func TestDirectoryWorkbenchPageRendersSharedSidebarAndDirectoryAnchors(t *testing.T) {
	args := &DirectoryArgs{
		Path:      "thoughts/creative-mode-agent/plans",
		UserEmail: "user@example.com",
		Items: []DirectoryItem{
			{
				Name:  "plan-a",
				Path:  "creative-mode-agent/plans/plan-a/",
				IsDir: true,
			},
			{
				Name: "design.md",
				Path: "creative-mode-agent/plans/plan-a/design.md",
			},
		},
		FileTree: []FileTreeNode{
			{
				Name:       "thoughts",
				Path:       "thoughts",
				IsDir:      true,
				IsExpanded: true,
				Children: []FileTreeNode{{
					Name:     "design.md",
					Path:     "thoughts/creative-mode-agent/plans/plan-a/design.md",
					IsActive: true,
				}},
			},
		},
	}
	state, err := workbench.BuildWorkbenchState(workbench.BuildWorkbenchStateInput{
		UserEmail:  args.UserEmail,
		Page:       workbench.WorkbenchPageThoughts,
		View:       workbench.WorkbenchViewFocus,
		ActivePath: args.Path,
		Regions: []workbench.WorkbenchRegion{
			{
				ID:       "thoughts-sections",
				Slot:     workbench.WorkbenchSlotNavigation,
				Kind:     workbench.RegionThoughtsTree,
				Visible:  true,
				TargetID: "thoughts-sidebar-region",
				Component: workbench.SharedSidebar(
					BuildThoughtsDirectorySidebarArgs(args),
				),
			},
			{
				ID:        "thoughts-document",
				Slot:      workbench.WorkbenchSlotPrimary,
				Kind:      workbench.RegionDocument,
				Visible:   true,
				TargetID:  "thoughts-directory-region",
				Component: DirectoryPrimaryPanel(args),
			},
			{
				ID:        "thoughts-context",
				Slot:      workbench.WorkbenchSlotContext,
				Kind:      workbench.RegionEmpty,
				Visible:   false,
				TargetID:  "thoughts-context-region",
				Component: EmptyDirectoryContextPanel(),
			},
		},
		NormalRegions: thoughtsNormalRegions(false),
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}

	var buf bytes.Buffer
	if err := DirectoryWorkbenchPage(
		DirectoryWorkbenchArgs{Directory: args, Workbench: state},
	).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`id="workbench-root"`,
		`id="thoughts-sidebar-region"`,
		`id="thoughts-directory-region"`,
		`id="thoughts-directory-primary"`,
		`id="thoughts-directory-scroll-region"`,
		`Files`,
		`Workspaces`,
		`href="/thoughts/creative-mode-agent/plans/plan-a"`,
		`href="/thoughts/creative-mode-agent/plans/plan-a/design.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("DirectoryWorkbenchPage render missing %q in %s", want, html)
		}
	}
	for _, unwanted := range []string{
		`action="/thoughts/actions/select-directory"`,
		`data-on:submit="@post(&#39;/thoughts/actions/select-directory&#39;, {contentType: &#39;form&#39;})"`,
		`name="dir_path"`,
		`action="/thoughts/actions/select-document"`,
		`data-on:submit="@post(&#39;/thoughts/actions/select-document&#39;, {contentType: &#39;form&#39;})"`,
		`name="doc_path"`,
		`$sidebarActiveTab = &#39;document&#39;`,
		`$sidebarActiveTab = 'document'`,
		`mx-auto max-w-4xl rounded-lg border border-border bg-card`,
		`shadow-sm md:p-6`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("directory workbench kept unwanted chrome %q: %s", unwanted, html)
		}
	}
}

func minimalCommentUIArgs() commentui.CommentableMarkdownArgs {
	return commentui.CommentableMarkdownArgs{
		Surface:  commentui.CommentSurfaceThoughts,
		IDPrefix: commentui.SafeCommentTargetSlug("thoughts", "design.md"),
		DocPath:  "thoughts/creative-mode-agent/plans/plan-a/design.md",
		Routes: commentui.CommentRoutes{
			Show:   "/forms/comments/show",
			Create: "/forms/comments",
			Cancel: "/forms/comments/cancel",
			Expand: "/forms/comments/expand",
		},
		HiddenFields: map[string]string{
			"file_path": "thoughts/creative-mode-agent/plans/plan-a/design.md",
		},
		SelectionSignals: commentui.SelectionSignalArgs{
			Prefix:    "comment_selection",
			ShowRoute: "/forms/comments/show",
			HiddenFields: map[string]string{
				"file_path": "thoughts/creative-mode-agent/plans/plan-a/design.md",
			},
		},
	}
}
