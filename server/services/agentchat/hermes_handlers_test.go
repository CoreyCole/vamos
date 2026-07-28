package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

type recordingHermesGateway struct {
	prompt        HermesPrompt
	session       string
	result        []byte
	completionErr error
}

func (g *recordingHermesGateway) DeliverPrompt(
	_ context.Context,
	prompt HermesPrompt,
) error {
	g.prompt = prompt
	return nil
}

func (g *recordingHermesGateway) DeliverPiCompletion(
	_ context.Context,
	session string,
	result []byte,
) error {
	g.session, g.result = session, append([]byte(nil), result...)
	return g.completionErr
}

func TestHandleHermesPromptOnlyDeliversForThreadOwner(t *testing.T) {
	gateway := &recordingHermesGateway{}
	service := &Service{
		hermesGateway: gateway,
		hermesThreadOwner: func(context.Context, string) (string, error) {
			return "owner@example.com", nil
		},
	}
	e := echo.New()
	h := NewHandler(service, nil)
	for _, user := range []string{"observer@example.com", "owner@example.com"} {
		req := httptest.NewRequest(
			http.MethodPost,
			"/",
			bytes.NewBufferString(
				"prompt=hello&plan_dir=%2Fplans%2Fa&context_paths=a.md%2Cb.md",
			),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_email", user)
		c.SetParamNames("thread_id")
		c.SetParamValues("thread-1")
		err := h.HandleHermesPrompt(c)
		if user == "observer@example.com" {
			if httpErr, ok := err.(*echo.HTTPError); !ok ||
				httpErr.Code != http.StatusForbidden {
				t.Fatalf("observer result = %#v", err)
			}
			continue
		}
		if err != nil || rec.Code != http.StatusAccepted {
			t.Fatalf("owner result err=%v status=%d", err, rec.Code)
		}
	}
	if gateway.prompt.OwnerEmail != "owner@example.com" ||
		len(gateway.prompt.ContextPaths) != 2 {
		t.Fatalf("gateway prompt = %#v", gateway.prompt)
	}
}

func TestHandleHermesEventRequiresCallbackCredential(t *testing.T) {
	e := echo.New()
	h := NewHandler(
		&Service{thoughtsRoot: t.TempDir()},
		nil,
		HandlerOptions{HermesCallbackToken: "secret"},
	)
	req := httptest.NewRequest(http.MethodPost, "/hermes/threads/thread-1/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("thread_id")
	c.SetParamValues("thread-1")

	err := h.HandleHermesEvent(c)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("HandleHermesEvent() error = %#v, want unauthorized", err)
	}
}

func TestHandleHermesEventAppendsContainedTranscriptInArrivalOrder(t *testing.T) {
	thoughts := t.TempDir()
	plan := filepath.Join(thoughts, "agent", "plans", "plan-a")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	h := NewHandler(
		&Service{thoughtsRoot: thoughts},
		nil,
		HandlerOptions{HermesCallbackToken: "secret"},
	)
	for _, event := range []struct{ ID, Content string }{{"one", "first"}, {"two", "second"}} {
		body, err := json.Marshal(
			map[string]string{
				"plan_dir": plan,
				"id":       event.ID,
				"type":     "final",
				"content":  event.Content,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("thread_id")
		c.SetParamValues("thread-1")
		if err := h.HandleHermesEvent(c); err != nil || rec.Code != http.StatusAccepted {
			t.Fatalf("HandleHermesEvent() err=%v status=%d", err, rec.Code)
		}
	}
	events, err := readHermesTranscript(plan, "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Content != "first" || events[1].Content != "second" {
		t.Fatalf("events = %#v, want arrival order", events)
	}
}

func TestHandleHermesPiCompletionReturnsNotFoundWithoutManager(t *testing.T) {
	thoughts := t.TempDir()
	plan := filepath.Join(thoughts, "agent", "plans", "plan-a")
	path := filepath.Join(plan, ".vamos", "sessions", "pi", "session-1_result.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("summary: current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	h := NewHandler(
		&Service{
			thoughtsRoot: thoughts,
			hermesGateway: &recordingHermesGateway{
				completionErr: ErrHermesManagerNotFound,
			},
		},
		nil,
		HandlerOptions{HermesCallbackToken: "secret"},
	)
	body, _ := json.Marshal(map[string]string{"plan_dir": plan})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("session_id")
	c.SetParamValues("session-1")

	err := h.HandleHermesPiCompletion(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("HandleHermesPiCompletion() error = %#v, want not found", err)
	}
}

func TestHandleHermesPiCompletionRereadsContainedResult(t *testing.T) {
	thoughts := t.TempDir()
	plan := filepath.Join(thoughts, "agent", "plans", "plan-a")
	path := filepath.Join(plan, ".vamos", "sessions", "pi", "session-1_result.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("summary: current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gateway := &recordingHermesGateway{}
	e := echo.New()
	h := NewHandler(
		&Service{thoughtsRoot: thoughts, hermesGateway: gateway},
		nil,
		HandlerOptions{HermesCallbackToken: "secret"},
	)
	body, _ := json.Marshal(map[string]string{"plan_dir": plan})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("session-1")

	if err := h.HandleHermesPiCompletion(
		c,
	); err != nil ||
		rec.Code != http.StatusAccepted {
		t.Fatalf("HandleHermesPiCompletion() err=%v status=%d", err, rec.Code)
	}
	if gateway.session != "session-1" || string(gateway.result) != "summary: current\n" {
		t.Fatalf(
			"gateway completion = session %q result %q",
			gateway.session,
			gateway.result,
		)
	}
}
