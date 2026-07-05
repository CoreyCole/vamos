package middleware

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestCappedResponseBodyCountsFullLengthButStoresPrefix(t *testing.T) {
	t.Parallel()

	body := newCappedResponseBody(4)
	body.Write([]byte("abc"))
	body.Write([]byte("def"))

	if got, want := body.Len(), 6; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	if got, want := body.String(), "abcd"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestLoggingMiddlewareDoesNotBufferEntireLargeResponse(t *testing.T) {
	t.Parallel()

	e := echo.New()
	largeBody := strings.Repeat("x", maxResponseBodyLogSize+100)
	handler := LoggingMiddleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, largeBody)
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/large",
		http.NoBody,
	)
	rec := httptest.NewRecorder()

	if err := handler(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.String(); got != largeBody {
		t.Fatalf(
			"response body mismatch: got %d bytes, want %d",
			len(got),
			len(largeBody),
		)
	}
}

func TestLoggingMiddlewarePreservesWebSocketHijack(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(LoggingMiddleware())
	e.GET("/ws", func(c echo.Context) error {
		conn, rw, err := http.NewResponseController(c.Response().Writer).Hijack()
		if err != nil {
			return err
		}
		defer conn.Close()

		accept := websocketAcceptKey(c.Request().Header.Get("Sec-WebSocket-Key"))
		_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
		if err != nil {
			return err
		}
		return rw.Flush()
	})

	server := httptest.NewServer(e)
	defer server.Close()

	conn := dialRawWebSocket(t, strings.Replace(server.URL, "http://", "", 1)+"/ws")
	defer conn.Close()

	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101 Switching Protocols") {
		t.Fatalf("websocket status = %q, want 101", status)
	}
}

func dialRawWebSocket(t testing.TB, address string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", address[:strings.Index(address, "/")])
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", address[strings.Index(address, "/"):], address[:strings.Index(address, "/")], base64.StdEncoding.EncodeToString(key))
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}

func websocketAcceptKey(key string) string {
	return base64.StdEncoding.EncodeToString(websocketSHA1(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
}

func websocketSHA1(value string) []byte {
	h := sha1.New()
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}
