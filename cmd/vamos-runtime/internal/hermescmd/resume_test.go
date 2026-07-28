package hermescmd

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestContinueLaunchesMappedPreviousSession(t *testing.T) {
	ctx := testPlan(t)
	if _, err := WritePiResult(
		ctx,
		PiResult{
			Session: "previous",
			Outcome: OutcomeComplete,
			Next:    NextVerify,
			Summary: "1. done",
		},
	); err != nil {
		t.Fatal(err)
	}
	id, err := recordManualResume(ctx, PiResult{Session: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	cmd := newContinueCommand(
		func(_ context.Context, _ string, args, _ []string, _, _ io.Writer) error {
			gotArgs = args
			return nil
		},
	)
	cmd.SetArgs([]string{id})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		strings.Join(gotArgs, " "),
		"--session-dir "+ctx.PlanDir+"/.vamos/sessions/pi",
	) ||
		!strings.Contains(strings.Join(gotArgs, " "), "@") {
		t.Fatalf("continue args = %#v", gotArgs)
	}
}
