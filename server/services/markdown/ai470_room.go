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
	kind, ok := agenthome.ParseKind(c.Param("kind"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown room kind")
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusNotFound, "room id required")
	}
	if s.workbenchThreadsRenderer == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"thread renderer is not configured",
		)
	}
	userEmail, _ := c.Get("user_email").(string)
	sel := agenthome.RosterSelection{Kind: kind, ID: id}
	threadID, err := s.resolveAI470Thread(c.Request().Context(), kind, id)
	if err != nil {
		return err
	}
	viewport := viewportClassForRequest(c)
	threadsOpen := workbench.ThreadsOpenFromRequest(c.Request())

	var chatComp templ.Component
	artifactComp := s.indexArtifactComponent(c, "", false)
	commentsComp := WorkbenchUnavailable("Select an artifact to view comments.")
	chatOpen := false

	if threadID == "" {
		chatComp = workbench.ChatColumnWithReopen(
			threadsOpen,
			ai470RoomTitle(kind, id),
			WorkbenchUnavailable("No shared thread mapped for this room yet."),
		)
	} else {
		chat, err := s.renderAI470SharedChat(
			c.Request().Context(), kind, id, threadID, userEmail,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "thread not found")
		}
		if err != nil {
			return err
		}
		chatComp = workbench.ChatColumnWithReopen(threadsOpen, ai470RoomTitle(kind, id), chat)
		artifactComp, commentsComp, err = s.threadArtifactAndComments(
			c, threadID, c.QueryParam("artifact"),
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		chatOpen = true
	}

	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		UserEmail:     userEmail,
		ViewportClass: viewport,
		SavedConfig:   s.savedThreadsWorkbenchConfig(c, userEmail, viewport),
		Threads:       agenthome.RosterRail(sel),
		Chat:          chatComp,
		Artifact:      artifactComp,
		Comments:      commentsComp,
		ThreadsOpen:   threadsOpen,
		ChatOpen:      chatOpen,
		ArtifactOpen:  true,
		CommentsOpen:  false,
	})
	if err != nil {
		return err
	}
	return ThreadWorkbenchPage(userEmail, state).Render(
		c.Request().Context(), c.Response().Writer,
	)
}

// sharedThreadIDLister is optional on the workbench thread renderer so agent/group
// rooms can fixture-map onto real shared threads without sketch RoomChatPane.
type sharedThreadIDLister interface {
	ListSharedThreadIDs(ctx context.Context) ([]string, error)
}

func (s *Service) resolveAI470Thread(
	ctx context.Context,
	kind agenthome.RoomKind,
	id string,
) (string, error) {
	id = strings.TrimSpace(id)

	// Plan rooms: prefer design.md identity.
	docs := []string{
		"thoughts/owner/plans/" + id + "/design.md",
		"thoughts/shared/plans/" + id + "/design.md",
		"thoughts/" + id + "/design.md",
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

	// Agent/group (and plan fallback): map onto real shared threads by stable index.
	// Fixtures OK until room↔thread schema lands (Corey P0: same component tree).
	ids, err := s.listSharedThreadIDs(ctx)
	if err != nil {
		return "", err
	}
	idx := -1
	switch {
	case kind == agenthome.KindDM && id == "bot":
		idx = 0
	case kind == agenthome.KindDM && id == "research":
		idx = 1
	case kind == agenthome.KindGroup && id == "vamos-dev":
		idx = 2
	case kind == agenthome.KindPlan && id == "alpha":
		idx = 0
	case kind == agenthome.KindDM || kind == agenthome.KindGroup || kind == agenthome.KindAgentDM:
		idx = 0
	}
	if idx >= 0 && idx < len(ids) {
		return ids[idx], nil
	}
	return "", nil
}

func (s *Service) listSharedThreadIDs(ctx context.Context) ([]string, error) {
	if lister, ok := s.workbenchThreadsRenderer.(sharedThreadIDLister); ok {
		return lister.ListSharedThreadIDs(ctx)
	}
	return nil, nil
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

type chromaFixtureChatRenderer interface {
	RenderSharedThreadChatWithChromaFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
	RenderSharedThreadChatWithGroupBubbleFixture(
		ctx context.Context,
		threadID, userEmail string,
	) (templ.Component, error)
}

func (s *Service) renderAI470SharedChat(
	ctx context.Context,
	kind agenthome.RoomKind,
	roomID, threadID, userEmail string,
) (templ.Component, error) {
	r, ok := s.workbenchThreadsRenderer.(chromaFixtureChatRenderer)
	if ok {
		// Seed visible fenced ```go bubble on dm/bot for chroma VA.
		if kind == agenthome.KindDM && roomID == "bot" {
			return r.RenderSharedThreadChatWithChromaFixture(ctx, threadID, userEmail)
		}
		// Group: multi-author bubbles + NestedQuoteBlock + chroma for Bot-vs-group VA.
		if kind == agenthome.KindGroup && roomID == "vamos-dev" {
			return r.RenderSharedThreadChatWithGroupBubbleFixture(ctx, threadID, userEmail)
		}
	}
	return s.workbenchThreadsRenderer.RenderSharedThreadChat(ctx, threadID, userEmail)
}

func ai470RoomTitle(kind agenthome.RoomKind, id string) string {
	switch {
	case kind == agenthome.KindDM && id == "bot":
		return "Bot"
	case kind == agenthome.KindDM && id == "research":
		return "Research agent"
	case kind == agenthome.KindGroup && id == "vamos-dev":
		return "Vamos dev"
	case kind == agenthome.KindPlan && id == "alpha":
		return "Alpha"
	default:
		if id != "" {
			return id
		}
		return "Chat"
	}
}

