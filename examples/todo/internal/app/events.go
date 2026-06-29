package app

import (
	"example.com/vamos-datastar-starter/internal/ui"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

func (s *Service) handleEvents(c echo.Context) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())

	ch := s.notifier.subscribe()
	defer s.notifier.unsubscribe(ch)
	if err := s.patchPanel(c, sse, ""); err != nil {
		return err
	}

	for {
		select {
		case <-ch:
			if err := s.patchPanel(c, sse, "Updated from SQLite."); err != nil {
				return err
			}
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

func (s *Service) patchPanel(c echo.Context, sse *datastar.ServerSentEventGenerator, message string) error {
	data, err := s.pageData(c.Request().Context(), message)
	if err != nil {
		data = ui.PageData{Message: "The app could not load state safely."}
	}
	return sse.PatchElementTempl(ui.AppPanel(data))
}
