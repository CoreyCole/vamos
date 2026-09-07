package agentchat

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type planWorkspaceArchiveRequest struct {
	PlanDirRel string `json:"plan_dir_rel"`
}

func (h *Handler) ListPlanWorkspacesAPI(c echo.Context) error {
	if _, ok := sessionActorEmail(c); !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	result, err := h.service.ListPlanWorkspacesByStatus(
		c.Request().Context(),
		c.QueryParam("status"),
		c.QueryParam("project_id"),
	)
	if err != nil {
		if errors.Is(err, ErrPlanWorkspaceInvalidListStatus) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) ArchivePlanWorkspaceAPI(c echo.Context) error {
	userEmail, ok := sessionActorEmail(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	planDirRel, err := readPlanDirRelRequest(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	view, err := h.service.ManualArchivePlanWorkspace(
		c.Request().Context(),
		planDirRel,
		userEmail,
	)
	if err != nil {
		return planWorkspaceArchiveHTTPError(err)
	}
	// Updated plan JSON is the UX hook for Archived header chrome (no FE redesign).
	return c.JSON(http.StatusOK, view)
}

func (h *Handler) UnarchivePlanWorkspaceAPI(c echo.Context) error {
	if _, ok := sessionActorEmail(c); !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	planDirRel, err := readPlanDirRelRequest(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	view, err := h.service.UnarchiveManualPlanWorkspace(
		c.Request().Context(),
		planDirRel,
	)
	if err != nil {
		return planWorkspaceArchiveHTTPError(err)
	}
	return c.JSON(http.StatusOK, view)
}

func sessionActorEmail(c echo.Context) (string, bool) {
	userEmail, ok := c.Get("user_email").(string)
	userEmail = strings.TrimSpace(userEmail)
	return userEmail, ok && userEmail != ""
}

func readPlanDirRelRequest(c echo.Context) (string, error) {
	if rel := strings.TrimSpace(c.FormValue("plan_dir_rel")); rel != "" {
		return rel, nil
	}
	if rel := strings.TrimSpace(c.QueryParam("plan_dir_rel")); rel != "" {
		return rel, nil
	}

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return "", err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return "", errors.New("plan_dir_rel is required")
	}
	var req planWorkspaceArchiveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", errors.New("plan_dir_rel is required")
	}
	rel := strings.TrimSpace(req.PlanDirRel)
	if rel == "" {
		return "", errors.New("plan_dir_rel is required")
	}
	return rel, nil
}

func planWorkspaceArchiveHTTPError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrPlanWorkspaceNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrPlanWorkspaceMissingFromDiskArchive),
		errors.Is(err, ErrPlanWorkspaceNotManuallyArchived),
		errors.Is(err, ErrPlanWorkspaceArchiveConflict):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		msg := err.Error()
		if strings.Contains(msg, "required") {
			return echo.NewHTTPError(http.StatusBadRequest, msg)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, msg)
	}
}
