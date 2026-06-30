package app

import (
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"example.com/vamos-wordle/internal/ui"
)

func (s *Service) handleEvents(c echo.Context) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ch := s.notifier.subscribe()
	defer s.notifier.unsubscribe(ch)
	if err := s.patchPanel(c, sse, "", renderEvent{}); err != nil {
		return err
	}
	for {
		select {
		case event := <-ch:
			if err := s.patchPanel(c, sse, "", event.Event); err != nil {
				return err
			}
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

func (s *Service) patchPanel(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	message string,
	event renderEvent,
) error {
	username, _ := readCookie(c, usernameCookie)
	tz, _ := readCookie(c, timezoneCookie)
	if event.Username != "" && event.Username != username {
		event = renderEvent{}
	}
	data, err := s.pageData(c.Request().Context(), username, tz, message, event)
	if err != nil {
		data = ui.PageData{Message: "The app could not load state safely."}
	}
	return sse.PatchElementTempl(ui.AppPanel(data))
}
