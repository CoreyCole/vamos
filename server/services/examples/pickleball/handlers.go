package pickleball

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"
)

func (s *Service) HandlePage(c echo.Context) error {
	session, err := s.EnsureSession(c.Request().Context(), userEmail(c))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	vm, err := s.GetState(c.Request().Context(), session.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return Page(vm).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) HandleStateStream(c echo.Context) error {
	sessionID := strings.TrimSpace(c.QueryParam("session"))
	if sessionID == "" {
		session, err := s.EnsureSession(c.Request().Context(), userEmail(c))
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		sessionID = session.ID
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	patch := func() error {
		vm, err := s.GetState(c.Request().Context(), sessionID)
		if err != nil {
			return err
		}
		return sse.PatchElementTempl(StatePanel(vm))
	}
	if err := patch(); err != nil {
		return err
	}
	ch := s.subscribe(sessionID)
	defer s.unsubscribe(sessionID, ch)
	for {
		select {
		case <-ch:
			if err := patch(); err != nil {
				c.Logger().Errorf("pickleball state patch: %v", err)
			}
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

func (s *Service) HandleSubmitPrompt(c echo.Context) error {
	req := PromptRequest{
		SessionID: strings.TrimSpace(c.FormValue("session_id")),
		Prompt:    c.FormValue("prompt"),
		UserEmail: userEmail(c),
	}
	if _, err := s.SubmitPrompt(c.Request().Context(), req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

func (s *Service) HandleShare(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) HandleDebugRestore(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "debug restore is not implemented yet")
}

func userEmail(c echo.Context) string {
	if email, ok := c.Get("user_email").(string); ok {
		return email
	}
	return ""
}
