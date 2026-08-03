package agentchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func newHermesStreamContext(
	t *testing.T, handler *Handler, user, plan, thread string, ctx context.Context,
) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	target := "/agent-chat/hermes/threads/" + thread + "/transcript?plan_dir=" + url.QueryEscape(plan)
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	c := echo.New().NewContext(req, recorder)
	c.SetPath("/agent-chat/hermes/threads/:thread_id/transcript")
	c.SetParamNames("thread_id")
	c.SetParamValues(thread)
	if user != "" {
		c.Set("user_email", user)
	}
	return c, recorder
}

func TestHandleHermesTranscriptStreamRequiresAuthAndContainedCanonicalIdentity(t *testing.T) {
	service, alphaDir, prompt := newHermesPromptFixture(t)
	handler := NewHandler(service, nil)

	for _, test := range []struct {
		name   string
		user   string
		plan   string
		thread string
		want   int
	}{
		{name: "authentication", plan: prompt.PlanDir, thread: prompt.ThreadID, want: http.StatusUnauthorized},
		{name: "canonical plan", user: "reader@example.com", plan: "thoughts/owner/plans/alpha", thread: prompt.ThreadID, want: http.StatusBadRequest},
		{name: "bounded thread", user: "reader@example.com", plan: prompt.PlanDir, thread: "bad/thread", want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := newHermesStreamContext(t, handler, test.user, test.plan, test.thread, t.Context())
			err := handler.HandleHermesTranscriptStream(c)
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != test.want {
				t.Fatalf("error = %#v, want HTTP %d", err, test.want)
			}
		})
	}

	betaIdentity := HermesPlanIdentity("owner/plans/beta")
	betaDir := filepath.Join(service.thoughtsRoot, filepath.FromSlash(string(betaIdentity)))
	if err := os.MkdirAll(betaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(betaDir, hermesMetadataFixture(betaIdentity, prompt.ThreadID)); err != nil {
		t.Fatal(err)
	}
	alphaPath, err := HermesTranscriptPath(alphaDir, prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	betaPath, err := HermesTranscriptPath(betaDir, prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	alphaBytes, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(betaPath, alphaBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	c, _ := newHermesStreamContext(t, handler, "reader@example.com", string(betaIdentity), prompt.ThreadID, t.Context())
	if err := handler.HandleHermesTranscriptStream(c); err == nil || err.(*echo.HTTPError).Code != http.StatusBadRequest {
		t.Fatalf("cross-plan error = %#v", err)
	}
}

func TestHandleHermesTranscriptStreamRereadsDiskInOrderAndStopsOnCancellation(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	handler := NewHandler(service, nil)
	oldInterval := hermesTranscriptRefreshInterval
	oldHook := hermesTranscriptStreamReadHook
	hermesTranscriptRefreshInterval = time.Millisecond
	renderedSnapshots := make(chan string, 16)
	var recorder *httptest.ResponseRecorder
	hermesTranscriptStreamReadHook = func() {
		snapshot := recorder.Body.String()
		select {
		case renderedSnapshots <- snapshot:
		default:
		}
	}
	t.Cleanup(func() {
		hermesTranscriptRefreshInterval = oldInterval
		hermesTranscriptStreamReadHook = oldHook
	})

	ctx, cancel := context.WithCancel(t.Context())
	c, streamRecorder := newHermesStreamContext(t, handler, "reader@example.com", prompt.PlanDir, prompt.ThreadID, ctx)
	recorder = streamRecorder
	done := make(chan error, 1)
	go func() { done <- handler.HandleHermesTranscriptStream(c) }()
	select {
	case <-renderedSnapshots:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	for _, event := range []HermesTranscriptEvent{
		{ID: "prompt_result", Type: "prompt_delivery", ThreadID: prompt.ThreadID, PlanDir: HermesPlanIdentity(prompt.PlanDir), DeliveryStatus: string(HermesPromptAccepted)},
		{ID: "settlement_visible", Type: "settlement_delivering", ThreadID: prompt.ThreadID, PlanDir: HermesPlanIdentity(prompt.PlanDir), Content: "opaque wrapped manager turn"},
	} {
		if err := AppendHermesTranscript(planDir, event); err != nil {
			t.Fatal(err)
		}
	}
	for {
		select {
		case snapshot := <-renderedSnapshots:
			patches := strings.Split(snapshot, "event: datastar-patch-elements")
			latest := patches[len(patches)-1]
			if strings.Contains(latest, "prompt_result") && strings.Contains(latest, "settlement_visible") {
				cancel()
				goto renderedBothEvents
			}
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}

renderedBothEvents:
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	body := recorder.Body.String()
	statusIndex := strings.LastIndex(body, "Prompt accepted by Hermes; execution is not confirmed.")
	settlementIndex := strings.LastIndex(body, "opaque wrapped manager turn")
	if statusIndex < 0 || settlementIndex < statusIndex {
		t.Fatalf("stream did not preserve append order:\n%s", body)
	}
	if !strings.Contains(body, `id="hermes-thread-transcript-region"`) ||
		!strings.Contains(body, `id="hermes-thread-delivery-status"`) ||
		!strings.Contains(body, `id="hermes-thread-messages"`) {
		t.Fatalf("stream lacks stable targets: %s", body)
	}
	patches := strings.Split(body, "event: datastar-patch-elements")
	latest := patches[len(patches)-1]
	for _, marker := range []string{"prompt_result", "settlement_visible", "opaque wrapped manager turn"} {
		if strings.Count(latest, marker) != 1 {
			t.Fatalf("latest morph contains %q %d times: %s", marker, strings.Count(latest, marker), latest)
		}
	}
}

func TestHermesMalformedAndCrossPlanCallbacksAppendNothing(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	handler := NewHandler(service, nil, HandlerOptions{HermesCallbackToken: "machine-secret"})
	path, err := HermesTranscriptPath(planDir, prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"plan_dir":"owner/plans/alpha","id":"bad/id","type":"final","content":"malformed"}`,
		`{"plan_dir":"owner/plans/beta","id":"cross_plan","type":"final","content":"crossed"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/agent-chat/api/hermes/threads/thread_1/events", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("Authorization", "Bearer machine-secret")
		c := echo.New().NewContext(req, httptest.NewRecorder())
		c.SetParamNames("thread_id")
		c.SetParamValues(prompt.ThreadID)
		if err := handler.HandleHermesEvent(c); err == nil {
			t.Fatalf("callback %s unexpectedly succeeded", body)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("malformed or cross-plan callback changed the durable transcript")
	}
}

func TestHandleHermesTranscriptStreamNeverRendersPartialAppend(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	handler := NewHandler(service, nil)
	oldWrite := writeHermesTranscriptPayload
	oldContention := hermesTranscriptLocalContentionHook
	oldHook := hermesTranscriptStreamReadHook
	enteredWrite := make(chan struct{})
	releaseWrite := make(chan struct{})
	contended := make(chan struct{}, 1)
	read := make(chan struct{}, 1)
	writeHermesTranscriptPayload = func(file *os.File, payload []byte) (int, error) {
		close(enteredWrite)
		<-releaseWrite
		return file.Write(payload)
	}
	hermesTranscriptLocalContentionHook = func(bool) {
		select {
		case contended <- struct{}{}:
		default:
		}
	}
	hermesTranscriptStreamReadHook = func() {
		select {
		case read <- struct{}{}:
		default:
		}
	}
	t.Cleanup(func() {
		writeHermesTranscriptPayload = oldWrite
		hermesTranscriptLocalContentionHook = oldContention
		hermesTranscriptStreamReadHook = oldHook
	})

	appendDone := make(chan error, 1)
	go func() {
		appendDone <- AppendHermesTranscript(planDir, HermesTranscriptEvent{
			ID: "complete_event", Type: "settlement_delivering", ThreadID: prompt.ThreadID,
			PlanDir: HermesPlanIdentity(prompt.PlanDir), Content: "complete opaque payload",
		})
	}()
	<-enteredWrite

	ctx, cancel := context.WithCancel(t.Context())
	c, recorder := newHermesStreamContext(t, handler, "reader@example.com", prompt.PlanDir, prompt.ThreadID, ctx)
	streamDone := make(chan error, 1)
	go func() { streamDone <- handler.HandleHermesTranscriptStream(c) }()
	<-contended
	select {
	case <-read:
		t.Fatal("stream read passed an in-progress append")
	default:
	}
	close(releaseWrite)
	if err := <-appendDone; err != nil {
		t.Fatal(err)
	}
	<-read
	cancel()
	if err := <-streamDone; err != nil {
		t.Fatal(err)
	}
	body := recorder.Body.String()
	if strings.Count(body, "complete opaque payload") != 1 {
		t.Fatalf("stream rendered partial or duplicate initial event: %s", body)
	}
}
