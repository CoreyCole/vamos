package chatcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/authcmd"
	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

type fakeStore struct{}

func (fakeStore) Save(string, authcmd.Profile, string) error { return nil }
func (fakeStore) Load(string) (authcmd.Profile, string, error) {
	return authcmd.Profile{ManagerURL: "https://main.workspaces.test", KeyID: "machine-1"}, "secret-1", nil
}

type fakeAPI struct {
	startReq      ChatStartRequest
	steerReq      ChatSteerRequest
	steerResponse ChatAPIResponse
	eventAfters   []int64
	streams       []string
}

func (f *fakeAPI) Start(_ context.Context, keyID, secret string, req ChatStartRequest) (ChatAPIResponse, error) {
	if keyID != "machine-1" || secret != "secret-1" {
		return ChatAPIResponse{}, context.Canceled
	}
	f.startReq = req
	return ChatAPIResponse{Type: "started", Ref: ChatRunRef{ProjectID: req.ProjectID, WorkspaceID: "ws-1", ThreadID: "thread-1", RunID: "run-1", ChatSessionID: "session-1", WebURL: "https://main.test/thoughts/?context=chat&thread=thread-1&run=run-1", CWD: "/repo", EventAfter: 0}}, nil
}

func (f *fakeAPI) Steer(_ context.Context, keyID, secret string, req ChatSteerRequest) (ChatAPIResponse, error) {
	if keyID != "machine-1" || secret != "secret-1" {
		return ChatAPIResponse{}, context.Canceled
	}
	f.steerReq = req
	if f.steerResponse.Type != "" {
		return f.steerResponse, nil
	}
	return ChatAPIResponse{Type: "steer_rejected", Ref: ChatRunRef{ThreadID: req.ThreadID}, Reason: "run_in_progress"}, nil
}

func (f *fakeAPI) Snapshot(context.Context, string, string, string) (snapshotResponse, error) {
	return snapshotResponse{Projection: chatsession.ChatProjection{Messages: []chatsession.ProjectedMessage{{Role: "assistant", Content: "assistant done", RunID: "run-1"}}}, LastSeq: 3}, nil
}

func (f *fakeAPI) Events(_ context.Context, _ string, _ string, _ string, after int64) (*http.Response, error) {
	f.eventAfters = append(f.eventAfters, after)
	body := "event: chat-session-event\nid: 1\ndata: {\"EventType\":\"run.progress\",\"Seq\":1,\"RunID\":\"run-1\"}\n\n" +
		"event: chat-session-event\nid: 2\ndata: {\"EventType\":\"run.completed\",\"Seq\":2,\"RunID\":\"run-1\"}\n\n"
	if len(f.streams) > 0 {
		body = f.streams[0]
		f.streams = f.streams[1:]
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestCommandExposesExplicitStartAndSteer(t *testing.T) {
	cmd := newCommand(deps{})
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := help.String()
	for _, want := range []string{"start", "steer"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}

	cmd = newCommand(deps{})
	cmd.SetArgs([]string{"hello"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "explicit subcommand") {
		t.Fatalf("top-level prompt err = %v", err)
	}
}

func TestRunStartPrintsStartedEventAndTerminalResult(t *testing.T) {
	api := &fakeAPI{}
	var out bytes.Buffer
	err := RunStart(context.Background(), StartOptions{ManagerURL: "https://main.workspaces.test", ProjectID: "github.com/coreycole/vamos", Prompt: "continue plan", Profile: "default"}, deps{
		Store: fakeStore{},
		APIClientNew: func(managerURL string) APIClient {
			if managerURL != "https://main.workspaces.test" {
				t.Fatalf("managerURL = %q", managerURL)
			}
			return api
		},
	}, &out)
	if err != nil {
		t.Fatalf("RunStart() error = %v", err)
	}
	if api.startReq.ProjectID != "github.com/coreycole/vamos" || api.startReq.Prompt != "continue plan" {
		t.Fatalf("start request = %+v", api.startReq)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d: %q", len(lines), out.String())
	}
	var first ChatNDJSONEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Type != "started" || first.Ref.RunID != "run-1" {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	var last ChatNDJSONEvent
	if err := json.Unmarshal([]byte(lines[3]), &last); err != nil || last.Type != "result" || last.Status != "complete" || last.Response != "assistant done" {
		t.Fatalf("last = %+v err=%v", last, err)
	}
}

func TestStreamRunNDJSONReconnectsWithLastSeq(t *testing.T) {
	api := &fakeAPI{streams: []string{
		"event: chat-session-event\nid: 7\ndata: {\"EventType\":\"run.progress\",\"Seq\":7,\"RunID\":\"run-1\"}\n\n",
		"event: chat-session-event\nid: 8\ndata: {\"EventType\":\"run.completed\",\"Seq\":8,\"RunID\":\"run-1\"}\n\n",
	}}
	var out bytes.Buffer
	err := StreamRunNDJSON(context.Background(), api, "machine-1", "secret-1", ChatRunRef{RunID: "run-1", ChatSessionID: "session-1", EventAfter: 5}, &out)
	if err != nil {
		t.Fatalf("StreamRunNDJSON() error = %v", err)
	}
	if got, want := api.eventAfters, []int64{5, 7}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("event afters = %v want %v", got, want)
	}
}

func TestRunSteerPrintsAcceptedAndStreamsResult(t *testing.T) {
	api := &fakeAPI{steerResponse: ChatAPIResponse{Type: "steer_accepted", Ref: ChatRunRef{WorkspaceID: "ws-1", ThreadID: "thread-1", RunID: "run-1", ChatSessionID: "session-1", WebURL: "https://main.test/thoughts/?thread=thread-1&run=run-1"}, InfluencesLatest: true}}
	var out bytes.Buffer
	err := RunSteer(context.Background(), SteerOptions{ManagerURL: "https://main.workspaces.test", ThreadID: "thread-1", Prompt: "recover", Profile: "default"}, deps{
		Store:        fakeStore{},
		APIClientNew: func(string) APIClient { return api },
	}, &out)
	if err != nil {
		t.Fatalf("RunSteer() error = %v", err)
	}
	if api.steerReq.ThreadID != "thread-1" || api.steerReq.Prompt != "recover" {
		t.Fatalf("steer request = %+v", api.steerReq)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d: %q", len(lines), out.String())
	}
	var first ChatNDJSONEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Type != "steer_accepted" || !first.InfluencesLatest {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	var last ChatNDJSONEvent
	if err := json.Unmarshal([]byte(lines[3]), &last); err != nil || last.Type != "result" || last.Status != "complete" {
		t.Fatalf("last = %+v err=%v", last, err)
	}
}

func TestRunSteerRejectedPrintsOneLineWithoutStreaming(t *testing.T) {
	api := &fakeAPI{steerResponse: ChatAPIResponse{Type: "steer_rejected", Ref: ChatRunRef{ThreadID: "thread-1", RunID: "run-active"}, Reason: "run_in_progress", LatestThreadID: "thread-1", InfluencesLatest: true}}
	var out bytes.Buffer
	err := RunSteer(context.Background(), SteerOptions{ManagerURL: "https://main.workspaces.test", ThreadID: "thread-1", Prompt: "recover", Profile: "default"}, deps{
		Store:        fakeStore{},
		APIClientNew: func(string) APIClient { return api },
	}, &out)
	if err != nil {
		t.Fatalf("RunSteer() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 || len(api.eventAfters) != 0 {
		t.Fatalf("lines=%d eventAfters=%v out=%q", len(lines), api.eventAfters, out.String())
	}
	var first ChatNDJSONEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Type != "steer_rejected" || first.Ref.RunID != "run-active" || first.LatestThreadID != "thread-1" {
		t.Fatalf("first = %+v err=%v", first, err)
	}
}

func TestRunStartRejectsBlankProjectAndPrompt(t *testing.T) {
	if err := RunStart(context.Background(), StartOptions{Prompt: "x"}, deps{}, io.Discard); err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("blank project err = %v", err)
	}
	if err := RunStart(context.Background(), StartOptions{ProjectID: "p"}, deps{}, io.Discard); err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("blank prompt err = %v", err)
	}
}

func TestCompletionFromSSEHandlesRunTerminalEvents(t *testing.T) {
	completed, done, err := completionFromSSE("chat-session-event", `{"EventType":"run.completed","RunID":"run-1"}`)
	if err != nil || !done || completed.RunID != "run-1" || completed.Failed {
		t.Fatalf("completed = %+v done=%v err=%v", completed, done, err)
	}
	failed, done, err := completionFromSSE("chat-session-event", `{"EventType":"run.failed","RunID":"run-2","PayloadJSON":{"error":"boom"}}`)
	if err != nil || !done || !failed.Failed || failed.Error != "boom" {
		t.Fatalf("failed = %+v done=%v err=%v", failed, done, err)
	}
	ignored, done, err := completionFromSSE("active.partial", `{"messages":[]}`)
	if err != nil || done || ignored.RunID != "" {
		t.Fatalf("ignored = %+v done=%v err=%v", ignored, done, err)
	}
}

func TestChatRunRefFromURLFallsBackThreadAsSessionID(t *testing.T) {
	ref := chatRunRefFromURL("https://stage.test/agent-chat/thread/thread-1?run=run-1")
	if ref.ThreadID != "thread-1" || ref.ChatSessionID != "thread-1" || ref.RunID != "run-1" {
		t.Fatalf("ref = %+v", ref)
	}
}
