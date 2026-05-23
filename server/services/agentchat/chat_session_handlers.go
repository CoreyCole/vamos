package agentchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

type chatSessionSnapshotResponse struct {
	Projection chatsession.ChatProjection `json:"projection"`
	LastSeq    int64                      `json:"last_seq"`
}

func (h *Handler) GetChatSessionSnapshot(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	session, err := h.service.queries.GetChatSession(
		c.Request().Context(),
		c.Param("session_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		session.WorkspaceID,
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	projection, err := chatsession.NewService(h.service.db, h.service.queries).
		Snapshot(c.Request().Context(), session.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(
		http.StatusOK,
		chatSessionSnapshotResponse{Projection: projection, LastSeq: projection.LastSeq},
	)
}

func (h *Handler) StreamChatSessionEvents(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()
	sessionID := strings.TrimSpace(c.Param("session_id"))
	session, err := h.service.queries.GetChatSession(ctx, sessionID)
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
	afterSeq := chatSessionAfterSeq(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)

	svc := chatsession.NewService(h.service.db, h.service.queries)
	lastSeq, err := writeChatSessionEvents(ctx, c.Response(), svc, sessionID, afterSeq)
	if err != nil {
		return err
	}
	if h.service.notifier == nil || c.QueryParam("tail") == "false" {
		return nil
	}

	ch := h.service.notifier.Subscribe(session.WorkspaceID)
	defer h.service.notifier.Unsubscribe(session.WorkspaceID, ch)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ch:
			var err error
			lastSeq, err = writeChatSessionEvents(
				ctx,
				c.Response(),
				svc,
				sessionID,
				lastSeq,
			)
			if err != nil {
				return err
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(c.Response(), ": heartbeat\n\n"); err != nil {
				return err
			}
			c.Response().Flush()
		}
	}
}

func (h *Handler) PostChatSessionCommand(c echo.Context) error {
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
	input, err := parseChatSessionCommand(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	input.WorkspaceID = session.WorkspaceID
	input.SessionID = session.ID
	input.ActorEmail = userEmail
	if len(input.AnnotationIDs) > 0 {
		annotationContext, err := h.service.chatSessions.AnnotationContext(
			ctx,
			input.AnnotationIDs,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		input.PayloadJSON = withAnnotationContext(input.PayloadJSON, annotationContext)
	}
	outcome, err := chatsession.NewService(h.service.db, h.service.queries).
		SubmitCommand(ctx, input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(session.WorkspaceID)
	}
	return c.JSON(http.StatusAccepted, outcome)
}

func chatSessionAfterSeq(c echo.Context) int64 {
	for _, value := range []string{c.QueryParam("after"), c.Request().Header.Get("Last-Event-ID")} {
		if seq, err := strconv.ParseInt(
			strings.TrimSpace(value),
			10,
			64,
		); err == nil &&
			seq >= 0 {
			return seq
		}
	}
	return 0
}

func writeChatSessionEvents(
	ctx context.Context,
	response *echo.Response,
	svc *chatsession.Service,
	sessionID string,
	afterSeq int64,
) (int64, error) {
	events, err := svc.EventsAfter(ctx, sessionID, afterSeq, 1000)
	if err != nil {
		return afterSeq, err
	}
	lastSeq := afterSeq
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return lastSeq, err
		}
		if _, err := fmt.Fprintf(
			response,
			"id: %d\nevent: chat-session-event\ndata: %s\n\n",
			event.Seq,
			payload,
		); err != nil {
			return lastSeq, err
		}
		lastSeq = event.Seq
	}
	response.Flush()
	return lastSeq, nil
}

type chatSessionCommandRequest struct {
	Type           string          `json:"type"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
	AnnotationIDs  []string        `json:"annotation_ids"`
}

func parseChatSessionCommand(c echo.Context) (chatsession.SubmitCommandInput, error) {
	contentType := c.Request().Header.Get(echo.HeaderContentType)
	if strings.Contains(contentType, echo.MIMEApplicationJSON) {
		var req chatSessionCommandRequest
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return chatsession.SubmitCommandInput{}, err
		}
		input := commandInputFromParts(req.Type, req.IdempotencyKey, req.Payload)
		input.AnnotationIDs = req.AnnotationIDs
		return input, nil
	}
	if err := c.Request().ParseForm(); err != nil {
		return chatsession.SubmitCommandInput{}, err
	}
	payload := json.RawMessage(c.FormValue("payload"))
	if len(strings.TrimSpace(string(payload))) == 0 {
		payloadBytes, err := json.Marshal(
			map[string]string{"prompt": c.FormValue("prompt")},
		)
		if err != nil {
			return chatsession.SubmitCommandInput{}, err
		}
		payload = payloadBytes
	}
	input := commandInputFromParts(
		c.FormValue("type"),
		c.FormValue("idempotency_key"),
		payload,
	)
	input.AnnotationIDs = annotationIDsFromForm(c)
	return input, nil
}

func commandInputFromParts(
	commandType, idempotencyKey string,
	payload json.RawMessage,
) chatsession.SubmitCommandInput {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" {
		commandType = "message.send"
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	return chatsession.SubmitCommandInput{
		Type:           chatsession.CommandType(commandType),
		IdempotencyKey: idempotencyKey,
		PayloadJSON:    payload,
	}
}

func annotationIDsFromForm(c echo.Context) []string {
	form := c.Request().Form
	values := append([]string{}, form["annotation_ids[]"]...)
	values = append(values, form["annotation_ids"]...)
	ids := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ids = append(ids, value)
		}
	}
	return ids
}

func withAnnotationContext(payload json.RawMessage, contextText string) json.RawMessage {
	contextText = strings.TrimSpace(contextText)
	if contextText == "" {
		return payload
	}
	body := map[string]any{}
	if len(strings.TrimSpace(string(payload))) > 0 {
		_ = json.Unmarshal(payload, &body)
	}
	if len(body) == 0 {
		body = map[string]any{"payload": string(payload)}
	}
	body["annotation_context"] = contextText
	encoded, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	return encoded
}
