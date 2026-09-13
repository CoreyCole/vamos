package agentchat

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	datastar "github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/datastarui/components/infinitescroll"
	"github.com/CoreyCole/vamos/pkg/db"
)

// StreamThreadHistory serves Pattern A above-edge pages for AgentChat.
// Chunk patches use View Transitions OFF; Host (#agent-chat-scroll-region) is never remorphed.
func (h *Handler) StreamThreadHistory(c echo.Context) error {
	if h == nil || h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agent chat unavailable")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread id required")
	}
	before := strings.TrimSpace(c.QueryParam("before"))
	if before == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "before cursor required")
	}
	limit := stableTranscriptInitialLimit
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid limit")
		}
		limit = n
	}

	userEmail, _ := c.Get("user_email").(string)
	ctx := c.Request().Context()
	thread, err := h.service.resolveThreadForHistory(ctx, userEmail, threadID)
	if err != nil {
		return err
	}

	stable, err := h.service.buildStableTranscript(ctx, thread)
	if err != nil {
		return err
	}
	older, nextBefore, hasMore := olderStablePage(stable, before, limit)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	if err := sse.PatchElementTempl(
		infinitescroll.Loading(infinitescroll.LoadingArgs{
			ID:        agentChatScrollSentinelAbove,
			Direction: infinitescroll.DirectionAbove,
		}),
		datastar.WithoutViewTransitions(),
	); err != nil {
		return err
	}

	if len(older) == 0 {
		return sse.RemoveElement("#"+agentChatScrollSentinelAbove, datastar.WithoutViewTransitions())
	}

	var buf bytes.Buffer
	for _, msg := range older {
		if err := TranscriptMessageWithFork(thread.ID, msg, "").Render(ctx, &buf); err != nil {
			return err
		}
	}
	if err := sse.PatchElements(
		buf.String(),
		datastar.WithSelectorID(agentChatScrollItemsID),
		datastar.WithModePrepend(),
		datastar.WithoutViewTransitions(),
	); err != nil {
		return err
	}

	if !hasMore || strings.TrimSpace(nextBefore) == "" {
		return sse.RemoveElement("#"+agentChatScrollSentinelAbove, datastar.WithoutViewTransitions())
	}
	return sse.PatchElementTempl(
		infinitescroll.Sentinel(infinitescroll.SentinelArgs{
			ID:        agentChatScrollSentinelAbove,
			Direction: infinitescroll.DirectionAbove,
			PatchExpr: agentChatHistoryPatchAboveExpr(thread.ID, nextBefore),
		}),
		datastar.WithoutViewTransitions(),
	)
}

func (s *Service) resolveThreadForHistory(
	ctx context.Context,
	userEmail, threadID string,
) (db.AgentThread, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return db.AgentThread{}, echo.NewHTTPError(http.StatusBadRequest, "thread id required")
	}
	// Workbench / shared A2A threads.
	thread, err := s.queries.GetSharedAgentThread(ctx, threadID)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.AgentThread{}, err
	}
	userEmail = strings.TrimSpace(userEmail)
	if userEmail == "" {
		return db.AgentThread{}, echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	thread, err = s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        threadID,
		UserEmail: userEmail,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return db.AgentThread{}, echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	return thread, err
}

func olderStablePage(
	all []TranscriptMessage,
	beforeDOMID string,
	limit int,
) (page []TranscriptMessage, nextBefore string, hasMore bool) {
	beforeDOMID = strings.TrimSpace(beforeDOMID)
	if beforeDOMID == "" || limit < 1 {
		return nil, "", false
	}
	idx := -1
	for i, msg := range all {
		if strings.TrimSpace(msg.DOMID) == beforeDOMID {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return nil, "", false
	}
	start := idx - limit
	if start < 0 {
		start = 0
	}
	page = all[start:idx]
	if start > 0 && len(page) > 0 {
		nextBefore = strings.TrimSpace(page[0].DOMID)
		hasMore = nextBefore != ""
	}
	return page, nextBefore, hasMore
}
