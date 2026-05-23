package storybook

import (
	"github.com/labstack/echo/v4"
	"github.com/CoreyCole/vamos/server/services/theme"
)

type Handler struct {
	themeService *theme.Service
}

func NewHandler(themeService *theme.Service) *Handler {
	return &Handler{themeService: themeService}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.HandlePage)
}

func (h *Handler) HandlePage(c echo.Context) error {
	userEmail, _ := c.Get("user_email").(string)
	currentTheme := "dark"
	if h.themeService != nil {
		currentTheme = h.themeService.GetCurrentThemeMode(c)
	}

	return Page(PageArgs{
		UserEmail:    userEmail,
		CurrentTheme: currentTheme,
	}).Render(c.Request().Context(), c.Response().Writer)
}
