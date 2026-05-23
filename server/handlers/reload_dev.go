//go:build dev

package handlers

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

// serverStartTime is set when the server starts, used to detect restarts
var serverStartTime = time.Now()

// SetupReloadRoutes registers reload endpoints for dev hot reload.
func SetupReloadRoutes(e *echo.Echo) {
	e.GET("/reload", handleReload)
	e.GET("/api/reload", handleReload)
}

func handleReload(c echo.Context) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	// Any client connecting within 2s of server start gets reload
	// Uses server start timestamp as ID to:
	// - Support multiple tabs (all reconnecting clients get reload)
	// - Prevent infinite loops (same timestamp = already reloaded)
	// - Handle rapid restarts (new timestamp = new reload)
	if time.Since(serverStartTime) < 2*time.Second {
		startMs := serverStartTime.UnixMilli()
		sse.ExecuteScript(fmt.Sprintf(`
			if (sessionStorage.getItem('_devReload') !== '%d') {
				sessionStorage.setItem('_devReload', '%d');
				window.location.reload();
			}
		`, startMs, startMs))
	}

	// Block until client disconnects
	<-c.Request().Context().Done()
	return nil
}
