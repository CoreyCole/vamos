package workspaces

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

type Verifier struct {
	Manager                Manager
	ManagerListenAddr      string
	Runs                   VerifyRunStore
	Tailer                 LogTailer
	Prober                 LocalProber
	InternalAgentChatToken string
	HTTPClient             *http.Client
}

func NewVerifier(
	manager Manager,
	managerListenAddr string,
	store VerifyRunStore,
	tailer LogTailer,
	prober LocalProber,
) *Verifier {
	return &Verifier{
		Manager:           manager,
		ManagerListenAddr: normalizeLoopbackListenAddr(managerListenAddr),
		Runs:              store,
		Tailer:            tailer,
		Prober:            prober,
	}
}

func normalizeLoopbackListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:4200"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func (h *Handler) RegisterInternalVerificationRoutes(e *echo.Echo, verifier *Verifier) {
	h.verifier = verifier
	e.POST("/internal/workspaces/verify", h.HandleVerifyWorkspace)
	e.GET("/internal/workspaces/verify/:run_id", h.HandleGetVerifyWorkspaceRun)
	e.GET("/internal/workspaces/verify/:run_id/events", h.HandleStreamVerifyWorkspaceRun)
	e.GET("/internal/workspaces/:slug/logs", h.HandleWorkspaceLogs)
	e.GET("/internal/workspaces/:slug/diagnostics", h.HandleWorkspaceDiagnostics)
}

func (h *Handler) requireRestartToken(c echo.Context) error {
	got := strings.TrimSpace(
		c.Request().Header.Get("X-Vamos-Workspace-Restart-Token"),
	)
	want := strings.TrimSpace(h.restartToken)
	if got == "" || want == "" || len(got) != len(want) ||
		subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return echo.NewHTTPError(http.StatusUnauthorized, "bad restart token")
	}
	return nil
}

func (h *Handler) HandleVerifyWorkspace(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.verifier == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace verifier is not configured",
		)
	}
	var req VerifyWorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	run, err := h.verifier.StartRun(c.Request().Context(), req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusAccepted, run)
}

func (h *Handler) HandleGetVerifyWorkspaceRun(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.verifier == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace verifier is not configured",
		)
	}
	run, err := h.verifier.GetRun(c.Request().Context(), c.Param("run_id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, run)
}

func (h *Handler) HandleStreamVerifyWorkspaceRun(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.verifier == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace verifier is not configured",
		)
	}
	updates, err := h.verifier.RunEvents(c.Request().Context(), c.Param("run_id"))
	if err != nil {
		return err
	}
	res := c.Response()
	res.Header().Set(echo.HeaderContentType, "text/event-stream")
	res.Header().Set(echo.HeaderCacheControl, "no-cache")
	res.WriteHeader(http.StatusOK)
	flusher, _ := res.Writer.(http.Flusher)
	encoder := json.NewEncoder(res.Writer)
	for run := range updates {
		if _, err := res.Writer.Write([]byte("event: update\ndata: ")); err != nil {
			return err
		}
		if err := encoder.Encode(run); err != nil {
			return err
		}
		if _, err := res.Writer.Write([]byte("\n")); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		if run.Status == VerifyRunPassed || run.Status == VerifyRunFailed {
			return nil
		}
	}
	return nil
}

func (h *Handler) HandleWorkspaceLogs(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.verifier == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace verifier is not configured",
		)
	}
	tail, err := h.verifier.Logs(c.Request().Context(), c.Param("slug"), tailLines(c))
	if err != nil {
		return err
	}
	return c.String(http.StatusOK, tail)
}

func (h *Handler) HandleWorkspaceDiagnostics(c echo.Context) error {
	if err := h.requireRestartToken(c); err != nil {
		return err
	}
	if h.verifier == nil {
		return echo.NewHTTPError(
			http.StatusNotFound,
			"workspace verifier is not configured",
		)
	}
	diagnostics, err := h.verifier.Diagnostics(
		c.Request().Context(),
		c.Param("slug"),
		tailLines(c),
	)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, diagnostics)
}

func tailLines(c echo.Context) int {
	n, _ := strconv.Atoi(c.QueryParam("tail"))
	if n <= 0 {
		return 200
	}
	if n > 2000 {
		return 2000
	}
	return n
}

func (v *Verifier) Diagnostics(
	ctx context.Context,
	slug string,
	tailLines int,
) (WorkspaceDiagnostics, error) {
	return BuildWorkspaceDiagnostics(ctx, v.Manager, v.Tailer, v.Prober, slug, tailLines)
}

func (v *Verifier) Logs(ctx context.Context, slug string, tailLines int) (string, error) {
	diagnostics, err := v.Diagnostics(ctx, slug, tailLines)
	if err != nil {
		return "", err
	}
	return diagnostics.LogTail, nil
}
