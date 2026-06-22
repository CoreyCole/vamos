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
	session, err := s.requestSession(c, c.QueryParam("session"))
	if err != nil {
		return err
	}
	sessionID := session.ID
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	patch := func() error {
		vm, err := s.GetState(c.Request().Context(), sessionID)
		if err != nil {
			return err
		}
		if err := sse.PatchElementTempl(StatePanel(vm)); err != nil {
			return err
		}
		return sse.PatchElementTempl(ChatToModifyPanel(vm))
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
	session, err := s.requestSession(c, c.FormValue("session_id"))
	if err != nil {
		return err
	}
	req := PromptRequest{
		SessionID: session.ID,
		Prompt:    c.FormValue("prompt"),
		UserEmail: userEmail(c),
	}
	if _, err := s.SubmitPrompt(c.Request().Context(), req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusAccepted)
}

func (s *Service) HandleShare(c echo.Context) error {
	session, err := s.requestSession(c, c.FormValue("session_id"))
	if err != nil {
		return err
	}
	if _, err := s.ShareModel(c.Request().Context(), session.ID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) HandleDebugRestore(c echo.Context) error {
	session, err := s.requestSession(c, c.FormValue("session_id"))
	if err != nil {
		return err
	}
	buildID := strings.TrimSpace(c.FormValue("build_id"))
	if buildID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "build_id is required")
	}
	if err := s.RestoreSnapshotForAI(c.Request().Context(), session.ID, buildID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) requestSession(c echo.Context, requestedID string) (PickleballSession, error) {
	session, err := s.EnsureSession(c.Request().Context(), userEmail(c))
	if err != nil {
		return PickleballSession{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	requestedID = strings.TrimSpace(requestedID)
	if requestedID != "" && requestedID != session.ID {
		return PickleballSession{}, echo.NewHTTPError(http.StatusForbidden, "session_id does not match current user")
	}
	return session, nil
}

func userEmail(c echo.Context) string {
	if email, ok := c.Get("user_email").(string); ok {
		return email
	}
	return ""
}
