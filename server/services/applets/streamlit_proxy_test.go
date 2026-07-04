package applets

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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/services/appletruntime"
	"github.com/labstack/echo/v4"
)

func TestStreamlitScopedServiceRouteWebSocketExchangesFrames(t *testing.T) {
	backend := newServiceWebSocketProofBackend(t)
	defer backend.Close()

	manager := &appletProxyServiceProofManager{target: backend.URL}
	e := echo.New()
	service := NewHTTPService(ServiceOptions{
		Resolver: Resolver{ExamplesRoot: writeStreamlitProofManifest(t)},
		Manager:  manager,
	})
	service.RegisterExampleRoutes(e, nil)
	server := httptest.NewServer(e)
	defer server.Close()

	conn := dialServiceRawWebSocket(t, server.URL+"/examples/streamlit/app/_stcore/stream?proof=scoped")
	defer conn.Close()
	writeServiceMaskedTextFrame(t, conn, "ping")
	if got := readServiceTextFrame(t, conn); got != "echo:ping" {
		t.Fatalf("websocket response = %q", got)
	}
	_ = conn.Close()

	seen := backend.SeenRequest(t)
	if seen.URL.RequestURI() != "/_stcore/stream?proof=scoped" {
		t.Fatalf("backend URI = %q", seen.URL.RequestURI())
	}
	assertStreamlitForwardedHeaders(t, seen.Header, "/examples/streamlit/app")
	assertSawAppID(t, manager.ensuredAppIDsSnapshot(), "streamlit")
	assertSawAppID(t, manager.proxiedAppIDsSnapshot(), "streamlit")
	assertSawActive(t, manager, 1)
	waitForServiceActiveConnections(t, manager, 0)
}

func TestStreamlitRootAliasServiceRouteWebSocketExchangesFrames(t *testing.T) {
	backend := newServiceWebSocketProofBackend(t)
	defer backend.Close()

	manager := &appletProxyServiceProofManager{target: backend.URL}
	e := echo.New()
	service := NewHTTPService(ServiceOptions{
		Resolver: Resolver{ExamplesRoot: writeStreamlitProofManifest(t)},
		Manager:  manager,
	})
	service.RegisterExampleRoutes(e, nil)
	if err := service.RegisterStartupAliases(e); err != nil {
		t.Fatalf("RegisterStartupAliases() error = %v", err)
	}
	server := httptest.NewServer(e)
	defer server.Close()

	conn := dialServiceRawWebSocket(t, server.URL+"/_stcore/stream?proof=alias")
	defer conn.Close()
	writeServiceMaskedTextFrame(t, conn, "ping")
	if got := readServiceTextFrame(t, conn); got != "echo:ping" {
		t.Fatalf("websocket response = %q", got)
	}
	_ = conn.Close()

	seen := backend.SeenRequest(t)
	if seen.URL.RequestURI() != "/_stcore/stream?proof=alias" {
		t.Fatalf("backend URI = %q", seen.URL.RequestURI())
	}
	assertStreamlitForwardedHeaders(t, seen.Header, "")
	assertSawAppID(t, manager.ensuredAppIDsSnapshot(), "streamlit")
	assertSawAppID(t, manager.proxiedAppIDsSnapshot(), "streamlit")
	assertSawActive(t, manager, 1)
	waitForServiceActiveConnections(t, manager, 0)
}

func writeStreamlitProofManifest(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "streamlit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
vamos_artifact: applet
applet:
  id: streamlit
  title: Streamlit Proof
  kind: streamlit
  source_dir: .
  files_root: files
  app_route: /examples/streamlit/app/
  start_command: [./start.sh]
---
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type appletProxyServiceProofManager struct {
	mu            sync.Mutex
	target        string
	ensuredAppIDs []string
	proxiedAppIDs []string
	active        int
	activeHistory []int
}

func (m *appletProxyServiceProofManager) EnsureStarted(_ context.Context, cfg appletruntime.RuntimeConfig) (appletruntime.AppletProcessState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensuredAppIDs = append(m.ensuredAppIDs, cfg.AppID)
	return appletruntime.AppletProcessState{AppID: cfg.AppID, Status: appletruntime.ProcessStatusHealthy, Healthy: true}, nil
}

func (m *appletProxyServiceProofManager) Start(ctx context.Context, cfg appletruntime.RuntimeConfig) (appletruntime.ProcessState, error) {
	return m.EnsureStarted(ctx, cfg)
}

func (m *appletProxyServiceProofManager) Stop(context.Context, string) error { return nil }

func (m *appletProxyServiceProofManager) Health(context.Context, string) (appletruntime.AppletProcessState, error) {
	return appletruntime.AppletProcessState{Status: appletruntime.ProcessStatusStopped}, nil
}

func (m *appletProxyServiceProofManager) ProxyTarget(appID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxiedAppIDs = append(m.proxiedAppIDs, appID)
	return m.target, m.target != ""
}

func (m *appletProxyServiceProofManager) Touch(_ string, delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active += delta
	m.activeHistory = append(m.activeHistory, m.active)
}

func (m *appletProxyServiceProofManager) SweepInactive(context.Context, time.Time) ([]appletruntime.AppletProcessState, error) {
	return nil, nil
}

func (m *appletProxyServiceProofManager) activeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *appletProxyServiceProofManager) sawActive(want int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, got := range m.activeHistory {
		if got == want {
			return true
		}
	}
	return false
}

func (m *appletProxyServiceProofManager) activeSnapshot() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int(nil), m.activeHistory...)
}

func (m *appletProxyServiceProofManager) ensuredAppIDsSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.ensuredAppIDs...)
}

func (m *appletProxyServiceProofManager) proxiedAppIDsSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.proxiedAppIDs...)
}

type serviceWebSocketProofBackend struct {
	*httptest.Server
	seen chan *http.Request
}

func newServiceWebSocketProofBackend(t testing.TB) serviceWebSocketProofBackend {
	t.Helper()
	backend := serviceWebSocketProofBackend{seen: make(chan *http.Request, 1)}
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
		accept := serviceWebSocketAcceptKey(r.Header.Get("Sec-WebSocket-Key"))
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
		if err := rw.Flush(); err != nil {
			t.Fatalf("flush 101: %v", err)
		}
		payload := readServiceTextFrame(t, conn)
		writeServiceServerTextFrame(t, conn, "echo:"+payload)
	}))
	return backend
}

func (b serviceWebSocketProofBackend) SeenRequest(t testing.TB) *http.Request {
	t.Helper()
	select {
	case req := <-b.seen:
		return req
	case <-time.After(time.Second):
		t.Fatal("backend did not receive WebSocket request")
		return nil
	}
}

func serviceWebSocketAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

type rawServiceWebSocketConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *rawServiceWebSocketConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func dialServiceRawWebSocket(t testing.TB, rawURL string) net.Conn {
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
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, u.Host, base64.StdEncoding.EncodeToString(key))

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
	return &rawServiceWebSocketConn{Conn: conn, reader: reader}
}

func writeServiceMaskedTextFrame(t testing.TB, conn net.Conn, payload string) {
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

func writeServiceServerTextFrame(t testing.TB, conn net.Conn, payload string) {
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

func readServiceTextFrame(t testing.TB, r io.Reader) string {
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

func assertStreamlitForwardedHeaders(t testing.TB, header http.Header, prefix string) {
	t.Helper()
	if got := header.Get("X-Vamos-Applet-Proxy"); got != "1" {
		t.Fatalf("X-Vamos-Applet-Proxy = %q", got)
	}
	if got := header.Get("X-Forwarded-Prefix"); got != prefix {
		t.Fatalf("X-Forwarded-Prefix = %q, want %q", got, prefix)
	}
	if got := header.Get("X-Forwarded-Host"); got == "" {
		t.Fatal("X-Forwarded-Host is empty")
	}
	if got := header.Get("X-Forwarded-Proto"); got == "" {
		t.Fatal("X-Forwarded-Proto is empty")
	}
}

func assertSawAppID(t testing.TB, got []string, want string) {
	t.Helper()
	for _, appID := range got {
		if appID == want {
			return
		}
	}
	t.Fatalf("app IDs = %#v, want %q", got, want)
}

func assertSawActive(t testing.TB, manager *appletProxyServiceProofManager, want int) {
	t.Helper()
	if !manager.sawActive(want) {
		t.Fatalf("active history never reached %d: %#v", want, manager.activeSnapshot())
	}
}

func waitForServiceActiveConnections(t testing.TB, manager *appletProxyServiceProofManager, want int) {
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
