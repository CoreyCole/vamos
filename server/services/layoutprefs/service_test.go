package layoutprefs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func TestServiceUpsertGetReset(t *testing.T) {
	t.Parallel()

	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	service := NewService(svc.Queries)
	cfg := workbench.DefaultWorkbenchConfig(
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
		"",
	)
	cfg.Regions[0].Visible = true
	cfg.Regions[0].Ratio = 0.31
	cfg.Mobile.ActiveRegionID = "agent-chat-navigation"

	saved, err := service.Upsert(t.Context(), Input{
		UserEmail: "agent@example.com",
		Page:      workbench.WorkbenchPageAgentChat,
		View:      workbench.WorkbenchViewFocus,
		Config:    cfg,
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if saved.Regions[0].Visible {
		t.Fatal("saved config should strip visible navigation")
	}
	if saved.Regions[0].Ratio != 0.31 {
		t.Fatalf("saved ratio = %v, want 0.31", saved.Regions[0].Ratio)
	}
	if saved.Mobile.ActiveRegionID != "agent-chat-primary" {
		t.Fatalf(
			"saved mobile active = %q, want default primary",
			saved.Mobile.ActiveRegionID,
		)
	}

	got, err := service.Get(
		t.Context(),
		"agent@example.com",
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
	)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Page != workbench.WorkbenchPageAgentChat ||
		got.View != workbench.WorkbenchViewFocus {
		t.Fatalf("Get() = %#v", got)
	}
	if got.Regions[0].Visible {
		t.Fatal("Get() should strip durable navigation visibility")
	}
	if got.Regions[0].Ratio != 0.31 {
		t.Fatalf("Get() ratio = %v, want 0.31", got.Regions[0].Ratio)
	}

	if err := service.Reset(
		t.Context(),
		"agent@example.com",
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
	); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	defaulted := service.GetOrDefault(
		t.Context(),
		"agent@example.com",
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
		"",
	)
	if defaulted.Regions[0].Visible {
		t.Fatal("GetOrDefault() should return hidden navigation after reset")
	}
}

func TestHandlerSaveAndReset(t *testing.T) {
	t.Parallel()

	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	handler := NewHandler(NewService(svc.Queries))
	e := echo.New()
	for _, tc := range []struct {
		page workbench.WorkbenchPage
		view workbench.WorkbenchView
	}{
		{workbench.WorkbenchPageAgentChat, workbench.WorkbenchViewFocus},
		{workbench.WorkbenchPageAgentChat, workbench.WorkbenchViewSplit},
		{workbench.WorkbenchPageThoughts, workbench.WorkbenchViewFocus},
		{workbench.WorkbenchPageThoughts, workbench.WorkbenchViewSplit},
	} {
		t.Run(string(tc.page)+"/"+string(tc.view), func(t *testing.T) {
			cfg := workbench.DefaultWorkbenchConfig(tc.page, tc.view, "")
			payload, err := json.Marshal(map[string]any{"config": cfg})
			if err != nil {
				t.Fatalf("Marshal save payload: %v", err)
			}
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/layout-preferences",
				bytes.NewReader(payload),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)
			ctx.Set("user_email", "agent@example.com")
			if err := handler.Save(ctx); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			if rec.Code != http.StatusOK ||
				!strings.Contains(rec.Body.String(), "workbenchSaved") {
				t.Fatalf("Save() status/body = %d/%q", rec.Code, rec.Body.String())
			}

			payload, err = json.Marshal(map[string]any{"page": tc.page, "view": tc.view})
			if err != nil {
				t.Fatalf("Marshal reset payload: %v", err)
			}
			req = httptest.NewRequest(
				http.MethodPost,
				"/api/layout-preferences/reset",
				bytes.NewReader(payload),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec = httptest.NewRecorder()
			ctx = e.NewContext(req, rec)
			ctx.Set("user_email", "agent@example.com")
			if err := handler.Reset(ctx); err != nil {
				t.Fatalf("Reset() error = %v", err)
			}
			if rec.Code != http.StatusOK ||
				!strings.Contains(rec.Body.String(), "workbenchSaved") ||
				!strings.Contains(rec.Body.String(), "window.location.reload();") {
				t.Fatalf("Reset() status/body = %d/%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandlerResetAcceptsFormPayload(t *testing.T) {
	t.Parallel()

	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	handler := NewHandler(NewService(svc.Queries))
	e := echo.New()
	body := strings.NewReader("page=agent-chat&view=focus")
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/layout-preferences/reset",
		body,
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_email", "agent@example.com")
	if err := handler.Reset(ctx); err != nil {
		t.Fatalf("Reset() form error = %v", err)
	}
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), "workbenchSaved") ||
		!strings.Contains(rec.Body.String(), "window.location.reload();") {
		t.Fatalf("Reset() form status/body = %d/%q", rec.Code, rec.Body.String())
	}
}

func TestHandlerRejectsUnauthenticatedAndInvalidPayloads(t *testing.T) {
	t.Parallel()

	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	handler := NewHandler(NewService(svc.Queries))
	e := echo.New()
	cfg := workbench.DefaultWorkbenchConfig(
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
		"",
	)
	payload, err := json.Marshal(map[string]any{"config": cfg})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/layout-preferences",
		bytes.NewReader(payload),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := handler.Save(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Save() unauthenticated error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Save() unauthenticated status = %d", rec.Code)
	}

	cfg.Regions[0].Ratio = -1
	payload, err = json.Marshal(map[string]any{"config": cfg})
	if err != nil {
		t.Fatalf("Marshal invalid payload: %v", err)
	}
	req = httptest.NewRequest(
		http.MethodPost,
		"/api/layout-preferences",
		bytes.NewReader(payload),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_email", "agent@example.com")
	if err := handler.Save(ctx); err != nil {
		t.Fatalf("Save() invalid payload error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Save() invalid payload status = %d", rec.Code)
	}
}

func TestServiceRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	svc, err := dbsvc.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	service := NewService(svc.Queries)
	cfg := workbench.DefaultWorkbenchConfig(
		workbench.WorkbenchPageAgentChat,
		workbench.WorkbenchViewFocus,
		"",
	)
	cfg.Regions[0].Ratio = -1
	if _, err := service.Upsert(t.Context(), Input{
		UserEmail: "agent@example.com",
		Page:      workbench.WorkbenchPageAgentChat,
		View:      workbench.WorkbenchViewFocus,
		Config:    cfg,
	}); err == nil {
		t.Fatal("Upsert() error = nil, want invalid config error")
	}
}
