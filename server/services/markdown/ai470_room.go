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

// ServeAI470Room renders leftover V2 workbench with RosterRail left + real
// chat/ThreadArtifactPane when a shared thread can be resolved.
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

	// Prefer real SharedThreadChat when a thread resolves. Otherwise keep sketch
	// RoomChatPane feel for VA until room↔thread mapping lands (Lead: bundle chat feel).
	chatComp := agenthome.RoomChatPane(kind, id, roomTitle(kind, id), "")
	artifactComp := s.indexArtifactComponent(c, "", false)
	commentsComp := WorkbenchUnavailable("Select an artifact to view comments.")
	chatOpen := true

	if threadID != "" {
		var chat templ.Component
		chat, err = s.workbenchThreadsRenderer.RenderSharedThreadChat(
			c.Request().Context(), threadID, userEmail,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "thread not found")
		}
		if err != nil {
			return err
		}
		chatComp = chat
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
		ThreadsOpen:   workbench.ThreadsOpenFromRequest(c.Request()),
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

func (s *Service) resolveAI470Thread(
	ctx context.Context,
	kind agenthome.RoomKind,
	id string,
) (string, error) {
	id = strings.TrimSpace(id)
	docs := []string{
		"thoughts/owner/plans/" + id + "/design.md",
		"thoughts/shared/plans/" + id + "/design.md",
		"thoughts/" + id + "/design.md",
	}
	if kind == agenthome.KindPlan {
		docs = append([]string{
			"thoughts/owner/plans/" + id + "/design.md",
		}, docs...)
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
	return "", nil
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

func roomTitle(kind agenthome.RoomKind, id string) string {
	switch kind {
	case agenthome.KindDM:
		if id == "bot" {
			return "Bot"
		}
		if id == "research" {
			return "Research agent"
		}
		return id
	case agenthome.KindGroup:
		if id == "vamos-dev" {
			return "Vamos dev"
		}
		return id
	case agenthome.KindPlan:
		if id == "alpha" {
			return "Alpha"
		}
		return id
	default:
		return id
	}
}
