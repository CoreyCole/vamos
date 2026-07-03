package pickleball

import (
	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/labstack/echo/v4"
)

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	group := e.Group("/examples/pickleball")
	if auth != nil {
		group.Use(auth)
	}
	group.GET("", s.HandlePage)
	group.GET("/state", s.HandleStateStream)
	group.POST("/prompts", s.HandleSubmitPrompt)
	group.POST("/share", s.HandleShare)
	group.POST("/debug/restore", s.HandleDebugRestore)
	if s.opts.AppletRuntime != nil {
		group.Any("/app/*", echo.WrapHandler(appletruntime.NewAppletProxy(
			s.opts.AppletRuntime,
			appletruntime.AppletProxyMatch{AppID: "pickleball", StripPrefix: "/examples/pickleball/app"},
			appletruntime.ProxyOptions{FlushSSE: true, RewriteCookiePath: true, AllowNullOriginCORS: true},
		)))
	}
}
