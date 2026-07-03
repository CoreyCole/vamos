package generic

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/applets"
	"github.com/labstack/echo/v4"
)

var reservedAliasPrefixes = []string{"/api", "/forms", "/thoughts", "/agent-chat", "/static"}

type Options struct {
	ExamplesRoot  string
	AppletRuntime appletruntime.Manager
	AppletService *applets.Service
}

type Service struct {
	examplesRoot string
	resolver     applets.Resolver
	applets      *applets.Service
	runtime      appletruntime.Manager
	aliases      *appletruntime.AliasRegistry
}

func NewService(opts Options) (*Service, error) {
	root := strings.TrimSpace(opts.ExamplesRoot)
	if root == "" {
		root = "examples"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve examples root: %w", err)
	}
	resolver := applets.Resolver{ExamplesRoot: abs}
	appletService := opts.AppletService
	if appletService == nil {
		appletService = applets.NewHTTPService(applets.ServiceOptions{Resolver: resolver, Manager: opts.AppletRuntime})
	}
	return &Service{
		examplesRoot: abs,
		resolver:     resolver,
		applets:      appletService,
		runtime:      opts.AppletRuntime,
		aliases:      appletruntime.NewAliasRegistry(reservedAliasPrefixes),
	}, nil
}

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	// Sandboxed applets have origin "null", so Datastar performs CORS
	// preflights without auth cookies. Answer app-route OPTIONS outside the
	// authenticated group; authenticated GET/POST still go through the group.
	e.OPTIONS("/examples/:id/app", s.HandleAppOptions)
	e.OPTIONS("/examples/:id/app/*", s.HandleAppOptions)
	e.Any("/examples/:id/app", s.HandleApp)
	e.Any("/examples/:id/app/*", s.HandleApp)
	s.registerRootAliases(e)

	group := e.Group("/examples")
	if auth != nil {
		group.Use(auth)
	}
	group.GET("/:id", s.HandlePage)
	group.GET("/:id/status", s.HandleStatus)
}

func (s *Service) HandleAppOptions(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) HandlePage(c echo.Context) error {
	return s.applets.HandleAppletPage(c)
}

func (s *Service) HandleStatus(c echo.Context) error {
	return s.applets.HandleAppletStatus(c)
}

func (s *Service) HandleStop(c echo.Context) error {
	return s.applets.HandleAppletStop(c)
}

func (s *Service) HandleRestart(c echo.Context) error {
	return s.applets.HandleAppletRestart(c)
}

func (s *Service) HandleApp(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	if c.Request().Method == http.MethodOptions {
		return c.NoContent(http.StatusNoContent)
	}
	applet, err := s.resolver.ResolveExampleApplet(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	if err := s.ensure(c, applet); err != nil {
		return err
	}
	match := appletruntime.AppletProxyMatch{AppID: applet.Manifest.ID, StripPrefix: strings.TrimRight(applet.IFrameSrc, "/")}
	proxy := appletruntime.NewAppletProxy(s.runtime, match, proxyOptions())
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func (s *Service) HandleAlias(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	if c.Request().Method == http.MethodOptions {
		return c.NoContent(http.StatusNoContent)
	}
	match, ok := s.aliases.Match(c.Request())
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown applet alias")
	}
	applet, err := s.resolver.ResolveExampleApplet(c.Request().Context(), match.AppID)
	if err != nil {
		return err
	}
	if err := s.ensure(c, applet); err != nil {
		return err
	}
	proxy := appletruntime.NewAppletProxy(s.runtime, match, proxyOptions())
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func (s *Service) ensure(c echo.Context, applet applets.AppletContext) error {
	if s.runtime == nil {
		return echo.NewHTTPError(http.StatusBadGateway, "applet runtime is unavailable")
	}
	if _, err := s.runtime.EnsureStarted(c.Request().Context(), applets.RuntimeConfigFromManifest(applet)); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return nil
}

func (s *Service) registerRootAliases(e *echo.Echo) {
	ids, err := filepath.Glob(filepath.Join(s.examplesRoot, "*", "AGENTS.md"))
	if err != nil {
		return
	}
	seenRoutes := map[string]struct{}{}
	for _, agentsPath := range ids {
		id := filepath.Base(filepath.Dir(agentsPath))
		ctx, err := s.resolver.ResolveExampleApplet(nil, id)
		if err != nil || len(ctx.Manifest.RootAliases) == 0 {
			continue
		}
		if err := s.aliases.Register(aliasRegistration(ctx)); err != nil {
			continue
		}
		for _, alias := range ctx.Manifest.RootAliases {
			route := echoRoute(alias.Pattern)
			if route == "" {
				continue
			}
			if _, ok := seenRoutes[route]; ok {
				continue
			}
			seenRoutes[route] = struct{}{}
			e.OPTIONS(route, s.HandleAppOptions)
			e.Any(route, s.HandleAlias)
		}
	}
}

func aliasRegistration(ctx applets.AppletContext) appletruntime.AliasRegistration {
	aliases := make([]appletruntime.RouteAlias, 0, len(ctx.Manifest.RootAliases))
	for _, alias := range ctx.Manifest.RootAliases {
		aliases = append(aliases, appletruntime.RouteAlias{Pattern: alias.Pattern, Methods: alias.Methods})
	}
	return appletruntime.AliasRegistration{AppID: ctx.Manifest.ID, Aliases: aliases}
}

func echoRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "/" || pattern == "/*" {
		return ""
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.TrimSuffix(pattern, "/*") + "/*"
	}
	return pattern
}

func proxyOptions() appletruntime.ProxyOptions {
	return appletruntime.ProxyOptions{FlushSSE: true, RewriteCookiePath: true, AllowNullOriginCORS: true}
}

func setAppletCORS(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "null")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Datastar-Request")
	header.Set("Access-Control-Max-Age", "600")
}
