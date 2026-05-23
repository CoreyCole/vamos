package workspaces

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func (m *ManagerService) HostDispatchMiddleware(
	managerHosts ...string,
) echo.MiddlewareFunc {
	managerHostSet := map[string]struct{}{}
	for _, host := range managerHosts {
		if normalized := normalizeHost(host); normalized != "" {
			managerHostSet[normalized] = struct{}{}
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			host := normalizeHost(c.Request().Host)
			if host == "" {
				return next(c)
			}
			if _, isManager := managerHostSet[host]; isManager {
				return next(c)
			}
			slug, ok := SlugFromHost(host, m.discovery.Domain)
			if !ok || slug == "main" {
				return next(c)
			}
			return m.ProxyByHost(c)
		}
	}
}

func (m *ManagerService) ProxyByHost(c echo.Context) error {
	ws, ok := m.LookupHost(c.Request().Host)
	if !ok {
		log.Printf(
			"workspace_proxy_unknown host=%q path=%q method=%q",
			c.Request().Host,
			c.Request().URL.Path,
			c.Request().Method,
		)
		return m.renderWorkspaceRecovery(c, Workspace{}, true)
	}
	if ws.Status != StatusRunning || ws.Port == 0 {
		log.Printf(
			"workspace_proxy_unavailable slug=%q host=%q status=%q pid=%d port=%d path=%q method=%q",
			ws.Slug,
			c.Request().Host,
			ws.Status,
			ws.PID,
			ws.Port,
			c.Request().URL.Path,
			c.Request().Method,
		)
		return m.renderWorkspaceRecovery(c, ws, false)
	}

	target := &url.URL{Scheme: "http", Host: ws.LocalAddr()}
	proxy := newWorkspaceReverseProxy(target)
	proxy.ServeHTTP(c.Response().Writer, c.Request())
	return nil
}

func (w Workspace) LocalAddr() string {
	if w.Port == 0 {
		return ""
	}
	return "127.0.0.1:" + strconv.Itoa(w.Port)
}

func newWorkspaceReverseProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		originalDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Host", originalHost)
		req.Header.Set("X-Forwarded-Proto", forwardedProto(req))
		req.Header.Set("X-Vamos-Workspace-Proxy", "1")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		status := http.StatusBadGateway
		if errors.Is(err, context.Canceled) {
			status = 499
		}
		http.Error(w, "workspace backend unavailable", status)
	}
	return proxy
}

func (m *ManagerService) renderWorkspaceRecovery(
	c echo.Context,
	ws Workspace,
	unknown bool,
) error {
	statusCode := http.StatusServiceUnavailable
	if unknown {
		statusCode = http.StatusNotFound
		ws.DisplayName = "Unknown workspace"
		ws.Status = StatusInvalid
		ws.Error = "workspace host is not registered with this manager"
	}
	status := RuntimeStatus{
		Status: ws.Status,
		Phase:  ws.Phase,
		Error:  ws.Error,
		Logs:   bundleLogs(ws.Bundle),
		Ports:  ws.Ports,
		PIDs:   ws.PIDs,
		Build:  ws.BuildStatus,
	}
	if !unknown && m.store != nil {
		if persisted, err := m.store.ReadStatus(ws); err == nil {
			status = persisted
		} else if status.Error == "" && ws.Status == StatusInvalid {
			status.Error = err.Error()
		}
	}
	returnTo := c.Request().URL.RequestURI()
	if returnTo == "" {
		returnTo = "/"
	}
	model := RecoveryModel{
		Workspace:     ws,
		ManagerURL:    m.runtime.ManagerURL,
		Status:        status,
		LogTails:      recoveryLogTails(status, 80),
		ReturnTo:      returnTo,
		Authenticated: strings.TrimSpace(userEmailFromContext(c)) != "",
		UnsafeRequest: isUnsafeMethod(c.Request().Method),
		Unknown:       unknown,
	}
	return render(c, statusCode, WorkspaceRecovery(model))
}

func recoveryLogTails(status RuntimeStatus, lines int) map[BundleComponent]string {
	tails := map[BundleComponent]string{}
	if status.Build.LogPath != "" {
		if tail, err := NewFileLogTailer().Tail(status.Build.LogPath, lines); err == nil &&
			strings.TrimSpace(tail) != "" {
			tails[BundleComponent("build")] = tail
		}
	}
	for component, path := range status.Logs {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if tail, err := NewFileLogTailer().Tail(path, lines); err == nil &&
			strings.TrimSpace(tail) != "" {
			tails[component] = tail
		}
	}
	return tails
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

func forwardedProto(req *http.Request) string {
	if proto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}
