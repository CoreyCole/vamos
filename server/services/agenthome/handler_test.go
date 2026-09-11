package agenthome

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRedirectAgentsLand(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/agents", nil), rec)
	if err := RedirectAgentsLand(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/threads" {
		t.Fatalf("Location = %q", got)
	}
}
