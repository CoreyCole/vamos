package appletruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAppletProxyStripsScopedPrefixAndPreservesQuery(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/events?x=1" {
			t.Fatalf("forwarded URI = %q", r.URL.RequestURI())
		}
		if got := r.Header.Get("X-Forwarded-Prefix"); got != "/thoughts/_render/app/wordle/app" {
			t.Fatalf("X-Forwarded-Prefix = %q", got)
		}
		if got := r.Header.Get("X-Vamos-Applet-Proxy"); got != "1" {
			t.Fatalf("X-Vamos-Applet-Proxy = %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer child.Close()

	manager := &proxyTestManager{target: child.URL}
	proxy := NewAppletProxy(
		manager,
		AppletProxyMatch{AppID: "wordle", StripPrefix: "/thoughts/_render/app/wordle/app"},
		ProxyOptions{FlushSSE: true},
	)
	server := httptest.NewServer(proxy)
	defer server.Close()

	resp, err := http.Get(server.URL + "/thoughts/_render/app/wordle/app/events?x=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if manager.active != 0 {
		t.Fatalf("active connections after request = %d", manager.active)
	}
}

func TestAppletProxyAliasKeepsRootPath(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Fatalf("forwarded path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Forwarded-Prefix"); got != "" {
			t.Fatalf("alias X-Forwarded-Prefix = %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "wordle", Alias: true},
		ProxyOptions{FlushSSE: true},
	))
	defer server.Close()

	resp, err := http.Get(server.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestAppletProxySSEFlushes(t *testing.T) {
	firstEventWritten := make(chan struct{})
	finish := make(chan struct{})
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(firstEventWritten)
		<-finish
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "wordle", StripPrefix: "/examples/wordle/app"},
		ProxyOptions{FlushSSE: true},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/examples/wordle/app/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

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
			t.Fatalf("first event = %q", string(buf))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first SSE event was not flushed before stream close")
	}
	<-firstEventWritten
	close(finish)
}

func TestRewriteCookiePath(t *testing.T) {
	header := http.Header{}
	header.Add("Set-Cookie", "sid=1; Path=/; HttpOnly")
	header.Add("Set-Cookie", "theme=dark; Path=/keep; Secure")

	RewriteCookiePath(header, "/examples/wordle/app/")

	cookies := header.Values("Set-Cookie")
	if len(cookies) != 2 {
		t.Fatalf("cookies = %#v", cookies)
	}
	if !strings.Contains(cookies[0], "Path=/examples/wordle/app") {
		t.Fatalf("root cookie path not rewritten: %#v", cookies)
	}
	if !strings.Contains(cookies[1], "Path=/keep") {
		t.Fatalf("non-root cookie path rewritten: %#v", cookies)
	}
}

func TestAppletProxyPreservesUpgradeHeaders(t *testing.T) {
	seen := make(chan http.Header, 1)
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "streamlit", StripPrefix: "/thoughts/_render/app/streamlit/app"},
		ProxyOptions{},
	))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/thoughts/_render/app/streamlit/app/_stcore/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	_, _ = http.DefaultClient.Do(req)

	select {
	case header := <-seen:
		if !strings.EqualFold(header.Get("Upgrade"), "websocket") {
			t.Fatalf("Upgrade = %q", header.Get("Upgrade"))
		}
	case <-time.After(time.Second):
		t.Fatal("child did not receive request")
	}
}

type proxyTestManager struct {
	target string
	active int
}

func (m *proxyTestManager) EnsureStarted(context.Context, RuntimeConfig) (AppletProcessState, error) {
	return AppletProcessState{}, nil
}
func (m *proxyTestManager) Start(context.Context, RuntimeConfig) (ProcessState, error) {
	return ProcessState{}, nil
}
func (m *proxyTestManager) Stop(context.Context, string) error { return nil }
func (m *proxyTestManager) Health(context.Context, string) (AppletProcessState, error) {
	return AppletProcessState{}, nil
}
func (m *proxyTestManager) ProxyTarget(string) (string, bool) { return m.target, m.target != "" }
func (m *proxyTestManager) Touch(_ string, activeDelta int)   { m.active += activeDelta }
func (m *proxyTestManager) SweepInactive(context.Context, time.Time) ([]AppletProcessState, error) {
	return nil, nil
}
