package agentchat

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

type hermesEventRequest struct {
	PlanDir     string          `json:"plan_dir"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Content     string          `json:"content"`
	Tool        *HermesToolCard `json:"tool"`
	PiSessionID string          `json:"pi_session_id"`
}

func (h *Handler) RegisterHermesRoutes(g *echo.Group) {
	g.POST("/hermes/threads/:thread_id/prompts", h.HandleHermesPrompt)
	g.POST("/hermes/threads/:thread_id/events", h.HandleHermesEvent)
	g.POST("/hermes/pi/:session_id/complete", h.HandleHermesPiCompletion)
}

func (h *Handler) HandleHermesPrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	contextPaths := strings.FieldsFunc(c.FormValue("context_paths"), func(r rune) bool {
		return r == ',' || r == '\n'
	})
	err := h.service.DeliverOwnedHermesPrompt(
		c.Request().Context(),
		userEmail,
		HermesPrompt{
			ThreadID: c.Param(
				"thread_id",
			),
			PlanDir:      strings.TrimSpace(c.FormValue("plan_dir")),
			ContextPaths: contextPaths,
			Prompt:       prompt,
		},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

func (h *Handler) authorizeHermes(c echo.Context) error {
	if h.hermesCallbackToken == "" {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"Hermes callbacks are not configured",
		)
	}
	raw := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(raw), []byte(h.hermesCallbackToken)) != 1 {
		return echo.NewHTTPError(
			http.StatusUnauthorized,
			"invalid Hermes callback credential",
		)
	}
	return nil
}

func (h *Handler) HandleHermesEvent(c echo.Context) error {
	if err := h.authorizeHermes(c); err != nil {
		return err
	}
	var req hermesEventRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid Hermes event")
	}
	event := HermesTranscriptEvent{
		ID:          req.ID,
		Type:        req.Type,
		ThreadID:    c.Param("thread_id"),
		Content:     req.Content,
		Tool:        req.Tool,
		PiSessionID: req.PiSessionID,
	}
	if !validHermesEventType(event.Type) || event.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported Hermes event")
	}
	if err := h.service.AppendHermesTranscript(
		c.Request().Context(),
		HermesCallbackEvent{PlanDir: req.PlanDir, HermesTranscriptEvent: event},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

func (h *Handler) HandleHermesPiCompletion(c echo.Context) error {
	if err := h.authorizeHermes(c); err != nil {
		return err
	}
	var req struct {
		PlanDir string `json:"plan_dir"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid Pi completion")
	}
	session := c.Param("session_id")
	result, err := h.service.HermesPiResult(req.PlanDir, session)
	if err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, "Pi result not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "invalid Pi completion")
	}
	if h.service.hermesGateway == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"Hermes gateway is not configured",
		)
	}
	if err := h.service.hermesGateway.DeliverPiCompletion(
		c.Request().Context(),
		session,
		result,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}
