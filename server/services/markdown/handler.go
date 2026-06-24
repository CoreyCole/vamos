package markdown

import (
	stdhtml "html"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

const (
	thoughtsContextModeComments = "comments"
	thoughtsContextModeChat     = "chat"

	thoughtsWorkspacesLimit = 200

	thoughtsNavigationRatio = 0.22
	thoughtsPrimaryRatio    = 0.56
	thoughtsContextRatio    = 0.22
	thoughtsNavigationMin   = 14
	thoughtsPrimaryMin      = 28
	thoughtsContextMin      = 18

	documentWorkbenchPrimarySelectorID  = "doc-workbench-viewer-region"
	directoryWorkbenchPrimarySelectorID = "thoughts-directory-region"
	thoughtsSharedSidebarSelectorID     = "thoughts-shared-sidebar"
	thoughtsURLSyncSelectorID           = "thoughts-url-sync"
)

// ServeMarkdown handles HTTP requests for markdown files and directories
func (s *Service) ServeMarkdown(c echo.Context) error {
	requestPath := c.Param("*")

	// Extract user email from context (set by auth middleware)
	userEmail := ""
	if email, ok := c.Get("user_email").(string); ok {
		userEmail = email
	}

	// Get current theme preferences
	currentTheme := ""
	currentThemeMode := ""
	if s.themeService != nil {
		currentTheme = s.themeService.GetCurrentTheme(c)
		currentThemeMode = s.themeService.GetCurrentThemeMode(c)
	}

	// Check if this is a directory request
	if requestPath == "" || strings.HasSuffix(requestPath, "/") {
		dirArgs, err := s.GetDirectoryListing(requestPath)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		dirArgs.UserEmail = userEmail
		dirArgs.CurrentTheme = currentThemeMode
		dirArgs.CurrentSyntaxTheme = currentTheme
		dirArgs.FileTree = s.GetFileTree(requestPath)
		dirArgs.ChatLinkState = EmbeddedChatLinkStateFromRequest(c, EmbeddedChatLinkState{
			Active:      thoughtsContextMode(c) == thoughtsContextModeChat,
			WorkspaceID: strings.TrimSpace(c.QueryParam("chat_workspace")),
			ThreadID:    strings.TrimSpace(c.QueryParam("thread")),
			RunID:       strings.TrimSpace(c.QueryParam("run")),
		})

		workbenchState, err := s.buildThoughtsDirectoryWorkbenchState(c, dirArgs)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		return DirectoryWorkbenchPage(DirectoryWorkbenchArgs{
			Directory: dirArgs,
			Workbench: workbenchState,
		}).Render(c.Request().Context(), c.Response().Writer)
	}

	// Render document file
	pageArgs, err := s.RenderThoughtsDocumentWithOptions(
		c.Request().Context(),
		requestPath,
		DocumentRenderOptions{CurrentTheme: currentThemeMode},
	)
	if err != nil {
		// Try directory listing if file not found or if path is a directory
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "is a directory") {
			dirArgs, dirErr := s.GetDirectoryListing(requestPath)
			if dirErr == nil {
				dirArgs.UserEmail = userEmail
				dirArgs.CurrentTheme = currentThemeMode
				dirArgs.CurrentSyntaxTheme = currentTheme
				dirArgs.FileTree = s.GetFileTree(requestPath)
				dirArgs.ChatLinkState = EmbeddedChatLinkStateFromRequest(c, EmbeddedChatLinkState{
					Active:      thoughtsContextMode(c) == thoughtsContextModeChat,
					WorkspaceID: strings.TrimSpace(c.QueryParam("chat_workspace")),
					ThreadID:    strings.TrimSpace(c.QueryParam("thread")),
					RunID:       strings.TrimSpace(c.QueryParam("run")),
				})
				workbenchState, stateErr := s.buildThoughtsDirectoryWorkbenchState(
					c,
					dirArgs,
				)
				if stateErr != nil {
					return c.String(http.StatusInternalServerError, stateErr.Error())
				}
				return DirectoryWorkbenchPage(DirectoryWorkbenchArgs{
					Directory: dirArgs,
					Workbench: workbenchState,
				}).Render(c.Request().Context(), c.Response().Writer)
			}
		}

		return c.String(http.StatusInternalServerError, err.Error())
	}
	pageArgs.UserEmail = userEmail
	pageArgs.CurrentTheme = currentThemeMode
	pageArgs.CurrentSyntaxTheme = currentTheme
	pageArgs.FileTree = s.GetFileTree(pageArgs.FilePath)
	pageArgs.ChatLinkState = EmbeddedChatLinkStateFromRequest(c, EmbeddedChatLinkState{
		Active:      thoughtsContextMode(c) == thoughtsContextModeChat,
		WorkspaceID: strings.TrimSpace(c.QueryParam("chat_workspace")),
		ThreadID:    strings.TrimSpace(c.QueryParam("thread")),
		RunID:       strings.TrimSpace(c.QueryParam("run")),
	})
	pageArgs.QRSPIMetadata = s.buildQRSPIMetadata(pageArgs)
	// Generate unique page session ID for this tab
	pageArgs.PageSessionID = uuid.Must(uuid.NewV4()).String()

	// Fetch workspace-scoped comments for this file.
	commentsResp, err := s.commentService.GetCommentsForScopeInternal(
		c.Request().Context(),
		pageArgs.FilePath,
	)
	if err != nil {
		// Log error but don't fail page load
		c.Logger().
			Errorf("Failed to fetch comments for file %s: %v", pageArgs.FilePath, err)
	} else {
		pageArgs.Comments = commentsResp
		c.Logger().
			Infof("Fetched %d comments for file: %s", len(commentsResp.Comments), pageArgs.FilePath)
	}

	threads := []commentui.CommentThreadView{}
	if commentsResp != nil {
		threads = thoughtsCommentThreads(commentsResp.Comments)
	}
	workspaceCtx := DocumentWorkspaceContext{}
	if s.workspaceResolver != nil && userEmail != "" {
		resolved, err := s.workspaceResolver.ResolveWorkspaceForDocument(
			c.Request().Context(),
			userEmail,
			pageArgs.FilePath,
		)
		if err != nil {
			c.Logger().Warnf(
				"Failed to resolve workspace for document %s: %v",
				pageArgs.FilePath,
				err,
			)
		} else {
			workspaceCtx = resolved
		}
	}
	if workspaceCtx.RootDocPath == "" {
		if root, ok := InferWorkspaceRoot(s.basePath, pageArgs.FilePath); ok {
			workspaceCtx.RootDocPath = root
			workspaceCtx.RelativePath = strings.TrimPrefix(
				NormalizeWorkspaceDocPath(pageArgs.FilePath),
				root+"/",
			)
			workspaceCtx.Attached = true
		}
	}
	pageArgs.WorkspaceContext = workspaceCtx
	pageArgs.CommentUI = s.buildCommentUI(pageArgs, userEmail, threads)
	pageArgs.ViewerArgs.BodyComponent = commentComponentForMode(pageArgs.ViewerArgs.CommentMode, pageArgs.CommentUI, pageArgs.ViewerArgs.BodyComponent)

	// Always create SectionsWithComments if sections exist (even if comments failed to
	// load)
	if len(pageArgs.ViewerArgs.Sections) > 0 {
		sectionsWithComments := make(
			[]SectionWithComments,
			0,
			len(pageArgs.ViewerArgs.Sections),
		)
		var commentsBySection map[string][]comments.CommentWithReplies

		if commentsResp != nil {
			commentsBySection = commentsResp.GroupBySectionID()
		}

		for _, section := range pageArgs.ViewerArgs.Sections {
			var sectionComments []comments.CommentWithReplies
			if commentsBySection != nil {
				sectionComments = commentsBySection[section.ID]
			}
			sectionsWithComments = append(sectionsWithComments, SectionWithComments{
				Section:  section,
				Comments: sectionComments,
			})
		}

		pageArgs.SectionsWithComments = sectionsWithComments
	}

	workbenchState, err := s.buildThoughtsWorkbenchState(c, pageArgs)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	return MarkdownWorkbenchPage(MarkdownWorkbenchArgs{
		PageArgs:  pageArgs,
		Workbench: workbenchState,
	}).Render(c.Request().Context(), c.Response().Writer)
}

func thoughtsViewFromQuery(c echo.Context) (workbench.WorkbenchView, string) {
	mode := thoughtsContextMode(c)
	switch mode {
	case thoughtsContextModeComments, thoughtsContextModeChat:
		return workbench.WorkbenchViewSplit, mode
	default:
		return workbench.WorkbenchViewFocus, ""
	}
}

func thoughtsContextMode(c echo.Context) string {
	mode := strings.TrimSpace(c.QueryParam("context"))
	if mode != "" {
		return mode
	}
	mode, _ = c.Get("thoughts_context_mode").(string)
	return strings.TrimSpace(mode)
}

func (s *Service) buildCommentUI(
	pageArgs *PageArgs,
	userEmail string,
	threads []commentui.CommentThreadView,
) commentui.CommentableMarkdownArgs {
	hiddenFields := map[string]string{"doc_path": pageArgs.FilePath}
	if pageArgs.ViewerArgs.CommentMode == CommentModeSelectionOnly {
		hiddenFields["comment_target_chrome"] = string(commentui.CommentTargetChromePatchOnly)
	}
	selection := commentui.SelectionSignalArgs{}
	if commentModeHasSelection(pageArgs.ViewerArgs.CommentMode) {
		selection = commentui.SelectionSignalArgs{
			Prefix:          "comment_selection",
			ExcludeSelector: "#comment-sidebar, [data-comment-target=true], [id^=comment-target-], [id^=section-comments-]",
			ShowRoute:       "/forms/comments/show",
			HiddenFields:    hiddenFields,
			ContainerID:     "thoughts-markdown-scroll-region",
		}
	}
	return commentui.CommentableMarkdownArgs{
		Surface:     commentui.CommentSurfaceThoughts,
		IDPrefix:    commentui.SafeCommentTargetSlug("thoughts", pageArgs.FilePath),
		DocPath:     pageArgs.FilePath,
		HTML:        pageArgs.ViewerArgs.HTMLContent,
		Sections:    commentSectionsFromMarkdown(pageArgs.ViewerArgs.Sections),
		Frontmatter: commentFrontmatterFromMarkdown(pageArgs.ViewerArgs.Frontmatter),
		Comments:    threads,
		UserEmail:   userEmail,
		Routes: commentui.CommentRoutes{
			Show:          "/forms/comments/show",
			Create:        "/forms/comments",
			Cancel:        "/forms/comments/cancel",
			Expand:        "/forms/comments/expand",
			SelectComment: "/thoughts/actions/select-comment",
			Reply: func(string) string {
				return "/forms/replies"
			},
			Resolve: func(string) string {
				return "/forms/resolve"
			},
		},
		HiddenFields:     hiddenFields,
		SelectionSignals: selection,
	}
}

func commentModeHasSelection(mode CommentMode) bool {
	return mode == "" || mode == CommentModeSections || mode == CommentModeSelectionOnly
}

func commentComponentForMode(mode CommentMode, args commentui.CommentableMarkdownArgs, existing templ.Component) templ.Component {
	switch mode {
	case "", CommentModeSections:
		return commentui.CommentableMarkdown(args)
	case CommentModeSelectionOnly:
		return commentui.CommentableSelectionHTML(args)
	default:
		return existing
	}
}

func viewportClassForRequest(c echo.Context) workbench.ViewportClass {
	return workbench.ResolveViewportClass(c.Request().Header, c.Request().UserAgent())
}

func (s *Service) savedThoughtsWorkbenchConfig(
	c echo.Context,
	userEmail string,
	view workbench.WorkbenchView,
	viewportClass workbench.ViewportClass,
) *workbench.WorkbenchConfig {
	if s.layoutPrefs == nil || strings.TrimSpace(userEmail) == "" {
		return nil
	}
	cfg, err := s.layoutPrefs.Get(
		c.Request().Context(),
		userEmail,
		workbench.WorkbenchPageThoughts,
		view,
		viewportClass,
	)
	if err != nil {
		return nil
	}
	return cfg
}

func (s *Service) buildThoughtsWorkbenchState(
	c echo.Context,
	pageArgs *PageArgs,
) (workbench.WorkbenchState, error) {
	view, contextMode := thoughtsViewFromQuery(c)
	viewportClass := viewportClassForRequest(c)
	saved := s.savedThoughtsWorkbenchConfig(c, pageArgs.UserEmail, view, viewportClass)
	rightTab := workbench.RightRailTabComments
	if contextMode == thoughtsContextModeChat {
		rightTab = workbench.RightRailTabChat
	}
	headerWorkspaceTree, hasHeaderWorkspaceTree, err := s.buildHeaderWorkspaceDocTree(
		c,
		pageArgs,
	)
	if err != nil {
		return workbench.WorkbenchState{}, err
	}
	chatComponent, chatURLReplacement, err := s.buildEmbeddedChatComponent(c, pageArgs)
	if chatURLReplacement.URL != "" && chatComponent != nil {
		chatComponent = EmbeddedChatInitialContent(chatURLReplacement.URL, chatComponent)
	}
	if err != nil {
		return workbench.WorkbenchState{}, err
	}
	return workbench.BuildDocWorkbenchState(workbench.WorkbenchDocContext{
		EntryMode:     workbench.DocEntryModeThoughts,
		UserEmail:     pageArgs.UserEmail,
		SelectedPath:  pageArgs.FilePath,
		RouteHref:     c.Request().URL.RequestURI(),
		View:          view,
		ViewportClass: viewportClass,
		SavedConfig:   saved,
		Sidebar: BuildThoughtsSidebarArgs(
			pageArgs,
			s.listWorkbenchWorkspaces(c),
		),
		InitialSidebarOpen: false,
		InitialRailOpen:    contextMode != "",
		Center: workbench.CenterDocPaneArgs{
			Title: DocumentTitle(pageArgs.FilePath, pageArgs.ViewerArgs.Frontmatter),
			Document: DocumentPanel(
				BuildDocumentPanelArgs(
					pageArgs,
					optionalHeaderWorkspaceTree(
						headerWorkspaceTree,
						hasHeaderWorkspaceTree,
					),
				),
			),
		},
		RightRail: workbench.RightRailArgs{
			ActiveTab: rightTab,
			Chat: ThoughtsContextPanel(ThoughtsContextArgs{
				Mode:      thoughtsContextModeChat,
				PageArgs:  pageArgs,
				CommentUI: pageArgs.CommentUI,
				Component: chatComponent,
			}),
			Comments: ThoughtsContextPanel(ThoughtsContextArgs{
				Mode:      thoughtsContextModeComments,
				PageArgs:  pageArgs,
				CommentUI: pageArgs.CommentUI,
			}),
		},
	})
}

func (s *Service) buildEmbeddedChatComponent(
	c echo.Context,
	pageArgs *PageArgs,
) (templ.Component, EmbeddedChatURLReplacement, error) {
	return s.buildEmbeddedChatComponentForRequest(c, EmbeddedChatRenderRequest{
		UserEmail:        pageArgs.UserEmail,
		DocPath:          pageArgs.FilePath,
		Context:          thoughtsContextMode(c),
		WorkspaceID:      strings.TrimSpace(c.QueryParam("chat_workspace")),
		ThreadID:         strings.TrimSpace(c.QueryParam("thread")),
		RunID:            strings.TrimSpace(c.QueryParam("run")),
		WorkspaceContext: pageArgs.WorkspaceContext,
	})
}

func (s *Service) buildEmbeddedChatComponentForRequest(
	c echo.Context,
	request EmbeddedChatRenderRequest,
) (templ.Component, EmbeddedChatURLReplacement, error) {
	if s.embeddedChatRenderer == nil {
		return nil, EmbeddedChatURLReplacement{}, nil
	}
	return s.embeddedChatRenderer.RenderEmbeddedChatPanel(
		c.Request().Context(),
		request,
	)
}

func (s *Service) buildHeaderWorkspaceDocTree(
	c echo.Context,
	pageArgs *PageArgs,
) (workbench.WorkspaceDocTreeHeaderModel, bool, error) {
	workspaceCtx := pageArgs.WorkspaceContext
	if workspaceCtx.RootDocPath == "" {
		if root, ok := InferWorkspaceRoot(s.basePath, pageArgs.FilePath); ok {
			workspaceCtx.RootDocPath = root
		}
	}
	if workspaceCtx.WorkspaceID == "" && workspaceCtx.RootDocPath == "" {
		return workbench.WorkspaceDocTreeHeaderModel{}, false, nil
	}

	nodes, err := s.headerWorkspaceDocTreeNodes(c, pageArgs)
	if err != nil {
		return workbench.WorkspaceDocTreeHeaderModel{}, false, err
	}

	rootLabel := workspaceDocRootLabel(workspaceCtx.RootDocPath)
	if rootLabel == "" {
		rootLabel = "Workspace docs"
	}
	return workbench.WorkspaceDocTreeHeaderModel{
		RootLabel:   rootLabel,
		CurrentPath: pageArgs.FilePath,
		Nodes:       nodes,
		EmptyLabel:  "Workspace docs will appear after the workspace sync runs.",
		TargetID:    "workspace-doc-tree-header",
	}, true, nil
}

func (s *Service) headerWorkspaceDocTreeNodes(
	c echo.Context,
	pageArgs *PageArgs,
) ([]workbench.WorkspaceDocNode, error) {
	workspaceCtx := pageArgs.WorkspaceContext
	root := workspaceCtx.RootDocPath
	if root == "" {
		if inferred, ok := InferWorkspaceRoot(s.basePath, pageArgs.FilePath); ok {
			root = inferred
		}
	}
	if root != "" {
		nodes, err := s.BuildWorkspaceDocTreeFromRoot(root, pageArgs.FilePath)
		if err != nil {
			return nil, err
		}
		return decorateWorkspaceDocNodeHrefs(nodes, pageArgs.ChatLinkState), nil
	}
	return s.workspaceDocTreeNodesFromIndex(c, pageArgs)
}

func decorateWorkspaceDocNodeHrefs(
	nodes []workbench.WorkspaceDocNode,
	chat EmbeddedChatLinkState,
) []workbench.WorkspaceDocNode {
	for i := range nodes {
		if nodes[i].Kind == workbench.WorkspaceDocKindFile {
			if strings.TrimSpace(nodes[i].Href) == "" {
				nodes[i].Href = workbench.WorkspaceDocNodeHref(
					workbench.DocEntryModeThoughts,
					nodes[i].Path,
				)
			}
			nodes[i].Href = chat.Preserve(nodes[i].Href)
		}
		nodes[i].Children = decorateWorkspaceDocNodeHrefs(nodes[i].Children, chat)
	}
	return nodes
}

func (s *Service) workspaceDocTreeNodesFromIndex(
	c echo.Context,
	pageArgs *PageArgs,
) ([]workbench.WorkspaceDocNode, error) {
	workspaceCtx := pageArgs.WorkspaceContext
	resolver, ok := s.workspaceResolver.(WorkspaceDocTreeResolver)
	if !ok || workspaceCtx.WorkspaceID == "" {
		return nil, nil
	}
	rows, err := resolver.ListWorkspaceDocs(
		c.Request().Context(),
		workspaceCtx.WorkspaceID,
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	tree := BuildWorkspaceDocTreeArgs(
		workspaceCtx.WorkspaceID,
		pageArgs.FilePath,
		workbench.DocEntryModeThoughts,
		rows,
		pageArgs.ChatLinkState,
	)
	if tree == nil {
		return nil, nil
	}
	return tree.Nodes, nil
}

func optionalHeaderWorkspaceTree(
	model workbench.WorkspaceDocTreeHeaderModel,
	ok bool,
) *workbench.WorkspaceDocTreeHeaderModel {
	if !ok {
		return nil
	}
	return &model
}

func workspaceDocRootLabel(rootDocPath string) string {
	trimmed := strings.Trim(strings.TrimSpace(rootDocPath), "/")
	if trimmed == "" {
		return ""
	}
	return filepath.Base(trimmed)
}

func (s *Service) buildThoughtsDirectoryWorkbenchState(
	c echo.Context,
	args *DirectoryArgs,
) (workbench.WorkbenchState, error) {
	view, contextMode := thoughtsViewFromQuery(c)
	viewportClass := viewportClassForRequest(c)
	saved := s.savedThoughtsWorkbenchConfig(c, args.UserEmail, view, viewportClass)
	chatComponent, chatURLReplacement, err := s.buildEmbeddedChatComponentForRequest(
		c,
		EmbeddedChatRenderRequest{
			UserEmail:   args.UserEmail,
			Context:     thoughtsContextMode(c),
			WorkspaceID: strings.TrimSpace(c.QueryParam("chat_workspace")),
			ThreadID:    strings.TrimSpace(c.QueryParam("thread")),
			RunID:       strings.TrimSpace(c.QueryParam("run")),
		},
	)
	if err != nil {
		return workbench.WorkbenchState{}, err
	}
	if chatURLReplacement.URL != "" && chatComponent != nil {
		chatComponent = EmbeddedChatInitialContent(chatURLReplacement.URL, chatComponent)
	}
	rightTab := workbench.RightRailTabComments
	if contextMode == thoughtsContextModeChat {
		rightTab = workbench.RightRailTabChat
	}
	selectedPath := args.Path
	if strings.TrimSpace(selectedPath) == "" {
		selectedPath = "/"
	}
	state, err := workbench.BuildDocWorkbenchState(workbench.WorkbenchDocContext{
		EntryMode:     workbench.DocEntryModeThoughts,
		UserEmail:     args.UserEmail,
		SelectedPath:  selectedPath,
		RouteHref:     c.Request().URL.RequestURI(),
		View:          view,
		ViewportClass: viewportClass,
		SavedConfig:   saved,
		Sidebar: BuildThoughtsDirectorySidebarArgs(
			args,
			s.listWorkbenchWorkspaces(c),
		),
		InitialSidebarOpen: false,
		InitialRailOpen:    contextMode != "",
		Center: workbench.CenterDocPaneArgs{
			Title:    DirectoryTitle(args.Path),
			Document: DirectoryPrimaryPanel(args),
		},
		RightRail: workbench.RightRailArgs{
			ActiveTab: rightTab,
			Chat: ThoughtsContextPanel(ThoughtsContextArgs{
				Mode:      thoughtsContextModeChat,
				Component: chatComponent,
			}),
			Comments: EmptyDirectoryContextPanel(),
		},
	})
	if err != nil {
		return workbench.WorkbenchState{}, err
	}
	if contextMode == thoughtsContextModeChat && (viewportClass != workbench.ViewportMobile || saved == nil) {
		state.Config.Mobile.ActiveRegionID = "doc-workbench-right"
	}
	return state, nil
}

func thoughtsNormalRegions(contextVisible bool) []workbench.RegionNormalState {
	return []workbench.RegionNormalState{
		{SignalKey: "thoughts-sections", Available: true, Visible: true},
		{SignalKey: "thoughts-document", Available: true, Visible: true},
		{SignalKey: "thoughts-context", Available: true, Visible: contextVisible},
	}
}

func thoughtsContextKind(mode string) workbench.RegionKind {
	switch mode {
	case thoughtsContextModeChat:
		return workbench.RegionChat
	case thoughtsContextModeComments:
		return workbench.RegionComments
	default:
		return workbench.RegionEmpty
	}
}

func BuildDocumentPanelArgs(
	pageArgs *PageArgs,
	workspaceTree ...*workbench.WorkspaceDocTreeHeaderModel,
) DocumentPanelArgs {
	args := DocumentPanelArgs{Document: BuildThoughtsDocument(pageArgs)}
	if len(workspaceTree) > 0 {
		args.WorkspaceTree = workspaceTree[0]
	}
	return args
}

func BuildSectionMapArgs(pageArgs *PageArgs) SectionMapArgs {
	return SectionMapArgs{
		FilePath: pageArgs.FilePath,
		TOC:      pageArgs.TableOfContents,
		Sections: pageArgs.ViewerArgs.Sections,
	}
}

func (s *Service) HandleSelectComment(c echo.Context) error {
	commentID := strings.TrimSpace(c.FormValue("comment_id"))
	if commentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "comment_id is required")
	}
	comment, err := s.commentService.GetDocumentComment(c.Request().Context(), commentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "comment not found")
	}
	return s.selectDocumentAndPatch(c, DocumentSelection{
		DocPath:      comment.DocPath,
		Hash:         comment.SectionHint.String,
		CommentID:    comment.ID,
		PreserveChat: true,
	})
}

func (s *Service) selectDocumentAndPatch(
	c echo.Context,
	selection DocumentSelection,
) error {
	docPath, err := CanonicalThoughtsDocPath(selection.DocPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	currentThemeMode := ""
	if s.themeService != nil {
		currentThemeMode = s.themeService.GetCurrentThemeMode(c)
	}
	pageArgs, err := s.RenderThoughtsDocumentWithOptions(
		c.Request().Context(),
		docPath,
		DocumentRenderOptions{CurrentTheme: currentThemeMode},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	userEmail, _ := c.Get("user_email").(string)
	pageArgs.UserEmail = userEmail
	pageArgs.FileTree = s.GetFileTree(pageArgs.FilePath)
	pageArgs.ChatLinkState = EmbeddedChatLinkStateFromRequest(c, EmbeddedChatLinkState{
		Active:      thoughtsContextMode(c) == thoughtsContextModeChat,
		WorkspaceID: firstNonEmptyString(c.FormValue("chat_workspace"), c.QueryParam("chat_workspace")),
		ThreadID:    firstNonEmptyString(c.FormValue("thread"), c.FormValue("thread_id"), c.QueryParam("thread")),
		RunID:       firstNonEmptyString(c.FormValue("run"), c.FormValue("run_id"), c.QueryParam("run")),
	})
	workspaceCtx := DocumentWorkspaceContext{}
	if s.workspaceResolver != nil && userEmail != "" {
		resolved, err := s.workspaceResolver.ResolveWorkspaceForDocument(
			c.Request().Context(),
			userEmail,
			pageArgs.FilePath,
		)
		if err != nil {
			c.Logger().Warnf(
				"Failed to resolve workspace for selected document %s: %v",
				pageArgs.FilePath,
				err,
			)
		} else {
			workspaceCtx = resolved
		}
	}
	if workspaceCtx.RootDocPath == "" {
		if root, ok := InferWorkspaceRoot(s.basePath, pageArgs.FilePath); ok {
			workspaceCtx = DocumentWorkspaceContext{
				RootDocPath: root,
				RelativePath: strings.TrimPrefix(
					NormalizeWorkspaceDocPath(pageArgs.FilePath),
					root+"/",
				),
				Attached: true,
			}
		}
	}
	pageArgs.WorkspaceContext = workspaceCtx
	pageArgs.QRSPIMetadata = s.buildQRSPIMetadata(pageArgs)
	commentsResp, err := s.commentService.GetCommentsForScopeInternal(
		c.Request().Context(),
		pageArgs.FilePath,
	)
	threads := []commentui.CommentThreadView{}
	if err == nil && commentsResp != nil {
		pageArgs.Comments = commentsResp
		threads = thoughtsCommentThreads(commentsResp.Comments)
	}
	pageArgs.CommentUI = s.buildCommentUI(pageArgs, userEmail, threads)
	pageArgs.ViewerArgs.BodyComponent = commentComponentForMode(pageArgs.ViewerArgs.CommentMode, pageArgs.CommentUI, pageArgs.ViewerArgs.BodyComponent)
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	workbenchState, err := s.buildThoughtsWorkbenchState(c, pageArgs)
	if err != nil {
		return err
	}
	if err := sse.PatchElementTempl(
		workbench.Workbench(workbenchState),
		datastar.WithSelectorID("workbench-root"),
		datastar.WithModeOuter(),
	); err != nil {
		return err
	}
	if strings.TrimSpace(selection.CommentID) != "" {
		if err := sse.MarshalAndPatchSignals(map[string]any{
			"rightRailActiveTab": "comments",
			"workbench": map[string]any{
				"focused": false,
				"regions": map[string]any{
					"docWorkbenchRight": map[string]any{"visible": true},
				},
			},
		}); err != nil {
			return err
		}
		if err := sse.PatchElementTempl(
			CommentsRightRailPanel(pageArgs.CommentUI),
			datastar.WithSelectorID("doc-right-comments-panel"),
		); err != nil {
			return err
		}
	}
	url := ThoughtsDocURL(pageArgs.FilePath, selection.Hash)
	if selection.PreserveChat {
		url = ThoughtsDocURLWithChatState(
			pageArgs.FilePath,
			selection.Hash,
			DocumentEmbeddedChatSelection{
				WorkspaceID: firstNonEmptyString(
					c.FormValue("chat_workspace"),
					c.QueryParam("chat_workspace"),
				),
				ThreadID: firstNonEmptyString(
					c.FormValue("thread"),
					c.FormValue("thread_id"),
					c.QueryParam("thread"),
				),
				RunID: firstNonEmptyString(
					c.FormValue("run"),
					c.FormValue("run_id"),
					c.QueryParam("run"),
				),
			},
		)
	}
	if err := patchThoughtsURL(sse, url); err != nil {
		return err
	}
	if script := CommentTargetScript(selection.CommentID, selection.Hash); script != "" {
		return sse.ExecuteScript(script)
	}
	return nil
}

func patchThoughtsURL(sse *datastar.ServerSentEventGenerator, url string) error {
	return sse.PatchElements(
		`<div id="thoughts-url-sync" data-replace-url="`+
			stdhtml.EscapeString(strconv.Quote(url))+`"></div>`,
		datastar.WithSelectorID(thoughtsURLSyncSelectorID),
	)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func CommentTargetScript(commentID, sectionID string) string {
	commentID = strings.TrimSpace(commentID)
	sectionID = strings.TrimSpace(strings.TrimPrefix(sectionID, "#"))
	if commentID == "" && sectionID == "" {
		return ""
	}
	if commentID == "" {
		return "requestAnimationFrame(() => {" +
			"document.dispatchEvent(new CustomEvent('workbench-section-nav', { bubbles: true, detail: { hash: " + strconvQuote(sectionID) + ", updateURL: false } }));" +
			"});"
	}
	commentTargetID := "comment-thread-" + commentID
	return "requestAnimationFrame(() => {" +
		"document.dispatchEvent(new CustomEvent('workbench-section-nav', { bubbles: true, detail: { hash: " + strconvQuote(sectionID) + ", updateURL: false } }));" +
		"const el = document.getElementById(" + strconvQuote(commentTargetID) + ");" +
		"if (!el) return;" +
		"el.scrollIntoView({block: 'center'});" +
		"el.classList.add('ring-2','ring-primary','ring-offset-2');" +
		"setTimeout(() => el.classList.remove('ring-2','ring-primary','ring-offset-2'), 1800);" +
		"});"
}

func strconvQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "'", "\\'") + "'"
}

func (s *Service) HandleOpenCommentsInPlace(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)
	docPath := strings.TrimSpace(c.FormValue("doc_path"))
	if docPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "doc_path is required")
	}
	resp, err := s.commentService.GetCommentsForScopeInternal(
		c.Request().Context(),
		docPath,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	panelArgs := commentui.CommentableMarkdownArgs{
		Surface:  commentui.CommentSurfaceThoughts,
		IDPrefix: commentui.SafeCommentTargetSlug("thoughts", docPath),
		DocPath:  docPath,
		Comments: thoughtsCommentThreads(resp.Comments),
		Routes: commentui.CommentRoutes{
			Show:          "/forms/comments/show",
			Create:        "/forms/comments",
			Cancel:        "/forms/comments/cancel",
			Expand:        "/forms/comments/expand",
			SelectComment: "/thoughts/actions/select-comment",
			Reply: func(string) string {
				return "/forms/replies"
			},
			Resolve: func(string) string {
				return "/forms/resolve"
			},
		},
		HiddenFields: map[string]string{"doc_path": docPath, "context_panel": "1"},
		UserEmail:    userEmail,
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := sse.MarshalAndPatchSignals(
		map[string]any{
			"rightRailActiveTab": "comments",
			"workbench": map[string]any{
				"focused": false,
				"regions": map[string]any{
					"docWorkbenchRight": map[string]any{"visible": true},
				},
			},
		},
	); err != nil {
		return err
	}
	return sse.PatchElementTempl(
		CommentsRightRailPanel(panelArgs),
		datastar.WithSelectorID("doc-right-comments-panel"),
	)
}

func (s *Service) OpenChatForDocument(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)
	if strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing user")
	}
	if s.chatWorkspaceResolver == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"chat workspace resolver is not configured",
		)
	}
	documentPath := strings.TrimSpace(c.FormValue("doc_path"))
	if documentPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "doc_path is required")
	}
	candidates, err := s.chatWorkspaceResolver.ResolveChatWorkspaceCandidates(
		c.Request().Context(),
		userEmail,
		documentPath,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if len(candidates) == 0 {
		return sse.PatchElementTempl(OpenChatEmptyState(documentPath))
	}
	selected := candidates[len(candidates)-1]
	result, err := s.chatWorkspaceResolver.OpenChatWorkspace(
		c.Request().Context(),
		userEmail,
		selected.RootPath,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := sse.MarshalAndPatchSignals(
		map[string]any{
			"rightRailActiveTab": "chat",
			"workbench": map[string]any{
				"focused": false,
				"regions": map[string]any{
					"docWorkbenchRight": map[string]any{"visible": true},
				},
			},
		},
	); err != nil {
		return err
	}
	attachmentPath := "thoughts/" + strings.TrimPrefix(
		filepath.ToSlash(documentPath),
		"thoughts/",
	)
	if err := sse.PatchElementTempl(
		OpenChatRightRailPanel(attachmentPath, result),
		datastar.WithSelectorID("doc-right-chat-panel"),
	); err != nil {
		return err
	}
	return sse.ExecuteScript(
		"document.getElementById('agent-chat-composer-input')?.focus()",
	)
}

func (s *Service) listWorkbenchWorkspaces(c echo.Context) []db.Workspace {
	resolver, ok := s.workspaceResolver.(WorkspaceListResolver)
	if !ok {
		return nil
	}
	workspaces, err := resolver.ListWorkspaces(
		c.Request().Context(),
		thoughtsWorkspacesLimit,
	)
	if err != nil {
		return nil
	}
	return workspaces
}

func (s *Service) SelectChatWorkspaceCandidate(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)
	if strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing user")
	}
	if s.chatWorkspaceResolver == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"chat workspace resolver is not configured",
		)
	}

	rootPath := strings.TrimSpace(c.FormValue("root_path"))
	result, err := s.chatWorkspaceResolver.OpenChatWorkspace(
		c.Request().Context(),
		userEmail,
		rootPath,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect(result.URL)
}
