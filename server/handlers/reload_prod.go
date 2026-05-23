//go:build !dev

package handlers

import "github.com/labstack/echo/v4"

// SetupReloadRoutes is a no-op in production
func SetupReloadRoutes(e *echo.Echo) {}
