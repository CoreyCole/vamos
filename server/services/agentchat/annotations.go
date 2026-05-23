package agentchat

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

func (h *Handler) CreateChatAnnotation(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()
	session, err := h.service.queries.GetChatSession(ctx, c.Param("session_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		session.WorkspaceID,
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	eventSeq, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("event_seq")), 10, 64)
	if err != nil || eventSeq < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "event_seq is required")
	}
	annotation, err := h.service.chatSessions.CreateAnnotation(
		ctx,
		chatsession.CreateAnnotationInput{
			WorkspaceID:  session.WorkspaceID,
			SessionID:    session.ID,
			NodeID:       c.FormValue("node_id"),
			EventSeq:     eventSeq,
			AuthorEmail:  userEmail,
			BodyMarkdown: c.FormValue("body_markdown"),
		},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(session.WorkspaceID)
	}
	return c.JSON(http.StatusCreated, annotation)
}

func (h *Handler) ResolveChatAnnotation(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()
	session, err := h.service.queries.GetChatSession(ctx, c.Param("session_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		session.WorkspaceID,
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if err := h.service.chatSessions.ResolveAnnotation(
		ctx,
		c.Param("annotation_id"),
		userEmail,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(session.WorkspaceID)
	}
	return c.NoContent(http.StatusNoContent)
}
