package pickleball

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func TestPageRendersMobileShell(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, nil, nil)
	session, err := svc.EnsureSession(context.Background(), "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	vm, err := svc.GetState(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	body := renderComponent(t, Page(vm))
	for _, want := range []string{
		`id="pickleball-app"`,
		`id="pickleball-state"`,
		`id="pickleball-prompt-form"`,
		`name="prompt"`,
		`name="session_id"`,
		`/examples/pickleball/state?session=player-example-com`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q:\n%s", want, body)
		}
	}
}

func TestPromptHandlerValidationAndAccepted(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, &fakeWorkflowStarter{}, nil)
	session, err := svc.EnsureSession(context.Background(), "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	e := echo.New()

	bad := httptest.NewRequest(http.MethodPost, "/examples/pickleball/prompts", strings.NewReader(url.Values{"session_id": {session.ID}}.Encode()))
	bad.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	badRec := httptest.NewRecorder()
	if err := svc.HandleSubmitPrompt(e.NewContext(bad, badRec)); err == nil {
		t.Fatal("HandleSubmitPrompt empty prompt expected error")
	}

	form := url.Values{"session_id": {session.ID}, "prompt": {"Add skill totals"}}
	req := httptest.NewRequest(http.MethodPost, "/examples/pickleball/prompts", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	if err := svc.HandleSubmitPrompt(e.NewContext(req, rec)); err != nil {
		t.Fatalf("HandleSubmitPrompt: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestStateStreamInitialPatch(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, nil, nil)
	session, err := svc.EnsureSession(context.Background(), "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/examples/pickleball/state?session="+session.ID, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() { done <- svc.HandleStateStream(e.NewContext(req, rec)) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleStateStream: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("state stream did not stop")
	}
	if !strings.Contains(rec.Body.String(), "pickleball-state") {
		t.Fatalf("stream body missing state patch:\n%s", rec.Body.String())
	}
}

func renderComponent(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}
