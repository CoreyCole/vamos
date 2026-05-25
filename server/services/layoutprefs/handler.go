package layoutprefs

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(g *echo.Group, service *Service) {
	h := NewHandler(service)
	g.POST("/layout-preferences", h.Save)
	g.POST("/layout-preferences/reset", h.Reset)
}

func (h *Handler) Save(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.String(http.StatusUnauthorized, "Not authenticated")
	}
	var payload struct {
		ViewportClass workbench.ViewportClass   `json:"viewportClass" form:"viewportClass"`
		Config        workbench.WorkbenchConfig `json:"config"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.String(http.StatusBadRequest, "Invalid JSON")
	}
	viewportClass, err := resolveWriteViewportClass(c, payload.ViewportClass, payload.Config.ViewportClass)
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	saved, err := h.service.Upsert(c.Request().Context(), Input{
		UserEmail:     userEmail,
		Page:          payload.Config.Page,
		View:          payload.Config.View,
		ViewportClass: viewportClass,
		Config:        payload.Config,
	})
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.MarshalAndPatchSignals(map[string]any{
		"workbenchSaved":         true,
		"workbenchConfigVersion": saved.Version,
	})
}

func (h *Handler) Reset(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.String(http.StatusUnauthorized, "Not authenticated")
	}
	payload, err := bindResetPayload(c)
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	if payload.Page == "" || payload.View == "" {
		return c.String(http.StatusBadRequest, "page and view are required")
	}
	viewportClass, err := resolveWriteViewportClass(c, payload.ViewportClass, "")
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	if err := h.service.Reset(
		c.Request().Context(),
		userEmail,
		payload.Page,
		payload.View,
		viewportClass,
	); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to reset layout")
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := sse.MarshalAndPatchSignals(
		map[string]any{"workbenchSaved": false},
	); err != nil {
		return err
	}
	return sse.ExecuteScript("window.location.reload();")
}

type resetPayload struct {
	Page          workbench.WorkbenchPage `json:"page" form:"page"`
	View          workbench.WorkbenchView `json:"view" form:"view"`
	ViewportClass workbench.ViewportClass `json:"viewportClass" form:"viewportClass"`
}

func bindResetPayload(c echo.Context) (resetPayload, error) {
	var payload resetPayload
	contentType := c.Request().Header.Get(echo.HeaderContentType)
	if strings.HasPrefix(contentType, echo.MIMEApplicationForm) ||
		strings.HasPrefix(contentType, echo.MIMEMultipartForm) {
		payload.Page = workbench.WorkbenchPage(c.FormValue("page"))
		payload.View = workbench.WorkbenchView(c.FormValue("view"))
		payload.ViewportClass = workbench.ViewportClass(c.FormValue("viewportClass"))
		return payload, nil
	}
	if err := c.Bind(&payload); err != nil {
		return resetPayload{}, err
	}
	return payload, nil
}

func resolveWriteViewportClass(
	c echo.Context,
	payloadClass workbench.ViewportClass,
	configClass workbench.ViewportClass,
) (workbench.ViewportClass, error) {
	if payloadClass != "" {
		parsed, ok := workbench.ParseViewportClass(string(payloadClass))
		if !ok {
			return "", echo.NewHTTPError(http.StatusBadRequest, "invalid viewportClass")
		}
		return parsed, nil
	}
	if configClass != "" {
		parsed, ok := workbench.ParseViewportClass(string(configClass))
		if !ok {
			return "", echo.NewHTTPError(http.StatusBadRequest, "invalid viewportClass")
		}
		return parsed, nil
	}
	return workbench.ResolveViewportClass(c.Request().Header, c.Request().UserAgent()), nil
}
