package workspaces

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestProxyByHostPreservesMethodPathQueryBody(t *testing.T) {
	child := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if r.Method != http.MethodPost {
				t.Errorf("method %s", r.Method)
			}
			if r.URL.Path != "/forms/comments" || r.URL.RawQuery != "x=1" {
				t.Errorf("url %s", r.URL.String())
			}
			if string(body) != "hello" {
				t.Errorf("body %q", body)
			}
			if r.Header.Get("X-Forwarded-Host") != "foo.cn-agents.test" {
				t.Errorf("xfh %q", r.Header.Get("X-Forwarded-Host"))
			}
			if r.Header.Get("X-Forwarded-Proto") != "http" {
				t.Errorf("xfp %q", r.Header.Get("X-Forwarded-Proto"))
			}
			if r.Header.Get("X-Vamos-Workspace-Proxy") != "1" {
				t.Errorf("workspace proxy header missing")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("proxied"))
		}),
	)
	defer child.Close()

	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			child.URL,
			StatusRunning,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(
		http.MethodPost,
		"/forms/comments?x=1",
		strings.NewReader("hello"),
	)
	req.Host = "foo.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "proxied" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHostDispatchMiddlewareBypassesManagerHost(t *testing.T) {
	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			"http://127.0.0.1:1",
			StatusRunning,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.GET("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(http.MethodGet, "/agent-chat", nil)
	req.Host = "main.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "manager" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestHostDispatchMiddlewareProxiesKnownAppPaths(t *testing.T) {
	var paths []string
	child := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			_, _ = w.Write([]byte("child"))
		}),
	)
	defer child.Close()

	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			child.URL,
			StatusRunning,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	for _, path := range []string{"/", "/agent-chat", "/thoughts/x", "/static/x"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "foo.cn-agents.test"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "child" {
			t.Fatalf("%s status=%d body=%q", path, rec.Code, rec.Body.String())
		}
	}
	if strings.Join(paths, ",") != "/,/agent-chat,/thoughts/x,/static/x" {
		t.Fatalf("paths=%v", paths)
	}
}

func TestProxyByHostStoppedWorkspaceReturnsUnavailable(t *testing.T) {
	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			"http://127.0.0.1:1",
			StatusStopped,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "foo.cn-agents.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://main.cn-agents.test/workspaces") {
		t.Fatalf("body missing manager workspaces link: %q", rec.Body.String())
	}
}

func TestProxyByHostSSEFlushes(t *testing.T) {
	firstEventWritten := make(chan struct{})
	finish := make(chan struct{})
	child := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: first\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			close(firstEventWritten)
			<-finish
			_, _ = w.Write([]byte("data: second\n\n"))
		}),
	)
	defer child.Close()

	e := echo.New()
	e.Use(
		testManagerWithWorkspace(
			t,
			child.URL,
			StatusRunning,
		).HostDispatchMiddleware("main.cn-agents.test"),
	)
	e.Any("/*", func(c echo.Context) error { return c.String(http.StatusOK, "manager") })
	server := httptest.NewServer(e)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "foo.cn-agents.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	buf := make([]byte, len("data: first\n\n"))
	readDone := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(resp.Body, buf)
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatal(err)
		}
		if string(buf) != "data: first\n\n" {
			t.Fatalf("first event=%q", string(buf))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first SSE event was not flushed before stream close")
	}
	<-firstEventWritten
	close(finish)
}

func testManagerWithWorkspace(
	t *testing.T,
	childURL string,
	status Status,
) *ManagerService {
	t.Helper()
	u, err := url.Parse(childURL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port := 0
	for _, r := range portText {
		port = port*10 + int(r-'0')
	}
	return &ManagerService{
		runtime:   RuntimeConfig{ManagerURL: "https://main.cn-agents.test"},
		discovery: DiscoveryConfig{Domain: "cn-agents.test"},
		workspaces: map[string]Workspace{
			"foo": {
				Slug:        "foo",
				DisplayName: "Foo",
				Host:        "foo.cn-agents.test",
				Status:      status,
				Port:        port,
			},
		},
	}
}
