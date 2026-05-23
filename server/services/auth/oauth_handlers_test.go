package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHandleLoginPageSanitizesStreamRedirect(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/login?redirect=%2Fagent-chat%2Fstream%3Fthread%3Dthread-1%26run%3Drun-1",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	service := &Service{}
	if err := service.HandleLoginPage(c); err != nil {
		t.Fatalf("HandleLoginPage returned error: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "/auth/google?redirect=%2Fthoughts%2F") {
		t.Fatalf("expected sanitized redirect in body, got %q", body)
	}
	if strings.Contains(body, "%2Fagent-chat%2Fstream") {
		t.Fatalf("expected stream redirect to be removed from body, got %q", body)
	}
}
