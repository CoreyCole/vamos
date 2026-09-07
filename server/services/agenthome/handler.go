package agenthome

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// RoomKind is one of the four AI-470 first-class room kinds.
type RoomKind string

const (
	KindDM      RoomKind = "dm"
	KindAgentDM RoomKind = "agent_dm"
	KindGroup   RoomKind = "group"
	KindPlan    RoomKind = "plan"
)

// ParseKind validates a room kind path segment.
func ParseKind(raw string) (RoomKind, bool) {
	switch RoomKind(strings.TrimSpace(raw)) {
	case KindDM, KindAgentDM, KindGroup, KindPlan:
		return RoomKind(raw), true
	default:
		return "", false
	}
}

// RegisterAgentsRoutes mounts GET /agents — redirects into roster land.
// serve is typically markdown.ServeAgentsLand.
func RegisterAgentsRoutes(g *echo.Group, serve echo.HandlerFunc) {
	g.GET("", serve)
}

// RegisterRoomRoutes mounts GET /rooms/:kind/:id.
// serve is typically markdownService.ServeAI470Room.
func RegisterRoomRoutes(g *echo.Group, serve echo.HandlerFunc) {
	g.GET("/:kind/:id", serve)
}

// RedirectAgentsLand is a tiny helper when markdown is unavailable in tests.
func RedirectAgentsLand(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/rooms/dm/bot")
}
