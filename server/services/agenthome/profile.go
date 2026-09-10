package agenthome

import (
	"github.com/labstack/echo/v4"
)

// ProfileFile is one on-disk file under thoughts/agents/{slug}/.
type ProfileFile struct {
	RelPath string
}

// ProfilePaneArgs renders the bot-home profile artifact pane.
type ProfilePaneArgs struct {
	Kind     RoomKind
	Slug     string
	Files    []ProfileFile
	Selected string
	Body     string
}

// ProfilePOSTPath is the write URL for a bot-home profile file.
func ProfilePOSTPath(kind RoomKind, slug string) string {
	return "/rooms/" + string(kind) + "/" + slug + "/profile"
}

// ProfileGETPath lists or opens a profile file from disk.
func ProfileGETPath(kind RoomKind, slug, file string) string {
	path := "/rooms/" + string(kind) + "/" + slug + "?view=profile"
	if file != "" {
		path += "&file=" + file
	}
	return path
}

// RegisterAgentProfileRoute mounts POST /rooms/:kind/:id/profile.
func RegisterAgentProfileRoute(g *echo.Group, update echo.HandlerFunc) {
	g.POST("/:kind/:id/profile", update)
}

// ProfileView reports whether the request asked for the profile pane.
func ProfileView(c echo.Context) bool {
	return c.QueryParam("view") == "profile"
}
