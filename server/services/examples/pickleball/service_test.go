package pickleball

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeWorkflowStarter struct{ calls []PromptRequest }

func (f *fakeWorkflowStarter) StartPickleballSelfModify(_ context.Context, req PromptRequest) (string, error) {
	f.calls = append(f.calls, req)
	return "run-123", nil
}

type fakeNotifier struct{ sessions []string }

func (f *fakeNotifier) NotifyPickleballSession(sessionID string) {
	f.sessions = append(f.sessions, sessionID)
}

func TestEnsureSessionCopiesSeedBundle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(t, nil, nil)

	session, err := svc.EnsureSession(ctx, "Player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if session.ID != "player-example-com" {
		t.Fatalf("session ID = %q", session.ID)
	}
	if session.State != AppStateIdle {
		t.Fatalf("state = %q", session.State)
	}
	for _, name := range []string{"go.mod", "main.go", "players.csv"} {
		if _, err := os.Stat(filepath.Join(session.WorkspacePath, name)); err != nil {
			t.Fatalf("seed file %s missing: %v", name, err)
		}
	}
}

func TestSubmitPromptStartsWorkflowAndMarksGenerating(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	starter := &fakeWorkflowStarter{}
	notifier := &fakeNotifier{}
	svc := newTestService(t, starter, notifier)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	accepted, err := svc.SubmitPrompt(ctx, PromptRequest{SessionID: session.ID, Prompt: "Add skill totals", UserEmail: "player@example.com"})
	if err != nil {
		t.Fatalf("SubmitPrompt: %v", err)
	}
	if accepted.RunID != "run-123" || accepted.State != AppStateGenerating {
		t.Fatalf("accepted = %+v", accepted)
	}
	loaded, err := svc.store.LoadSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.State != AppStateGenerating || loaded.ActiveRunID != "run-123" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(starter.calls) != 1 || len(notifier.sessions) == 0 {
		t.Fatalf("starter calls=%d notifier=%d", len(starter.calls), len(notifier.sessions))
	}

	again, err := svc.SubmitPrompt(ctx, PromptRequest{SessionID: session.ID, Prompt: "Second"})
	if err != nil {
		t.Fatalf("SubmitPrompt active: %v", err)
	}
	if again.RunID != "run-123" || len(starter.calls) != 1 {
		t.Fatalf("active prompt should reuse run; accepted=%+v calls=%d", again, len(starter.calls))
	}
}

func TestPromoteSnapshotAndFailurePreservesLastGood(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(t, nil, nil)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	snapshot := BuildSnapshot{
		BuildID:          "build-1",
		PromptSummary:    "seed",
		Mode:             "one_shot",
		Status:           "succeeded",
		SnapshotPath:     "creative-mode-agent/examples/pickleball/sessions/player/snapshots/build-1",
		ManifestPath:     "creative-mode-agent/examples/pickleball/sessions/player/snapshots/build-1/manifest.json",
		HTMLThoughtsPath: "creative-mode-agent/examples/pickleball/sessions/player/snapshots/build-1/app.html",
		CSVThoughtsPath:  "creative-mode-agent/examples/pickleball/sessions/player/snapshots/build-1/results.csv",
	}
	if err := svc.PromoteSnapshot(ctx, session.ID, snapshot); err != nil {
		t.Fatalf("PromoteSnapshot: %v", err)
	}
	if err := svc.MarkFailed(ctx, session.ID, os.ErrInvalid, strings.Repeat("x", maxLogTailBytes+10)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	vm, err := svc.GetState(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if vm.State != AppStateFailed || vm.LastGood == nil || vm.LastGood.BuildID != "build-1" {
		t.Fatalf("vm = %+v", vm)
	}
	if len(vm.LogTail) != maxLogTailBytes {
		t.Fatalf("log tail length = %d", len(vm.LogTail))
	}
	if vm.Share.PreviewURL != "/thoughts/creative-mode-agent/examples/pickleball/sessions/player/snapshots/build-1/app.html" {
		t.Fatalf("preview URL = %q", vm.Share.PreviewURL)
	}
}

func TestSnapshotHistoryNewestFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(t, nil, nil)
	session, err := svc.EnsureSession(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	older := BuildSnapshot{BuildID: "old", CreatedAt: time.Now().Add(-time.Hour)}
	newer := BuildSnapshot{BuildID: "new", CreatedAt: time.Now()}
	if err := svc.store.SaveSnapshot(ctx, session.ID, older); err != nil {
		t.Fatalf("SaveSnapshot old: %v", err)
	}
	if err := svc.store.SaveSnapshot(ctx, session.ID, newer); err != nil {
		t.Fatalf("SaveSnapshot new: %v", err)
	}
	history, err := svc.SnapshotHistoryForPrompt(ctx, session.ID)
	if err != nil {
		t.Fatalf("SnapshotHistoryForPrompt: %v", err)
	}
	if len(history) != 2 || history[0].BuildID != "new" || history[1].BuildID != "old" {
		t.Fatalf("history = %+v", history)
	}
}

func TestBuildAIPromptIncludesRulesAndHistory(t *testing.T) {
	t.Parallel()
	prompt := BuildAIPrompt(PromptRequest{Prompt: "Make courts colorful"}, []BuildSnapshot{{BuildID: "build-1", PromptSummary: "seed", HTMLThoughtsPath: "creative-mode-agent/examples/pickleball/sessions/default/snapshots/build-1/app.html"}})
	for _, want := range []string{"Edit only the generated bundle workspace", "app.html, results.csv, and manifest.json", "build-1: seed", "Make courts colorful"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "/home/") {
		t.Fatalf("prompt should not include host absolute paths:\n%s", prompt)
	}
}

func newTestService(t *testing.T, starter WorkflowStarter, notifier Notifier) *Service {
	t.Helper()
	seedDir := filepath.Join(t.TempDir(), "seed")
	for name, content := range map[string]string{
		"go.mod":      "module seed\n\ngo 1.24\n",
		"main.go":     "package main\nfunc main(){}\n",
		"players.csv": "name,skill\nAvery,5\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(seedDir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(Options{ThoughtsRoot: t.TempDir(), SeedBundleDir: seedDir, WorkflowStarter: starter, Notifier: notifier})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
