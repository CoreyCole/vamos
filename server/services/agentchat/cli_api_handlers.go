package agentchat

import (
	"net/http"

	"github.com/labstack/echo/v4"

	servercfg "github.com/CoreyCole/vamos/server"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

func (h *Handler) PostCLIChatRun(c echo.Context) error {
	actor, err := serverauth.AuthenticateMachineAPIRequest(
		c.Request().Context(),
		c.Request(),
		h.machineCredentials,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	var req ChatStartRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	resolution, err := servercfg.ResolveProjectCheckout(h.projectsConfig, req.ProjectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ref, err := h.service.StartCLIChatRun(
		c.Request().Context(),
		actor,
		resolution,
		req,
		h.publicBaseURL,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(ref.WorkspaceID)
	}
	return c.JSON(http.StatusAccepted, ChatAPIResponse{Type: "started", Ref: ref})
}
