package hermescmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPlan(t *testing.T) PlanContext {
	t.Helper()
	root := t.TempDir()
	t.Setenv("VAMOS_HERMES_RESUME_INDEX", filepath.Join(root, "manual-resume.json"))
	dir := filepath.Join(root, "thoughts", "me", "plans", "example")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "AGENTS.md"),
		[]byte(
			"---\nplan_goal: test goal\nimplementation_workspace: /tmp/impl\n---\n# Plan\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"design.md", "outline.md", "plan.md"} {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte("# "+name+"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := LoadPlanContext(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestWritePiResultOverwritesCurrentConclusion(t *testing.T) {
	ctx := testPlan(t)
	_, err := WritePiResult(
		ctx,
		PiResult{
			Session: "one",
			Outcome: OutcomeHandoff,
			Next:    NextImplement,
			Summary: "1. first",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	path, err := WritePiResult(
		ctx,
		PiResult{
			Session: "one",
			Outcome: OutcomeComplete,
			Next:    NextNone,
			Summary: "1. final",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ReadPiResult(ctx.PlanDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete || result.Summary != "1. final" {
		t.Fatalf("result=%+v", result)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
}

func TestDoneWritesResultThenNotifiesVamosFromHostConfig(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("vamos_url: https://vamos.example\ncallback_token: callback-secret\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var gotConfig hostConfig
	var gotPlan, gotSession string
	cmd := newDoneCommand(
		func(_ context.Context, config hostConfig, plan, session string) error {
			gotConfig, gotPlan, gotSession = config, plan, session
			return nil
		},
	)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "none", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotConfig.CallbackToken != "callback-secret" || gotPlan != ctx.PlanDir ||
		gotSession != "session-1" {
		t.Fatalf(
			"notification = config=%+v plan=%q session=%q",
			gotConfig,
			gotPlan,
			gotSession,
		)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "session-1"); err != nil {
		t.Fatalf("Pi result was not written before notification: %v", err)
	}
}

func TestNotifyPiCompletionReportsMissingHermesManager(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()
	if err := notifyPiCompletion(
		t.Context(),
		hostConfig{VamosURL: server.URL, CallbackToken: "secret"},
		"/plans/example",
		"session-1",
	); !errors.Is(err, errHermesManagerNotFound) {
		t.Fatalf("notifyPiCompletion() error = %v, want manager not found", err)
	}
}

func TestDoneUsesPiSessionID(t *testing.T) {
	ctx := testPlan(t)
	t.Setenv("PI_SESSION_ID", "pi-session")
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		t.Fatal("manual completion must not notify")
		return nil
	})
	cmd.SetArgs([]string{
		"--plan",
		ctx.PlanDir,
		"--config",
		filepath.Join(t.TempDir(), "missing-hermes.yaml"),
		"--outcome",
		"complete",
		"--next",
		"none",
		"--summary",
		"1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "pi-session"); err != nil {
		t.Fatalf("Pi session result = %v", err)
	}
}

func TestDoneWithoutHermesManagerPrintsManualContinuation(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("vamos_url: https://vamos.example\ncallback_token: secret\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		return errHermesManagerNotFound
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "implement", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		"no Hermes manager owns this session",
		"Human action required to start the recommended next session.",
		"Do not run this command from the active Pi worker.",
		"vamos hermes pi continue ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("managerless completion output = %q, want %q", got, want)
		}
	}
}

func TestDoneWithoutHostConfigRecordsLocalResult(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "missing-hermes.yaml")
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		t.Fatal("manual completion must not notify")
		return nil
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "implement", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "session-1"); err != nil {
		t.Fatalf("manual Pi result = %v", err)
	}
	got := output.String()
	if !strings.Contains(
		got,
		"Human action required to start the recommended next session.",
	) ||
		!strings.Contains(got, "Do not run this command from the active Pi worker.") ||
		!strings.Contains(got, "vamos hermes pi continue ") {
		t.Fatalf("manual completion output = %q", got)
	}
}

func TestEnumsRejectUnknownValues(t *testing.T) {
	if _, err := ParseOutcome("bad"); err == nil {
		t.Fatal("accepted bad outcome")
	}
	if _, err := ParseNextAction("bad"); err == nil {
		t.Fatal("accepted bad next")
	}
}

func TestPlanContextUsesThoughtsRelativeIdentity(t *testing.T) {
	ctx := testPlan(t)
	if ctx.PlanRel != "me/plans/example" || ctx.PlanGoal != "test goal" {
		t.Fatalf("context=%+v", ctx)
	}
}
