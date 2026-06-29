package app

import (
	"net/http"
	"strings"

	"example.com/vamos-datastar-starter/internal/ui"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func (s *Service) Routes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `${time_rfc3339} method=${method} uri=${uri} status=${status} error="${error}" latency=${latency_human}` + "\n",
	}))
	e.Static("/static", "static")
	e.GET("/favicon.ico", s.handleFavicon)
	e.GET("/healthz", s.handleHealth)
	e.GET("/events", s.handleEvents)
	e.POST("/items", s.handleCreateItem)
	e.POST("/items/toggle", s.handleToggleItem)
	e.POST("/items/delete", s.handleDeleteItem)
	e.GET("/", s.handleHome)
	return e
}

func (s *Service) handleFavicon(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) handleHealth(c echo.Context) error {
	return c.String(http.StatusOK, "ok\n")
}

func (s *Service) handleHome(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	if err := ui.Page().Render(c.Request().Context(), c.Response().Writer); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "render page")
	}
	return nil
}

func (s *Service) handleCreateItem(c echo.Context) error {
	title := strings.TrimSpace(c.FormValue("title"))
	if title != "" {
		_, _ = s.queries.CreateItem(c.Request().Context(), title)
		s.notifier.notify()
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) handleToggleItem(c echo.Context) error {
	id, err := parseID(c.FormValue("id"))
	if err == nil {
		_, _ = s.queries.ToggleItem(c.Request().Context(), id)
		s.notifier.notify()
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) handleDeleteItem(c echo.Context) error {
	id, err := parseID(c.FormValue("id"))
	if err == nil {
		_ = s.queries.DeleteItem(c.Request().Context(), id)
		s.notifier.notify()
	}
	return c.NoContent(http.StatusNoContent)
}
