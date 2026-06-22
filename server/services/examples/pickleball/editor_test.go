package pickleball

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	servercfg "github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/services/agentchat"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

type fakeAgentChatStarter struct {
	actor      serverauth.MachineAPIActor
	resolution servercfg.ProjectCheckoutResolution
	req        agentchat.ChatStartRequest
	baseURL    string
}

func (f *fakeAgentChatStarter) StartCLIChatRun(_ context.Context, actor serverauth.MachineAPIActor, resolution servercfg.ProjectCheckoutResolution, req agentchat.ChatStartRequest, publicBaseURL string) (agentchat.ChatRunRef, error) {
	f.actor = actor
	f.resolution = resolution
	f.req = req
	f.baseURL = publicBaseURL
	return agentchat.ChatRunRef{ThreadID: "thread-1", RunID: "run-1", ChatSessionID: "chat-1", WebURL: "https://vamos.example.test/agent-chat?thread=thread-1", CWD: resolution.RootPath}, nil
}

func TestAgentChatEditorStartsRunWithAppletPrompt(t *testing.T) {
	t.Parallel()
	starter := &fakeAgentChatStarter{}
	editor := AgentChatEditor{Starter: starter, ProjectID: "github.com/CoreyCole/vamos", PublicBaseURL: "https://vamos.example.test"}
	result, err := editor.ApplyPrompt(context.Background(), AppletEditInput{
		Prompt:        "Make schedule prettier",
		FilesRoot:     "/tmp/files",
		CurrentAppDir: "/tmp/files/apps/current",
		IterationDir:  "/tmp/files/apps/iterations/iter-1",
		UserEmail:     "player@example.com",
	})
	if err != nil {
		t.Fatalf("ApplyPrompt: %v", err)
	}
	if result.ChatSessionID != "chat-1" || result.ThreadID != "thread-1" || result.RunID != "run-1" || result.WebURL == "" {
		t.Fatalf("result refs = %+v", result)
	}
	for _, want := range []string{"Make schedule prettier", "/tmp/files/apps/iterations/iter-1", "/tmp/files", "non-technical summary", "go test ./..."} {
		if !strings.Contains(starter.req.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, starter.req.Prompt)
		}
	}
	if starter.actor.ActorEmail != "player@example.com" {
		t.Fatalf("actor = %+v", starter.actor)
	}
	if starter.resolution.RootPath != "/tmp/files/apps/iterations/iter-1" {
		t.Fatalf("resolution = %+v", starter.resolution)
	}
}

func TestFixtureEditorRequiresExplicitEnablement(t *testing.T) {
	t.Parallel()
	_, err := FixtureEditor{}.ApplyPrompt(context.Background(), AppletEditInput{})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled fixture err = %v", err)
	}
}

func TestFixtureEditorUpdatesSourceWhenEnabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main\nfunc main(){}\n")
	result, err := (FixtureEditor{Enabled: true}).ApplyPrompt(context.Background(), AppletEditInput{IterationDir: dir, Prompt: "Add friendlier titles"})
	if err != nil {
		t.Fatalf("ApplyPrompt: %v", err)
	}
	if result.UserSummary == "" || len(result.ChangedFiles) == 0 {
		t.Fatalf("result = %+v", result)
	}
	if got := readTestFile(t, dir, "main.go"); !strings.Contains(got, "Last friendly prompt") {
		t.Fatalf("source not updated:\n%s", got)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
