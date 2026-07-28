package agentchat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHermesCallbacksRejectPlanOutsideThoughtsRoot(t *testing.T) {
	thoughts := t.TempDir()
	outside := t.TempDir()
	service := &Service{thoughtsRoot: thoughts}

	err := service.AppendHermesTranscript(t.Context(), HermesCallbackEvent{
		PlanDir: outside,
		HermesTranscriptEvent: HermesTranscriptEvent{
			ID: "event-1", Type: "final", ThreadID: "thread-1", Content: "done",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes thoughts root") {
		t.Fatalf("AppendHermesTranscript() error = %v, want containment rejection", err)
	}
}

func TestHermesPiResultReadsOnlyContainedPlan(t *testing.T) {
	thoughts := t.TempDir()
	plan := filepath.Join(thoughts, "agent", "plans", "plan-a")
	resultPath := filepath.Join(plan, ".vamos", "sessions", "pi", "session-1_result.yaml")
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		resultPath,
		[]byte("session: session-1\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	service := &Service{thoughtsRoot: thoughts}

	result, err := service.HermesPiResult(plan, "session-1")
	if err != nil {
		t.Fatalf("HermesPiResult() error = %v", err)
	}
	if got, want := string(result), "session: session-1\n"; got != want {
		t.Fatalf("HermesPiResult() = %q, want %q", got, want)
	}
	if _, err := service.HermesPiResult(plan, "../secret"); err == nil {
		t.Fatal("HermesPiResult() accepted traversal session ID")
	}
	if _, err := service.HermesPiResult(t.TempDir(), "session-1"); err == nil {
		t.Fatal("HermesPiResult() accepted plan outside thoughts root")
	}
}
