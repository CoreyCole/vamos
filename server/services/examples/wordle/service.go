package wordle

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
		legacyDir := filepath.Join(opts.FilesRoot, "apps", "current")
		if info, statErr := os.Stat(legacyDir); statErr == nil && info.IsDir() {
			opts.CurrentAppDir = legacyDir
		} else {
			opts.CurrentAppDir = filepath.Dir(opts.FilesRoot)
		}
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
		proxy := echo.WrapHandler(appletruntime.NewAppletProxy(
			s.opts.AppletRuntime,
			appletruntime.AppletProxyMatch{AppID: "wordle", StripPrefix: "/examples/wordle/app"},
			appletruntime.ProxyOptions{FlushSSE: true, RewriteCookiePath: true, AllowNullOriginCORS: true},
		))
		group.Any("/app", proxy)
		group.Any("/app/*", proxy)
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
	runtimeRoot, env := s.runtimeRootAndEnv()
	_, err := runtime.Start(ctx, appletruntime.RuntimeConfig{
		AppID:        "wordle",
		FilesRoot:    runtimeRoot,
		SourceDir:    s.opts.CurrentAppDir,
		StartCommand: s.startCommand(),
		HealthPath:   "/healthz",
		Env:          env,
	})
	if err != nil {
		return fmt.Errorf("start current wordle applet: %w", err)
	}
	return nil
}

func (s *Service) runtimeRootAndEnv() (string, map[string]string) {
	if strings.HasPrefix(s.opts.CurrentAppDir, s.opts.FilesRoot+string(os.PathSeparator)) || s.opts.CurrentAppDir == s.opts.FilesRoot {
		return s.opts.FilesRoot, nil
	}
	return filepath.Dir(s.opts.FilesRoot), map[string]string{"VAMOS_APP_FILES_ROOT": s.opts.FilesRoot}
}

func (s *Service) startCommand() []string {
	if _, err := os.Stat(filepath.Join(s.opts.CurrentAppDir, "cmd", "app")); err == nil {
		return []string{"go", "run", "./cmd/app"}
	}
	return []string{"go", "run", "."}
}

func (s *Service) HandlePage(c echo.Context) error {
	if err := s.EnsureCurrentApplet(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return Page().Render(c.Request().Context(), c.Response().Writer)
}
