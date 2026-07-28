package hermescmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPlan(t *testing.T) PlanContext {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "thoughts", "me", "plans", "example")
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
	if got := output.String(); !strings.Contains(
		got,
		"recommended: pi @.pi/skills/q-implement/SKILL.md",
	) {
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
