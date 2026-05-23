package webhook

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Handler handles HTTP requests for webhooks
type Handler struct {
	service *Service
}

// NewHandler creates a new webhook handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers webhook routes on the Echo instance
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.POST("/api/webhook/github", h.HandleGitHubWebhook)
}

// HandleGitHubWebhook handles incoming GitHub webhook requests
func (h *Handler) HandleGitHubWebhook(c echo.Context) error {
	// Read the raw body for signature verification
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}

	// Verify the signature
	signature := c.Request().Header.Get("X-Hub-Signature-256")
	if !VerifySignature(body, signature, h.service.secret) {
		h.service.logEvent("webhook_signature_invalid", map[string]any{
			"signature": signature,
		})
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	// Check the event type
	eventType := c.Request().Header.Get("X-GitHub-Event")
	if eventType != "push" {
		h.service.logEvent("webhook_event_ignored", map[string]any{
			"event_type": eventType,
		})
		// Return 200 OK for non-push events (GitHub expects this)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored",
			"reason": "only push events are processed",
		})
	}

	// Process the push event
	if err := h.service.HandlePush(c.Request().Context(), body); err != nil {
		// Log the error but return 200 OK to GitHub
		// (GitHub will retry on non-2xx which we don't want)
		h.service.logEvent("webhook_processing_error", map[string]any{
			"error": err.Error(),
		})
		return c.JSON(http.StatusOK, map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "success",
	})
}
