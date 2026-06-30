package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"example.com/vamos-wordle/internal/ui"
)

const (
	usernameCookie = "wordle_username"
	timezoneCookie = "wordle_timezone"
)

func (s *Service) Routes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Static("/static", "static")
	e.GET("/favicon.ico", s.handleFavicon)
	e.GET("/healthz", s.handleHealth)
	e.GET("/events", s.handleEvents)
	e.POST("/login", s.handleLogin)
	e.POST("/logout", s.handleLogout)
	e.POST("/guesses", s.handleGuess)
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

func (s *Service) handleLogin(c echo.Context) error {
	username, err := NormalizeUsername(c.FormValue("username"))
	if err != nil {
		data := ui.PageData{Auth: ui.AuthView{LoggedIn: false}, Message: err.Error()}
		return datastar.NewSSE(c.Response().Writer, c.Request()).
			PatchElementTempl(ui.AppPanel(data))
	}
	tz := strings.TrimSpace(c.FormValue("timezone"))
	if tz == "" {
		tz = s.location.String()
	}
	setCookie(c, usernameCookie, username)
	setCookie(c, timezoneCookie, tz)
	_, _ = s.pageData(c.Request().Context(), username, tz, "", renderEvent{})
	s.notifier.notify(notifierEvent{})
	return datastar.NewSSE(c.Response().Writer, c.Request()).
		ExecuteScript("window.location.reload()")
}

func (s *Service) handleLogout(c echo.Context) error {
	clearCookie(c, usernameCookie)
	clearCookie(c, timezoneCookie)
	s.notifier.notify(notifierEvent{})
	return datastar.NewSSE(c.Response().Writer, c.Request()).
		ExecuteScript("window.location.reload()")
}

func (s *Service) handleGuess(c echo.Context) error {
	username, ok := readCookie(c, usernameCookie)
	if !ok {
		return c.NoContent(http.StatusNoContent)
	}
	tz, _ := readCookie(c, timezoneCookie)
	rawGuess := c.FormValue("guess")
	result, err := s.recordGuess(
		c.Request().Context(),
		username,
		tz,
		rawGuess,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "record guess")
	}
	event := newRenderEvent(username, result, rawGuess)
	s.notifier.notify(notifierEvent{Username: username, Event: event})
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if result.Outcome == GuessAccepted {
		return sse.MarshalAndPatchSignals(map[string]any{"guess": ""})
	}
	return sse.ExecuteScript("document.getElementById('wordle-panel')?.focus()")
}

func readCookie(c echo.Context, name string) (string, bool) {
	cookie, err := c.Cookie(name)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	return strings.TrimSpace(cookie.Value), true
}

func setCookie(c echo.Context, name, value string) {
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  time.Now().Add(365 * 24 * time.Hour),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(c echo.Context, name string) {
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}
