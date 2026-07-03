package generic

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/applets"
	"github.com/labstack/echo/v4"
)

type Options struct {
	ExamplesRoot  string
	AppletRuntime appletruntime.Manager
	AppletService *applets.Service
}

type Service struct {
	examplesRoot string
	applets      *applets.Service
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
	appletService := opts.AppletService
	if appletService == nil {
		appletService = applets.NewHTTPService(applets.ServiceOptions{
			Resolver: applets.Resolver{ExamplesRoot: abs},
			Manager:  opts.AppletRuntime,
		})
	}
	return &Service{examplesRoot: abs, applets: appletService}, nil
}

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	s.applets.RegisterExampleRoutes(e, auth)
	_ = s.applets.RegisterStartupAliases(e)
}

func (s *Service) HandleAppOptions(c echo.Context) error { return s.applets.HandleAppletOptions(c) }

func (s *Service) HandlePage(c echo.Context) error { return s.applets.HandleAppletPage(c) }

func (s *Service) HandleStatus(c echo.Context) error { return s.applets.HandleAppletStatus(c) }

func (s *Service) HandleStop(c echo.Context) error { return s.applets.HandleAppletStop(c) }

func (s *Service) HandleRestart(c echo.Context) error { return s.applets.HandleAppletRestart(c) }

func (s *Service) HandleApp(c echo.Context) error { return s.applets.HandleAppletProxy(c) }

func (s *Service) HandleAlias(c echo.Context) error { return s.applets.HandleAlias(c) }
