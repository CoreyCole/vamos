package agenthome

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// RoomKind is one of the four AI-470 first-class room kinds.
type RoomKind string

const (
	KindDM       RoomKind = "dm"
	KindAgentDM  RoomKind = "agent_dm"
	KindGroup    RoomKind = "group"
	KindPlan     RoomKind = "plan"
	KindA2A      RoomKind = "a2a"
	KindFreeform RoomKind = "freeform"
)

// ParseKind validates a room kind path segment.
func ParseKind(raw string) (RoomKind, bool) {
	switch RoomKind(strings.TrimSpace(raw)) {
	case KindDM, KindAgentDM, KindGroup, KindPlan, KindA2A:
		return RoomKind(raw), true
	default:
		return "", false
	}
}

// RegisterAgentsRoutes mounts GET /agents — redirects onto /threads.
// serve is typically markdown.ServeAgentsLand.
func RegisterAgentsRoutes(g *echo.Group, serve echo.HandlerFunc) {
	g.GET("", serve)
}

// RegisterCreateAgentRoute mounts POST /agents.
func RegisterCreateAgentRoute(g *echo.Group, create echo.HandlerFunc) {
	g.POST("", create)
}

// RegisterRoomRoutes mounts GET /rooms/freeform, GET /rooms/a2a/:a/:b,
// and GET /rooms/:kind/:id. serveFreeform must be a one-segment handler;
// ParseKind 404s unknown kinds on /:kind/:id.
func RegisterRoomRoutes(g *echo.Group, serve, serveFreeform echo.HandlerFunc) {
	if serveFreeform != nil {
		g.GET("/freeform", serveFreeform)
	}
	g.GET("/a2a/:a/:b", serve)
	g.GET("/:kind/:id", serve)
}

// RegisterBindPlanLeadRoute mounts POST /rooms/plan/:id/lead.
func RegisterBindPlanLeadRoute(g *echo.Group, bind echo.HandlerFunc) {
	g.POST("/plan/:id/lead", bind)
}

// RegisterEmptyScopeThreadRoutes mounts N=0 composer POSTs that create a thread.
func RegisterEmptyScopeThreadRoutes(
	g *echo.Group,
	createDM, createPlan, createFreeform echo.HandlerFunc,
) {
	if createDM != nil {
		g.POST("/dm/:slug/threads", createDM)
	}
	if createPlan != nil {
		g.POST("/plan/:id/threads", createPlan)
	}
	if createFreeform != nil {
		g.POST("/freeform/threads", createFreeform)
	}
}

// RedirectAgentsLand is a tiny helper when markdown is unavailable in tests.
func RedirectAgentsLand(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/threads")
}
