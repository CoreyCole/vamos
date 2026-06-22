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

func TestPreviewRendersFilesAndLiveAppOnly(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, PreviewCard(PickleballViewModel{SessionID: "player"}))
	for _, want := range []string{
		`Files and app`,
		`players.csv`,
		`matchups.csv`,
		`tournament.html`,
		`src="/examples/pickleball/app/"`,
		`Open app`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview missing %q:\n%s", want, body)
		}
	}
	assertNoTechnicalPickleballTerms(t, body)
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
	badCtx := e.NewContext(bad, badRec)
	badCtx.Set("user_email", "player@example.com")
	if err := svc.HandleSubmitPrompt(badCtx); err == nil {
		t.Fatal("HandleSubmitPrompt empty prompt expected error")
	}

	form := url.Values{"session_id": {session.ID}, "prompt": {"Add skill totals"}}
	req := httptest.NewRequest(http.MethodPost, "/examples/pickleball/prompts", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	reqCtx := e.NewContext(req, rec)
	reqCtx.Set("user_email", "player@example.com")
	if err := svc.HandleSubmitPrompt(reqCtx); err != nil {
		t.Fatalf("HandleSubmitPrompt: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlersRejectMismatchedSession(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, &fakeWorkflowStarter{}, nil)
	other, err := svc.EnsureSession(context.Background(), "other@example.com")
	if err != nil {
		t.Fatalf("EnsureSession other: %v", err)
	}
	e := echo.New()
	form := url.Values{"session_id": {other.ID}, "prompt": {"Add skill totals"}}
	req := httptest.NewRequest(http.MethodPost, "/examples/pickleball/prompts", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("user_email", "player@example.com")
	err = svc.HandleSubmitPrompt(ctx)
	if err == nil {
		t.Fatal("HandleSubmitPrompt mismatched session expected error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusForbidden {
		t.Fatalf("error = %#v", err)
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
	streamCtx := e.NewContext(req, rec)
	streamCtx.Set("user_email", "player@example.com")
	done := make(chan error, 1)
	go func() { done <- svc.HandleStateStream(streamCtx) }()
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

func TestNormalPageHidesTechnicalMechanics(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, Page(PickleballViewModel{
		SessionID:    "player",
		State:        AppStateFailed,
		UserMessage:  "I couldn't make that change safely. Your app is unchanged.",
		ErrorMessage: "exec failed: /tmp/app/main.go: panic stack trace",
		LogTail:      "raw hidden diagnostics",
	}))
	for _, want := range []string{"Files", "Chat", "pickleball", `id="pickleball-prompt-form"`, "Your app is unchanged"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q:\n%s", want, body)
		}
	}
	assertNoTechnicalPickleballTerms(t, body)
}

func assertNoTechnicalPickleballTerms(t *testing.T, body string) {
	t.Helper()
	if idx := strings.Index(body, `id="pickleball-app"`); idx >= 0 {
		body = body[idx:]
	}
	lower := strings.ToLower(body)
	for _, forbidden := range []string{
		"workspace",
		"build status",
		"run id",
		"manifest",
		"promotion",
		"iteration",
		"filesystem",
		"stack trace",
		"restore source",
		"snapshot",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("normal UI leaked %q:\n%s", forbidden, body)
		}
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
