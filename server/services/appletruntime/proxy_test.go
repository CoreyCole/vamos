package appletruntime

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
	waitForActiveConnections(t, manager, 0)
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

func TestRewriteAppletCookiePathsFansOutRootCookiesToAliases(t *testing.T) {
	header := http.Header{}
	header.Add("Set-Cookie", "wordle_user=e2e; Path=/; HttpOnly")
	header.Add("Set-Cookie", "theme=dark; Path=/keep; Secure")

	RewriteAppletCookiePaths(header, CookieRewriteConfig{
		ScopedPrefix: "/examples/wordle/app/",
		AliasPaths:   []string{"/events", "/guesses", "/events"},
	})

	cookies := header.Values("Set-Cookie")
	assertCookiePath(t, cookies, "wordle_user=e2e", "/examples/wordle/app")
	assertCookiePath(t, cookies, "wordle_user=e2e", "/events")
	assertCookiePath(t, cookies, "wordle_user=e2e", "/guesses")
	assertCookiePath(t, cookies, "theme=dark", "/keep")
	if containsCookiePath(cookies, "theme=dark", "/events") {
		t.Fatalf("non-root cookie fanned out to alias: %#v", cookies)
	}
	if containsCookiePath(cookies, "wordle_user=e2e", "/") {
		t.Fatalf("root cookie leaked through unchanged: %#v", cookies)
	}
}

func TestAppletProxyScopedRouteFansOutAliasCookies(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "sid=1; Path=/; HttpOnly")
		_, _ = w.Write([]byte("ok"))
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "wordle", StripPrefix: "/examples/wordle/app", AliasCookiePaths: []string{"/events", "/guesses"}},
		ProxyOptions{RewriteCookiePath: true},
	))
	defer server.Close()

	resp, err := http.Get(server.URL + "/examples/wordle/app/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cookies := resp.Header.Values("Set-Cookie")
	assertCookiePath(t, cookies, "sid=1", "/examples/wordle/app")
	assertCookiePath(t, cookies, "sid=1", "/events")
	assertCookiePath(t, cookies, "sid=1", "/guesses")
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

func TestAppletProxyScopedStreamlitWebSocketExchangesFrames(t *testing.T) {
	backend := newWebSocketProofBackend(t)
	defer backend.Close()

	manager := &proxyTestManager{target: backend.URL}
	server := httptest.NewServer(NewAppletProxy(
		manager,
		AppletProxyMatch{AppID: "streamlit", StripPrefix: "/examples/streamlit/app"},
		ProxyOptions{},
	))
	defer server.Close()

	conn := dialRawWebSocket(t, server.URL+"/examples/streamlit/app/_stcore/stream?proof=scoped", nil)
	defer conn.Close()
	writeMaskedTextFrame(t, conn, "ping")
	if got := readTextFrame(t, conn); got != "echo:ping" {
		t.Fatalf("websocket response = %q", got)
	}
	_ = conn.Close()

	seen := backend.SeenRequest(t)
	if seen.URL.RequestURI() != "/_stcore/stream?proof=scoped" {
		t.Fatalf("backend URI = %q", seen.URL.RequestURI())
	}
	if got := seen.Header.Get("X-Forwarded-Prefix"); got != "/examples/streamlit/app" {
		t.Fatalf("X-Forwarded-Prefix = %q", got)
	}
	assertStreamlitForwardedProxyHeaders(t, seen.Header)
	if !manager.sawActive(1) {
		t.Fatalf("active history never reached 1: %#v", manager.activeSnapshot())
	}
	waitForActiveConnections(t, manager, 0)
}

func TestAppletProxyAliasStreamlitWebSocketExchangesFrames(t *testing.T) {
	backend := newWebSocketProofBackend(t)
	defer backend.Close()

	manager := &proxyTestManager{target: backend.URL}
	server := httptest.NewServer(NewAppletProxy(
		manager,
		AppletProxyMatch{AppID: "streamlit", Alias: true},
		ProxyOptions{},
	))
	defer server.Close()

	conn := dialRawWebSocket(t, server.URL+"/_stcore/stream?proof=alias", nil)
	defer conn.Close()
	writeMaskedTextFrame(t, conn, "ping")
	if got := readTextFrame(t, conn); got != "echo:ping" {
		t.Fatalf("websocket response = %q", got)
	}
	_ = conn.Close()

	seen := backend.SeenRequest(t)
	if seen.URL.RequestURI() != "/_stcore/stream?proof=alias" {
		t.Fatalf("backend URI = %q", seen.URL.RequestURI())
	}
	if got := seen.Header.Get("X-Forwarded-Prefix"); got != "" {
		t.Fatalf("alias X-Forwarded-Prefix = %q", got)
	}
	assertStreamlitForwardedProxyHeaders(t, seen.Header)
	if !manager.sawActive(1) {
		t.Fatalf("active history never reached 1: %#v", manager.activeSnapshot())
	}
	waitForActiveConnections(t, manager, 0)
}

func TestAppletProxyAliasPreservesStreamlitWebSocketPath(t *testing.T) {
	seen := make(chan *http.Request, 1)
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(r.Context())
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "streamlit", Alias: true},
		ProxyOptions{FlushSSE: true},
	))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/_stcore/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	_, _ = http.DefaultClient.Do(req)

	select {
	case got := <-seen:
		if got.URL.Path != "/_stcore/stream" {
			t.Fatalf("path = %q", got.URL.Path)
		}
		if !strings.EqualFold(got.Header.Get("Upgrade"), "websocket") {
			t.Fatalf("Upgrade = %q", got.Header.Get("Upgrade"))
		}
	case <-time.After(time.Second):
		t.Fatal("child did not receive Streamlit websocket alias request")
	}
}

func TestAppletProxyAliasPreservesVendorAssetPath(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer child.Close()

	server := httptest.NewServer(NewAppletProxy(
		&proxyTestManager{target: child.URL},
		AppletProxyMatch{AppID: "streamlit", Alias: true},
		ProxyOptions{FlushSSE: true},
	))
	defer server.Close()

	resp, err := http.Get(server.URL + "/vendor/bootstrap.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "/vendor/bootstrap.js" {
		t.Fatalf("vendor path = %q", body)
	}
}

func assertCookiePath(t *testing.T, cookies []string, prefix, path string) {
	t.Helper()
	if !containsCookiePath(cookies, prefix, path) {
		t.Fatalf("cookies missing %s with Path=%s: %#v", prefix, path, cookies)
	}
}

func containsCookiePath(cookies []string, prefix, path string) bool {
	for _, cookie := range cookies {
		if !strings.HasPrefix(cookie, prefix) {
			continue
		}
		for _, part := range strings.Split(cookie, ";") {
			if strings.EqualFold(strings.TrimSpace(part), "Path="+path) {
				return true
			}
		}
	}
	return false
}

type websocketProofBackend struct {
	*httptest.Server
	seen chan *http.Request
}

func newWebSocketProofBackend(t testing.TB) websocketProofBackend {
	t.Helper()
	backend := websocketProofBackend{seen: make(chan *http.Request, 1)}
	backend.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend.seen <- r.Clone(r.Context())
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("response writer cannot hijack")
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		defer conn.Close()
		accept := websocketAcceptKey(r.Header.Get("Sec-WebSocket-Key"))
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
		if err := rw.Flush(); err != nil {
			t.Fatalf("flush 101: %v", err)
		}
		payload := readTextFrame(t, conn)
		writeServerTextFrame(t, conn, "echo:"+payload)
	}))
	return backend
}

func (b websocketProofBackend) SeenRequest(t testing.TB) *http.Request {
	t.Helper()
	select {
	case req := <-b.seen:
		return req
	case <-time.After(time.Second):
		t.Fatal("backend did not receive WebSocket request")
		return nil
	}
}

func websocketAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

type rawWebSocketConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *rawWebSocketConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func dialRawWebSocket(t testing.TB, rawURL string, headers http.Header) net.Conn {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse websocket URL: %v", err)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		_ = conn.Close()
		t.Fatalf("websocket key: %v", err)
	}
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n", path, u.Host, base64.StdEncoding.EncodeToString(key))
	for name, values := range headers {
		for _, value := range values {
			fmt.Fprintf(conn, "%s: %s\r\n", name, value)
		}
	}
	_, _ = io.WriteString(conn, "\r\n")

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket status: %v", err)
	}
	if !strings.Contains(status, " 101 ") {
		_ = conn.Close()
		t.Fatalf("websocket status = %q", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read websocket header: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}
	return &rawWebSocketConn{Conn: conn, reader: reader}
}

func writeMaskedTextFrame(t testing.TB, conn net.Conn, payload string) {
	t.Helper()
	body := []byte(payload)
	if len(body) >= 126 {
		t.Fatalf("payload too long for test frame: %d", len(body))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		t.Fatalf("mask: %v", err)
	}
	frame := []byte{0x81, byte(0x80 | len(body))}
	frame = append(frame, mask...)
	for i, b := range body {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("write websocket frame: %v", err)
	}
}

func writeServerTextFrame(t testing.TB, conn net.Conn, payload string) {
	t.Helper()
	body := []byte(payload)
	if len(body) >= 126 {
		t.Fatalf("payload too long for test frame: %d", len(body))
	}
	frame := append([]byte{0x81, byte(len(body))}, body...)
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("write server websocket frame: %v", err)
	}
}

func readTextFrame(t testing.TB, r io.Reader) string {
	t.Helper()
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("read websocket frame header: %v", err)
	}
	if header[0]&0x0f != 0x1 {
		t.Fatalf("websocket opcode = %d, want text", header[0]&0x0f)
	}
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)
	if length >= 126 {
		t.Fatalf("unsupported websocket payload length = %d", length)
	}
	mask := []byte{0, 0, 0, 0}
	if masked {
		if _, err := io.ReadFull(r, mask); err != nil {
			t.Fatalf("read websocket mask: %v", err)
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read websocket payload: %v", err)
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return string(payload)
}

func assertStreamlitForwardedProxyHeaders(t testing.TB, header http.Header) {
	t.Helper()
	if got := header.Get("X-Vamos-Applet-Proxy"); got != "1" {
		t.Fatalf("X-Vamos-Applet-Proxy = %q", got)
	}
	if got := header.Get("X-Forwarded-Host"); got == "" {
		t.Fatal("X-Forwarded-Host is empty")
	}
	if got := header.Get("X-Forwarded-Proto"); got == "" {
		t.Fatal("X-Forwarded-Proto is empty")
	}
}

func waitForActiveConnections(t testing.TB, manager *proxyTestManager, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := manager.activeCount(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("active connections = %d, want %d", manager.activeCount(), want)
}

type proxyTestManager struct {
	mu            sync.Mutex
	target        string
	active        int
	activeHistory []int
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
func (m *proxyTestManager) ProxyTarget(string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.target, m.target != ""
}
func (m *proxyTestManager) Touch(_ string, activeDelta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active += activeDelta
	m.activeHistory = append(m.activeHistory, m.active)
}
func (m *proxyTestManager) SweepInactive(context.Context, time.Time) ([]AppletProcessState, error) {
	return nil, nil
}

func (m *proxyTestManager) activeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *proxyTestManager) sawActive(want int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, got := range m.activeHistory {
		if got == want {
			return true
		}
	}
	return false
}

func (m *proxyTestManager) activeSnapshot() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int(nil), m.activeHistory...)
}
