package generic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

type Options struct {
	ExamplesRoot string
}

type Service struct {
	examplesRoot string
	mu           sync.Mutex
	starts       map[string]*sync.Mutex
}

type agentFrontmatter struct {
	VamosArtifact string       `yaml:"vamos_artifact"`
	Applet        AppletConfig `yaml:"applet"`
}

type AppletConfig struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	FilesRoot     string   `yaml:"files_root"`
	AppDir        string   `yaml:"app_dir"`
	CurrentAppDir string   `yaml:"current_app_dir"`
	Route         string   `yaml:"route"`
	AppRoute      string   `yaml:"app_route"`
	StartCommand  []string `yaml:"start_command"`
	HealthPath    string   `yaml:"health_path"`
	Port          int      `yaml:"port"`
	BackendPort   int      `yaml:"backend_port"`
}

type resolvedApplet struct {
	ID           string
	Title        string
	ExampleRoot  string
	AppDir       string
	FilesRoot    string
	Route        string
	AppRoute     string
	StartCommand []string
	HealthPath   string
	Port         int
	BackendPort  int
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
	return &Service{examplesRoot: abs, starts: make(map[string]*sync.Mutex)}, nil
}

func (s *Service) RegisterRoutes(e *echo.Echo, auth echo.MiddlewareFunc) {
	// Sandboxed applets have origin "null", so Datastar performs CORS
	// preflights without auth cookies. Answer app-route OPTIONS outside the
	// authenticated group; authenticated GET/POST still go through the group.
	e.OPTIONS("/examples/:id/app", s.HandleAppOptions)
	e.OPTIONS("/examples/:id/app/*", s.HandleAppOptions)
	e.Any("/examples/:id/app", s.HandleApp)
	e.Any("/examples/:id/app/*", s.HandleApp)

	group := e.Group("/examples")
	if auth != nil {
		group.Use(auth)
	}
	group.GET("/:id", s.HandlePage)
}

func (s *Service) HandleAppOptions(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	return c.NoContent(http.StatusNoContent)
}

func (s *Service) HandlePage(c echo.Context) error {
	app, err := s.load(c.Param("id"))
	if err != nil {
		return err
	}
	if err := s.ensure(c.Request().Context(), app); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return Page(PageModel{ID: app.ID, Title: app.Title, AppURL: strings.TrimRight(app.AppRoute, "/") + "/", AppDir: app.AppDir}).Render(c.Request().Context(), c.Response().Writer)
}

func (s *Service) HandleApp(c echo.Context) error {
	setAppletCORS(c.Response().Header())
	c.Response().Before(func() { setAppletCORS(c.Response().Header()) })
	if c.Request().Method == http.MethodOptions {
		return c.NoContent(http.StatusNoContent)
	}
	app, err := s.load(c.Param("id"))
	if err != nil {
		return err
	}
	if err := s.ensure(c.Request().Context(), app); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(app.Port))
	proxy := httputil.NewSingleHostReverseProxy(target)
	prefix := strings.TrimRight(app.AppRoute, "/")
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		path := strings.TrimPrefix(req.URL.Path, prefix)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req.URL.Path = path
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		setAppletCORS(resp.Header)
		return nil
	}
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func setAppletCORS(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "null")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Datastar-Request")
	header.Set("Access-Control-Max-Age", "600")
}

func (s *Service) load(id string) (resolvedApplet, error) {
	id = strings.TrimSpace(id)
	if !safeID(id) {
		return resolvedApplet{}, echo.NewHTTPError(http.StatusNotFound, "unknown example")
	}
	exampleRoot := filepath.Join(s.examplesRoot, id)
	agentsPath := filepath.Join(exampleRoot, "AGENTS.md")
	body, err := os.ReadFile(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return resolvedApplet{}, echo.NewHTTPError(http.StatusNotFound, "unknown example")
		}
		return resolvedApplet{}, err
	}
	fm, err := parseFrontmatter(body)
	if err != nil {
		return resolvedApplet{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if fm.VamosArtifact != "applet" {
		return resolvedApplet{}, echo.NewHTTPError(http.StatusNotFound, "unknown example")
	}
	cfg := fm.Applet
	if cfg.ID == "" {
		cfg.ID = id
	}
	if cfg.ID != id {
		return resolvedApplet{}, echo.NewHTTPError(http.StatusBadRequest, "example id does not match AGENTS.md applet id")
	}
	if cfg.Title == "" {
		cfg.Title = id
	}
	if cfg.AppDir == "" {
		cfg.AppDir = cfg.CurrentAppDir
	}
	if cfg.AppDir == "" {
		cfg.AppDir = "."
	}
	if cfg.FilesRoot == "" {
		cfg.FilesRoot = "files"
	}
	if cfg.Route == "" {
		cfg.Route = "/examples/" + id
	}
	if cfg.AppRoute == "" {
		cfg.AppRoute = cfg.Route + "/app/"
	}
	if len(cfg.StartCommand) == 0 {
		cfg.StartCommand = []string{"just", "build"}
	}
	if cfg.HealthPath == "" {
		cfg.HealthPath = "/healthz"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.BackendPort == 0 {
		cfg.BackendPort = 18080
	}
	return resolvedApplet{
		ID: cfg.ID, Title: cfg.Title, ExampleRoot: exampleRoot,
		AppDir: filepath.Join(exampleRoot, cfg.AppDir), FilesRoot: filepath.Join(exampleRoot, cfg.FilesRoot),
		Route: cfg.Route, AppRoute: cfg.AppRoute, StartCommand: cfg.StartCommand,
		HealthPath: cfg.HealthPath, Port: cfg.Port, BackendPort: cfg.BackendPort,
	}, nil
}

func parseFrontmatter(body []byte) (agentFrontmatter, error) {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("---\n")) {
		return agentFrontmatter{}, errors.New("AGENTS.md missing frontmatter")
	}
	rest := trimmed[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return agentFrontmatter{}, errors.New("AGENTS.md frontmatter is not closed")
	}
	var fm agentFrontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return agentFrontmatter{}, err
	}
	return fm, nil
}

func (s *Service) ensure(ctx context.Context, app resolvedApplet) error {
	if healthOK(ctx, app.Port, app.HealthPath) {
		return nil
	}
	lock := s.startLock(app.ID)
	lock.Lock()
	defer lock.Unlock()
	if healthOK(ctx, app.Port, app.HealthPath) {
		return nil
	}
	if len(app.StartCommand) == 0 || app.StartCommand[0] == "" {
		return fmt.Errorf("example %s has no start command", app.ID)
	}
	startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(startCtx, app.StartCommand[0], app.StartCommand[1:]...)
	cmd.Dir = app.AppDir
	cmd.Env = append(os.Environ(),
		"PORT="+strconv.Itoa(app.Port),
		"BACKEND_PORT="+strconv.Itoa(app.BackendPort),
		"VAMOS_APP_FILES_ROOT="+app.FilesRoot,
	)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(&out)
	cmd.Stderr = io.MultiWriter(&out)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start example %s: %w\n%s", app.ID, err, out.String())
	}
	if !healthOK(ctx, app.Port, app.HealthPath) {
		return fmt.Errorf("example %s did not become healthy on port %d", app.ID, app.Port)
	}
	return nil
}

func (s *Service) startLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.starts[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.starts[id] = lock
	}
	return lock
}

func healthOK(ctx context.Context, port int, path string) bool {
	if path == "" {
		path = "/healthz"
	}
	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func safeID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
