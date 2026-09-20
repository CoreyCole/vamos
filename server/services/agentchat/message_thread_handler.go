package agentchat

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	datastar "github.com/starfederation/datastar-go/datastar"
)

func (h *Handler) GetMessageThread(c echo.Context) error {
	threadID := strings.TrimSpace(c.Param("thread_id"))
	parentID := strings.TrimSpace(c.QueryParam("parent"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	ctx := c.Request().Context()
	thread, err := h.service.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	stable, err := h.service.buildStableTranscript(ctx, thread)
	if err != nil {
		return err
	}
	composerOff := thread.RoomKind == RoomKindPairwise
	parent := findTranscriptMessage(stable, parentID)
	view, err := h.service.loadMessageThreadView(
		ctx,
		threadID,
		parentID,
		parent,
		composerOff,
	)
	if err != nil {
		return err
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(MessageThreadHost(view))
}

func (h *Handler) PostMessageThreadReply(c echo.Context) error {
	threadID := strings.TrimSpace(c.Param("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	ctx := c.Request().Context()
	thread, err := h.service.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	if thread.RoomKind == RoomKindPairwise {
		return echo.NewHTTPError(http.StatusForbidden, "pairwise threads are view-only")
	}
	parentID := strings.TrimSpace(c.FormValue("parent_entry_id"))
	body := strings.TrimSpace(c.FormValue("body"))
	userEmail, _ := c.Get("user_email").(string)
	if _, err := h.service.insertMessageThreadReply(
		ctx,
		threadID,
		parentID,
		userEmail,
		body,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	stable, err := h.service.buildStableTranscript(ctx, thread)
	if err != nil {
		return err
	}
	parent := findTranscriptMessage(stable, parentID)
	view, err := h.service.loadMessageThreadView(ctx, threadID, parentID, parent, false)
	if err != nil {
		return err
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if parent.ThreadSummary != nil {
		if err := sse.PatchElementTempl(ThreadSummaryRow(threadID, parent)); err != nil {
			return err
		}
	}
	return sse.PatchElementTempl(MessageThreadHost(view))
}
