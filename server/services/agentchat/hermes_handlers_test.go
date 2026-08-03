package agentchat

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHermesRouteRegistrationSeparatesBrowserSessionAndMachineCallbacks(t *testing.T) {
	e := echo.New()
	handler := NewHandler(&Service{}, nil)
	browser := e.Group("/agent-chat")
	handler.RegisterRuntimeRoutes(browser)
	machine := e.Group("/agent-chat/api")
	handler.RegisterMachineAPIRoutes(machine)
	routes := map[string]bool{}
	for _, route := range e.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"POST /agent-chat/hermes/threads",
		"POST /agent-chat/hermes/threads/:thread_id/prompts",
		"POST /agent-chat/api/hermes/threads/:thread_id/events",
		"POST /agent-chat/api/hermes/pi/:session_id/complete",
	} {
		if !routes[want] {
			t.Fatalf("missing route %q: %#v", want, routes)
		}
	}
	if routes["POST /agent-chat/api/hermes/threads/:thread_id/prompts"] ||
		routes["POST /agent-chat/hermes/threads/:thread_id/events"] {
		t.Fatalf("browser and machine Hermes middleware boundaries overlap: %#v", routes)
	}
}

func TestHermesMachineCallbacksRequireMachineTokenWhileBrowserWritesUseSessionIdentity(t *testing.T) {
	service, _, prompt := newHermesPromptFixture(t)
	handler := NewHandler(service, nil, HandlerOptions{HermesCallbackToken: "machine-secret"})
	e := echo.New()

	machineReq := httptest.NewRequest(http.MethodPost, "/agent-chat/api/hermes/threads/thread_1/events", strings.NewReader(`{"plan_dir":"owner/plans/alpha","id":"callback_1","type":"final","content":"done"}`))
	machineReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	machineCtx := e.NewContext(machineReq, httptest.NewRecorder())
	machineCtx.SetParamNames("thread_id")
	machineCtx.SetParamValues(prompt.ThreadID)
	if err := handler.HandleHermesEvent(machineCtx); err == nil || err.(*echo.HTTPError).Code != http.StatusUnauthorized {
		t.Fatalf("machine callback without token = %#v", err)
	}

	browserValues := url.Values{"plan_dir": {prompt.PlanDir}, "command_id": {prompt.CommandID}, "prompt": {prompt.Prompt}}
	browserReq := httptest.NewRequest(http.MethodPost, "/agent-chat/hermes/threads/thread_1/prompts", strings.NewReader(browserValues.Encode()))
	browserReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	browserCtx := e.NewContext(browserReq, httptest.NewRecorder())
	browserCtx.SetParamNames("thread_id")
	browserCtx.SetParamValues(prompt.ThreadID)
	browserCtx.Set("user_email", "owner@example.com")
	service.hermesGateway = &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
	if err := handler.HandleHermesPrompt(browserCtx); err != nil {
		t.Fatalf("browser write required machine token: %v", err)
	}
}

func TestHandleCreateHermesThreadUsesAuthenticatedContainedPlan(t *testing.T) {
	service, _, _ := newHermesPromptFixture(t)
	handler := NewHandler(service, nil)
	e := echo.New()
	values := url.Values{
		"plan_dir":  {"owner/plans/alpha"},
		"title":     {"Created from form"},
		"return_to": {"/thoughts/owner/plans/alpha/design.md?context=threads&thread=chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/agent-chat/hermes/threads", strings.NewReader(values.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(req, recorder)
	ctx.Set("user_email", "creator@example.com")
	if err := handler.HandleCreateHermesThread(ctx); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusSeeOther || !strings.Contains(recorder.Header().Get("Location"), "hermes_thread=") ||
		!strings.Contains(recorder.Header().Get("Location"), "thread=chat") {
		t.Fatalf("create response = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	threads, err := service.ListHermesThreads(t.Context(), ThreadQuery{PlanDir: "owner/plans/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 || threads[0].Title != "Created from form" {
		t.Fatalf("threads = %#v", threads)
	}
}

func TestHandleHermesPromptMapsDeliveryOutcomesAndUsesRealFormFields(t *testing.T) {
	for _, test := range []struct {
		name       string
		gateway    HermesGatewayClient
		wantStatus int
	}{
		{name: "accepted", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}, wantStatus: http.StatusSeeOther},
		{name: "rejected", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptRejected}}, wantStatus: http.StatusUnprocessableEntity},
		{name: "failed", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptFailed}}, wantStatus: http.StatusFailedDependency},
		{name: "failed detail cannot imply unavailable", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptFailed, Detail: "dependency not configured by upstream"}}, wantStatus: http.StatusFailedDependency},
		{name: "uncertain", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptUncertain}}, wantStatus: http.StatusGatewayTimeout},
		{name: "unavailable", gateway: nil, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, prompt := newHermesPromptFixture(t)
			service.hermesGateway = test.gateway
			handler := NewHandler(service, nil)
			e := echo.New()
			values := url.Values{
				"plan_dir":      {prompt.PlanDir},
				"command_id":    {prompt.CommandID},
				"prompt":        {prompt.Prompt},
				"context_paths": {prompt.ContextPaths[0]},
				"return_to":     {"/thoughts/owner/plans/alpha/design.md?context=threads&thread=chat&hermes_thread=thread_1"},
			}
			req := httptest.NewRequest(http.MethodPost, "/agent-chat/hermes/threads/thread_1/prompts", strings.NewReader(values.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			recorder := httptest.NewRecorder()
			ctx := e.NewContext(req, recorder)
			ctx.SetPath("/agent-chat/hermes/threads/:thread_id/prompts")
			ctx.SetParamNames("thread_id")
			ctx.SetParamValues(prompt.ThreadID)
			ctx.Set("user_email", "owner@example.com")
			err := handler.HandleHermesPrompt(ctx)
			if test.wantStatus == http.StatusSeeOther {
				if err != nil || recorder.Code != test.wantStatus {
					t.Fatalf("response/error = %d/%v", recorder.Code, err)
				}
				if location := recorder.Header().Get("Location"); !strings.Contains(location, "thread=chat") || !strings.Contains(location, "hermes_thread=thread_1") {
					t.Fatalf("redirect = %q", location)
				}
				return
			}
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != test.wantStatus {
				t.Fatalf("error = %#v, want HTTP %d", err, test.wantStatus)
			}
		})
	}
}

func TestHandleHermesPromptDistinguishesAuthenticationAuthorizationConflictAndInProgress(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
	service.hermesGateway = gateway
	handler := NewHandler(service, nil)
	invoke := func(user, command, body string) error {
		e := echo.New()
		values := url.Values{"plan_dir": {prompt.PlanDir}, "command_id": {command}, "prompt": {body}}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		ctx := e.NewContext(req, httptest.NewRecorder())
		ctx.SetParamNames("thread_id")
		ctx.SetParamValues(prompt.ThreadID)
		if user != "" {
			ctx.Set("user_email", user)
		}
		return handler.HandleHermesPrompt(ctx)
	}
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "unauthenticated", err: invoke("", "auth_command", "prompt"), status: http.StatusUnauthorized},
		{name: "unauthorized", err: invoke("reader@example.com", "denied_command", "prompt"), status: http.StatusForbidden},
	} {
		httpErr, ok := test.err.(*echo.HTTPError)
		if !ok || httpErr.Code != test.status {
			t.Fatalf("%s error = %#v", test.name, test.err)
		}
	}
	lock, err := tryAcquireHermesCommandLock(t.Context(), planDir, prompt.ThreadID, "locked_command")
	if err != nil {
		t.Fatal(err)
	}
	err = invoke("owner@example.com", "locked_command", "prompt")
	_ = lock.Close()
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusLocked {
		t.Fatalf("in-progress error = %#v", err)
	}
	prompt.CommandID = "conflict_command"
	if _, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt); err != nil {
		t.Fatal(err)
	}
	err = invoke("owner@example.com", prompt.CommandID, "different")
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusConflict {
		t.Fatalf("conflict error = %#v", err)
	}
}
