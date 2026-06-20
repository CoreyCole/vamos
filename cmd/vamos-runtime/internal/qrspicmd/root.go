package qrspicmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var ErrNotImplemented = errors.New("qrspi command behavior not implemented")

func NewCommand() *cobra.Command { return newCommand(deps{}) }

func newCommand(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qrspi",
		Short: "Manage local QRSPI stage orchestration",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errors.New("use an explicit subcommand")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("use an explicit subcommand")
		},
	}
	cmd.AddCommand(
		newInitCommand(d),
		newRunChildCommand(d),
		newValidateResultCommand(d),
		newDecideNextCommand(d),
		newRenderPromptCommand(d),
	)
	return cmd
}

func newInitCommand(d deps) *cobra.Command {
	opts := InitOptions{}
	cmd := &cobra.Command{
		Use:   "init --plan-dir <path>",
		Short: "Initialize q-manager state for a QRSPI plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunInit(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.ProjectRoot, "project-root", "", "project repository root")
	cmd.Flags().StringVar(&opts.PolicyFile, "policy-file", "", "optional policy JSON file")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "replace existing expired/inactive state")
	return cmd
}

func newRunChildCommand(d deps) *cobra.Command {
	opts := RunChildOptions{Split: "right", Timeout: 12 * time.Hour}
	cmd := &cobra.Command{
		Use:   "run-child --plan-dir <path> --stage <node> --cwd <path> --prompt-file <file>",
		Short: "Launch a visible child QRSPI stage session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunChild(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "QRSPI node ID to run")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "child process working directory")
	cmd.Flags().StringVar(&opts.PromptFile, "prompt-file", "", "rendered child prompt file")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction")
	cmd.Flags().StringVar(&opts.ManagerRunID, "manager-run-id", "", "manager run ID")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 12*time.Hour, "maximum time to wait for child result")
	return cmd
}

func newValidateResultCommand(d deps) *cobra.Command {
	opts := ValidateResultOptions{}
	cmd := &cobra.Command{
		Use:   "validate-result --stage <node> --state-file <file> --result-file <file> --plan-dir <path>",
		Short: "Validate a child QRSPI result against the canonical graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunValidateResult(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected QRSPI node ID")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "child result file")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.RunID, "run-id", "", "child run ID")
	cmd.Flags().StringVar(&opts.SessionID, "session-id", "", "child session ID")
	return cmd
}

func newDecideNextCommand(d deps) *cobra.Command {
	opts := DecideNextOptions{}
	cmd := &cobra.Command{
		Use:   "decide-next --state-file <file> --result-file <file> --plan-dir <path>",
		Short: "Persist the transition decision for a validated QRSPI result",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunDecideNext(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "child result file")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	return cmd
}

func newRenderPromptCommand(d deps) *cobra.Command {
	opts := RenderPromptOptions{}
	cmd := &cobra.Command{
		Use:   "render-prompt --state-file <file> --node <node> --plan-dir <path>",
		Short: "Render a graph-selected prompt for a child QRSPI stage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRenderPrompt(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.NodeID, "node", "", "QRSPI node ID")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	return cmd
}

func RunInit(ctx context.Context, opts InitOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	ensureWriter(out)
	return ErrNotImplemented
}

func RunChild(ctx context.Context, opts RunChildOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	if strings.TrimSpace(opts.Stage) == "" {
		return errors.New("stage is required")
	}
	if strings.TrimSpace(opts.Cwd) == "" {
		return errors.New("cwd is required")
	}
	if strings.TrimSpace(opts.PromptFile) == "" {
		return errors.New("prompt-file is required")
	}
	ensureWriter(out)
	return ErrNotImplemented
}

func RunValidateResult(ctx context.Context, opts ValidateResultOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.Stage) == "" {
		return errors.New("stage is required")
	}
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.ResultFile) == "" {
		return errors.New("result-file is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	ensureWriter(out)
	return ErrNotImplemented
}

func RunDecideNext(ctx context.Context, opts DecideNextOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.ResultFile) == "" {
		return errors.New("result-file is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	ensureWriter(out)
	return ErrNotImplemented
}

func RunRenderPrompt(ctx context.Context, opts RenderPromptOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.NodeID) == "" {
		return errors.New("node is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	ensureWriter(out)
	return ErrNotImplemented
}

func ensureWriter(out io.Writer) io.Writer {
	if out == nil {
		return os.Stdout
	}
	return out
}
