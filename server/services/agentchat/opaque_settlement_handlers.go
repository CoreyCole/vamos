package agentchat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// HandleOpaqueSettlementDelivery is machine-only. Pi cannot call it: the
// configured manager credential protects the server-side delivery boundary.
func (h *Handler) HandleOpaqueSettlementDelivery(c echo.Context) error {
	if err := h.authorizeHermes(c); err != nil {
		return err
	}
	var request OpaqueSettlementDeliveryRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&request); err != nil {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"invalid opaque settlement delivery",
		)
	}
	if err := h.service.ReceiveOpaqueSettlement(
		c.Request().Context(),
		request,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

// HandleOpaqueSettlementSuccessor records a manager-authorized human decision.
// It deliberately does not call any child, command, callback, or gateway.
func (h *Handler) HandleOpaqueSettlementSuccessor(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)
	if strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := c.Param("thread_id")
	owner, err := h.service.hermesOwner(c.Request().Context(), threadID)
	if err != nil ||
		!strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(userEmail)) {
		return echo.NewHTTPError(
			http.StatusForbidden,
			"only the thread owner may select a successor",
		)
	}
	if err := h.service.DecideOpaqueSettlementSuccessor(
		c.Request().Context(),
		c.FormValue("plan_dir"),
		threadID,
		c.Param("session"),
		c.Param("entry"),
		OpaqueSettlementSuccessor{
			Action:    c.FormValue("action"),
			Target:    c.FormValue("target"),
			Discovery: c.FormValue("discovery"),
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}
