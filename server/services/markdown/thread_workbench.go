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
	chatOpen = routeChatOpen && !commentsOpen
	return chatOpen, commentsOpen
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
	if strings.TrimSpace(planDir) == "" {
		return "", false, nil
	}
	artifact, err := CanonicalThoughtsDocPath(path.Join(planDir, "design.md"))
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
			workbench.ArtifactOpenFromRequest(c.Request()),
			"Chat",
			WorkbenchUnavailable("Select a thread to open chat."),
			BuildChatHeaderOverflow(artifactPage, artifactDoc, true),
		),
		Artifact: artifactComp,
		Comments: WorkbenchUnavailable(
			"Select an artifact to view comments.",
		),
		ThreadsOpen: workbench.ThreadsOpenFromRequest(c.Request()),
		ChatOpen:    false,
		// Index land: cookie missing => open so route-defaults Stories stay green.
		ArtifactOpen: workbench.ArtifactOpenFromRequest(c.Request()),
		CommentsOpen: false,
	})
	if err != nil {
		return err
	}
	// Bare /threads: keep artifact column Visible for route-defaults, but first-paint
	// mobile pane is the roster (not the empty artifact).
	if !hasArtifact {
		state.Config.Mobile.ActiveRegionID = workbench.WorkbenchV2ThreadsRegionID
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
	artifact, comments, artifactPage, artifactDoc, err := s.threadArtifactAndComments(
		c,
		threadID,
		c.QueryParam("artifact"),
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
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(
				c.Request().Context(),
				s.rosterSelectionForLiveThread(c.Request().Context(), threadID),
			),
		),
		Chat: chatColumn(
			workbench.ThreadsOpenFromRequest(c.Request()),
			workbench.ArtifactOpenFromRequest(c.Request()),
			title,
			chat,
			BuildChatHeaderOverflow(artifactPage, artifactDoc, true),
		),
		Artifact:     artifact,
		Comments:     comments,
		ThreadsOpen:  workbench.ThreadsOpenFromRequest(c.Request()),
		ChatOpen:     chatOpen,
		ArtifactOpen: workbench.ArtifactOpenFromRequest(c.Request()),
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
