package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/agenthome"
)

// ServeAgentsLand redirects /agents into roster land (dm/bot).
func ServeAgentsLand(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/rooms/dm/bot")
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
	if kind == agenthome.KindPlan && !hasArtifact {
		planRel, resolveErr := s.resolvePlanDirRelForRoom(
			c.Request().Context(), id, "",
		)
		if resolveErr != nil {
			return resolveErr
		}
		if design := thoughtsDesignDocPath(planRel); design != "" {
			artifactPath, hasArtifact, err = optionalThreadArtifact(design)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
	}
	planDoc := artifactPath
	if planDoc != "" && !strings.HasPrefix(planDoc, "thoughts/") {
		planDoc = "thoughts/" + planDoc
	}
	threadID, err := s.resolveAI470Thread(
		c.Request().Context(), kind, id, pairA, pairB, planDoc, userEmail,
	)
	if err != nil {
		return err
	}
	planNeedsLead := false
	if kind == agenthome.KindPlan {
		planNeedsLead, err = s.planRoomNeedsLead(
			c.Request().Context(), id, planDoc,
		)
		if err != nil {
			return err
		}
	}
	viewport := viewportClassForRequest(c)
	threadsOpen := workbench.ThreadsOpenFromRequest(c.Request())

	var chatComp templ.Component
	artifactComp := s.indexArtifactComponent(c, artifactPath, hasArtifact)
	commentsComp := WorkbenchUnavailable("Select an artifact to view comments.")
	chatOpen := false

	if threadID == "" {
		chatBody := WorkbenchUnavailable("No shared thread mapped for this room yet.")
		if planNeedsLead {
			chatBody = s.planLeadBindComponent(c.Request().Context(), id, planDoc)
		}
		chatComp = workbench.ChatColumnWithReopen(
			threadsOpen,
			roomTitle,
			chatBody,
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
		if planNeedsLead {
			chat = s.planLeadBindComponent(c.Request().Context(), id, planDoc)
		}
		chatComp = workbench.ChatColumnWithReopen(
			threadsOpen,
			roomTitle,
			chat,
		)
		artifactQuery := c.QueryParam("artifact")
		if strings.TrimSpace(artifactQuery) == "" {
			artifactQuery = planDoc
		}
		artifactComp, commentsComp, err = s.threadArtifactAndComments(
			c, threadID, artifactQuery,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		chatOpen = true
	}
	if kind == agenthome.KindDM {
		chatOpen = true
	}

	if profileView(c) && kind == agenthome.KindDM {
		artifactComp = s.agentProfilePane(kind, id, c.QueryParam("file"))
		_ = mobileActiveRegionForRoom("profile")
	}

	chatOpen, commentsOpen := chatCommentsOpen(c.Request(), chatOpen)
	artifactOpen := hasArtifact || kind == agenthome.KindPlan
	if viewport.IsDesktop() {
		artifactOpen = true
	}
	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads: agenthome.RosterRail(
			s.liveRoster(c.Request().Context(), sel),
		),
		Chat:                     chatComp,
		Artifact:                 artifactComp,
		Comments:                 commentsComp,
		ThreadsOpen:              threadsOpen,
		ChatOpen:                 chatOpen,
		ArtifactOpen:             artifactOpen,
		CommentsOpen:             commentsOpen,
		MobileChatCommentsHeader: true,
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
	artifact = strings.TrimSpace(artifact)
	if kind == agenthome.KindA2A {
		return s.resolvePairwiseThread(ctx, pairA, pairB, userEmail)
	}

	docs := make([]string, 0, 4)
	if artifact != "" {
		docs = append(docs, artifact)
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		if seen[doc] {
			continue
		}
		seen[doc] = true
		threadID, err := s.workbenchThreadsRenderer.FindSharedThreadForDoc(ctx, doc)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(threadID) != "" {
			return threadID, nil
		}
	}
	if kind == agenthome.KindPlan && artifact != "" &&
		strings.TrimSpace(userEmail) != "" {
		threadID, err := s.workbenchThreadsRenderer.EnsureSharedThreadForDoc(
			ctx, artifact, userEmail,
		)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(threadID) != "" {
			return threadID, nil
		}
		return "", nil
	}
	if kind == agenthome.KindPlan {
		return "", nil
	}

	if kind == agenthome.KindDM {
		return s.resolveBotHomeThread(ctx, id, userEmail)
	}
	return "", nil
}

func (s *Service) resolveBotHomeThread(
	ctx context.Context,
	slug, userEmail string,
) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || s.queries == nil {
		return "", nil
	}
	agent, err := s.queries.GetAgentBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
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
	if s == nil || s.queries == nil {
		return view
	}
	agents, err := s.queries.ListAgents(ctx)
	if err != nil {
		return view
	}
	for _, agent := range agents {
		view.Bots = append(view.Bots, agenthome.RosterBotRow{
			Slug:  agent.Slug,
			Title: agenthome.RosterBotTitle(agent.Name, agent.Slug),
		})
	}
	view.Plans = s.liveRosterPlans(ctx)
	return view
}

func rosterSelectionForThread(threadID string) agenthome.RosterSelection {
	switch strings.TrimSpace(threadID) {
	case "bot", "dm-bot":
		return agenthome.RosterSelection{Kind: agenthome.KindDM, ID: "bot"}
	case "research", "dm-research":
		return agenthome.RosterSelection{Kind: agenthome.KindDM, ID: "research"}
	case "vamos-dev", "group-vamos-dev":
		return agenthome.RosterSelection{Kind: agenthome.KindGroup, ID: "vamos-dev"}
	case "alpha", "plan-alpha":
		return agenthome.RosterSelection{Kind: agenthome.KindPlan, ID: "alpha"}
	default:
		return agenthome.RosterSelection{}
	}
}

func (s *Service) renderAI470SharedChat(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.workbenchThreadsRenderer.RenderSharedThreadChat(ctx, threadID, userEmail)
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
	if kind == agenthome.KindDM && s != nil && s.queries != nil && id != "" {
		agent, err := s.queries.GetAgentBySlug(ctx, id)
		if err == nil {
			return agenthome.RosterBotTitle(agent.Name, agent.Slug)
		}
	}
	if id != "" {
		return id
	}
	return "Chat"
}
