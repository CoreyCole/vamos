package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func chatCommentsOpen(r *http.Request, routeChatOpen bool) (chatOpen, commentsOpen bool) {
	commentsOpen = workbench.CommentsOpenFromRequest(r)
	chatOpen = routeChatOpen && workbench.ChatOpenFromRequest(r) && !commentsOpen
	return chatOpen, commentsOpen
}

const thoughtsSharedThreadUnavailable = "No shared thread mapped for this document yet."

func thoughtsEnsureDocPath(docPath string) string {
	canonical, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		canonical, err = CanonicalThoughtsDirPath(docPath)
		if err != nil {
			return strings.TrimSpace(docPath)
		}
	}
	canonical = strings.Trim(strings.TrimSpace(canonical), "/")
	if canonical == "" {
		return strings.TrimSpace(docPath)
	}
	if strings.HasPrefix(canonical, "thoughts/") {
		return canonical
	}
	return "thoughts/" + canonical
}

func (s *Service) thoughtsWorkbenchChatColumn(
	c echo.Context,
	docPath string,
	userEmail string,
	pageArgs *PageArgs,
) (templ.Component, error) {
	_ = userEmail
	threadsOpen := workbench.ThreadsOpenFromRequest(c.Request())
	title := "Chat"
	if id := planLeadRoomID(docPath); id != "" {
		if h := workbench.HumanizePlanRoomID(id); h != "" {
			title = h
		}
	}
	overflow := BuildChatHeaderOverflow(pageArgs, docPath, true)
	body := WorkbenchUnavailable(thoughtsSharedThreadUnavailable)
	scopeID := ""
	if s != nil {
		scopeID = thoughtsChatHref(s.basePath, docPath)
	}
	if scopeID == "" {
		body = WorkbenchUnavailable(thoughtsSharedThreadUnavailable)
	} else {
		artifact := thoughtsEnsureDocPath(docPath)
		roomID := ""
		if root, ok := InferWorkspaceRoot(s.basePath, docPath); ok {
			roomID = thoughtsAgentsRoomID(root)
		}
		if roomID == "" {
			roomID = planLeadRoomID(docPath)
		}
		rows, err := s.planScopedConversationRows(
			c.Request().Context(), roomID, artifact,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		body = renderScopedThreadListOrComposer(
			rows,
			"plan",
			roomID,
			artifact,
		)
	}
	column := workbench.ChatColumnWithReopen
	if thoughtsChatHref(s.basePath, docPath) != "" || planLeadRoomID(docPath) != "" {
		column = workbench.ChatColumnWithPlanReopen
	}
	return column(
		threadsOpen,
		workbench.ArtifactOpenFromRequest(c.Request()),
		title,
		body,
		overflow,
	), nil
}

func (s *Service) savedThreadsWorkbenchConfig(
	c echo.Context,
	userEmail string,
	viewport workbench.ViewportClass,
) *workbench.WorkbenchConfig {
	if s.layoutPrefs == nil || strings.TrimSpace(userEmail) == "" {
		return nil
	}
	config, err := s.layoutPrefs.Get(
		c.Request().Context(),
		userEmail,
		workbench.WorkbenchPageThreads,
		workbench.WorkbenchViewSplit,
		viewport,
	)
	if err != nil {
		return nil
	}
	return config
}

func (s *Service) ResolveThreadArtifact(
	ctx context.Context,
	threadID, rawDoc string,
) (string, error) {
	artifact, _, err := s.resolveThreadArtifact(
		ctx, threadID, rawDoc, strings.TrimSpace(rawDoc) != "",
	)
	if err != nil {
		return "", err
	}
	if _, err := s.GetDirectoryListing(artifact); err == nil {
		return artifact, nil
	}
	if _, err := s.RenderThoughtsDocument(ctx, artifact); err != nil {
		return "", err
	}
	return artifact, nil
}

func (s *Service) resolveThreadArtifact(
	ctx context.Context,
	threadID, rawDoc string,
	hasArtifact bool,
) (string, bool, error) {
	if s.workbenchThreadsRenderer == nil {
		return "", false, errors.New("thread renderer is not configured")
	}
	planDir, err := s.workbenchThreadsRenderer.ResolveSharedThreadPlanDir(ctx, threadID)
	if err != nil {
		return "", false, err
	}
	if hasArtifact {
		artifact, explicit, err := optionalThreadArtifact(rawDoc)
		return artifact, explicit, err
	}
	if strings.TrimSpace(rawDoc) != "" {
		artifact, explicit, err := optionalThreadArtifact(rawDoc)
		return artifact, explicit, err
	}
	if strings.TrimSpace(planDir) == "" {
		return "", false, nil
	}
	artifact, err := CanonicalThoughtsDocPath(s.thoughtsPlanArtifactPath(planDir))
	return artifact, false, err
}

func optionalThreadArtifact(raw string) (string, bool, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", false, nil
	}
	if trimmed == "thoughts" {
		return "", true, nil
	}
	artifact, err := CanonicalThoughtsDocPath(raw)
	if err == nil {
		return artifact, true, nil
	}
	artifact, dirErr := CanonicalThoughtsDirPath(raw)
	if dirErr != nil {
		return "", false, err
	}
	return artifact, true, nil
}

func (s *Service) artifactContent(
	c echo.Context,
	artifact string,
	explicit bool,
) (templ.Component, *PageArgs, bool) {
	if !explicit {
		return WorkbenchUnavailable("Select a thread to view an artifact."), nil, false
	}
	page, err := s.RenderThoughtsDocument(c.Request().Context(), artifact)
	if err == nil {
		page.UserEmail, _ = c.Get("user_email").(string)
		return DocumentPanel(BuildDocumentPanelArgs(page)), page, false
	}
	if _, dirErr := s.GetDirectoryListing(artifact); dirErr == nil {
		return WorkbenchUnavailable("Select a file from the artifact browser."), nil, true
	}
	return WorkbenchUnavailable("The requested artifact is unavailable."), nil, false
}

func (s *Service) indexArtifactComponent(
	c echo.Context,
	artifact string,
	explicit bool,
) (templ.Component, *PageArgs, string) {
	content, page, _ := s.artifactContent(c, artifact, explicit)
	if !explicit {
		return content, nil, ""
	}
	browser, err := s.threadArtifactBrowser(c, "", artifact)
	if err != nil {
		return WorkbenchUnavailable("The artifact is unavailable."), page, artifact
	}
	setViewDocumentToggle(&browser, false, "")
	applyWorkbenchPathHeaderThreadsReopen(c, &browser)
	browser.HeaderActions = BuildThreadArtifactHeaderActions(page, browser.DocPath)
	if page != nil {
		panelArgs := BuildDocumentPanelArgs(page)
		panelArgs.Document.WorkbenchActions = nil
		content = DocumentPanel(panelArgs)
	}
	return ThreadArtifactPane(browser, content), page, browser.DocPath
}

// threadChatHeaderTitle resolves the /threads/:id header room name.
// Plan-linked threads pass the plan-dir basename (often YYYY-MM-DD_HH-MM-SS_*)
// so ChatColumnWithPlanReopen + ParseChatHeaderTitle yield Display + Datetime.
// Non-plan threads fall back to roster/bot title or "Chat".
func (s *Service) threadChatHeaderTitle(
	ctx context.Context,
	threadID string,
) (title string, planLinked bool) {
	title = "Chat"
	if s == nil || s.workbenchThreadsRenderer == nil {
		return title, false
	}
	planDir, err := s.workbenchThreadsRenderer.ResolveSharedThreadPlanDir(ctx, threadID)
	if err == nil && strings.TrimSpace(planDir) != "" {
		id := planLeadRoomID(planDir)
		if id == "" {
			id = path.Base(strings.Trim(planDir, "/"))
		}
		if id != "" && id != "." && id != "/" {
			return id, true
		}
	}
	sel := s.rosterSelectionForLiveThread(ctx, threadID)
	if sel.ID != "" && (sel.Kind == agenthome.KindDM ||
		sel.Kind == agenthome.KindA2A ||
		sel.Kind == agenthome.KindPlan) {
		if t := s.ai470RoomTitle(ctx, sel.Kind, sel.ID); strings.TrimSpace(t) != "" {
			return t, sel.Kind == agenthome.KindPlan
		}
	}
	return title, false
}

func (s *Service) ServeThreads(c echo.Context) error {
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	artifactPath, hasArtifact, err := optionalThreadArtifact(c.QueryParam("artifact"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if hasArtifact {
		href := thoughtsChatHref(s.basePath, artifactPath)
		if href == "" {
			href = thoughtsChatHref("", artifactPath)
		}
		if href != "" {
			if dir := strings.TrimSpace(c.QueryParam("artifact_dir")); dir != "" {
				href = withArtifactDirQuery(href, dir)
			}
			return c.Redirect(http.StatusSeeOther, href)
		}
	}
	threads, err := s.workbenchThreadsRenderer.RenderWorkbenchThreadList(
		c.Request().Context(), "", artifactPath,
	)
	if err != nil {
		return err
	}
	viewport := viewportClassForRequest(c)
	_ = threads // AI-470 converge: left rail is roster, not thread list.
	artifactComp, artifactPage, artifactDoc := s.indexArtifactComponent(
		c, artifactPath, hasArtifact,
	)
	chat := WorkbenchUnavailable("Pick a roster scope to see its threads.")
	chatTitle := "Threads"
	chatOpen, commentsOpen := chatCommentsOpen(c.Request(), true)
	artifactOpen := hasArtifact && workbench.ArtifactOpenFromRequest(c.Request())
	if !hasArtifact {
		commentsOpen = false
	}
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(
				c.Request().Context(),
				agenthome.RosterSelection{},
			),
		),
		Chat: workbench.ChatColumnWithReopen(
			workbench.ThreadsOpenFromRequest(c.Request()),
			artifactOpen,
			chatTitle,
			chat,
			BuildChatHeaderOverflow(artifactPage, artifactDoc, true),
		),
		Artifact: artifactComp,
		Comments: WorkbenchUnavailable(
			"Select an artifact to view comments.",
		),
		ThreadsOpen:  workbench.ThreadsOpenFromRequest(c.Request()),
		ChatOpen:     chatOpen,
		ArtifactOpen: artifactOpen,
		CommentsOpen: commentsOpen,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(
		userEmail,
		state,
	).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) ServeThread(c echo.Context) error {
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	threadID := strings.TrimSpace(c.Param("threadID"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	userEmail, _ := c.Get("user_email").(string)
	threads, err := s.workbenchThreadsRenderer.RenderWorkbenchThreadList(
		c.Request().Context(), threadID, "",
	)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	if err != nil {
		return err
	}
	chat, err := s.renderSharedThreadChatForRequest(
		c,
		threadID,
		userEmail,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	if err != nil {
		return err
	}
	sel := s.rosterSelectionForLiveThread(c.Request().Context(), threadID)
	artifactQuery := c.QueryParam("artifact")
	if sel.Kind == agenthome.KindDM && !c.Request().URL.Query().Has("artifact") {
		artifactQuery = s.thoughtsBotHomeArtifactPath(sel.ID)
	}
	artifact, comments, artifactPage, artifactDoc, err := s.threadArtifactAndComments(
		c,
		threadID,
		artifactQuery,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	viewport := viewportClassForRequest(c)
	_ = threads // AI-470 converge: left rail is roster, not thread list.
	chatOpen, commentsOpen := chatCommentsOpen(c.Request(), true)
	title, planLinked := s.threadChatHeaderTitle(c.Request().Context(), threadID)
	chatColumn := workbench.ChatColumnWithReopen
	if planLinked {
		chatColumn = workbench.ChatColumnWithPlanReopen
	}
	hasSplitArtifact := planLinked ||
		c.Request().URL.Query().Has("artifact")
	paneMounted := hasSplitArtifact || sel.Kind == agenthome.KindDM
	artifactOpen := paneMounted &&
		workbench.ArtifactOpenFromRequest(c.Request())
	if !hasSplitArtifact && sel.Kind != agenthome.KindDM {
		commentsOpen = false
	}
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(
				c.Request().Context(),
				sel,
			),
		),
		Chat: chatColumn(
			workbench.ThreadsOpenFromRequest(c.Request()),
			artifactOpen,
			title,
			chat,
			BuildChatHeaderOverflow(artifactPage, artifactDoc, true),
		),
		Artifact:     artifact,
		Comments:     comments,
		ThreadsOpen:  workbench.ThreadsOpenFromRequest(c.Request()),
		ChatOpen:     chatOpen,
		ArtifactOpen: artifactOpen,
		CommentsOpen: commentsOpen,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(
		userEmail,
		state,
	).Render(c.Request().Context(), c.Response().Writer)
}

type densityFixtureChatRenderer interface {
	RenderSharedThreadChatWithDensityFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

type groupBubbleFixtureChatRenderer interface {
	RenderSharedThreadChatWithGroupBubbleFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

type pairwiseFixtureChatRenderer interface {
	RenderSharedThreadChatWithPairwiseFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

type historyFixtureChatRenderer interface {
	RenderSharedThreadChatWithHistoryFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

type messageThreadFixtureChatRenderer interface {
	RenderSharedThreadChatWithMessageThreadFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

type openMessageThreadChatRenderer interface {
	RenderSharedThreadChatOpen(
		ctx context.Context,
		threadID, userEmail, openParentEntryID string,
	) (templ.Component, error)
}

func densityFixtureRequested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("density_fixture")) == "1" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.QueryParam("fixture")), "density")
}

func groupBubbleFixtureRequested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("group_bubble_fixture")) == "1" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.QueryParam("fixture")), "group")
}

func pairwiseFixtureRequested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("pairwise_fixture")) == "1" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.QueryParam("fixture")), "pairwise")
}

func historyFixtureRequested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("history_fixture")) == "1" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.QueryParam("fixture")), "history")
}

func messageThreadFixtureRequested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("message_thread_fixture")) == "1" {
		return true
	}
	alias := strings.ToLower(strings.TrimSpace(c.QueryParam("fixture")))
	return alias == "replies" || alias == "message_thread"
}

func openMessageThreadParent(c echo.Context, threadID string) string {
	open := strings.TrimSpace(c.QueryParam("thread"))
	if open == "" || open == strings.TrimSpace(threadID) {
		return ""
	}
	return open
}

func (s *Service) renderSharedThreadChatForRequest(
	c echo.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	ctx := c.Request().Context()
	// Prefer explicit *_fixture=1 keys; dual-form ?fixture= aliases match FE contract.
	if densityFixtureRequested(c) {
		if r, ok := s.workbenchThreadsRenderer.(densityFixtureChatRenderer); ok {
			return r.RenderSharedThreadChatWithDensityFixture(ctx, threadID, userEmail)
		}
	}
	if groupBubbleFixtureRequested(c) {
		if r, ok := s.workbenchThreadsRenderer.(groupBubbleFixtureChatRenderer); ok {
			return r.RenderSharedThreadChatWithGroupBubbleFixture(
				ctx,
				threadID,
				userEmail,
			)
		}
	}
	if pairwiseFixtureRequested(c) {
		if r, ok := s.workbenchThreadsRenderer.(pairwiseFixtureChatRenderer); ok {
			return r.RenderSharedThreadChatWithPairwiseFixture(ctx, threadID, userEmail)
		}
	}
	// Priority: density > group_bubble > pairwise > history > message_thread/replies > open ?thread= > none (FE/E2E contract).
	if historyFixtureRequested(c) {
		if r, ok := s.workbenchThreadsRenderer.(historyFixtureChatRenderer); ok {
			return r.RenderSharedThreadChatWithHistoryFixture(ctx, threadID, userEmail)
		}
	}
	if messageThreadFixtureRequested(c) {
		if r, ok := s.workbenchThreadsRenderer.(messageThreadFixtureChatRenderer); ok {
			return r.RenderSharedThreadChatWithMessageThreadFixture(
				ctx,
				threadID,
				userEmail,
			)
		}
	}
	if open := openMessageThreadParent(c, threadID); open != "" {
		if r, ok := s.workbenchThreadsRenderer.(openMessageThreadChatRenderer); ok {
			return r.RenderSharedThreadChatOpen(ctx, threadID, userEmail, open)
		}
	}
	return s.workbenchThreadsRenderer.RenderSharedThreadChat(ctx, threadID, userEmail)
}
