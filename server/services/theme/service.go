package theme

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
	"github.com/CoreyCole/vamos/pkg/db"
)

// Default themes
const (
	DefaultDarkTheme       = "github-dark"
	DefaultLightTheme      = "solarized-light"
	DefaultThemeMode       = "dark"
	DefaultDarkSyntaxTheme = "github-dark"
)

// Service handles user theme preferences stored in the database
type Service struct {
	queries *db.Queries
}

// NewService creates a new theme service with database access
func NewService(queries *db.Queries) *Service {
	return &Service{
		queries: queries,
	}
}

// UserPreferences holds theme preferences for a user
type UserPreferences struct {
	Theme       string // "dark" or "light"
	SyntaxTheme string // e.g., "github-dark", "dracula", etc.
}

// GetCurrentTheme returns the current syntax theme for the user (implements ThemeProvider interface)
func (s *Service) GetCurrentTheme(c echo.Context) string {
	prefs := s.GetUserPreferences(c)
	return prefs.SyntaxTheme
}

// GetCurrentThemeMode returns the current theme mode (dark/light) for the user
func (s *Service) GetCurrentThemeMode(c echo.Context) string {
	prefs := s.GetUserPreferences(c)
	return prefs.Theme
}

// GetUserPreferences returns the theme preferences for the current user
func (s *Service) GetUserPreferences(c echo.Context) UserPreferences {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		// Return defaults for unauthenticated users
		return UserPreferences{
			Theme:       DefaultThemeMode,
			SyntaxTheme: DefaultDarkSyntaxTheme,
		}
	}

	prefs, err := s.queries.GetUserPreferences(c.Request().Context(), userEmail)
	if err != nil {
		// Return defaults if no preferences found
		return UserPreferences{
			Theme:       DefaultThemeMode,
			SyntaxTheme: DefaultDarkSyntaxTheme,
		}
	}

	return UserPreferences{
		Theme:       prefs.Theme,
		SyntaxTheme: prefs.SyntaxTheme,
	}
}

// HandleThemeChange handles dark/light theme change requests via SSE
func (s *Service) HandleThemeChange(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.String(http.StatusUnauthorized, "Not authenticated")
	}

	// Parse JSON body from Datastar
	var data map[string]any
	if err := c.Bind(&data); err != nil {
		return c.String(http.StatusBadRequest, "Invalid JSON")
	}

	// Extract theme from signals
	theme, _ := data["theme"].(string)
	if theme == "" {
		return c.String(http.StatusBadRequest, "Missing theme")
	}

	// Validate theme
	if theme != "dark" && theme != "light" {
		return c.String(http.StatusBadRequest, "Invalid theme: must be 'dark' or 'light'")
	}

	// Update theme in database
	_, err := s.queries.UpdateTheme(c.Request().Context(), db.UpdateThemeParams{
		UserEmail: userEmail,
		Theme:     theme,
	})
	if err != nil {
		c.Logger().Errorf("Failed to update theme: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to save theme")
	}

	// Return SSE response - no HTML patch needed, frontend handles dark class toggle
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	// Just acknowledge the change - the frontend already toggled the dark class
	fmt.Fprintf(c.Response(), "event: datastar-patch-signals\n")
	fmt.Fprintf(c.Response(), "data: signals {}\n\n")

	c.Response().Flush()
	return nil
}

// HandleSyntaxThemeChange handles syntax theme change requests via SSE
func (s *Service) HandleSyntaxThemeChange(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.String(http.StatusUnauthorized, "Not authenticated")
	}

	// Parse JSON body from Datastar
	var data map[string]any
	if err := c.Bind(&data); err != nil {
		return c.String(http.StatusBadRequest, "Invalid JSON")
	}

	// Extract syntax_theme_select.value from the nested signals structure
	theme := ""
	if selectData, ok := data["syntax_theme_select"].(map[string]any); ok {
		if value, ok := selectData["value"].(string); ok {
			theme = value
		}
	}

	if theme == "" {
		return c.String(http.StatusBadRequest, "Missing syntax_theme_select.value")
	}

	// Validate theme
	if !isValidSyntaxTheme(theme) {
		return c.String(http.StatusBadRequest, "Invalid syntax theme")
	}

	// Update syntax theme in database
	_, err := s.queries.UpdateSyntaxTheme(c.Request().Context(), db.UpdateSyntaxThemeParams{
		UserEmail:   userEmail,
		SyntaxTheme: theme,
	})
	if err != nil {
		c.Logger().Errorf("Failed to update syntax theme: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to save syntax theme")
	}

	// Return SSE response with HTML patch for the stylesheet link
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	html := fmt.Sprintf(
		`<link id="syntax-theme" rel="stylesheet" href="/css/syntax-%s.css">`,
		theme,
	)
	fmt.Fprintf(c.Response(), "event: datastar-patch-elements\n")
	fmt.Fprintf(c.Response(), "data: elements %s\n\n", html)

	c.Response().Flush()
	return nil
}

// GetPreferencesForEmail returns preferences for a specific email (used during page render)
func (s *Service) GetPreferencesForEmail(ctx context.Context, userEmail string) UserPreferences {
	if userEmail == "" {
		return UserPreferences{
			Theme:       DefaultThemeMode,
			SyntaxTheme: DefaultDarkSyntaxTheme,
		}
	}

	prefs, err := s.queries.GetUserPreferences(ctx, userEmail)
	if err != nil {
		if err == sql.ErrNoRows {
			// No preferences yet, return defaults
			return UserPreferences{
				Theme:       DefaultThemeMode,
				SyntaxTheme: DefaultDarkSyntaxTheme,
			}
		}
		// Log error but return defaults
		return UserPreferences{
			Theme:       DefaultThemeMode,
			SyntaxTheme: DefaultDarkSyntaxTheme,
		}
	}

	return UserPreferences{
		Theme:       prefs.Theme,
		SyntaxTheme: prefs.SyntaxTheme,
	}
}

// isValidSyntaxTheme validates syntax theme name
func isValidSyntaxTheme(theme string) bool {
	validThemes := []string{
		// Dark themes
		"monokai", "dracula", "nord", "github-dark",
		"tokyonight-night", "catppuccin-mocha",
		// Light themes
		"github", "solarized-light", "catppuccin-latte",
		"tokyonight-day",
	}

	return slices.Contains(validThemes, theme)
}

// ThemeOption represents a theme choice
type ThemeOption struct {
	Value string
	Label string
}

// DarkThemes available dark syntax themes
var DarkThemes = []ThemeOption{
	{Value: "github-dark", Label: "GitHub Dark"},
	{Value: "dracula", Label: "Dracula"},
	{Value: "nord", Label: "Nord"},
	{Value: "tokyonight-night", Label: "Tokyo Night"},
	{Value: "catppuccin-mocha", Label: "Catppuccin Mocha"},
}

// LightThemes available light syntax themes
var LightThemes = []ThemeOption{
	{Value: "solarized-light", Label: "Solarized Light"},
	{Value: "catppuccin-latte", Label: "Catppuccin Latte"},
	{Value: "tokyonight-day", Label: "Tokyo Night Day"},
}

// GetAllThemes returns all available themes
func GetAllThemes() []ThemeOption {
	return append(DarkThemes, LightThemes...)
}
