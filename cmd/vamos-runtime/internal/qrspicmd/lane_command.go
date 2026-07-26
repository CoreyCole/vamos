package qrspicmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

type LaneRunOptions struct {
	LaneSpec
}

func newLaneRunCommand(d deps) *cobra.Command {
	opts := LaneRunOptions{LaneSpec: LaneSpec{Attempt: 1, Timeout: time.Hour}}
	cmd := &cobra.Command{
		Use:   "lane-run --plan-dir <path> --coordinator-id <id> --role <role> --prompt-file <file> --report-path <file>",
		Short: "Run one detached, non-interactive QRSPI lane",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunLane(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.ID, "lane-id", "", "parent-selected lane ID")
	f.StringVar(&opts.CoordinatorID, "coordinator-id", "", "parent stage-child ID")
	f.IntVar(
		&opts.CoordinatorGen,
		"coordinator-generation",
		0,
		"parent stage-child generation",
	)
	f.StringVar((*string)(&opts.Role), "role", "", "allowlisted read-only lane role")
	f.StringVar(&opts.PromptFile, "prompt-file", "", "absolute rendered lane prompt")
	f.StringVar(&opts.Cwd, "cwd", "", "lane working directory")
	f.StringVar(&opts.PlanDir, "plan-dir", "", "absolute plan directory")
	f.StringVar(&opts.ReviewDir, "review-dir", "", "absolute parent review directory")
	f.StringVar(
		&opts.ReportPath,
		"report-path",
		"",
		"absolute parent-allocated Markdown report",
	)
	f.StringVar(&opts.SessionID, "session-id", "", "parent-selected Pi session ID")
	f.StringVar(&opts.SessionDir, "session-dir", "", "absolute Pi session directory")
	f.IntVar(&opts.Attempt, "attempt", 1, "lane attempt (1 or 2)")
	f.DurationVar(&opts.Timeout, "timeout", time.Hour, "maximum lane runtime")
	return cmd
}

func RunLane(ctx context.Context, opts LaneRunOptions, d deps, out io.Writer) error {
	runner := &DetachedLaneRunner{ProcessRunner: d.LaneProcessRunner}
	record, err := runner.Start(ctx, opts.LaneSpec)
	if err != nil {
		return err
	}
	record, waitErr := runner.Wait(ctx, record)
	if err := json.NewEncoder(out).Encode(record); err != nil {
		return err
	}
	if waitErr != nil {
		return waitErr
	}
	if record.State != LaneSuccess {
		return fmt.Errorf("lane %q exited unsuccessfully", record.Spec.ID)
	}
	return nil
}
