package agentchat

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	g.POST("/hermes/threads", h.HandleCreateHermesThread)
	g.POST("/hermes/threads/:thread_id/prompts", h.HandleHermesPrompt)
}

func (h *Handler) RegisterHermesMachineRoutes(g *echo.Group) {
	g.POST("/hermes/threads/:thread_id/events", h.HandleHermesEvent)
	g.POST("/hermes/pi/:session_id/complete", h.HandleHermesPiCompletion)
}

func (h *Handler) HandleCreateHermesThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	identity := HermesPlanIdentity(strings.TrimSpace(c.FormValue("plan_dir")))
	thread, err := h.service.CreateHermesThread(c.Request().Context(), CreateHermesThreadInput{
		PlanDir: identity, CreatorEmail: userEmail, Title: c.FormValue("title"),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, hermesFormReturnURL(c.FormValue("return_to"), thread.ID))
}

func (h *Handler) HandleHermesPrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	if err := c.Request().ParseForm(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid prompt form")
	}
	prompt := c.Request().PostFormValue("prompt")
	if strings.TrimSpace(prompt) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	contextPaths := make([]string, 0)
	for _, value := range c.Request().PostForm["context_paths"] {
		contextPaths = append(contextPaths, strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '\n'
		})...)
	}
	result, err := h.service.SubmitOwnedHermesPrompt(
		c.Request().Context(),
		userEmail,
		HermesPrompt{
			CommandID:             strings.TrimSpace(c.Request().PostFormValue("command_id")),
			ThreadID:              strings.TrimSpace(c.Param("thread_id")),
			PlanDir:               strings.TrimSpace(c.Request().PostFormValue("plan_dir")),
			ConversationReference: strings.TrimSpace(c.Request().PostFormValue("conversation_reference")),
			ContextPaths:          contextPaths,
			Prompt:                prompt,
		},
	)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrHermesPromptUnauthorized):
			status = http.StatusForbidden
		case errors.Is(err, ErrHermesPromptConflict):
			status = http.StatusConflict
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
		}
		return echo.NewHTTPError(status, err.Error())
	}
	if result.InProgress {
		return echo.NewHTTPError(http.StatusLocked, ErrHermesPromptInProgress.Error())
	}
	switch result.Status {
	case HermesPromptAccepted:
		return c.Redirect(http.StatusSeeOther, hermesFormReturnURL(c.Request().PostFormValue("return_to"), c.Param("thread_id")))
	case HermesPromptRejected:
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "Hermes rejected the prompt command")
	case HermesPromptFailed:
		if result.Reason == HermesPromptGatewayUnavailable {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "Hermes prompt delivery is unavailable")
		}
		return echo.NewHTTPError(http.StatusFailedDependency, "Hermes prompt delivery failed")
	case HermesPromptUncertain:
		return echo.NewHTTPError(http.StatusGatewayTimeout, "Hermes prompt delivery is uncertain and will not be retried")
	default:
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Hermes prompt delivery is unavailable")
	}
}

func hermesFormReturnURL(raw, selected string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.IsAbs() || !strings.HasPrefix(parsed.Path, "/thoughts/") {
		parsed = &url.URL{Path: "/thoughts/"}
	}
	query := parsed.Query()
	query.Set("context", "threads")
	if strings.TrimSpace(selected) != "" {
		query.Set("hermes_thread", selected)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
	if !validHermesEventType(event.Type) || event.ID == "" ||
		(event.Type == "pi_run" && strings.TrimSpace(event.PiSessionID) == "") {
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
	threadID, err := h.service.HermesThreadForPiRun(req.PlanDir, session)
	if err != nil {
		if errors.Is(err, ErrHermesPiRunNotFound) ||
			errors.Is(err, ErrHermesPiRunAmbiguous) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
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
		threadID,
		session,
		result,
	); err != nil {
		if errors.Is(err, ErrHermesManagerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}
