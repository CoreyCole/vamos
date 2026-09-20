package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/agenthome"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

// ServeAgentsLand redirects /agents onto the threads roster.
func ServeAgentsLand(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/threads")
}

// ServeFreeformRoom is GET /rooms/freeform: scoped freeform thread list (or N=0 composer).
func (s *Service) ServeFreeformRoom(c echo.Context) error {
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	_ = userEmail
	sel := agenthome.RosterSelection{Kind: agenthome.KindFreeform}
	roomTitle := s.ai470RoomTitle(c.Request().Context(), agenthome.KindFreeform, "")
	artifactPath, hasArtifact, err := optionalThreadArtifact(c.QueryParam("artifact"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	c.Set(artifactThreadsReopenKey, true)
	viewport := viewportClassForRequest(c)
	threadsOpen := workbench.ThreadsOpenFromRequest(c.Request())
	artifactComp, artifactPage, artifactDoc := s.indexArtifactComponent(
		c, artifactPath, hasArtifact,
	)
	commentsComp := WorkbenchUnavailable("Select an artifact to view comments.")
	artifactChromeOpen := workbench.ArtifactOpenFromRequest(c.Request())
	rows, listErr := s.freeformScopedConversationRows(c.Request().Context())
	if listErr != nil {
		return listErr
	}
	if hasArtifact && artifactPage != nil {
		commentsComp = s.commentsPanelForThoughtsPage(c, artifactPage)
	}
	chatComp := chatColumnForAI470Room(
		agenthome.KindFreeform,
		threadsOpen,
		artifactChromeOpen,
		roomTitle,
		renderScopedThreadListOrComposer(
			rows,
			emptyScopeComposerAction("freeform", ""),
			artifactPath,
		),
		BuildChatHeaderOverflow(artifactPage, artifactDoc, true),
	)
	chatOpen, commentsOpen := chatCommentsOpen(c.Request(), true)
	artifactOpen := hasArtifact
	if viewport.IsDesktop() {
		artifactOpen = artifactChromeOpen
	}
	if !hasArtifact {
		commentsOpen = false
	}
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(c.Request().Context(), sel),
		),
		Chat:         chatComp,
		Artifact:     artifactComp,
		Comments:     commentsComp,
		ThreadsOpen:  threadsOpen,
		ChatOpen:     chatOpen,
		ArtifactOpen: artifactOpen,
		CommentsOpen: commentsOpen,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(userEmail, state).Render(
		c.Request().Context(), c.Response().Writer,
	)
}

// ServeAI470Room renders leftover V2 workbench with RosterRail left + the same
// SharedThreadChat / ThreadArtifactPane tree as /threads/:id (never sketch RoomChatPane).
func (s *Service) ServeAI470Room(c echo.Context) error {
	kind, id, pairA, pairB, err := parseAI470RoomParams(c)
	if err != nil {
		return err
	}
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	sel := agenthome.RosterSelection{Kind: kind, ID: id}
	roomTitle := s.ai470RoomTitle(c.Request().Context(), kind, id)
	artifactPath, hasArtifact, err := optionalThreadArtifact(c.QueryParam("artifact"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	c.Set(artifactThreadsReopenKey, true)
	if kind == agenthome.KindPlan && !hasArtifact {
		planRel, resolveErr := s.resolvePlanDirRelForRoom(
			c.Request().Context(), id, "",
		)
		if resolveErr != nil {
			return resolveErr
		}
		if doc := s.thoughtsPlanArtifactPath(planRel); doc != "" {
			artifactPath, hasArtifact, err = optionalThreadArtifact(doc)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
	}
	planDoc := artifactPath
	if planDoc != "" && !strings.HasPrefix(planDoc, "thoughts/") {
		planDoc = "thoughts/" + planDoc
	}
	// Plan rooms no longer gate chat on binding a roster persona as "lead".
	// Each plan dir owns its plan-lead identity; composer must work without
	// a SetPlanWorkspaceLeadAgent bind (Corey UX lock).
	viewport := viewportClassForRequest(c)
	threadsOpen := workbench.ThreadsOpenFromRequest(c.Request())

	var chatComp templ.Component
	artifactComp, artifactPage, artifactDoc := s.indexArtifactComponent(
		c, artifactPath, hasArtifact,
	)
	commentsComp := WorkbenchUnavailable("Select an artifact to view comments.")
	chatOpen := false
	includePlanChat := kind != agenthome.KindPlan
	// View Chat from thoughts must leave the artifact open even if the user
	// previously hid details (wb2_artifact_open=0). Close details still works
	// after landing; this only forces open on plan+artifact entry.
	forceArtifactOpen := kind == agenthome.KindPlan && hasArtifact
	artifactChromeOpen := workbench.ArtifactOpenFromRequest(c.Request())
	if forceArtifactOpen {
		artifactChromeOpen = true
	}

	if kind == agenthome.KindDM || kind == agenthome.KindPlan {
		var rows []agenthome.ConversationRowArgs
		var listErr error
		if kind == agenthome.KindDM {
			if err := s.requireKnownBot(id); err != nil {
				return err
			}
			rows, listErr = s.botScopedConversationRows(c.Request().Context(), id)
		} else {
			rows, listErr = s.planScopedConversationRows(
				c.Request().Context(), id, planDoc,
			)
		}
		if listErr != nil {
			return listErr
		}
		if hasArtifact && artifactPage != nil {
			commentsComp = s.commentsPanelForThoughtsPage(c, artifactPage)
		}
		scopeKind := "dm"
		if kind == agenthome.KindPlan {
			scopeKind = "plan"
		}
		chatComp = chatColumnForAI470Room(
			kind,
			threadsOpen,
			artifactChromeOpen,
			roomTitle,
			renderScopedThreadListOrComposer(
				rows,
				emptyScopeComposerAction(scopeKind, id),
				artifactPath,
			),
			BuildChatHeaderOverflow(artifactPage, artifactDoc, includePlanChat),
		)
		chatOpen = true
	} else {
		threadID, err := s.resolveAI470Thread(
			c.Request().Context(), kind, id, pairA, pairB, planDoc, userEmail,
		)
		if err != nil {
			return err
		}
		if threadID == "" {
			chatBody := WorkbenchUnavailable("No shared thread mapped for this room yet.")
			chatComp = chatColumnForAI470Room(
				kind,
				threadsOpen,
				artifactChromeOpen,
				roomTitle,
				chatBody,
				BuildChatHeaderOverflow(artifactPage, artifactDoc, includePlanChat),
			)
			chatOpen = kind == agenthome.KindPlan && hasArtifact
		} else {
			chat, err := s.renderAI470SharedChat(
				c.Request().Context(), threadID, userEmail,
			)
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "thread not found")
			}
			if err != nil {
				return err
			}
			artifactQuery := c.QueryParam("artifact")
			if strings.TrimSpace(artifactQuery) == "" {
				artifactQuery = planDoc
			}
			artifactComp, commentsComp, artifactPage, artifactDoc, err = s.threadArtifactAndComments(
				c,
				threadID,
				artifactQuery,
			)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			chatComp = chatColumnForAI470Room(
				kind,
				threadsOpen,
				artifactChromeOpen,
				roomTitle,
				chat,
				BuildChatHeaderOverflow(artifactPage, artifactDoc, includePlanChat),
			)
			chatOpen = true
		}
	}

	if profileView(c) && kind == agenthome.KindDM {
		artifactComp = s.agentProfilePane(kind, id, c.QueryParam("file"))
		_ = mobileActiveRegionForRoom("profile")
	}

	chatOpen, commentsOpen := chatCommentsOpen(c.Request(), chatOpen)
	artifactOpen := hasArtifact || kind == agenthome.KindPlan
	if viewport.IsDesktop() {
		// Class B cookie↔SSR (Threads-reopen pattern); default open when missing.
		artifactOpen = artifactChromeOpen
	}
	if forceArtifactOpen {
		artifactOpen = true
		workbench.WriteArtifactOpenCookie(c.Response(), true)
	}
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(c.Request().Context(), sel),
		),
		Chat:         chatComp,
		Artifact:     artifactComp,
		Comments:     commentsComp,
		ThreadsOpen:  threadsOpen,
		ChatOpen:     chatOpen,
		ArtifactOpen: artifactOpen,
		CommentsOpen: commentsOpen,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(userEmail, state).Render(
		c.Request().Context(), c.Response().Writer,
	)
}

func parseAI470RoomParams(
	c echo.Context,
) (agenthome.RoomKind, string, string, string, error) {
	pairA := strings.TrimSpace(c.Param("a"))
	pairB := strings.TrimSpace(c.Param("b"))
	if pairA != "" || pairB != "" {
		if pairA == "" || pairB == "" {
			return "", "", "", "", echo.NewHTTPError(
				http.StatusNotFound, "pairwise rooms require two slugs",
			)
		}
		return agenthome.KindA2A, pairA + "/" + pairB, pairA, pairB, nil
	}
	kind, ok := agenthome.ParseKind(c.Param("kind"))
	if !ok {
		return "", "", "", "", echo.NewHTTPError(http.StatusNotFound, "unknown room kind")
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return "", "", "", "", echo.NewHTTPError(http.StatusNotFound, "room id required")
	}
	if kind == agenthome.KindA2A {
		return "", "", "", "", echo.NewHTTPError(
			http.StatusNotFound, "pairwise rooms require /rooms/a2a/{a}/{b}",
		)
	}
	if kind == agenthome.KindGroup || kind == agenthome.KindAgentDM {
		return "", "", "", "", echo.NewHTTPError(
			http.StatusNotFound, "room kind is not in v1",
		)
	}
	return kind, id, "", "", nil
}

func (s *Service) resolveAI470Thread(
	ctx context.Context,
	kind agenthome.RoomKind,
	id, pairA, pairB, artifact, userEmail string,
) (string, error) {
	id = strings.TrimSpace(id)
	_ = strings.TrimSpace(artifact)
	if kind == agenthome.KindA2A {
		return s.resolvePairwiseThread(ctx, pairA, pairB, userEmail)
	}

	if kind == agenthome.KindPlan {
		return "", nil
	}
	if kind == agenthome.KindDM {
		return s.resolveBotHomeThread(ctx, id, userEmail)
	}
	return "", nil
}

func (s *Service) commentsPanelForThoughtsPage(
	c echo.Context,
	page *PageArgs,
) templ.Component {
	if page == nil {
		return WorkbenchUnavailable("Select an artifact to view comments.")
	}
	userEmail, _ := c.Get("user_email").(string)
	threads := []commentui.CommentThreadView{}
	if s.commentService != nil {
		if response, err := s.commentService.GetCommentsForScopeInternal(
			c.Request().Context(),
			page.FilePath,
		); err == nil {
			page.Comments = response
			threads = thoughtsCommentThreads(response.Comments)
		}
	}
	page.CommentUI = s.buildCommentUI(page, userEmail, threads)
	return commentui.CommentsContextPanel(
		commentui.BuildCommentsPanelArgs(page.CommentUI, ""),
	)
}

func (s *Service) resolveBotHomeThread(
	ctx context.Context,
	slug, userEmail string,
) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || s.queries == nil || s.roster == nil {
		return "", nil
	}
	agent, err := s.roster.Get(slug)
	if errors.Is(err, roster.ErrNotFound) || errors.Is(err, roster.ErrArchived) {
		return "", echo.NewHTTPError(http.StatusNotFound, "agent not found")
	}
	if err != nil {
		return "", err
	}
	return s.ensureBotHomeThread(ctx, agent, userEmail)
}

func (s *Service) liveRoster(
	ctx context.Context,
	sel agenthome.RosterSelection,
) agenthome.RosterView {
	view := agenthome.RosterView{Selection: sel}
	view.Docs = s.liveRosterDocs()
	if s == nil || s.roster == nil {
		view.Plans = s.liveRosterPlans(ctx)
		return view
	}
	agents, err := s.roster.List()
	if err != nil {
		view.Plans = s.liveRosterPlans(ctx)
		return view
	}
	for _, agent := range agents {
		preview := lastBotHomePreview(ctx, s.queries, s.basePath, agent.Slug)
		view.Bots = append(view.Bots, agenthome.RosterBotRow{
			Slug:    agent.Slug,
			Title:   agenthome.RosterBotTitle(agent.Name, agent.Slug),
			Preview: preview.Text,
			Time:    rosterPlanTime(preview.Time),
		})
	}
	view.Plans = s.liveRosterPlans(ctx)
	return view
}

func (s *Service) rosterSelectionForLiveThread(
	ctx context.Context,
	threadID string,
) agenthome.RosterSelection {
	threadID = strings.TrimSpace(threadID)
	if s == nil || s.queries == nil || threadID == "" {
		return agenthome.RosterSelection{}
	}
	thread, err := s.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return agenthome.RosterSelection{}
	}
	switch thread.RoomKind {
	case "bot_home":
		if !thread.AgentSlug.Valid || strings.TrimSpace(thread.AgentSlug.String) == "" {
			return agenthome.RosterSelection{}
		}
		return agenthome.RosterSelection{
			Kind: agenthome.KindDM,
			ID:   thread.AgentSlug.String,
		}
	case "plan":
		rel := ""
		if thread.PlanDirRel.Valid {
			rel = thread.PlanDirRel.String
		}
		id := thoughtsAgentsRoomID(rel)
		if id == "" {
			id = planLeadRoomID(rel)
		}
		if id == "" {
			return agenthome.RosterSelection{}
		}
		return agenthome.RosterSelection{Kind: agenthome.KindPlan, ID: id}
	case "pairwise":
		return s.pairwiseRosterSelection(ctx, thread)
	default:
		return agenthome.RosterSelection{}
	}
}

func (s *Service) pairwiseRosterSelection(
	ctx context.Context,
	thread db.AgentThread,
) agenthome.RosterSelection {
	_ = ctx
	if !thread.PairAgentSlugA.Valid || !thread.PairAgentSlugB.Valid {
		return agenthome.RosterSelection{}
	}
	left := strings.TrimSpace(thread.PairAgentSlugA.String)
	right := strings.TrimSpace(thread.PairAgentSlugB.String)
	if left == "" || right == "" {
		return agenthome.RosterSelection{}
	}
	if right < left {
		left, right = right, left
	}
	return agenthome.RosterSelection{
		Kind: agenthome.KindA2A,
		ID:   left + "/" + right,
	}
}

func (s *Service) renderAI470SharedChat(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.workbenchThreadsRenderer.RenderSharedThreadChat(ctx, threadID, userEmail)
}

func chatColumnForAI470Room(
	kind agenthome.RoomKind,
	threadsOpen bool,
	artifactOpen bool,
	title string,
	body templ.Component,
	overflow templ.Component,
) templ.Component {
	if kind == agenthome.KindPlan {
		return workbench.ChatColumnWithPlanReopen(
			threadsOpen, artifactOpen, title, body, overflow,
		)
	}
	return workbench.ChatColumnWithReopen(
		threadsOpen, artifactOpen, title, body, overflow,
	)
}

// AI470RoomComposerDisabled is the view-only composer gate.
// Pairwise /rooms/a2a/{a}/{b} (TODO-3.2) has no send form. Do not treat
// leftover KindAgentDM as pairwise.
func AI470RoomComposerDisabled(kind agenthome.RoomKind, id string) bool {
	_ = id
	return kind == agenthome.KindA2A
}

// AI470PairwiseComposerDisabled is the template flag for two-slug pairwise rooms.
func AI470PairwiseComposerDisabled() bool {
	return true
}

func (s *Service) ai470RoomTitle(
	ctx context.Context,
	kind agenthome.RoomKind,
	id string,
) string {
	id = strings.TrimSpace(id)
	if kind == agenthome.KindDM && s != nil && s.roster != nil && id != "" {
		agent, err := s.roster.Get(id)
		if err == nil {
			return agenthome.RosterBotTitle(agent.Name, agent.Slug)
		}
	}
	if kind == agenthome.KindPlan && id != "" {
		if h := workbench.HumanizePlanRoomID(id); h != "" {
			return h
		}
	}
	if kind == agenthome.KindFreeform {
		return "Freeform"
	}
	if id != "" {
		return id
	}
	return "Chat"
}
