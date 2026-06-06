package chatcmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPAPIClientWireContract(t *testing.T) {
	var steerCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer vamos_machine_key-1.secret-1"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}

		switch r.URL.Path {
		case "/agent-chat/api/runs":
			if r.Method != http.MethodPost {
				t.Fatalf("start method = %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("start Content-Type = %q", got)
			}
			var req ChatStartRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode start request: %v", err)
			}
			if req.ProjectID != "github.com/coreycole/vamos" || req.Prompt != "hello" {
				t.Fatalf("start request = %+v", req)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"started","ref":{"project_id":"github.com/coreycole/vamos","workspace_id":"ws-1","thread_id":"thread-1","run_id":"run-1","chat_session_id":"session-1","web_url":"https://main.test/thoughts/?context=chat&thread=thread-1&run=run-1","event_after":4}}`))

		case "/agent-chat/api/chat-sessions/session-1/events":
			if r.Method != http.MethodGet {
				t.Fatalf("events method = %s", r.Method)
			}
			if got := r.URL.Query().Get("after"); got != "4" {
				t.Fatalf("events after = %q", got)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: chat-session-event\nid: 5\ndata: {\"EventType\":\"run.completed\",\"Seq\":5,\"RunID\":\"run-1\"}\n\n"))

		case "/agent-chat/api/steer":
			if r.Method != http.MethodPost {
				t.Fatalf("steer method = %s", r.Method)
			}
			var req ChatSteerRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode steer request: %v", err)
			}
			if req.ThreadID != "thread-1" || req.Prompt != "follow up" {
				t.Fatalf("steer request = %+v", req)
			}
			steerCalls++
			if steerCalls == 1 {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"type":"steer_accepted","ref":{"thread_id":"thread-1","run_id":"run-2","chat_session_id":"session-1"},"influences_latest":true}`))
				return
			}
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"type":"steer_rejected","reason":"run_in_progress","ref":{"thread_id":"thread-1","run_id":"run-active"},"latest_thread_id":"thread-1","latest_web_url":"https://main.test/thoughts/?context=chat&thread=thread-1","influences_latest":true}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(server.Close)

	client := HTTPAPIClient{HTTPClient: server.Client(), ManagerURL: server.URL}
	ctx := context.Background()

	started, err := client.Start(ctx, "key-1", "secret-1", ChatStartRequest{ProjectID: "github.com/coreycole/vamos", Prompt: "hello"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Type != "started" || started.Ref.ChatSessionID != "session-1" || started.Ref.EventAfter != 4 {
		t.Fatalf("started = %+v", started)
	}

	resp, err := client.Events(ctx, "key-1", "secret-1", started.Ref.ChatSessionID, started.Ref.EventAfter)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var seen StreamEvent
	if err := ReadSSEEvents(ctx, resp.Body, func(event StreamEvent) error {
		seen = event
		return errTerminalSeen
	}); !errorsIsTerminalSeen(err) {
		t.Fatalf("ReadSSEEvents() error = %v", err)
	}
	if seen.ID != 5 || seen.ChatEvent.RunID != "run-1" {
		t.Fatalf("seen event = %+v", seen)
	}

	accepted, err := client.Steer(ctx, "key-1", "secret-1", ChatSteerRequest{ThreadID: "thread-1", Prompt: "follow up"})
	if err != nil {
		t.Fatalf("Steer accepted error = %v", err)
	}
	if accepted.Type != "steer_accepted" || accepted.Ref.RunID != "run-2" || !accepted.InfluencesLatest {
		t.Fatalf("accepted = %+v", accepted)
	}

	rejected, err := client.Steer(ctx, "key-1", "secret-1", ChatSteerRequest{ThreadID: "thread-1", Prompt: "follow up"})
	if err != nil {
		t.Fatalf("Steer rejected error = %v", err)
	}
	if rejected.Type != "steer_rejected" || rejected.Reason != "run_in_progress" || rejected.Ref.RunID != "run-active" || rejected.LatestThreadID != "thread-1" {
		t.Fatalf("rejected = %+v", rejected)
	}
}
