package agentchat

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	servercfg "github.com/CoreyCole/vamos/server"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

func (h *Handler) PostCLIChatRun(c echo.Context) error {
	actor, err := h.authenticateCLIActor(c)
	if err != nil {
		return err
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

func (h *Handler) PostCLIChatSteer(c echo.Context) error {
	actor, err := h.authenticateCLIActor(c)
	if err != nil {
		return err
	}
	var req ChatSteerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	ref, disposition, err := h.service.SteerCLIChatThread(
		c.Request().Context(),
		actor,
		req,
		h.publicBaseURL,
	)
	if err != nil {
		response := ChatAPIResponse{
			Type:             "steer_rejected",
			Ref:              ref,
			Error:            err.Error(),
			Reason:           disposition.Reason,
			LatestThreadID:   disposition.LatestThreadID,
			LatestWebURL:     disposition.LatestWebURL,
			InfluencesLatest: disposition.InfluencesLatest,
		}
		switch {
		case errors.Is(err, ErrThreadRunInProgress):
			if response.Reason == "" {
				response.Reason = "run_in_progress"
			}
			return c.JSON(http.StatusConflict, response)
		case errors.Is(err, sql.ErrNoRows):
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		default:
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(ref.WorkspaceID)
	}
	return c.JSON(http.StatusAccepted, ChatAPIResponse{
		Type:             "steer_accepted",
		Ref:              ref,
		LatestThreadID:   disposition.LatestThreadID,
		LatestWebURL:     disposition.LatestWebURL,
		InfluencesLatest: disposition.InfluencesLatest,
	})
}

func (h *Handler) GetCLIChatSession(c echo.Context) error {
	actor, err := h.authenticateCLIActor(c)
	if err != nil {
		return err
	}
	snapshot, err := h.getAuthorizedChatSessionSnapshot(
		c.Request().Context(),
		c.Param("session_id"),
		actor.ActorEmail,
	)
	if err != nil {
		return chatSessionSnapshotHTTPError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) StreamCLIChatSessionEvents(c echo.Context) error {
	actor, err := h.authenticateCLIActor(c)
	if err != nil {
		return err
	}
	return h.streamAuthorizedChatSessionEvents(c, c.Param("session_id"), actor.ActorEmail)
}

func (h *Handler) authenticateCLIActor(c echo.Context) (serverauth.MachineAPIActor, error) {
	actor, err := serverauth.AuthenticateMachineAPIRequest(
		c.Request().Context(),
		c.Request(),
		h.machineCredentials,
	)
	if err != nil {
		return serverauth.MachineAPIActor{}, echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}
	return actor, nil
}
