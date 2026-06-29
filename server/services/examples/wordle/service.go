package wordle

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/labstack/echo/v4"
)

type Options struct {
	AppletRuntime appletruntime.Manager
	FilesRoot     string
	CurrentAppDir string
}

type Service struct {
	opts Options
}

func NewService(opts Options) (*Service, error) {
	var err error
	if strings.TrimSpace(opts.FilesRoot) == "" {
		opts.FilesRoot = filepath.Join("examples", "wordle", "files")
	}
	if opts.FilesRoot, err = filepath.Abs(opts.FilesRoot); err != nil {
		return nil, fmt.Errorf("resolve wordle files root: %w", err)
	}
	if strings.TrimSpace(opts.CurrentAppDir) == "" {
		opts.CurrentAppDir = filepath.Join(opts.FilesRoot, "apps", "current")
	}
	if opts.CurrentAppDir, err = filepath.Abs(opts.CurrentAppDir); err != nil {
		return nil, fmt.Errorf("resolve wordle current app dir: %w", err)
	}
	return &Service{opts: opts}, nil
}

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	group := e.Group("/examples/wordle")
	if auth != nil {
		group.Use(auth)
	}
	group.GET("", s.HandlePage)
	if s.opts.AppletRuntime != nil {
		group.Any("/app/*", echo.WrapHandler(appletruntime.NewAppletProxy(s.opts.AppletRuntime, "wordle", "/examples/wordle/app")))
	}
}

func (s *Service) EnsureCurrentApplet(ctx context.Context) error {
	runtime := s.opts.AppletRuntime
	if runtime == nil {
		return nil
	}
	if _, ok := runtime.ProxyTarget("wordle"); ok {
		return nil
	}
	_, err := runtime.Start(ctx, appletruntime.RuntimeConfig{
		AppID:        "wordle",
		FilesRoot:    s.opts.FilesRoot,
		SourceDir:    s.opts.CurrentAppDir,
		StartCommand: []string{"go", "run", "."},
		HealthPath:   "/healthz",
	})
	if err != nil {
		return fmt.Errorf("start current wordle applet: %w", err)
	}
	return nil
}

func (s *Service) HandlePage(c echo.Context) error {
	if err := s.EnsureCurrentApplet(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return Page().Render(c.Request().Context(), c.Response().Writer)
}
