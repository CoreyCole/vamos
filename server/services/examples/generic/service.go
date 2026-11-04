package generic

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/CoreyCole/vamos/server/services/applets"
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

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) error {
	s.applets.RegisterExampleRoutes(e, auth)
	if err := s.applets.RegisterStartupAliases(e); err != nil {
		return fmt.Errorf("register example applet aliases: %w", err)
	}
	return nil
}
