package middleware

import (
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
