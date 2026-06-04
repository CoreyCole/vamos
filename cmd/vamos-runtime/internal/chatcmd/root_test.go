package chatcmd

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/authcmd"
)

type fakeStore struct{}

func (fakeStore) Save(string, authcmd.Profile, string) error { return nil }
func (fakeStore) Load(string) (authcmd.Profile, string, error) {
	return authcmd.Profile{ManagerURL: "https://main.workspaces.test", KeyID: "machine-1"}, "secret-1", nil
}

type fakeClient struct{ got authcmd.MintRequest }

func (f *fakeClient) MintBrowserToken(_ context.Context, keyID, secret string, req authcmd.MintRequest) (authcmd.MintResponse, error) {
	if keyID != "machine-1" || secret != "secret-1" {
		return authcmd.MintResponse{}, context.Canceled
	}
	f.got = req
	return authcmd.MintResponse{Token: "browser-token"}, nil
}
func (f *fakeClient) Status(context.Context, string, string, authcmd.MintRequest) error { return nil }

type fakeBrowser struct {
	loginURL string
	prompt   string
}

func (b *fakeBrowser) Login(_ context.Context, loginURL string) error {
	b.loginURL = loginURL
	return nil
}
func (b *fakeBrowser) SubmitComposerPrompt(_ context.Context, prompt string) (ChatRunRef, error) {
	b.prompt = prompt
	return ChatRunRef{ChatSessionID: "session-1", RunID: "run-1"}, nil
}
func (b *fakeBrowser) Cookies(context.Context, string) ([]*http.Cookie, error) {
	return []*http.Cookie{{Name: "thoughts_session", Value: "s1"}}, nil
}
func (b *fakeBrowser) Close(context.Context) error { return nil }

type fakeWatcher struct{}

func (fakeWatcher) WatchUntilComplete(context.Context, string, string, int64) (ChatCompletion, error) {
	return ChatCompletion{RunID: "run-1", Response: "assistant done"}, nil
}

func TestRunMintsHermesTokenSubmitsBrowserPromptAndPrintsResponse(t *testing.T) {
	client := &fakeClient{}
	browser := &fakeBrowser{}
	var out bytes.Buffer
	err := Run(context.Background(), Options{Slug: "stage", Email: "lead@example.test", Prompt: "continue plan", Profile: "default"}, deps{
		Store:      fakeStore{},
		Client:     client,
		BrowserNew: func(Options) (BrowserAutomation, error) { return browser, nil },
		WatcherNew: func(*http.Client) CompletionWatcher { return fakeWatcher{} },
	}, &out)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.got.Purpose != purposeHermesChat || client.got.RedirectPath != "/agent-chat" || client.got.Email != "lead@example.test" {
		t.Fatalf("mint request = %+v", client.got)
	}
	if !strings.Contains(browser.loginURL, "/internal/agent-auth/browser-login") || !strings.Contains(browser.loginURL, "purpose=hermes_chat") {
		t.Fatalf("login URL = %q", browser.loginURL)
	}
	if browser.prompt != "continue plan" {
		t.Fatalf("prompt = %q", browser.prompt)
	}
	if strings.TrimSpace(out.String()) != "assistant done" {
		t.Fatalf("output = %q", out.String())
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
