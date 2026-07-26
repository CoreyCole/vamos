package qrspicmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type LaneRunOptions struct {
	LaneSpec
	SpecsFile   string
	MaxParallel int
}

type LaneStatusOptions struct {
	PlanDir string
	Record  string
	Output  string
}

type laneStatusView struct {
	State      LaneState `json:"state"`
	Attempt    int       `json:"attempt"`
	ReportPath string    `json:"reportPath"`
	SessionID  string    `json:"sessionId"`
	SessionDir string    `json:"sessionDir"`
	StatusPath string    `json:"statusPath"`
	EventsPath string    `json:"eventsPath"`
	OutputPath string    `json:"outputPath"`
	PID        int       `json:"pid,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	ErrorTail  string    `json:"errorTail,omitempty"`
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
	f.StringVar(
		&opts.SpecsFile,
		"specs-file",
		"",
		"absolute JSON array of parent-selected lane specs",
	)
	f.IntVar(
		&opts.MaxParallel,
		"max-parallel",
		1,
		"maximum concurrent lanes for --specs-file",
	)
	return cmd
}

func newLaneStatusCommand(_ deps) *cobra.Command {
	opts := LaneStatusOptions{Output: "json"}
	cmd := &cobra.Command{
		Use:   "lane-status --plan-dir <path> --record <path>",
		Short: "Inspect read-only QRSPI lane diagnostics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunLaneStatus(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "absolute plan directory")
	cmd.Flags().StringVar(&opts.Record, "record", "", "absolute lane record path")
	cmd.Flags().StringVar(&opts.Output, "output", "json", "output format: json or text")
	return cmd
}

func RunLaneStatus(opts LaneStatusOptions, out io.Writer) error {
	if opts.Output == "" {
		opts.Output = "json"
	}
	if opts.PlanDir == "" {
		return fmt.Errorf("plan-dir is required")
	}
	if opts.Record == "" {
		return fmt.Errorf("record is required")
	}
	record, err := InspectLaneRecord(opts.PlanDir, opts.Record)
	if err != nil {
		return err
	}
	view := laneStatusView{
		State:      record.State,
		Attempt:    record.Spec.Attempt,
		ReportPath: record.Spec.ReportPath,
		SessionID:  record.Spec.SessionID,
		SessionDir: record.Spec.SessionDir,
		StatusPath: record.StatusPath,
		EventsPath: record.EventsPath,
		OutputPath: record.OutputPath,
		PID:        record.PID,
		StartedAt:  record.StartedAt,
		FinishedAt: record.FinishedAt,
		ErrorTail:  boundedLaneDiagnostic(record.ErrorTail),
	}
	if opts.Output == "text" {
		_, err = fmt.Fprintf(
			out,
			"state: %s\nattempt: %d\nreport: %s\nsession: %s\n",
			view.State,
			view.Attempt,
			view.ReportPath,
			view.SessionID,
		)
		return err
	}
	if opts.Output != "json" {
		return fmt.Errorf("unsupported output format %q", opts.Output)
	}
	return json.NewEncoder(out).Encode(view)
}

func RunLane(ctx context.Context, opts LaneRunOptions, d deps, out io.Writer) error {
	runner := &DetachedLaneRunner{ProcessRunner: d.LaneProcessRunner}
	if opts.SpecsFile != "" {
		data, err := os.ReadFile(opts.SpecsFile)
		if err != nil {
			return err
		}
		var specs []LaneSpec
		if err := json.Unmarshal(data, &specs); err != nil {
			return fmt.Errorf("decode lane specs: %w", err)
		}
		records, err := (LaneCoordinator{Runner: runner, MaxParallel: opts.MaxParallel}).Run(
			ctx,
			specs,
		)
		if encodeErr := json.NewEncoder(out).Encode(struct {
			Records []LaneRecord `json:"records"`
			Reports []string     `json:"reports"`
		}{records, laneReports(records)}); encodeErr != nil {
			return encodeErr
		}
		return err
	}
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
	if err := ValidateLaneReport(record.Spec); err != nil {
		return err
	}
	return nil
}

func laneReports(records []LaneRecord) []string {
	reports := make([]string, 0, len(records))
	for _, record := range records {
		if record.State == LaneSuccess {
			reports = append(reports, record.Spec.ReportPath)
		}
	}
	return reports
}
