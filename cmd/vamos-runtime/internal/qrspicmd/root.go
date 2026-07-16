package qrspicmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"gopkg.in/yaml.v3"
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
		newStartNextCommand(d),
		newSteerChildCommand(d),
		newSetPolicyCommand(d),
		newRunChildCommand(d),
		newChildCompleteCommand(d),
		newManagerReadyCommand(d),
		newDoctorCommand(d),
		newRepairStateCommand(d),
		newMarkChildActiveCommand(d),
		newInspectCommand(d),
		newFindLatestChildCommand(d),
		newRebindChildCommand(d),
		newValidateLatestCommand(d),
		newRecoverManualCommand(d),
		newRecoverSummaryCommand(d),
		newValidateResultCommand(d),
		newDecideNextCommand(d),
		newRepromptChildCommand(d),
		newContinueCommand(d),
		newRenderPromptCommand(d),
	)
	suppressRuntimeUsage(cmd)
	return cmd
}

func suppressRuntimeUsage(cmd *cobra.Command) *cobra.Command {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, child := range cmd.Commands() {
		suppressRuntimeUsage(child)
	}
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
	cmd.Flags().
		StringVar(&opts.ProjectRoot, "project-root", "", "project repository root")
	cmd.Flags().
		StringVar(&opts.PolicyFile, "policy-file", "", "optional policy JSON file")
	cmd.Flags().
		StringVar(&opts.PolicyPreset, "policy-preset", "", "policy preset: discuss, guided, autopilot, autopilot-no-plan-reviews, fast")
	cmd.Flags().
		StringVar(&opts.NodeID, "node", "", "initial QRSPI node ID (defaults to graph start)")
	cmd.Flags().StringVar(&opts.NodeID, "stage", "", "alias for --node")
	cmd.Flags().
		StringVar(&opts.ImplementationCwd, "implementation-cwd", "", "implementation workspace cwd for implementation/review/verify stages")
	cmd.Flags().
		StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().
		StringVar(&opts.PiModel, "model", "", "Pi model pattern or ID for child sessions (passed to pi --model)")
	cmd.Flags().
		BoolVar(&opts.Force, "force", false, "replace existing expired/inactive state")
	return cmd
}

func newStartNextCommand(d deps) *cobra.Command {
	opts := StartNextOptions{Split: "right", Timeout: 0, Output: "text"}
	var usagePercent float64
	var usageTokens int
	var usageWindow int
	var usageSource string
	cmd := &cobra.Command{
		Use:   "start-next --plan-dir <path> --project-root <path>",
		Short: "Initialize or resume q-manager state and launch the graph-selected child",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Usage = usageFromChangedFlags(
				cmd,
				usagePercent,
				usageTokens,
				usageWindow,
				usageSource,
			)
			_, err := RunStartNext(cmd.Context(), opts, d, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().
		StringVar(&opts.ProjectRoot, "project-root", "", "project repository root")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.PolicyFile, "policy-file", "", "optional policy JSON file")
	cmd.Flags().
		StringVar(&opts.PolicyPreset, "policy-preset", "", "policy preset: discuss, guided, autopilot, autopilot-no-plan-reviews, fast")
	cmd.Flags().StringVar(&opts.NodeID, "node", "", "QRSPI node ID override")
	cmd.Flags().StringVar(&opts.NodeID, "stage", "", "alias for --node")
	cmd.Flags().
		StringVar(&opts.ImplementationCwd, "implementation-cwd", "", "implementation workspace cwd for implementation/review/verify stages")
	cmd.Flags().
		StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().
		StringVar(&opts.PiModel, "model", "", "Pi model pattern or ID for child sessions (passed to pi --model)")
	cmd.Flags().
		StringVar(&opts.LatestResultFile, "latest-result-file", "", "file containing latest fenced qrspi_result YAML")
	cmd.Flags().
		BoolVar(&opts.LatestResultStdin, "latest-result-stdin", false, "read latest fenced qrspi_result YAML from stdin")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "child cwd override")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction")
	cmd.Flags().
		DurationVar(&opts.Timeout, "timeout", 0, "maximum time to wait for child; 0 returns after launch")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	cmd.Flags().
		BoolVar(&opts.Force, "force", false, "replace existing expired/inactive state")
	cmd.Flags().
		Float64Var(&usagePercent, "manager-usage-percent", 0, "parent manager context usage percent for optional compaction")
	cmd.Flags().
		IntVar(&usageTokens, "manager-usage-tokens", 0, "parent manager context token count for optional compaction")
	cmd.Flags().
		IntVar(&usageWindow, "manager-usage-window", 0, "parent manager context window size for optional compaction")
	cmd.Flags().
		StringVar(&usageSource, "manager-usage-source", "", "diagnostic source for parent manager context usage")
	_ = cmd.Flags().MarkHidden("manager-usage-source")
	return cmd
}

func newSteerChildCommand(d deps) *cobra.Command {
	opts := SteerChildOptions{Output: "text", RequireActive: true}
	cmd := &cobra.Command{
		Use:   "steer-child --state-file <file> (--feedback-file <file> | --feedback <text>)",
		Short: "Send human or task feedback to the active q-manager child",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := RunSteerChild(cmd.Context(), opts, d, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.FeedbackFile, "feedback-file", "", "file containing feedback for the active child")
	cmd.Flags().
		StringVar(&opts.Feedback, "feedback", "", "inline feedback for the active child")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected active child node")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	return cmd
}

func newSetPolicyCommand(d deps) *cobra.Command {
	opts := SetPolicyOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "set-policy --state-file <file> [--preset <name>]",
		Short: "Update q-manager QRSPI policy for future transitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.EnablePlanReviewsSet = cmd.Flags().Changed("enable-plan-reviews")
			opts.InvalidRetryLimitSet = cmd.Flags().Changed("invalid-result-retry-limit")
			return RunSetPolicy(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.Preset, "preset", "", "policy preset: discuss, guided, autopilot, autopilot-no-plan-reviews, fast")
	cmd.Flags().
		StringVar(&opts.AdvanceMode, "advance-mode", "", "advance mode: discuss, guided, autopilot")
	cmd.Flags().
		BoolVar(&opts.EnablePlanReviews, "enable-plan-reviews", true, "run planning review stages")
	cmd.Flags().
		IntVar(&opts.InvalidResultRetryLimit, "invalid-result-retry-limit", 0, "invalid result retry limit")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
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
	cmd.Flags().
		StringVar(&opts.PromptFile, "prompt-file", "", "rendered child prompt file")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction")
	cmd.Flags().StringVar(&opts.ManagerRunID, "manager-run-id", "", "manager run ID")
	cmd.Flags().
		StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().
		StringVar(&opts.PiModel, "model", "", "Pi model pattern or ID for this child session (passed to pi --model)")
	cmd.Flags().
		DurationVar(&opts.Timeout, "timeout", 12*time.Hour, "maximum time to wait for child done marker")
	return cmd
}

func newChildCompleteCommand(d deps) *cobra.Command {
	opts := ChildCompletionOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "child-complete --state-file <file> [--child-id <id>]",
		Short: "Validate active child completion and write q-manager validation status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := RunChildComplete(cmd.Context(), opts, d, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.ChildID, "child-id", "", "expected active child ID")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newManagerReadyCommand(d deps) *cobra.Command {
	opts := ManagerReadyOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "manager-ready --state-file <file>",
		Short: "Mark q-manager ready and flush any queued validated child wake",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunManagerReady(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the resumed parent q-manager session")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	return cmd
}

func newDoctorCommand(d deps) *cobra.Command {
	opts := DoctorOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose q-manager runtime dependencies and active child health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunDoctor(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().
		StringVar(&opts.Output, "output", "text", "output format: text, ndjson, or json")
	return cmd
}

func newRepairStateCommand(d deps) *cobra.Command {
	opts := RepairStateOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "repair-state --state-file <file> --align-active-child",
		Short: "Repair q-manager workflow state from active child evidence",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRepairState(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		BoolVar(&opts.AlignActiveChild, "align-active-child", false, "align current workflow node to the active child stage when safe")
	cmd.Flags().
		BoolVar(&opts.ClearFailedChild, "clear-failed-child", false, "clear active child only when status/output prove terminal launch failure")
	cmd.Flags().
		BoolVar(&opts.Relaunch, "relaunch", false, "after clearing a terminal failed child, relaunch the same graph node")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	return cmd
}

func newMarkChildActiveCommand(d deps) *cobra.Command {
	opts := MarkChildActiveOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "mark-child-active --state-file <file> --child-id <id>",
		Short: "Mark an active child as manually reprompted and supersede stale queued wakes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunMarkChildActive(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.ChildID, "child-id", "", "active child ID")
	cmd.Flags().
		StringVar(&opts.Reason, "reason", "manual-reprompt", "reason for marking child active")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	return cmd
}

func newInspectCommand(d deps) *cobra.Command {
	opts := InspectOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "inspect --state-file <file>",
		Short: "Inspect q-manager state and child linkage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunInspect(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		BoolVar(&opts.Sessions, "sessions", false, "include active child session refs")
	cmd.Flags().
		BoolVar(&opts.Latest, "latest", false, "include latest relevant session candidate")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newFindLatestChildCommand(d deps) *cobra.Command {
	opts := FindLatestChildOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "find-latest-child --state-file <file>",
		Short: "Find the latest relevant q-manager child session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunFindLatestChild(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected QRSPI node ID")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newRebindChildCommand(d deps) *cobra.Command {
	opts := RebindChildOptions{Output: "text", Reason: "manual-new"}
	cmd := &cobra.Command{
		Use:   "rebind-child --state-file <file> --session-file <jsonl>",
		Short: "Rebind active child to a manually advanced session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRebindChild(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.SessionFile, "session-file", "", "child Pi session JSONL file")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected QRSPI node ID")
	cmd.Flags().StringVar(&opts.Reason, "reason", "manual-new", "rebind reason")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newValidateLatestCommand(d deps) *cobra.Command {
	opts := ValidateLatestOptions{Output: "text"}
	cmd := &cobra.Command{
		Use:   "validate-latest --state-file <file>",
		Short: "Validate the latest relevant child session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunValidateLatest(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected QRSPI node ID")
	cmd.Flags().
		BoolVar(&opts.ApplyRebind, "apply-rebind", false, "rebind active child before validating")
	cmd.Flags().
		BoolVar(&opts.Continue, "continue", false, "continue graph after validation")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newRecoverManualCommand(d deps) *cobra.Command {
	opts := RecoverManualOptions{Output: "text", Mode: "latest-session"}
	cmd := &cobra.Command{
		Use:   "recover-manual --state-file <file> --mode latest-session",
		Short: "Recover q-manager after manual child interaction",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRecoverManual(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Mode, "mode", "latest-session", "recovery mode")
	cmd.Flags().
		BoolVar(&opts.Continue, "continue", false, "continue graph after rebind/validation")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newRecoverSummaryCommand(d deps) *cobra.Command {
	opts := RecoverSummaryOptions{Output: "text", PiBinary: "pi"}
	cmd := &cobra.Command{
		Use:   "recover-summary --state-file <file> --session-file <jsonl>",
		Short: "Write a same-stage recovery summary prompt for a failed child session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRecoverSummary(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.SessionFile, "session-file", "", "failed child Pi session JSONL file")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "QRSPI stage/node to recover")
	cmd.Flags().
		StringVar(&opts.PiBinary, "pi-binary", "pi", "Pi binary to launch for non-dry-run summarization")
	cmd.Flags().
		BoolVar(&opts.DryRun, "dry-run", false, "write prompt and recovery note target without launching Pi")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or json")
	return cmd
}

func newValidateResultCommand(d deps) *cobra.Command {
	opts := ValidateResultOptions{}
	cmd := &cobra.Command{
		Use:   "validate-result --stage <node> --state-file <file> --plan-dir <path>",
		Short: "Validate a child QRSPI result against the canonical graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunValidateResult(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected QRSPI node ID")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.SessionFile, "session-file", "", "explicit child Pi session JSONL file")
	cmd.Flags().
		StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback only when session/latest-session recovery is unavailable")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.RunID, "run-id", "", "child run ID")
	cmd.Flags().StringVar(&opts.SessionID, "session-id", "", "child session ID")
	return cmd
}

func newDecideNextCommand(d deps) *cobra.Command {
	opts := DecideNextOptions{}
	cmd := &cobra.Command{
		Use:   "decide-next --state-file <file> --plan-dir <path>",
		Short: "Persist the transition decision for a validated QRSPI result",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunDecideNext(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.SessionFile, "session-file", "", "explicit child Pi session JSONL file")
	cmd.Flags().
		StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback only when session/latest-session recovery is unavailable")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	return cmd
}

func newRepromptChildCommand(d deps) *cobra.Command {
	opts := RepromptChildOptions{Attempt: 1}
	cmd := &cobra.Command{
		Use:   "reprompt-child --state-file <file> --plan-dir <path> --stage <node> --attempt <n>",
		Short: "Paste QRSPI correction prompt into the active child pane",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRepromptChild(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "QRSPI node ID being retried")
	cmd.Flags().IntVar(&opts.Attempt, "attempt", 1, "validation retry attempt number")
	cmd.Flags().StringVar(&opts.ErrorText, "error", "", "validation error text")
	cmd.Flags().
		StringVar(&opts.ErrorFile, "error-file", "", "file containing validation error text")
	return cmd
}

func newContinueCommand(d deps) *cobra.Command {
	opts := ContinueOptions{Split: "right", Timeout: 0, Output: "text"}
	var usagePercent float64
	var usageTokens int
	var usageWindow int
	var usageSource string
	cmd := &cobra.Command{
		Use:   "continue --state-file <file>",
		Short: "Validate active child and continue the QRSPI graph when safe",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Usage = usageFromChangedFlags(
				cmd,
				usagePercent,
				usageTokens,
				usageWindow,
				usageSource,
			)
			return RunContinue(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().
		StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory; defaults from state")
	cmd.Flags().
		StringVar(&opts.Stage, "stage", "", "expected active child node; defaults from state")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "next child cwd override")
	cmd.Flags().
		StringVar(&opts.Split, "split", "right", "tmux split direction for next child")
	cmd.Flags().
		StringVar(&opts.PiModel, "model", "", "Pi model pattern or ID for child sessions (passed to pi --model)")
	cmd.Flags().
		StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().
		DurationVar(&opts.Timeout, "timeout", 0, "maximum time to wait for next child; 0 returns after launch")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	cmd.Flags().
		Float64Var(&usagePercent, "manager-usage-percent", 0, "parent manager context usage percent for optional compaction")
	cmd.Flags().
		IntVar(&usageTokens, "manager-usage-tokens", 0, "parent manager context token count for optional compaction")
	cmd.Flags().
		IntVar(&usageWindow, "manager-usage-window", 0, "parent manager context window size for optional compaction")
	cmd.Flags().
		StringVar(&usageSource, "manager-usage-source", "", "diagnostic source for parent manager context usage")
	_ = cmd.Flags().MarkHidden("manager-usage-source")
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
	out = ensureWriter(out)

	projectRoot := strings.TrimSpace(opts.ProjectRoot)
	if projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		projectRoot = cwd
	}
	policy, err := initialPolicy(opts.PolicyFile, opts.PolicyPreset)
	if err != nil {
		return err
	}
	state, err := InitialManagerState(opts.PlanDir, projectRoot, policy)
	if err != nil {
		return err
	}
	if isFastPolicyPreset(opts.PolicyPreset) && strings.TrimSpace(opts.NodeID) == "" {
		opts.NodeID = string(qrspi.NodeOutline)
	}
	if err := ApplyInitOverrides(
		&state,
		InitOverrides{
			NodeID:            opts.NodeID,
			ImplementationCwd: opts.ImplementationCwd,
			PiModel:           opts.PiModel,
		},
	); err != nil {
		return err
	}
	state.ManagerPaneID = CaptureManagerPaneID(opts.ManagerPane)
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	state.ManagerRunID = managerRunID(clock())

	root, err := stateRoot(d)
	if err != nil {
		return err
	}
	store := stateStore(d, root, clock)
	key := LockKey{RepoID: state.RepoID, CanonicalPlanDir: state.CanonicalPlanDir}
	lock, err := store.AcquireLock(ctx, key, state.ManagerRunID, lockTTL)
	if err != nil {
		return err
	}
	stateFile := StatePath(root, key, state.ManagerRunID)
	if err := store.Save(stateFile, state); err != nil {
		return err
	}
	return WriteNDJSON(out, Event{
		Type: "initialized",
		Ref: map[string]any{
			"stateFile":    stateFile,
			"lockFile":     lock.Path,
			"managerRunId": state.ManagerRunID,
			"currentNode":  state.Workflow.CurrentNodeID,
		},
	})
}

func RunStartNext(
	ctx context.Context,
	opts StartNextOptions,
	d deps,
	out io.Writer,
) (*StartNextResult, error) {
	out = ensureWriter(out)
	if strings.TrimSpace(opts.StateFile) == "" && strings.TrimSpace(opts.PlanDir) == "" {
		return nil, errors.New("plan-dir is required")
	}
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	if strings.TrimSpace(opts.StateFile) == "" {
		root, err := stateRoot(d)
		if err != nil {
			return nil, err
		}
		preflight, err := CheckQRSPIPreflight(
			ctx,
			ManagerState{},
			PreflightOptions{StateRootPath: root, UsesExtension: true},
			d,
		)
		if err != nil {
			return nil, err
		}
		if card := BuildPreflightFailedCard(preflight.Pi, ""); card != nil {
			return nil, writeManagerActionCard(out, *card, opts.Output)
		}
		if !preflight.StateRoot.OK || !preflight.Tmux.OK {
			card := ManagerActionCard{
				Kind:     ActionPiCompatibilityFailed,
				Severity: "error",
				Summary:  "q-manager preflight failed",
				Evidence: append(
					preflight.StateRoot.Evidence,
					preflight.Tmux.Evidence...),
				RecommendedAction: "fix q-manager runtime dependencies before launching child",
				SafeCommand:       "vamos qrspi doctor",
				RequiresHuman:     false,
			}
			return nil, writeManagerActionCard(out, card, opts.Output)
		}
	}
	state, stateFile, err := resolveOrInitStartState(ctx, opts, d)
	if err != nil {
		return nil, err
	}
	store := stateStore(d, "", clock)
	result := StartNextResult{StateFile: stateFile}
	if strings.TrimSpace(opts.StateFile) != "" {
		var stopped bool
		state, stopped, err = applyManagerPaneAdoption(
			ctx,
			stateFile,
			state,
			ManagerPaneAdoptionOptions{
				Command:      ManagerPaneAdoptionStartNext,
				ExplicitPane: opts.ManagerPane,
				CurrentPane:  CaptureManagerPaneID(""),
			},
			store,
			d,
			out,
			opts.Output,
		)
		if err != nil {
			return nil, err
		}
		if stopped {
			return nil, nil
		}
	}
	if strings.TrimSpace(opts.StateFile) != "" {
		preflight, err := CheckQRSPIPreflight(
			ctx,
			state,
			PreflightOptions{
				StateFile:     stateFile,
				ManagerPaneID: state.ManagerPaneID,
				UsesExtension: true,
			},
			d,
		)
		if err != nil {
			return nil, err
		}
		if card := BuildPreflightFailedCard(preflight.Pi, stateFile); card != nil {
			state.LastActionCard = card
			_ = store.Save(stateFile, state)
			return nil, writeManagerActionCard(out, *card, opts.Output)
		}
	}
	if state.ActiveChild != nil {
		health, err := InspectActiveChildHealth(ctx, state, stateFile, d)
		if err != nil {
			return nil, err
		}
		if opts.Force && IsTerminalFailedChild(health) {
			state, err = ClearFailedActiveChild(state, health)
			if err != nil {
				return nil, err
			}
			if err := store.Save(stateFile, state); err != nil {
				return nil, err
			}
		} else {
			result.ActiveChild = state.ActiveChild
			result.CurrentNode = state.ActiveChild.Stage
			result.StopReason = "active child already running"
			if health.Status != ActiveChildRunning {
				result.StopReason = string(health.Status)
			}
			result.NextCommand = continueCommand(stateFile)
			result.FeedbackCommand = feedbackCommand(stateFile)
			return &result, writeStartNextOutput(out, result, opts.Output)
		}
	}
	if seed, err := readLatestResultSeed(opts); err != nil {
		return nil, err
	} else if strings.TrimSpace(seed) != "" {
		parsed, err := applyLatestResultSeed(&state, seed)
		if err != nil {
			return nil, err
		}
		if err := store.Save(stateFile, state); err != nil {
			return nil, err
		}
		result.CurrentNode = string(parsed.Result.SourceNodeID)
		result.StopReason = parsed.Decision.StopReason
		if parsed.Decision.WaitingHuman {
			result.StopReason = "result requested human input"
			result.NextCommand = feedbackCommand(stateFile)
			return &result, writeStartNextOutput(out, result, opts.Output)
		}
		if !parsed.Decision.StartNext {
			if result.StopReason == "" {
				result.StopReason = "next node ready; advance mode does not auto-launch"
			}
			if parsed.Decision.NextNodeID != "" {
				result.CurrentNode = string(parsed.Decision.NextNodeID)
			}
			result.NextCommand = fmt.Sprintf(
				"vamos qrspi start-next --state-file %s",
				stateFile,
			)
			return &result, writeStartNextOutput(out, result, opts.Output)
		}
	}
	node, err := selectLaunchNode(state, opts)
	if err != nil {
		return nil, err
	}
	result.CurrentNode = string(node.ID)
	promptFile, err := WriteStagePromptFile(
		ctx,
		state,
		node,
		PromptFileOptions{
			StateFile: stateFile,
			NodeID:    string(node.ID),
			Timestamp: clock(),
		},
	)
	if err != nil {
		return nil, err
	}
	result.PromptFile = promptFile
	cwd, err := defaultChildCwd(state, node.ID, opts.Cwd)
	if err != nil {
		return nil, err
	}
	runOut := io.Writer(io.Discard)
	if strings.EqualFold(opts.Output, "ndjson") {
		runOut = out
	}
	if err := RunChild(ctx, RunChildOptions{
		PlanDir:     planDirForStart(state, opts),
		Stage:       string(node.ID),
		Cwd:         cwd,
		PromptFile:  promptFile,
		StateFile:   stateFile,
		Split:       opts.Split,
		PiModel:     resolvePiModel(opts.PiModel, state.PiModel),
		ManagerPane: opts.ManagerPane,
		Timeout:     opts.Timeout,
	}, d, runOut); err != nil {
		return nil, err
	}
	launched, err := store.Load(stateFile)
	if err != nil {
		return nil, err
	}
	launched, _, err = maybeStartManagerCompaction(
		ctx,
		launched,
		stateFile,
		opts.Usage,
		d,
		out,
	)
	if err != nil {
		return nil, err
	}
	result.ActiveChild = launched.ActiveChild
	result.NextCommand = continueCommand(stateFile)
	result.FeedbackCommand = feedbackCommand(stateFile)
	return &result, writeStartNextOutput(out, result, opts.Output)
}

func applyManagerPaneAdoption(
	ctx context.Context,
	stateFile string,
	state ManagerState,
	opts ManagerPaneAdoptionOptions,
	store StateStore,
	d deps,
	out io.Writer,
	output string,
) (ManagerState, bool, error) {
	opts.StateFile = stateFile
	adoption, err := ResolveManagerPaneAdoption(ctx, state, opts, d)
	if err != nil {
		return state, false, err
	}
	state = adoption.State
	if adoption.ActionCard != nil {
		state.LastActionCard = adoption.ActionCard
		if err := store.Save(stateFile, state); err != nil {
			return state, true, err
		}
		return state, true, writeManagerActionCard(out, *adoption.ActionCard, output)
	}
	if adoption.Changed {
		if err := store.Save(stateFile, state); err != nil {
			return state, false, err
		}
	}
	return state, false, nil
}

func readLatestResultSeed(opts StartNextOptions) (string, error) {
	if opts.LatestResultStdin && strings.TrimSpace(opts.LatestResultFile) != "" {
		return "", errors.New(
			"use only one of --latest-result-stdin or --latest-result-file",
		)
	}
	if strings.TrimSpace(opts.LatestResultFile) != "" {
		data, err := os.ReadFile(opts.LatestResultFile)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if opts.LatestResultStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", nil
}

func applyLatestResultSeed(
	state *ManagerState,
	resultText string,
) (*ParsedDecision, error) {
	if state == nil {
		return nil, errors.New("state is required")
	}
	parsed, err := ParseNormalizeValidateDecide(
		resultText,
		*state,
		wruntime.ParseContext{ExpectedNodeID: state.Workflow.CurrentNodeID},
	)
	if err != nil {
		return nil, err
	}
	state.Workflow = parsed.Decision.State
	*state = UpdateImplementationCwd(*state, parsed.Result)
	if parsed.Decision.StartNext {
		*state = markPendingCleanup(*state)
	}
	return &parsed, nil
}

func writeStartNextOutput(out io.Writer, result StartNextResult, mode string) error {
	notice := startNextNotice(result)
	if strings.EqualFold(mode, "ndjson") {
		return writeManagerNotice(out, notice, mode)
	}
	return writeStartNextText(out, result)
}

func startNextNotice(result StartNextResult) ManagerNotice {
	notice := ManagerNotice{
		Kind:            "launched",
		StateFile:       result.StateFile,
		Stage:           result.CurrentNode,
		Summary:         result.StopReason,
		NextCommand:     result.NextCommand,
		FeedbackCommand: result.FeedbackCommand,
	}
	if result.ActiveChild != nil {
		notice.ChildPane = result.ActiveChild.TmuxPaneID
		if notice.Stage == "" {
			notice.Stage = result.ActiveChild.Stage
		}
	}
	if result.ActiveChild != nil && result.StopReason == "active child already running" {
		notice.Kind = "active_child"
		notice.Summary = "active child already running; do not launch duplicate child"
	}
	if result.StopReason != "" && result.ActiveChild == nil {
		notice.Kind = "validated"
		notice.Validated = true
		notice.ManagerNeeded = true
	}
	return notice
}

func writeStartNextText(out io.Writer, result StartNextResult) error {
	if result.StateFile != "" {
		if _, err := fmt.Fprintf(out, "state: %s\n", result.StateFile); err != nil {
			return err
		}
	}
	if result.ActiveChild != nil && result.StopReason == "active child already running" {
		if _, err := fmt.Fprintf(
			out,
			"active child: %s (%s), session %s\n",
			result.ActiveChild.Stage,
			result.ActiveChild.TmuxPaneID,
			emptyAsPending(result.ActiveChild.SessionID),
		); err != nil {
			return err
		}
		_, err := fmt.Fprintln(
			out,
			"next: wait for validated wake or inspect active child; do not launch duplicate child",
		)
		return err
	}
	if result.CurrentNode != "" {
		if _, err := fmt.Fprintf(out, "node: %s\n", result.CurrentNode); err != nil {
			return err
		}
	}
	if result.PromptFile != "" {
		if _, err := fmt.Fprintf(out, "prompt: %s\n", result.PromptFile); err != nil {
			return err
		}
	}
	if result.ActiveChild != nil {
		if _, err := fmt.Fprintf(
			out,
			"started child: %s (%s)\n",
			result.ActiveChild.Stage,
			result.ActiveChild.TmuxPaneID,
		); err != nil {
			return err
		}
	}
	if result.StopReason != "" && result.ActiveChild == nil {
		if _, err := fmt.Fprintf(out, "stop: %s\n", result.StopReason); err != nil {
			return err
		}
	}
	if result.NextCommand != "" {
		if _, err := fmt.Fprintf(out, "next: %s\n", result.NextCommand); err != nil {
			return err
		}
	}
	if result.FeedbackCommand != "" {
		if _, err := fmt.Fprintf(
			out,
			"feedback: %s\n",
			result.FeedbackCommand,
		); err != nil {
			return err
		}
	}
	return nil
}

func startNextRef(result StartNextResult) map[string]any {
	ref := map[string]any{"stateFile": result.StateFile}
	if result.CurrentNode != "" {
		ref["currentNode"] = result.CurrentNode
	}
	if result.PromptFile != "" {
		ref["promptFile"] = result.PromptFile
	}
	if result.ActiveChild != nil {
		ref["activeChild"] = childRef(result.ActiveChild)
	}
	if result.StopReason != "" {
		ref["stopReason"] = result.StopReason
	}
	if result.NextCommand != "" {
		ref["nextCommand"] = result.NextCommand
	}
	if result.FeedbackCommand != "" {
		ref["feedbackCommand"] = result.FeedbackCommand
	}
	return ref
}

func continueCommand(stateFile string) string {
	return fmt.Sprintf("vamos qrspi continue --state-file %s", stateFile)
}

func feedbackCommand(stateFile string) string {
	return fmt.Sprintf(
		"vamos qrspi steer-child --state-file %s --feedback-file <file>",
		stateFile,
	)
}

func RunSetPolicy(
	ctx context.Context,
	opts SetPolicyOptions,
	d deps,
	out io.Writer,
) error {
	_ = ctx
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	policy, err := policyFromSetOptions(opts, qrspi.ParsePolicy(state.Workflow.Policy))
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	state.Workflow.Policy = encoded
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	summary := policySummary(policy)
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(
			out,
			Event{Type: "policy_updated", Ref: map[string]any{"policy": summary}},
		)
	}
	fmt.Fprintf(
		out,
		"policy: %s, plan reviews %s, retries %d\n",
		summary.AdvanceMode,
		onOff(summary.EnablePlanReviews),
		summary.InvalidResultRetryLimit,
	)
	fmt.Fprintf(out, "next: %s\n", continueCommand(opts.StateFile))
	return nil
}

func isFastPolicyPreset(preset string) bool {
	return strings.TrimSpace(strings.ToLower(preset)) == "fast"
}

func policyFromSetOptions(
	opts SetPolicyOptions,
	current qrspi.Policy,
) (qrspi.Policy, error) {
	policy := current
	switch strings.TrimSpace(strings.ToLower(opts.Preset)) {
	case "":
	case "discuss":
		policy = qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeDiscuss,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		}
	case "guided":
		policy = qrspi.DefaultPolicy()
	case "autopilot":
		policy = qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeAutopilot,
			AutoMode:                true,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		}
	case "fast", "autopilot-no-plan-reviews", "autopilot_no_plan_reviews":
		policy = qrspi.Policy{
			AdvanceMode:             qrspi.AdvanceModeAutopilot,
			AutoMode:                true,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 1,
		}
	default:
		return qrspi.Policy{}, fmt.Errorf("unknown policy preset %q", opts.Preset)
	}
	if mode := strings.TrimSpace(opts.AdvanceMode); mode != "" {
		policy.AdvanceMode = qrspi.AdvanceMode(mode)
	}
	if opts.EnablePlanReviewsSet {
		policy.EnablePlanReviews = opts.EnablePlanReviews
	}
	if opts.InvalidRetryLimitSet {
		policy.InvalidResultRetryLimit = opts.InvalidResultRetryLimit
	}
	policy.AutoMode = policy.EffectiveAdvanceMode() == qrspi.AdvanceModeAutopilot
	if err := qrspi.ValidateConfig(policy); err != nil {
		return qrspi.Policy{}, err
	}
	return policy, nil
}

func policySummary(policy qrspi.Policy) PolicySummary {
	return PolicySummary{
		AdvanceMode:             string(policy.EffectiveAdvanceMode()),
		AutoMode:                policy.IsAutoMode(),
		EnablePlanReviews:       policy.EnablePlanReviews,
		InvalidResultRetryLimit: policy.InvalidResultRetryLimit,
	}
}

func activePolicySummary(state ManagerState) PolicySummary {
	return policySummary(qrspi.ParsePolicy(state.Workflow.Policy))
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func ChildWorkspaceSessionDir(state ManagerState, opts RunChildOptions) string {
	planDir := strings.TrimSpace(state.CanonicalPlanDir)
	if planDir == "" {
		planDir = strings.TrimSpace(opts.PlanDir)
	}
	if planDir != "" {
		return filepath.Join(planDir, ".sessions", "pi")
	}
	if cwd := strings.TrimSpace(opts.Cwd); cwd != "" {
		return filepath.Join(cwd, ".sessions", "pi")
	}
	return SessionDir(filepath.Dir(opts.StateFile), childRunID(opts.Stage, time.Now()))
}

func debugCommandForState(stateFile, action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "continue"
	}
	return fmt.Sprintf("vamos qrspi %s --state-file %s", action, stateFile)
}

func writeManagerNotice(out io.Writer, notice ManagerNotice, mode string) error {
	if strings.EqualFold(mode, "ndjson") {
		return WriteNDJSON(out, Event{Type: "manager_notice", Ref: noticeRef(notice)})
	}
	if notice.Validated {
		if _, err := fmt.Fprintf(
			out,
			"validated: %s %s\n",
			notice.Stage,
			notice.Status,
		); err != nil {
			return err
		}
		if notice.Outcome != "" {
			if _, err := fmt.Fprintf(out, "outcome: %s\n", notice.Outcome); err != nil {
				return err
			}
		}
	}
	if notice.Artifact != "" {
		if _, err := fmt.Fprintf(out, "artifact: %s\n", notice.Artifact); err != nil {
			return err
		}
	}
	if notice.Policy.AdvanceMode != "" {
		if _, err := fmt.Fprintf(
			out,
			"policy: %s, plan reviews %s, retries %d\n",
			notice.Policy.AdvanceMode,
			onOff(notice.Policy.EnablePlanReviews),
			notice.Policy.InvalidResultRetryLimit,
		); err != nil {
			return err
		}
	}
	if notice.NextChild.Stage != "" {
		if _, err := fmt.Fprintf(
			out,
			"next child: %s\n",
			notice.NextChild.Stage,
		); err != nil {
			return err
		}
		if notice.NextChild.WorkingOn != "" {
			if _, err := fmt.Fprintf(
				out,
				"working on: %s\n",
				notice.NextChild.WorkingOn,
			); err != nil {
				return err
			}
		}
		if notice.NextChild.Cwd != "" {
			if _, err := fmt.Fprintf(out, "cwd: %s\n", notice.NextChild.Cwd); err != nil {
				return err
			}
		}
	}
	if notice.ChildPane != "" && notice.Kind == "active_child" {
		if _, err := fmt.Fprintf(
			out,
			"active child: %s (%s)\n",
			notice.Stage,
			notice.ChildPane,
		); err != nil {
			return err
		}
	}
	if notice.RetryExhausted {
		if _, err := fmt.Fprintln(out, "retry: exhausted"); err != nil {
			return err
		}
	}
	if notice.Summary != "" {
		if _, err := fmt.Fprintf(out, "stop: %s\n", notice.Summary); err != nil {
			return err
		}
	}
	if notice.Summary != "" && notice.Validated {
		if _, err := fmt.Fprintf(out, "summary: %s\n", notice.Summary); err != nil {
			return err
		}
	}
	if notice.ManagerGuidance != "" {
		if _, err := fmt.Fprintf(
			out,
			"guidance: %s\n",
			notice.ManagerGuidance,
		); err != nil {
			return err
		}
	}
	if notice.NextCommand != "" {
		if _, err := fmt.Fprintf(out, "next: %s\n", notice.NextCommand); err != nil {
			return err
		}
	}
	if notice.FeedbackCommand != "" {
		if _, err := fmt.Fprintf(
			out,
			"feedback: %s\n",
			notice.FeedbackCommand,
		); err != nil {
			return err
		}
	}
	return nil
}

func noticeRef(notice ManagerNotice) map[string]any {
	ref := map[string]any{
		"kind":           notice.Kind,
		"validated":      notice.Validated,
		"managerNeeded":  notice.ManagerNeeded,
		"retryExhausted": notice.RetryExhausted,
	}
	if notice.StateFile != "" {
		ref["stateFile"] = notice.StateFile
	}
	if notice.Stage != "" {
		ref["stage"] = notice.Stage
	}
	if notice.Status != "" {
		ref["status"] = notice.Status
	}
	if notice.Outcome != "" {
		ref["outcome"] = notice.Outcome
	}
	if notice.Policy.AdvanceMode != "" {
		ref["policy"] = notice.Policy
	}
	if notice.NextChild.Stage != "" {
		ref["nextChild"] = notice.NextChild
	}
	if notice.Artifact != "" {
		ref["artifact"] = notice.Artifact
	}
	if notice.ChildPane != "" {
		ref["childPane"] = notice.ChildPane
	}
	if notice.Summary != "" {
		ref["summary"] = notice.Summary
	}
	if notice.ManagerGuidance != "" {
		ref["managerGuidance"] = notice.ManagerGuidance
	}
	if notice.NextCommand != "" {
		ref["nextCommand"] = notice.NextCommand
	}
	if notice.FeedbackCommand != "" {
		ref["feedbackCommand"] = notice.FeedbackCommand
	}
	return ref
}

func appendValidationRecoveryLog(stateFile string, entry ValidationRecoveryLog) error {
	stateFile = strings.TrimSpace(stateFile)
	if stateFile == "" {
		return nil
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	path := filepath.Join(filepath.Dir(stateFile), "validation-recoveries.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func planDirForStart(state ManagerState, opts StartNextOptions) string {
	if strings.TrimSpace(opts.PlanDir) != "" {
		return opts.PlanDir
	}
	return state.CanonicalPlanDir
}

func emptyAsPending(value string) string {
	if strings.TrimSpace(value) == "" {
		return "pending"
	}
	return value
}

func resolvePiModel(preferred, fallback string) string {
	if model := strings.TrimSpace(preferred); model != "" {
		return model
	}
	return strings.TrimSpace(fallback)
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
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if _, err := os.Stat(opts.PromptFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("prompt-file does not exist: %s", opts.PromptFile)
		}
		return err
	}
	out = ensureWriter(out)
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	piModel := resolvePiModel(opts.PiModel, state.PiModel)
	if strings.TrimSpace(opts.PiModel) != "" {
		state.PiModel = piModel
	}
	parentPaneID := strings.TrimSpace(opts.ManagerPane)
	if parentPaneID == "" {
		parentPaneID = strings.TrimSpace(state.ManagerPaneID)
	}
	if parentPaneID == "" {
		parentPaneID = CaptureManagerPaneID("")
	}
	if parentPaneID != "" && state.ManagerPaneID == "" {
		state.ManagerPaneID = parentPaneID
	}
	childID := nextChildRunID(state, opts.Stage, clock())
	runRoot := filepath.Dir(opts.StateFile)
	extensionPath, err := ResolveChildExtensionPath(runRoot)
	if err != nil {
		return err
	}
	managerRunID := strings.TrimSpace(opts.ManagerRunID)
	if managerRunID == "" {
		managerRunID = state.ManagerRunID
	}
	sessionDir := ChildWorkspaceSessionDir(state, opts)
	req := ChildRunRequest{
		ID:                   childID,
		Stage:                opts.Stage,
		Cwd:                  opts.Cwd,
		ManagerRunID:         managerRunID,
		PromptFile:           opts.PromptFile,
		OutputPath:           OutputPath(runRoot, childID),
		SessionID:            ChildSessionID(childID),
		SessionDir:           sessionDir,
		SessionName:          fmt.Sprintf("q-manager %s %s", opts.Stage, childID),
		DonePath:             DonePath(runRoot, childID),
		StatusPath:           StatusPath(runRoot, childID),
		ValidationStatusPath: ValidationStatusPath(runRoot, childID),
		Split:                normalizeSplit(opts.Split),
		ParentPaneID:         parentPaneID,
		StateFile:            opts.StateFile,
		PlanDir:              opts.PlanDir,
		ExtensionPath:        extensionPath,
		PiModel:              piModel,
	}
	if err := ensureRunFiles(req); err != nil {
		return err
	}
	runner := childRunner(d)
	run, err := runner.Start(ctx, req)
	if err != nil {
		return err
	}
	replacement := &ChildRunRef{
		ID:                   childID,
		Stage:                opts.Stage,
		Cwd:                  opts.Cwd,
		TmuxPaneID:           run.Pane.ID,
		OutputPath:           req.OutputPath,
		SessionID:            req.SessionID,
		SessionDir:           req.SessionDir,
		DonePath:             req.DonePath,
		StatusPath:           req.StatusPath,
		ValidationStatusPath: req.ValidationStatusPath,
		LifecycleStatus:      "running",
		Generation:           1,
	}
	if opts.Launch != nil {
		replacement.LaunchKind = opts.Launch.Kind
		if opts.Launch.Kind == ChildLaunchResumeHandoff {
			replacement.ContinuationOf = opts.Launch.SourceChildID
			replacement.ContinuationArtifact = opts.Launch.PrimaryArtifact
			replacement.ContinuationDeliveryID = opts.Launch.DeliveryID
		}
	}
	state.ActiveChild = replacement
	if err := store.Save(opts.StateFile, state); err != nil {
		cleanupErr := tmuxClient(d).KillPane(ctx, run.Pane)
		if cleanupErr != nil {
			return fmt.Errorf("persist started child: %w (cleanup untracked pane %s: %v)", err, run.Pane.ID, cleanupErr)
		}
		return fmt.Errorf("persist started child: %w", err)
	}
	if err := WriteNDJSON(
		out,
		Event{Type: "child_started", Ref: childRef(state.ActiveChild)},
	); err != nil {
		return err
	}
	if state.PendingCleanupChild != nil && !opts.DeferPendingCleanup {
		pending := state.PendingCleanupChild
		cleaned, cleanupErr := cleanupPendingChildAfterNextStart(ctx, state, d.Tmux)
		if cleanupErr != nil {
			if err := WriteNDJSON(
				out,
				Event{
					Type:  "child_cleanup_failed",
					Ref:   childRef(pending),
					Error: cleanupErr.Error(),
				},
			); err != nil {
				return err
			}
		} else {
			state = cleaned
			if err := store.Save(opts.StateFile, state); err != nil {
				return err
			}
			if err := WriteNDJSON(
				out,
				Event{Type: "child_cleaned", Ref: childRef(pending)},
			); err != nil {
				return err
			}
		}
	}
	if opts.Timeout == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if _, err := runner.Wait(waitCtx, run); err != nil {
		return err
	}
	if err := waitForDone(waitCtx, req.DonePath, 100*time.Millisecond); err != nil {
		return fmt.Errorf(
			"timed out waiting for child done marker %s (pane %s, output %s, sessionDir %s, sessionID %s): %w",
			req.DonePath,
			run.Pane.ID,
			req.OutputPath,
			req.SessionDir,
			req.SessionID,
			err,
		)
	}
	sessionPath, err := ResolveSessionPath(req.SessionDir, req.SessionID, req.Cwd)
	if err != nil {
		return fmt.Errorf(
			"resolve child session path after done marker %s (pane %s, output %s, sessionDir %s, sessionID %s): %w",
			req.DonePath,
			run.Pane.ID,
			req.OutputPath,
			req.SessionDir,
			req.SessionID,
			err,
		)
	}
	state.ActiveChild.SessionPath = sessionPath
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	return WriteNDJSON(
		out,
		Event{Type: "child_finished", Ref: childRef(state.ActiveChild)},
	)
}

func RunChildComplete(
	ctx context.Context,
	opts ChildCompletionOptions,
	d deps,
	out io.Writer,
) (*ChildCompletionStatus, error) {
	if strings.TrimSpace(opts.StateFile) == "" {
		return nil, errors.New("state-file is required")
	}
	out = ensureWriter(out)
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	operationLock, err := store.AcquireOperationLock(ctx, opts.StateFile)
	if err != nil {
		return nil, err
	}
	defer operationLock.Release()
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return nil, err
	}
	requestedChildID := strings.TrimSpace(opts.ChildID)
	if requestedChildID != "" {
		if continuation, ok := existingHandoffContinuationForSource(state, requestedChildID); ok {
			status := handoffContinuationStatus(state, requestedChildID, *continuation)
			if err := writeChildCompletionOutput(out, opts.Output, status); err != nil {
				return nil, err
			}
			return &status, nil
		}
	}
	if state.ActiveChild == nil {
		return nil, errors.New("no active child to complete")
	}
	if requestedChildID != "" && state.ActiveChild.ID != requestedChildID {
		return nil, fmt.Errorf(
			"active child %q does not match requested child %q",
			state.ActiveChild.ID,
			requestedChildID,
		)
	}
	child := state.ActiveChild
	status := ChildCompletionStatus{
		ChildID:    child.ID,
		Attempt:    child.ValidationRetryCount,
		RetryLimit: invalidResultRetryLimit(state),
	}
	if evidence, ok, evidenceErr := terminalEvidenceForActiveChildWithRefresh(
		state,
	); evidenceErr == nil && ok &&
		evidence.ContextWindowError {
		status = childCompletionStatusFromTerminalEvidence(state, *child, evidence)
		state.ActiveChild.LifecycleStatus = "awaiting_manager"
		state.ActiveChild.LastDeliveryID = status.DeliveryID
		health := ActiveChildHealth{
			Status:           ActiveChildProviderContextError,
			ChildID:          child.ID,
			Stage:            child.Stage,
			PaneID:           child.TmuxPaneID,
			SessionDir:       child.SessionDir,
			SessionPath:      evidence.SessionPath,
			TerminalEvidence: &evidence,
			Evidence:         providerContextEvidenceLines(evidence),
		}
		status.ActionCard = BuildChildContextExhaustedCard(health, state, opts.StateFile)
		state.LastActionCard = status.ActionCard
		state, status.Wake, err = queueOrDeliverWake(
			ctx,
			opts.StateFile,
			state,
			status,
			d,
		)
		if err != nil {
			return nil, err
		}
	} else {
		text, parseCtx, readErr := ReadChildResultText(state, ResultSourceOptions{})
		err = readErr
		if err == nil {
			parseCtx.ExpectedNodeID = wruntime.NodeID(child.Stage)
			parsed, parseErr := ParseNormalizeValidateDecide(text, state, parseCtx)
			if parseErr == nil {
				status.Validated = true
				status.DeliveryID = childCompletionDeliveryID(*child, &parsed, false)
				status.Result = childCompletionResult(parsed.Result)
				status.NextChild = nextChildInfo(state, parsed.Decision.NextNodeID)
				status.ManagerNeeded = childCompletionManagerNeeded(status.Result.Status)
				status.Normalizations = parsed.Normalizations
				if parsed.Result.Status == wruntime.StatusHandoff && parsed.Decision.StartNext {
					intent, intentErr := deriveChildLaunchIntent(state, *child, parsed.Result, parsed.Decision)
					if intentErr != nil {
						status.ManagerNeeded = true
						status.Reason = intentErr.Error()
						status.ActionCard = buildInvalidHandoffArtifactCard(
							opts.StateFile,
							*child,
							parsed.Result,
							intentErr,
						)
						state.LastActionCard = status.ActionCard
						state.ActiveChild.LifecycleStatus = "awaiting_manager"
						state.ActiveChild.LastDeliveryID = status.DeliveryID
						state, status.Wake, err = queueOrDeliverWake(
							ctx,
							opts.StateFile,
							state,
							status,
							d,
						)
					} else {
						state, status, err = completeHandoffContinuation(
							ctx,
							opts,
							state,
							*child,
							parsed,
							intent,
							store,
							d,
						)
					}
				} else {
					if parsed.Result.Status == wruntime.StatusHandoff {
						status.ManagerNeeded = true
						status.Reason = "handoff_waiting_for_manager_policy"
						state.ActiveChild.LifecycleStatus = "awaiting_manager"
					} else {
						state.ActiveChild.LifecycleStatus = "completed"
					}
					state.ActiveChild.LastDeliveryID = status.DeliveryID
					state, status.Wake, err = queueOrDeliverWake(
						ctx,
						opts.StateFile,
						state,
						status,
						d,
					)
				}
				if err != nil {
					return nil, err
				}
			} else {
				err = parseErr
			}
		}
		if err != nil {
			if shouldRepromptAfterValidationError(
				state,
				ContinueOptions{
					StateFile: opts.StateFile,
					PlanDir:   state.CanonicalPlanDir,
					Stage:     child.Stage,
				},
				err,
			) {
				attempt := child.ValidationRetryCount + 1
				if repromptErr := continueReprompt(
					ctx,
					state,
					ContinueOptions{
						StateFile: opts.StateFile,
						PlanDir:   state.CanonicalPlanDir,
						Stage:     child.Stage,
					},
					d,
					io.Discard,
					err,
				); repromptErr != nil {
					return nil, repromptErr
				}
				latest, loadErr := store.Load(opts.StateFile)
				if loadErr != nil {
					return nil, loadErr
				}
				state = latest
				child = state.ActiveChild
				status.ChildID = child.ID
				status.Attempt = attempt
				status.RetryLimit = invalidResultRetryLimit(state)
				status.Reason = "retryable_invalid_result"
				status.Wake = WakeDeliveryInstruction{
					Mode:   "suppress",
					Reason: "retryable_invalid_result",
				}
			} else if isRetryExhaustedValidationError(
				state,
				ContinueOptions{
					StateFile: opts.StateFile,
					PlanDir:   state.CanonicalPlanDir,
					Stage:     child.Stage,
				},
				err,
			) {
				status.Validated = false
				status.ManagerNeeded = true
				status.RetryExhausted = true
				status.DeliveryID = childCompletionDeliveryID(*child, nil, true)
				status.Result = ChildCompletionResult{
					Stage:   child.Stage,
					Status:  "invalid_result",
					Summary: err.Error(),
				}
				status.Reason = err.Error()
				state.ActiveChild.LifecycleStatus = "awaiting_manager"
				state.ActiveChild.LastDeliveryID = status.DeliveryID
				state, status.Wake, err = queueOrDeliverWake(
					ctx,
					opts.StateFile,
					state,
					status,
					d,
				)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}
	if child != nil && strings.TrimSpace(child.ValidationStatusPath) != "" {
		if writeErr := writeValidationStatus(child.ValidationStatusPath, status); writeErr != nil {
			return nil, writeErr
		}
	}
	if saveErr := store.Save(opts.StateFile, state); saveErr != nil {
		return nil, saveErr
	}
	if err := writeChildCompletionOutput(out, opts.Output, status); err != nil {
		return nil, err
	}
	return &status, nil
}

func writeChildCompletionOutput(
	out io.Writer,
	mode string,
	status ChildCompletionStatus,
) error {
	if strings.EqualFold(mode, "json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	_, err := fmt.Fprintf(
		out,
		"child completion: %s\nwake: %s\n",
		status.Result.Status,
		status.Wake.Mode,
	)
	return err
}

func completeHandoffContinuation(
	ctx context.Context,
	opts ChildCompletionOptions,
	state ManagerState,
	source ChildRunRef,
	parsed ParsedDecision,
	intent ChildLaunchIntent,
	store StateStore,
	d deps,
) (ManagerState, ChildCompletionStatus, error) {
	status := ChildCompletionStatus{
		Validated:           true,
		ManagerNeeded:       false,
		ContinuationStarted: true,
		ChildID:             source.ID,
		DeliveryID:          intent.DeliveryID,
		Result:              childCompletionResult(parsed.Result),
		NextChild:           nextChildInfoFromIntent(intent),
		Normalizations:      parsed.Normalizations,
		Reason:              "handoff_auto_resumed",
		Attempt:             source.ValidationRetryCount,
		RetryLimit:          invalidResultRetryLimit(state),
	}
	state.Workflow = parsed.Decision.State
	state = UpdateImplementationCwd(state, parsed.Result)
	if state.ActiveChild == nil || state.ActiveChild.ID != source.ID {
		return state, ChildCompletionStatus{}, errors.New("source child changed before handoff continuation claim")
	}
	state.ActiveChild.LifecycleStatus = "continuing"
	state.ActiveChild.LastDeliveryID = intent.DeliveryID
	state = markPendingCleanup(state)
	if err := store.Save(opts.StateFile, state); err != nil {
		return recordHandoffContinuationFailure(
			ctx,
			opts.StateFile,
			state,
			source,
			parsed,
			intent,
			fmt.Errorf("persist handoff continuation claim: %w", err),
			store,
			d,
		)
	}

	launched, err := startChildFromIntent(
		ctx,
		state,
		intent,
		ContinueOptions{
			StateFile: opts.StateFile,
			PlanDir:   state.CanonicalPlanDir,
		},
		true,
		d,
		io.Discard,
	)
	if err != nil {
		return recordHandoffContinuationFailure(
			ctx,
			opts.StateFile,
			state,
			source,
			parsed,
			intent,
			err,
			store,
			d,
		)
	}
	if continuation, ok := existingHandoffContinuation(
		launched,
		source.ID,
		intent.DeliveryID,
	); !ok || continuation.ID == source.ID {
		return recordHandoffContinuationFailure(
			ctx,
			opts.StateFile,
			launched,
			source,
			parsed,
			intent,
			errors.New("handoff replacement was not persisted with source lineage"),
			store,
			d,
		)
	}
	if err := writeValidationStatus(source.ValidationStatusPath, status); err != nil {
		return launched, ChildCompletionStatus{}, err
	}
	launched, status.Wake, err = queueOrDeliverWake(
		ctx,
		opts.StateFile,
		launched,
		status,
		d,
	)
	if err != nil {
		return launched, ChildCompletionStatus{}, err
	}
	if err := store.Save(opts.StateFile, launched); err != nil {
		return launched, ChildCompletionStatus{}, err
	}
	cleaned, cleanupErr := cleanupPendingChildAfterNotification(ctx, launched, d.Tmux)
	if cleanupErr != nil {
		cleaned.LastActionCard = buildPendingChildCleanupFailureCard(
			opts.StateFile,
			cleaned.PendingCleanupChild,
			cleanupErr,
		)
	}
	if err := store.Save(opts.StateFile, cleaned); err != nil {
		return cleaned, ChildCompletionStatus{}, err
	}
	return cleaned, status, nil
}

func recordHandoffContinuationFailure(
	ctx context.Context,
	stateFile string,
	state ManagerState,
	source ChildRunRef,
	parsed ParsedDecision,
	intent ChildLaunchIntent,
	launchErr error,
	store StateStore,
	d deps,
) (ManagerState, ChildCompletionStatus, error) {
	if durable, err := store.Load(stateFile); err == nil {
		state = durable
	}
	status := ChildCompletionStatus{
		Validated:      true,
		ManagerNeeded:  true,
		ChildID:        source.ID,
		DeliveryID:     intent.DeliveryID + ":continuation_failed",
		Result:         childCompletionResult(parsed.Result),
		Normalizations: parsed.Normalizations,
		Reason:         "handoff_continuation_failed",
		Attempt:        source.ValidationRetryCount,
		RetryLimit:     invalidResultRetryLimit(state),
	}
	status.ActionCard = buildHandoffContinuationFailureCard(
		stateFile,
		source,
		parsed.Result,
		launchErr,
	)
	state.LastActionCard = status.ActionCard
	if state.ActiveChild != nil && state.ActiveChild.ID == source.ID {
		state.ActiveChild.LifecycleStatus = "awaiting_manager"
		state.ActiveChild.LastDeliveryID = status.DeliveryID
	}
	state, status.Wake, _ = queueOrDeliverWake(ctx, stateFile, state, status, d)
	if err := store.Save(stateFile, state); err != nil {
		return state, ChildCompletionStatus{}, err
	}
	return state, status, nil
}

func nextChildInfoFromIntent(intent ChildLaunchIntent) NextChildInfo {
	return NextChildInfo{
		Stage:     string(intent.NodeID),
		Skill:     intent.SkillPath,
		Cwd:       intent.Cwd,
		WorkingOn: nextChildWorkingOn(intent.NodeID),
	}
}

func existingHandoffContinuationForSource(
	state ManagerState,
	sourceChildID string,
) (*ChildRunRef, bool) {
	for _, candidate := range []*ChildRunRef{state.ActiveChild, state.PendingCleanupChild} {
		if candidate == nil || candidate.LaunchKind != ChildLaunchResumeHandoff ||
			candidate.ContinuationOf != sourceChildID ||
			strings.TrimSpace(candidate.ContinuationDeliveryID) == "" {
			continue
		}
		copy := *candidate
		return &copy, true
	}
	return nil, false
}

func handoffContinuationStatus(
	state ManagerState,
	sourceChildID string,
	continuation ChildRunRef,
) ChildCompletionStatus {
	status := ChildCompletionStatus{
		Validated:           true,
		ContinuationStarted: true,
		ChildID:             sourceChildID,
		DeliveryID:          continuation.ContinuationDeliveryID,
		Result:              childCompletionResultFromSnapshot(state.Workflow.LastResult),
		NextChild: NextChildInfo{
			Stage:     continuation.Stage,
			Skill:     ".pi/skills/q-resume/SKILL.md",
			Cwd:       continuation.Cwd,
			WorkingOn: nextChildWorkingOn(wruntime.NodeID(continuation.Stage)),
		},
		Wake: WakeDeliveryInstruction{
			Mode:   "suppress",
			Reason: "existing_handoff_continuation",
		},
		Reason:     "handoff_auto_resumed",
		RetryLimit: invalidResultRetryLimit(state),
	}
	if status.Result.Artifact == "" {
		status.Result = ChildCompletionResult{
			Stage:    continuation.Stage,
			Status:   string(wruntime.StatusHandoff),
			Artifact: continuation.ContinuationArtifact,
		}
	}
	return status
}

func childCompletionResultFromSnapshot(
	snapshot *wruntime.WorkflowResultSnapshot,
) ChildCompletionResult {
	if snapshot == nil {
		return ChildCompletionResult{}
	}
	out := ChildCompletionResult{
		Stage:    string(snapshot.SourceNodeID),
		Status:   string(snapshot.Status),
		Outcome:  string(snapshot.Outcome),
		Artifact: snapshot.PrimaryArtifact,
		Summary:  snapshot.Summary,
	}
	var parsed qrspi.Result
	if len(snapshot.Raw) > 0 && json.Unmarshal(snapshot.Raw, &parsed) == nil {
		out.PlanGoal = parsed.Summary.PlanGoal
		out.StageCompleted = parsed.Summary.StageCompleted
		out.KeyDecisions = parsed.Summary.KeyDecisions
		out.ChildPolicy = policySummary(qrspi.Policy{
			AdvanceMode:             parsed.Policy.AdvanceMode,
			AutoMode:                parsed.Policy.AutoMode,
			EnablePlanReviews:       parsed.Policy.EnablePlanReviews,
			InvalidResultRetryLimit: parsed.Policy.InvalidResultRetryLimit,
		})
	}
	return out
}

func childCompletionResult(result wruntime.WorkflowResult) ChildCompletionResult {
	out := ChildCompletionResult{
		Stage:    string(result.SourceNodeID),
		Status:   string(result.Status),
		Outcome:  string(result.Outcome),
		Artifact: result.PrimaryArtifact,
		Summary:  result.Summary,
	}
	var parsed qrspi.Result
	if len(result.Raw) > 0 && json.Unmarshal(result.Raw, &parsed) == nil {
		out.PlanGoal = parsed.Summary.PlanGoal
		out.StageCompleted = parsed.Summary.StageCompleted
		out.KeyDecisions = parsed.Summary.KeyDecisions
		out.ChildPolicy = policySummary(qrspi.Policy{
			AdvanceMode:             parsed.Policy.AdvanceMode,
			AutoMode:                parsed.Policy.AutoMode,
			EnablePlanReviews:       parsed.Policy.EnablePlanReviews,
			InvalidResultRetryLimit: parsed.Policy.InvalidResultRetryLimit,
		})
	}
	return out
}

func childCompletionDeliveryID(
	child ChildRunRef,
	parsed *ParsedDecision,
	exhausted bool,
) string {
	parts := []string{child.ID, fmt.Sprintf("%d", child.Generation)}
	if exhausted || parsed == nil {
		parts = append(parts, "invalid_result")
	} else {
		parts = append(
			parts,
			string(parsed.Result.SourceNodeID),
			string(parsed.Result.Status),
			string(parsed.Result.Outcome),
			parsed.Result.PrimaryArtifact,
		)
	}
	return strings.Join(parts, ":")
}

func resolvePlanArtifact(
	canonicalRepoRoot, canonicalPlanDir, sourceChildCwd, artifact string,
) (string, error) {
	if strings.TrimSpace(canonicalRepoRoot) == "" {
		return "", errors.New("canonical repo root is required")
	}
	if strings.TrimSpace(canonicalPlanDir) == "" {
		return "", errors.New("canonical plan directory is required")
	}
	if strings.TrimSpace(sourceChildCwd) == "" {
		return "", errors.New("source child cwd is required")
	}
	if strings.TrimSpace(artifact) == "" {
		return "", errors.New("handoff artifact is required")
	}

	repoRoot, err := filepath.Abs(canonicalRepoRoot)
	if err != nil {
		return "", err
	}
	planDir, err := filepath.Abs(canonicalPlanDir)
	if err != nil {
		return "", err
	}
	planRel, err := filepath.Rel(repoRoot, planDir)
	if err != nil {
		return "", err
	}
	if pathEscapesRoot(planRel) {
		return "", fmt.Errorf("canonical plan directory %q escapes repo root %q", planDir, repoRoot)
	}

	mappedPlanDir := filepath.Join(sourceChildCwd, planRel)
	mappedHandoffsDir := filepath.Join(mappedPlanDir, "handoffs")
	artifactPath := artifact
	if !filepath.IsAbs(artifactPath) {
		artifactPath = filepath.Join(sourceChildCwd, artifactPath)
	}

	realPlanDir, err := filepath.EvalSymlinks(mappedPlanDir)
	if err != nil {
		return "", fmt.Errorf("resolve mapped plan directory: %w", err)
	}
	realHandoffsDir, err := filepath.EvalSymlinks(mappedHandoffsDir)
	if err != nil {
		return "", fmt.Errorf("resolve handoffs directory: %w", err)
	}
	realArtifact, err := filepath.EvalSymlinks(artifactPath)
	if err != nil {
		return "", fmt.Errorf("resolve handoff artifact: %w", err)
	}
	if err := requirePathWithin(realPlanDir, realHandoffsDir, "handoffs directory"); err != nil {
		return "", err
	}
	if err := requirePathWithin(realHandoffsDir, realArtifact, "handoff artifact"); err != nil {
		return "", err
	}
	info, err := os.Stat(realArtifact)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("handoff artifact %q is not a regular file", realArtifact)
	}
	return realArtifact, nil
}

func pathEscapesRoot(rel string) bool {
	return filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func requirePathWithin(root, path, label string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if pathEscapesRoot(rel) {
		return fmt.Errorf("%s %q escapes %q", label, path, root)
	}
	return nil
}

func readHandoffArtifact(path string) (HandoffArtifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return HandoffArtifact{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return HandoffArtifact{}, errors.New("handoff artifact must start with YAML frontmatter")
	}
	var frontmatter strings.Builder
	closed := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "---" {
			closed = true
			break
		}
		frontmatter.WriteString(scanner.Text())
		frontmatter.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return HandoffArtifact{}, err
	}
	if !closed {
		return HandoffArtifact{}, errors.New("handoff artifact frontmatter is not closed")
	}
	var metadata struct {
		Stage  string `yaml:"stage"`
		Status string `yaml:"status"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter.String()), &metadata); err != nil {
		return HandoffArtifact{}, fmt.Errorf("parse handoff frontmatter: %w", err)
	}
	return HandoffArtifact{
		Path:   path,
		Stage:  wruntime.NodeID(strings.TrimSpace(metadata.Stage)),
		Status: strings.TrimSpace(metadata.Status),
	}, nil
}

func validateHandoffArtifact(
	state ManagerState,
	source ChildRunRef,
	result wruntime.WorkflowResult,
) (HandoffArtifact, error) {
	if result.Status != wruntime.StatusHandoff {
		return HandoffArtifact{}, fmt.Errorf("result status %q is not handoff", result.Status)
	}
	if result.Outcome != "" {
		return HandoffArtifact{}, fmt.Errorf("handoff outcome must be empty, got %q", result.Outcome)
	}
	path, err := resolvePlanArtifact(
		state.RepoID,
		state.CanonicalPlanDir,
		source.Cwd,
		result.PrimaryArtifact,
	)
	if err != nil {
		return HandoffArtifact{}, err
	}
	handoff, err := readHandoffArtifact(path)
	if err != nil {
		return HandoffArtifact{}, err
	}
	if handoff.Stage != result.SourceNodeID {
		return HandoffArtifact{}, fmt.Errorf(
			"handoff stage %q does not match result source %q",
			handoff.Stage,
			result.SourceNodeID,
		)
	}
	if handoff.Status != "in_progress" {
		return HandoffArtifact{}, fmt.Errorf(
			"handoff status %q must be in_progress",
			handoff.Status,
		)
	}
	return handoff, nil
}

func deriveChildLaunchIntent(
	state ManagerState,
	source ChildRunRef,
	result wruntime.WorkflowResult,
	decision wruntime.TransitionDecision,
) (ChildLaunchIntent, error) {
	def, err := Definition()
	if err != nil {
		return ChildLaunchIntent{}, err
	}
	authoritative, err := wruntime.DecideTransition(def, state.Workflow, result)
	if err != nil {
		return ChildLaunchIntent{}, err
	}
	if !reflect.DeepEqual(authoritative, decision) {
		return ChildLaunchIntent{}, errors.New("launch decision does not match manager workflow policy")
	}
	if !decision.StartNext || decision.NextNodeID == "" {
		return ChildLaunchIntent{}, errors.New("workflow decision does not authorize child launch")
	}
	node, ok := def.Nodes[decision.NextNodeID]
	if !ok {
		return ChildLaunchIntent{}, fmt.Errorf("node %q is not in QRSPI definition", decision.NextNodeID)
	}
	cwd, err := defaultChildCwd(state, decision.NextNodeID, "")
	if err != nil {
		return ChildLaunchIntent{}, err
	}
	intent := ChildLaunchIntent{
		Kind:            ChildLaunchNormal,
		NodeID:          decision.NextNodeID,
		SkillPath:       node.Prompt.SkillPath,
		PrimaryArtifact: result.PrimaryArtifact,
		Cwd:             cwd,
		SourceChildID:   source.ID,
	}
	if result.Status != wruntime.StatusHandoff {
		return intent, nil
	}
	if result.Outcome != "" || decision.NextNodeID != result.SourceNodeID ||
		wruntime.NodeID(source.Stage) != result.SourceNodeID {
		return ChildLaunchIntent{}, errors.New("handoff launch must resume the exact source node without outcome")
	}
	handoff, err := validateHandoffArtifact(state, source, result)
	if err != nil {
		return ChildLaunchIntent{}, err
	}
	intent.Kind = ChildLaunchResumeHandoff
	intent.SkillPath = ".pi/skills/q-resume/SKILL.md"
	intent.PrimaryArtifact = handoff.Path
	if strings.TrimSpace(source.Cwd) != "" {
		intent.Cwd = source.Cwd
	}
	intent.DeliveryID = childCompletionDeliveryID(source, &ParsedDecision{
		Result:   result,
		Decision: decision,
	}, false)
	return intent, nil
}

func existingHandoffContinuation(
	state ManagerState,
	sourceChildID, deliveryID string,
) (*ChildRunRef, bool) {
	for _, candidate := range []*ChildRunRef{state.ActiveChild, state.PendingCleanupChild} {
		if candidate == nil || candidate.LaunchKind != ChildLaunchResumeHandoff {
			continue
		}
		if candidate.ContinuationOf == sourceChildID &&
			candidate.ContinuationDeliveryID == deliveryID {
			copy := *candidate
			return &copy, true
		}
	}
	return nil, false
}

func buildInvalidHandoffArtifactCard(
	stateFile string,
	source ChildRunRef,
	result wruntime.WorkflowResult,
	validationErr error,
) *ManagerActionCard {
	return &ManagerActionCard{
		Kind:     ActionInvalidHandoffArtifact,
		Severity: "warning",
		Summary:  "validated handoff cannot auto-resume because its artifact is unsafe or inconsistent",
		Evidence: []string{
			fmt.Sprintf("source child: %s", source.ID),
			fmt.Sprintf("stage: %s", result.SourceNodeID),
			fmt.Sprintf("artifact: %s", result.PrimaryArtifact),
			validationErr.Error(),
		},
		RecommendedAction: "repair the handoff artifact, then retry manager continuation",
		SafeCommand:       continueCommand(stateFile),
		ContinueCommand:   continueCommand(stateFile),
		RequiresHuman:     false,
	}
}

func buildHandoffContinuationFailureCard(
	stateFile string,
	source ChildRunRef,
	result wruntime.WorkflowResult,
	launchErr error,
) *ManagerActionCard {
	return &ManagerActionCard{
		Kind:     ActionHandoffContinuationFailed,
		Severity: "error",
		Summary:  "validated handoff continuation did not produce a durable replacement child",
		Evidence: []string{
			fmt.Sprintf("source child: %s", source.ID),
			fmt.Sprintf("stage: %s", result.SourceNodeID),
			fmt.Sprintf("artifact: %s", result.PrimaryArtifact),
			launchErr.Error(),
		},
		RecommendedAction: "inspect launch evidence, then retry the same durable handoff",
		SafeCommand:       continueCommand(stateFile),
		ContinueCommand:   continueCommand(stateFile),
		RequiresHuman:     false,
	}
}

func buildPendingChildCleanupFailureCard(
	stateFile string,
	pending *ChildRunRef,
	cleanupErr error,
) *ManagerActionCard {
	evidence := []string{cleanupErr.Error()}
	if pending != nil {
		evidence = append(evidence,
			fmt.Sprintf("pending child: %s", pending.ID),
			fmt.Sprintf("pending pane: %s", pending.TmuxPaneID),
		)
	}
	return &ManagerActionCard{
		Kind:              ActionPendingChildCleanupFailed,
		Severity:          "warning",
		Summary:           "replacement child is active but old-pane cleanup is incomplete",
		Evidence:          evidence,
		RecommendedAction: "inspect state; the next manager operation may retry idempotent cleanup",
		SafeCommand:       fmt.Sprintf("vamos qrspi inspect --state-file %s", stateFile),
		RequiresHuman:     false,
	}
}

func terminalEvidenceForActiveChildWithRefresh(
	state ManagerState,
) (AssistantTerminalEvidence, bool, error) {
	var lastErr error
	var latest AssistantTerminalEvidence
	found := false
	for attempt := 0; attempt < 4; attempt++ {
		evidence, ok, err := LatestTerminalEvidenceForActiveChild(state)
		if err == nil && ok {
			latest = evidence
			found = true
			if evidence.ContextWindowError {
				return evidence, true, nil
			}
		}
		lastErr = err
		if attempt < 3 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if found {
		return latest, true, nil
	}
	return AssistantTerminalEvidence{}, false, lastErr
}

func childCompletionStatusFromTerminalEvidence(
	state ManagerState,
	child ChildRunRef,
	evidence AssistantTerminalEvidence,
) ChildCompletionStatus {
	status := ChildCompletionStatus{
		Validated:        false,
		ManagerNeeded:    true,
		RetryExhausted:   false,
		ChildID:          child.ID,
		DeliveryID:       childCompletionDeliveryIDForTerminalEvidence(child, evidence),
		TerminalEvidence: &evidence,
		Reason:           "provider_context_error",
		Attempt:          child.ValidationRetryCount,
		RetryLimit:       invalidResultRetryLimit(state),
		Result: ChildCompletionResult{
			Stage:          child.Stage,
			Status:         ActionChildContextExhausted,
			Summary:        providerContextSummary(evidence),
			StageCompleted: providerContextSummary(evidence),
		},
	}
	if prior := readPriorValidationStatus(child.ValidationStatusPath); prior != nil {
		status.Result.Artifact = prior.Result.Artifact
		status.Result.PlanGoal = prior.Result.PlanGoal
		status.Result.KeyDecisions = prior.Result.KeyDecisions
	}
	return status
}

func childCompletionDeliveryIDForTerminalEvidence(
	child ChildRunRef,
	evidence AssistantTerminalEvidence,
) string {
	return strings.Join([]string{
		child.ID,
		fmt.Sprintf("%d", child.Generation),
		"provider_context_error",
		evidence.EvidenceID,
	}, ":")
}

func providerContextSummary(e AssistantTerminalEvidence) string {
	return fmt.Sprintf(
		"child provider context-window error in %s line %d: %s",
		firstNonEmpty(e.SessionID, filepath.Base(e.SessionPath)),
		e.Line,
		strings.TrimSpace(e.ErrorMessage),
	)
}

func readPriorValidationStatus(path string) *ChildCompletionStatus {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var status ChildCompletionStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil
	}
	return &status
}

func nextChildInfo(state ManagerState, node wruntime.NodeID) NextChildInfo {
	if node == "" {
		return NextChildInfo{}
	}
	info := NextChildInfo{
		Stage:     string(node),
		Cwd:       defaultContinueCwd(state, node),
		WorkingOn: nextChildWorkingOn(node),
	}
	if def, err := Definition(); err == nil {
		if graphNode, ok := def.Nodes[node]; ok {
			info.Skill = graphNode.Prompt.SkillPath
		}
	}
	return info
}

func nextChildWorkingOn(node wruntime.NodeID) string {
	switch node {
	case qrspi.NodeQuestion:
		return "Decompose the task into QRSPI research questions or follow-up questions."
	case qrspi.NodeResearch:
		return "Answer the research questions with factual codebase evidence."
	case qrspi.NodeDesign:
		return "Create or update design.md and make aligned assumptions unless truly blocked."
	case qrspi.NodeOutline:
		return "Create the structured implementation outline from approved design context."
	case qrspi.NodeReviewOutline:
		return "Review outline.md for readiness before plan creation."
	case qrspi.NodePlan:
		return "Expand the reviewed outline into tactical plan.md."
	case qrspi.NodeReviewPlan:
		return "Review plan.md for workspace/implementation readiness."
	case qrspi.NodeWorkspace:
		return "Prepare or confirm the implementation workspace and branch routing."
	case qrspi.NodeImplement:
		return "Execute the next implementation slice in the implementation workspace."
	case qrspi.NodeReviewImplementation:
		return "Review completed implementation and identify follow-up work or human-review readiness."
	case qrspi.NodeVerify:
		return "Run verification, inspect artifacts, and produce verify.md."
	case qrspi.NodeHumanReviewImplementation:
		return "Wait for final human implementation approval."
	default:
		return "Run the next graph-selected QRSPI stage."
	}
}

func childCompletionManagerNeeded(status string) bool {
	return status == string(wruntime.StatusNeedsHuman) ||
		status == string(wruntime.StatusBlocked) ||
		status == string(wruntime.StatusError) ||
		status == "invalid_result"
}

func queueOrDeliverWake(
	ctx context.Context,
	stateFile string,
	state ManagerState,
	status ChildCompletionStatus,
	d deps,
) (ManagerState, WakeDeliveryInstruction, error) {
	if status.Wake.Mode == "suppress" {
		return state, status.Wake, nil
	}
	if strings.TrimSpace(status.DeliveryID) == "" {
		return state, WakeDeliveryInstruction{
			Mode:   "suppress",
			Reason: "missing_delivery_id",
		}, nil
	}
	if state.Delivery.LastDeliveryID == status.DeliveryID {
		return state, WakeDeliveryInstruction{
			Mode:   "suppress",
			Reason: "duplicate_delivery",
		}, nil
	}
	payload := childCompletionWakePayload(stateFile, state, status)
	queue := func(reason string) (ManagerState, WakeDeliveryInstruction, error) {
		state = queueManagerWake(
			state,
			status,
			payload,
			QueuedWakePasteAndSubmit,
			"",
		)
		return state, WakeDeliveryInstruction{Mode: "queue", Payload: payload, Reason: reason}, nil
	}
	if strings.EqualFold(state.Delivery.Status, "compacting") {
		return queue("manager_compacting")
	}
	paneID := managerDeliveryPane(state)
	if paneID == "" {
		return queue("manager_pane_missing")
	}
	if live := managerPaneLiveness(ctx, paneID, d); live.Checked && !live.Exists {
		evidence := managerPaneEvidence(
			state,
			ManagerPaneAdoptionOptions{
				StateFile: stateFile,
				Command:   ManagerPaneAdoptionManagerReady,
			},
			managerPaneLiveness(ctx, state.ManagerPaneID, d),
			managerPaneLiveness(ctx, state.Delivery.ManagerPaneID, d),
			PaneLiveness{},
		)
		evidence = append(
			evidence,
			fmt.Sprintf("selected manager pane unavailable: %s", paneID),
		)
		state.LastActionCard = buildManagerPaneActionCard(
			state,
			ManagerPaneAdoptionOptions{
				StateFile: stateFile,
				Command:   ManagerPaneAdoptionManagerReady,
			},
			evidence,
			ActionManagerPaneUnavailable,
		)
		return queue("manager_pane_unavailable")
	}
	tmux := tmuxClient(d)
	pane := TmuxPane{ID: paneID}
	if err := tmux.PasteText(ctx, pane, payload); err != nil {
		state = queueManagerWake(
			state,
			status,
			payload,
			QueuedWakePasteAndSubmit,
			"",
		)
		state = recordManagerDeliveryFailure(state, status, paneID, "paste", err)
		return state, WakeDeliveryInstruction{
			Mode:    "queue",
			Payload: payload,
			Reason:  "manager_delivery_failed",
		}, nil
	}
	if err := tmux.SendKeys(ctx, pane, []string{"Enter"}); err != nil {
		state = queueManagerWake(
			state,
			status,
			payload,
			QueuedWakeSubmitOnly,
			paneID,
		)
		state = recordManagerDeliveryFailure(state, status, paneID, "submit", err)
		return state, WakeDeliveryInstruction{
			Mode:    "queue",
			Payload: payload,
			Reason:  "manager_delivery_failed",
		}, nil
	}
	state.Delivery.LastDeliveryID = status.DeliveryID
	return state, WakeDeliveryInstruction{Mode: "deliver", Payload: payload}, nil
}

func queueManagerWake(
	state ManagerState,
	status ChildCompletionStatus,
	payload string,
	delivery QueuedWakeDelivery,
	pastedPaneID string,
) ManagerState {
	childID := status.ChildID
	generation := activeChildGeneration(state)
	if status.ContinuationStarted && state.ActiveChild != nil &&
		state.ActiveChild.ContinuationDeliveryID == status.DeliveryID {
		childID = state.ActiveChild.ID
		generation = activeChildGeneration(state)
	}
	state.Delivery.QueuedWake = &QueuedWake{
		DeliveryID:      status.DeliveryID,
		ChildID:         childID,
		ChildGeneration: generation,
		Payload:         payload,
		Delivery:        delivery,
		PastedPaneID:    pastedPaneID,
		QueuedAt:        time.Now().Format(time.RFC3339),
	}
	return state
}

func recordManagerDeliveryFailure(
	state ManagerState,
	status ChildCompletionStatus,
	paneID, phase string,
	deliveryErr error,
) ManagerState {
	if status.ActionCard != nil {
		return state
	}
	state.LastActionCard = &ManagerActionCard{
		Kind:     ActionManagerDeliveryFailed,
		Severity: "warning",
		Summary:  "manager wake delivery is queued after a tmux write failure",
		Evidence: []string{
			fmt.Sprintf("delivery: %s", status.DeliveryID),
			fmt.Sprintf("pane: %s", paneID),
			fmt.Sprintf("phase: %s", phase),
			deliveryErr.Error(),
		},
		RecommendedAction: "run manager-ready from the intended manager pane",
		RequiresHuman:     false,
	}
	return state
}

func managerDeliveryPane(state ManagerState) string {
	if strings.TrimSpace(state.Delivery.ManagerPaneID) != "" {
		return strings.TrimSpace(state.Delivery.ManagerPaneID)
	}
	return strings.TrimSpace(state.ManagerPaneID)
}

func activeChildGeneration(state ManagerState) int {
	if state.ActiveChild == nil || state.ActiveChild.Generation == 0 {
		return 1
	}
	return state.ActiveChild.Generation
}

func pasteWake(ctx context.Context, d deps, paneID, payload string) error {
	tmux := d.Tmux
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	pane := TmuxPane{ID: paneID}
	if err := tmux.PasteText(ctx, pane, payload); err != nil {
		return err
	}
	return tmux.SendKeys(ctx, pane, []string{"Enter"})
}

func childCompletionWake(
	stateFile string,
	state ManagerState,
	status ChildCompletionStatus,
) WakeDeliveryInstruction {
	if status.DeliveryID != "" && state.Delivery.LastDeliveryID == status.DeliveryID {
		return WakeDeliveryInstruction{Mode: "suppress", Reason: "duplicate_delivery"}
	}
	return WakeDeliveryInstruction{
		Mode:    "deliver",
		Payload: childCompletionWakePayload(stateFile, state, status),
	}
}

func childCompletionWakePayload(
	stateFile string,
	state ManagerState,
	status ChildCompletionStatus,
) string {
	managerNeeded := status.ManagerNeeded || status.RetryExhausted ||
		childCompletionManagerNeeded(status.Result.Status)
	policy := activePolicySummary(state)
	var payload strings.Builder
	fmt.Fprintf(&payload, "```yaml\nq_manager_child_wake:\n")
	fmt.Fprintf(&payload, "  validated: %t\n", status.Validated)
	fmt.Fprintf(&payload, "  manager_needed: %t\n", managerNeeded)
	fmt.Fprintf(&payload, "  continuation_started: %t\n", status.ContinuationStarted)
	fmt.Fprintf(&payload, "  retry_exhausted: %t\n", status.RetryExhausted)
	fmt.Fprintf(&payload, "  stage: %q\n", status.Result.Stage)
	fmt.Fprintf(&payload, "  status: %q\n", status.Result.Status)
	fmt.Fprintf(&payload, "  outcome: %q\n", status.Result.Outcome)
	fmt.Fprintf(&payload, "  artifact: %q\n", status.Result.Artifact)
	fmt.Fprintf(&payload, "  child_id: %q\n", status.ChildID)
	fmt.Fprintf(&payload, "  state_file: %q\n", stateFile)
	fmt.Fprintf(&payload, "  reason: %q\n", status.Reason)
	payload.WriteString(childCompletionWakeTerminalEvidencePayload(status.TerminalEvidence))
	fmt.Fprintf(&payload, "  policy:\n")
	fmt.Fprintf(&payload, "    advance_mode: %q\n", policy.AdvanceMode)
	fmt.Fprintf(&payload, "    auto_mode: %t\n", policy.AutoMode)
	fmt.Fprintf(&payload, "    enable_plan_reviews: %t\n", policy.EnablePlanReviews)
	fmt.Fprintf(&payload, "    invalid_result_retry_limit: %d\n", policy.InvalidResultRetryLimit)
	fmt.Fprintf(&payload, "    note: %q\n", "manager policy is authoritative; child-emitted policy is ignored for transitions")
	fmt.Fprintf(&payload, "  summary:\n")
	fmt.Fprintf(&payload, "    plan_goal: %q\n", status.Result.PlanGoal)
	fmt.Fprintf(&payload, "    stage_completed: %q\n", status.Result.StageCompleted)
	fmt.Fprintf(&payload, "    key_decisions: %q\n", status.Result.KeyDecisions)
	fmt.Fprintf(&payload, "  next_child:\n")
	fmt.Fprintf(&payload, "    stage: %q\n", status.NextChild.Stage)
	fmt.Fprintf(&payload, "    skill: %q\n", status.NextChild.Skill)
	fmt.Fprintf(&payload, "    cwd: %q\n", status.NextChild.Cwd)
	fmt.Fprintf(&payload, "    working_on: %q\n", status.NextChild.WorkingOn)
	if !status.ContinuationStarted {
		fmt.Fprintf(&payload, "  next:\n")
		fmt.Fprintf(&payload, "    - action: run_command\n")
		fmt.Fprintf(&payload, "      param: %q\n", continueCommand(stateFile))
	}
	payload.WriteString("```")
	return payload.String()
}

func childCompletionWakeTerminalEvidencePayload(e *AssistantTerminalEvidence) string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"  terminal_evidence:\n    session_path: %q\n    session_id: %q\n    line: %d\n    timestamp: %q\n    stop_reason: %q\n    error_message: %q\n    evidence_id: %q\n    context_window_error: %t\n",
		e.SessionPath,
		e.SessionID,
		e.Line,
		e.Timestamp,
		e.StopReason,
		e.ErrorMessage,
		e.EvidenceID,
		e.ContextWindowError,
	)
}

func writeValidationStatus(path string, status ChildCompletionStatus) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func usageFromChangedFlags(
	cmd *cobra.Command,
	usagePercent float64,
	usageTokens, usageWindow int,
	usageSource string,
) ManagerUsageInput {
	var input ManagerUsageInput
	if cmd.Flags().Changed("manager-usage-percent") {
		input.UsagePercent = &usagePercent
	}
	if cmd.Flags().Changed("manager-usage-tokens") {
		input.Tokens = &usageTokens
	}
	if cmd.Flags().Changed("manager-usage-window") {
		input.Window = &usageWindow
	}
	if strings.TrimSpace(usageSource) != "" {
		input.Source = strings.TrimSpace(usageSource)
	}
	return input
}

func managerUsagePercent(input ManagerUsageInput) (float64, bool) {
	if input.UsagePercent != nil {
		return *input.UsagePercent, true
	}
	if input.Tokens != nil && input.Window != nil && *input.Window > 0 {
		return (float64(*input.Tokens) / float64(*input.Window)) * 100, true
	}
	return 0, false
}

func managerUsageSample(input ManagerUsageInput, now time.Time) *ManagerUsageSample {
	percent, hasPercent := managerUsagePercent(input)
	if !hasPercent && input.Tokens == nil && input.Window == nil {
		return nil
	}
	sample := &ManagerUsageSample{
		Tokens:    input.Tokens,
		Window:    input.Window,
		Source:    firstNonEmpty(strings.TrimSpace(input.Source), "cli-explicit"),
		SampledAt: now.Format(time.RFC3339),
	}
	if hasPercent {
		sample.Percent = &percent
	}
	return sample
}

func readyCommand(stateFile string) string {
	return fmt.Sprintf(
		"vamos qrspi manager-ready --state-file %s --manager-pane $TMUX_PANE",
		stateFile,
	)
}

func maybeStartManagerCompaction(
	ctx context.Context,
	state ManagerState,
	stateFile string,
	usage ManagerUsageInput,
	d deps,
	out io.Writer,
) (ManagerState, ManagerCompactionStatus, error) {
	_ = ctx
	out = ensureWriter(out)
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock()
	}
	if sample := managerUsageSample(usage, now); sample != nil {
		state.LastManagerUsage = sample
	}

	percent, ok := managerUsagePercent(usage)
	if !ok {
		status := ManagerCompactionStatus{Reason: "no_explicit_usage_input"}
		saveUsageDiagnosticIfNeeded(d, stateFile, state, now)
		return state, status, writeCompactionDiagnostic(out, status)
	}
	if percent < managerCompactionThresholdPercent {
		status := ManagerCompactionStatus{
			Reason:       "below_threshold",
			UsagePercent: fmt.Sprintf("%.1f", percent),
		}
		saveUsageDiagnosticIfNeeded(d, stateFile, state, now)
		return state, status, writeCompactionDiagnostic(out, status)
	}

	handoffPath, err := writeManagerOperationalHandoff(state, stateFile, now)
	if err != nil {
		return state, ManagerCompactionStatus{}, err
	}
	state.Delivery.Status = "compacting"
	if strings.TrimSpace(state.Delivery.ManagerPaneID) == "" {
		state.Delivery.ManagerPaneID = strings.TrimSpace(state.ManagerPaneID)
	}
	if err := stateStore(
		d,
		"",
		func() time.Time { return now },
	).Save(stateFile, state); err != nil {
		return state, ManagerCompactionStatus{}, err
	}
	status := ManagerCompactionStatus{
		Started:      true,
		Reason:       "threshold_met",
		UsagePercent: fmt.Sprintf("%.1f", percent),
		HandoffPath:  handoffPath,
		ReadyCommand: readyCommand(stateFile),
	}
	return state, status, writeCompactionDiagnostic(out, status)
}

func saveUsageDiagnosticIfNeeded(
	d deps,
	stateFile string,
	state ManagerState,
	now time.Time,
) {
	if strings.TrimSpace(stateFile) == "" || state.LastManagerUsage == nil {
		return
	}
	if _, err := os.Stat(stateFile); err != nil {
		return
	}
	_ = stateStore(d, "", func() time.Time { return now }).Save(stateFile, state)
}

func writeCompactionDiagnostic(out io.Writer, status ManagerCompactionStatus) error {
	if out == nil {
		return nil
	}
	if status.Started {
		if _, err := fmt.Fprintf(
			out,
			"manager compaction: started; usage %s%% >= %.0f%%; handoff written\n",
			status.UsagePercent,
			managerCompactionThresholdPercent,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "q-manager-parent-compact: started"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "handoff: %s\n", status.HandoffPath); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			out,
			"resume: pi @%s\n",
			status.HandoffPath,
		); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "ready: %s\n", status.ReadyCommand)
		return err
	}
	switch status.Reason {
	case "below_threshold":
		_, err := fmt.Fprintf(
			out,
			"manager compaction: skipped; usage %s%% < %.0f%%\n",
			status.UsagePercent,
			managerCompactionThresholdPercent,
		)
		return err
	default:
		_, err := fmt.Fprintln(
			out,
			"manager compaction: skipped; no explicit usage input",
		)
		return err
	}
}

func writeManagerOperationalHandoff(
	state ManagerState,
	stateFile string,
	now time.Time,
) (string, error) {
	planDir := strings.TrimSpace(state.CanonicalPlanDir)
	if planDir == "" {
		return "", errors.New("canonical plan dir is required for manager handoff")
	}
	planPath := planDir
	if !filepath.IsAbs(planPath) {
		base := strings.TrimSpace(state.ImplementationCwd)
		if base == "" {
			base = strings.TrimSpace(state.SourceCwd)
		}
		if base == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			base = cwd
		}
		planPath = filepath.Join(base, planDir)
	}
	handoffDir := filepath.Join(planPath, "handoffs")
	if err := os.MkdirAll(handoffDir, 0o755); err != nil {
		return "", err
	}
	filename := now.Format("2006-01-02_15-04-05") + "_q-manager-operational-handoff.md"
	path := filepath.Join(handoffDir, filename)
	content := buildManagerOperationalHandoff(state, stateFile, now, path)
	if err := writeFileAtomically(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildManagerOperationalHandoff(
	state ManagerState,
	stateFile string,
	now time.Time,
	path string,
) string {
	child := "none"
	if state.ActiveChild != nil {
		child = fmt.Sprintf(
			"stage=%s childID=%s pane=%s sessionID=%s sessionPath=%s statusPath=%s donePath=%s",
			state.ActiveChild.Stage,
			state.ActiveChild.ID,
			state.ActiveChild.TmuxPaneID,
			state.ActiveChild.SessionID,
			state.ActiveChild.SessionPath,
			state.ActiveChild.StatusPath,
			state.ActiveChild.DonePath,
		)
	}
	return fmt.Sprintf(`---
date: %s
stage: q-manager
artifact: manager-operational-handoff
---

# q-manager operational handoff

Done: parent manager usage met native compaction threshold after launching child; delivery marked compacting so child wake queues safely before parent Pi ctx.compact() runs.

Next: resume manager from this handoff, then mark manager ready to flush queued child wake.

## Durable workflow refs

- Plan dir: %s
- Current graph node: %s
- Implementation cwd: %s
- Handoff path: %s

## Local / ephemeral manager refs

- State file: %s
- Source cwd: %s
- Manager run ID: %s
- Manager pane: %s
- Active child: %s

## Exact next commands

`+"```bash"+`
# In the fresh manager session after reading this handoff:
vamos qrspi manager-ready --state-file %q --manager-pane "$TMUX_PANE"

# Then follow the flushed wake, or continue manually if already validated:
vamos qrspi continue --state-file %q
`+"```"+`
`, now.Format(time.RFC3339), state.CanonicalPlanDir, state.Workflow.CurrentNodeID, state.ImplementationCwd, path, stateFile, state.SourceCwd, state.ManagerRunID, state.ManagerPaneID, child, stateFile, stateFile)
}

func RunManagerReady(
	ctx context.Context,
	opts ManagerReadyOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	operationLock, err := store.AcquireOperationLock(ctx, opts.StateFile)
	if err != nil {
		return err
	}
	defer operationLock.Release()
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	currentPane := CaptureManagerPaneID("")
	var stopped bool
	state, stopped, err = applyManagerPaneAdoption(
		ctx,
		opts.StateFile,
		state,
		ManagerPaneAdoptionOptions{
			Command:      ManagerPaneAdoptionManagerReady,
			ExplicitPane: opts.ManagerPane,
			CurrentPane:  currentPane,
		},
		store,
		d,
		out,
		opts.Output,
	)
	if err != nil {
		return err
	}
	if stopped {
		return nil
	}
	pane := strings.TrimSpace(opts.ManagerPane)
	if pane == "" {
		pane = currentPane
	}
	if pane == "" {
		pane = managerDeliveryPane(state)
	}
	state.Delivery.Status = "ready"
	var flushed bool
	state, flushed, err = flushQueuedWake(ctx, state, pane, d)
	if err != nil {
		return err
	}
	if state.Delivery.QueuedWake == nil && state.PendingCleanupChild != nil {
		var cleanupErr error
		state, cleanupErr = cleanupPendingChildAfterNotification(ctx, state, d.Tmux)
		if cleanupErr != nil {
			state.LastActionCard = buildPendingChildCleanupFailureCard(
				opts.StateFile,
				state.PendingCleanupChild,
				cleanupErr,
			)
		}
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(
			out,
			Event{
				Type: "manager_ready",
				Ref:  map[string]any{"flushed": flushed, "stateFile": opts.StateFile},
			},
		)
	}
	if flushed {
		_, err = fmt.Fprintln(out, "manager ready: flushed queued wake")
	} else {
		_, err = fmt.Fprintln(out, "manager ready: no queued wake")
	}
	return err
}

func flushQueuedWake(
	ctx context.Context,
	state ManagerState,
	pane string,
	d deps,
) (ManagerState, bool, error) {
	queued := state.Delivery.QueuedWake
	if queued == nil {
		return state, false, nil
	}
	if queued.DeliveryID == "" || queued.DeliveryID == state.Delivery.LastDeliveryID {
		state.Delivery.QueuedWake = nil
		return state, false, nil
	}
	matchingContinuation := state.ActiveChild != nil &&
		state.ActiveChild.LaunchKind == ChildLaunchResumeHandoff &&
		state.ActiveChild.ContinuationDeliveryID == queued.DeliveryID
	if state.ActiveChild != nil && !matchingContinuation {
		if queued.ChildGeneration != activeChildGeneration(state) ||
			state.ActiveChild.LifecycleStatus == "running" ||
			state.ActiveChild.LifecycleStatus == "manual_reprompt" {
			state.LastActionCard = &ManagerActionCard{
				Kind:              ActionSupersededQueuedWake,
				Severity:          "info",
				Summary:           "queued child wake superseded by active child generation",
				RecommendedAction: "wait for newer child completion",
				RequiresHuman:     false,
			}
			state.Delivery.QueuedWake = nil
			return state, false, nil
		}
	}
	paneID := strings.TrimSpace(pane)
	if paneID == "" {
		paneID = managerDeliveryPane(state)
	}
	if paneID == "" {
		return state, false, errors.New("manager pane is required to flush queued wake")
	}
	tmux := tmuxClient(d)
	delivery := queued.Delivery
	if delivery == "" {
		delivery = QueuedWakePasteAndSubmit
	}
	if delivery == QueuedWakeSubmitOnly && queued.PastedPaneID == paneID {
		if err := tmux.SendKeys(ctx, TmuxPane{ID: paneID}, []string{"Enter"}); err != nil {
			state = recordQueuedWakeDeliveryFailure(state, queued, paneID, "submit", err)
			return state, false, nil
		}
	} else {
		if err := tmux.PasteText(ctx, TmuxPane{ID: paneID}, queued.Payload); err != nil {
			queued.Delivery = QueuedWakePasteAndSubmit
			queued.PastedPaneID = ""
			state = recordQueuedWakeDeliveryFailure(state, queued, paneID, "paste", err)
			return state, false, nil
		}
		queued.Delivery = QueuedWakeSubmitOnly
		queued.PastedPaneID = paneID
		if err := tmux.SendKeys(ctx, TmuxPane{ID: paneID}, []string{"Enter"}); err != nil {
			state = recordQueuedWakeDeliveryFailure(state, queued, paneID, "submit", err)
			return state, false, nil
		}
	}
	queued.DeliveredAt = time.Now().Format(time.RFC3339)
	state.Delivery.LastDeliveryID = queued.DeliveryID
	state.Delivery.QueuedWake = nil
	return state, true, nil
}

func recordQueuedWakeDeliveryFailure(
	state ManagerState,
	queued *QueuedWake,
	paneID, phase string,
	deliveryErr error,
) ManagerState {
	state.Delivery.QueuedWake = queued
	state.LastActionCard = &ManagerActionCard{
		Kind:     ActionManagerDeliveryFailed,
		Severity: "warning",
		Summary:  "queued manager wake delivery remains retryable",
		Evidence: []string{
			fmt.Sprintf("delivery: %s", queued.DeliveryID),
			fmt.Sprintf("pane: %s", paneID),
			fmt.Sprintf("phase: %s", phase),
			deliveryErr.Error(),
		},
		RecommendedAction: "retry manager-ready from the intended manager pane",
		RequiresHuman:     false,
	}
	return state
}

func supersedeQueuedWakeForActiveChild(
	state ManagerState,
	childID, reason string,
) ManagerState {
	if state.Delivery.QueuedWake == nil || state.Delivery.QueuedWake.ChildID != childID {
		return state
	}
	state.Delivery.QueuedWake = nil
	state.LastActionCard = &ManagerActionCard{
		Kind:              ActionSupersededQueuedWake,
		Severity:          "info",
		Summary:           reason,
		RecommendedAction: "wait for newer child completion",
		RequiresHuman:     false,
	}
	return state
}

func RunRepairState(
	ctx context.Context,
	opts RepairStateOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	switch {
	case opts.AlignActiveChild:
		return runAlignActiveChildRepair(opts, out, store, state)
	case opts.ClearFailedChild:
		return runClearFailedChildRepair(ctx, opts, d, out, store, state)
	default:
		return errors.New(
			"one repair action is required: --align-active-child or --clear-failed-child",
		)
	}
}

func runAlignActiveChildRepair(
	opts RepairStateOptions,
	out io.Writer,
	store StateStore,
	state ManagerState,
) error {
	if state.ActiveChild == nil || strings.TrimSpace(state.ActiveChild.Stage) == "" {
		return errors.New("no active child evidence to align")
	}
	card := buildStateDesyncActionCard(
		state,
		opts.StateFile,
		fmt.Errorf(
			"workflow cursor %s differs from active child %s",
			state.Workflow.CurrentNodeID,
			state.ActiveChild.Stage,
		),
	)
	state.Workflow.CurrentNodeID = wruntime.NodeID(state.ActiveChild.Stage)
	state.LastActionCard = &card
	if err := appendRecoveryIncident(opts.StateFile, card, true); err != nil {
		return err
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "ndjson") {
		return writeManagerActionCard(out, card, opts.Output)
	}
	fmt.Fprintf(
		out,
		"repaired: aligned current node to active child %s\n",
		state.ActiveChild.Stage,
	)
	return writeManagerActionCard(out, card, opts.Output)
}

func runClearFailedChildRepair(
	ctx context.Context,
	opts RepairStateOptions,
	d deps,
	out io.Writer,
	store StateStore,
	state ManagerState,
) error {
	health, err := InspectActiveChildHealth(ctx, state, opts.StateFile, d)
	if err != nil {
		return err
	}
	if !IsTerminalFailedChild(health) {
		return fmt.Errorf("active child is not terminal failed: %s", health.Status)
	}
	cleared, err := ClearFailedActiveChild(state, health)
	if err != nil {
		return err
	}
	card := BuildChildLaunchFailedCard(health, state, opts.StateFile)
	if card == nil {
		return errors.New("failed child action card could not be built")
	}
	cleared.LastActionCard = card
	if err := appendRecoveryIncident(opts.StateFile, *card, true); err != nil {
		return err
	}
	if err := store.Save(opts.StateFile, cleared); err != nil {
		return err
	}
	fmt.Fprintf(out, "repaired: cleared failed child %s\n", health.ChildID)
	if !opts.Relaunch {
		return writeManagerActionCard(out, *card, opts.Output)
	}
	_, err = RunStartNext(
		ctx,
		StartNextOptions{StateFile: opts.StateFile, Output: opts.Output, Force: true},
		d,
		out,
	)
	return err
}

func ClearFailedActiveChild(
	state ManagerState,
	health ActiveChildHealth,
) (ManagerState, error) {
	if !IsTerminalFailedChild(health) {
		return state, fmt.Errorf(
			"refusing to clear non-terminal child: %s",
			health.Status,
		)
	}
	if state.ActiveChild == nil || state.ActiveChild.ID != health.ChildID {
		return state, errors.New("active child changed while inspecting health")
	}
	state.PendingCleanupChild = state.ActiveChild
	state.ActiveChild = nil
	return state, nil
}

func RunMarkChildActive(
	ctx context.Context,
	opts MarkChildActiveOptions,
	d deps,
	out io.Writer,
) error {
	_ = ctx
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.ChildID) == "" {
		return errors.New("child-id is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if state.ActiveChild == nil ||
		state.ActiveChild.ID != strings.TrimSpace(opts.ChildID) {
		return fmt.Errorf("active child does not match requested child %q", opts.ChildID)
	}
	state.ActiveChild.LifecycleStatus = "manual_reprompt"
	state.ActiveChild.Generation = activeChildGeneration(state) + 1
	state = supersedeQueuedWakeForActiveChild(
		state,
		state.ActiveChild.ID,
		markChildActiveReason(opts.Reason),
	)
	if state.LastActionCard == nil {
		card := manualChildSteerActionCard(state, opts.StateFile)
		state.LastActionCard = &card
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(
			out,
			Event{
				Type:       "child_marked_active",
				ActionCard: state.LastActionCard,
				Ref:        childRef(state.ActiveChild),
			},
		)
	}
	fmt.Fprintf(
		out,
		"child active: %s generation %d\n",
		state.ActiveChild.ID,
		state.ActiveChild.Generation,
	)
	return writeManagerActionCard(out, *state.LastActionCard, opts.Output)
}

func markChildActiveReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual reprompt marked child active"
	}
	return reason
}

func RunValidateResult(
	ctx context.Context,
	opts ValidateResultOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.Stage) == "" {
		return errors.New("stage is required")
	}
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	text, parseCtx, err := ReadChildResultText(
		state,
		ResultSourceOptions{
			ResultFile:  opts.ResultFile,
			SessionFile: opts.SessionFile,
			SessionID:   opts.SessionID,
			RunID:       opts.RunID,
		},
	)
	if err != nil {
		return err
	}
	parseCtx.ExpectedNodeID = wruntime.NodeID(opts.Stage)
	parsed, err := ParseNormalizeValidateDecide(text, state, parseCtx)
	if err != nil {
		return err
	}
	return WriteNDJSON(out, Event{Type: "validated", Decision: &parsed})
}

func RunRepromptChild(
	ctx context.Context,
	opts RepromptChildOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	if strings.TrimSpace(opts.Stage) == "" {
		return errors.New("stage is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if state.ActiveChild == nil {
		return errors.New("no active child to reprompt")
	}
	if state.ActiveChild.Stage != opts.Stage {
		return fmt.Errorf(
			"active child stage %q does not match requested stage %q",
			state.ActiveChild.Stage,
			opts.Stage,
		)
	}
	if strings.TrimSpace(state.ActiveChild.TmuxPaneID) == "" {
		return errors.New("active child has no tmux pane ID")
	}
	errText, err := repromptErrorText(opts)
	if err != nil {
		return err
	}
	prompt := ChildRecoveryPrompt(errors.New(errText), opts.Attempt)
	tmux := d.Tmux
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	pane := TmuxPane{ID: state.ActiveChild.TmuxPaneID}
	if err := tmux.PasteText(ctx, pane, prompt); err != nil {
		return err
	}
	if err := tmux.SendKeys(ctx, pane, []string{"Enter"}); err != nil {
		return err
	}
	return WriteNDJSON(
		out,
		Event{Type: "child_reprompted", Ref: childRef(state.ActiveChild)},
	)
}

func RunSteerChild(
	ctx context.Context,
	opts SteerChildOptions,
	d deps,
	out io.Writer,
) (*SteerChildResult, error) {
	if strings.TrimSpace(opts.StateFile) == "" {
		return nil, errors.New("state-file is required")
	}
	feedback, feedbackPath, err := readFeedback(opts)
	if err != nil {
		return nil, err
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return nil, err
	}
	if state.ActiveChild == nil {
		return nil, fmt.Errorf(
			"no active child to steer; next: %s or %s",
			fmt.Sprintf("vamos qrspi start-next --state-file %s", opts.StateFile),
			continueCommand(opts.StateFile),
		)
	}
	child := *state.ActiveChild
	if strings.TrimSpace(opts.Stage) != "" && child.Stage != opts.Stage {
		return nil, fmt.Errorf(
			"active child stage %q does not match requested stage %q",
			child.Stage,
			opts.Stage,
		)
	}
	if strings.TrimSpace(child.TmuxPaneID) == "" {
		return nil, errors.New("active child has no tmux pane ID")
	}
	prompt := buildChildSteerPrompt(state, child, feedback, feedbackPath)
	tmux := d.Tmux
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	pane := TmuxPane{ID: child.TmuxPaneID}
	if err := tmux.PasteText(ctx, pane, prompt); err != nil {
		return nil, err
	}
	if err := tmux.SendKeys(ctx, pane, []string{"Enter"}); err != nil {
		return nil, err
	}
	result := &SteerChildResult{
		StateFile:    opts.StateFile,
		Stage:        child.Stage,
		PaneID:       child.TmuxPaneID,
		FeedbackPath: feedbackPath,
		NextCommand:  continueCommand(opts.StateFile),
	}
	return result, writeSteerChildOutput(out, *result, opts.Output)
}

func readFeedback(opts SteerChildOptions) (string, string, error) {
	if strings.TrimSpace(opts.FeedbackFile) != "" &&
		strings.TrimSpace(opts.Feedback) != "" {
		return "", "", errors.New("use only one of --feedback-file or --feedback")
	}
	if strings.TrimSpace(opts.FeedbackFile) != "" {
		data, err := os.ReadFile(opts.FeedbackFile)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", "", errors.New("feedback is required")
		}
		return string(data), opts.FeedbackFile, nil
	}
	if strings.TrimSpace(opts.Feedback) != "" {
		return opts.Feedback, "", nil
	}
	return "", "", errors.New("feedback-file or feedback is required")
}

func buildChildSteerPrompt(
	state ManagerState,
	child ChildRunRef,
	feedback, feedbackPath string,
) string {
	var b strings.Builder
	b.WriteString("q-manager steering feedback\n")
	b.WriteString("source: human_feedback\n")
	if strings.TrimSpace(child.Stage) != "" {
		fmt.Fprintf(&b, "stage: %s\n", child.Stage)
	}
	if strings.TrimSpace(feedbackPath) != "" {
		fmt.Fprintf(&b, "feedback_file: %s\n", feedbackPath)
	}
	if strings.TrimSpace(state.CanonicalPlanDir) != "" {
		fmt.Fprintf(&b, "plan_dir: %s\n", state.CanonicalPlanDir)
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(feedback))
	b.WriteString("\n\n")
	b.WriteString(
		"Instruction: incorporate this feedback into your current QRSPI stage. Update artifacts if needed. Then emit the required fenced YAML result when complete or ask one concise follow-up if still blocked.\n",
	)
	return b.String()
}

func writeSteerChildOutput(out io.Writer, result SteerChildResult, mode string) error {
	if strings.EqualFold(mode, "ndjson") {
		return WriteNDJSON(out, Event{Type: "child_steered", Ref: steerChildRef(result)})
	}
	if _, err := fmt.Fprintf(
		out,
		"steered child: %s (%s)\n",
		result.Stage,
		result.PaneID,
	); err != nil {
		return err
	}
	if result.FeedbackPath != "" {
		if _, err := fmt.Fprintf(out, "feedback: %s\n", result.FeedbackPath); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "next: %s\n", result.NextCommand)
	return err
}

func steerChildRef(result SteerChildResult) map[string]any {
	ref := map[string]any{
		"stateFile":   result.StateFile,
		"stage":       result.Stage,
		"paneId":      result.PaneID,
		"nextCommand": result.NextCommand,
	}
	if result.FeedbackPath != "" {
		ref["feedbackPath"] = result.FeedbackPath
	}
	return ref
}

func repromptErrorText(opts RepromptChildOptions) (string, error) {
	if strings.TrimSpace(opts.ErrorText) != "" {
		return opts.ErrorText, nil
	}
	if strings.TrimSpace(opts.ErrorFile) != "" {
		data, err := os.ReadFile(opts.ErrorFile)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "child result failed QRSPI validation", nil
}

func RunContinue(ctx context.Context, opts ContinueOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	operationLock, err := store.AcquireOperationLock(ctx, opts.StateFile)
	if err != nil {
		return err
	}
	defer operationLock.Release()
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if piModel := strings.TrimSpace(opts.PiModel); piModel != "" {
		state.PiModel = piModel
		if err := store.Save(opts.StateFile, state); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		opts.PlanDir = state.CanonicalPlanDir
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	var stopped bool
	state, stopped, err = applyManagerPaneAdoption(
		ctx,
		opts.StateFile,
		state,
		ManagerPaneAdoptionOptions{
			Command:      ManagerPaneAdoptionContinue,
			ExplicitPane: opts.ManagerPane,
			CurrentPane:  CaptureManagerPaneID(""),
		},
		store,
		d,
		out,
		opts.Output,
	)
	if err != nil {
		return err
	}
	if stopped {
		return nil
	}
	if state.ActiveChild == nil {
		card := buildContinueActionCard(
			state,
			opts,
			errors.New("no active child to continue"),
		)
		state.LastActionCard = card
		_ = store.Save(opts.StateFile, state)
		if card != nil {
			return writeManagerActionCard(out, *card, opts.Output)
		}
		return errors.New("no active child to continue")
	}
	if strings.TrimSpace(opts.Stage) == "" {
		opts.Stage = state.ActiveChild.Stage
	}
	if strings.TrimSpace(opts.Stage) == "" {
		return errors.New("stage is required")
	}
	health, healthErr := InspectActiveChildHealth(ctx, state, opts.StateFile, d)
	if healthErr != nil {
		return healthErr
	}
	if IsRecoverableNoResultChild(health) {
		card := BuildChildContextExhaustedCard(health, state, opts.StateFile)
		if card != nil {
			state.LastActionCard = card
			_ = store.Save(opts.StateFile, state)
			return writeManagerActionCard(out, *card, opts.Output)
		}
	}
	if IsTerminalFailedChild(health) {
		card := BuildChildLaunchFailedCard(health, state, opts.StateFile)
		if card != nil {
			state.LastActionCard = card
			_ = store.Save(opts.StateFile, state)
			return writeManagerActionCard(out, *card, opts.Output)
		}
	}

	result := ContinueResult{}
	parsed, err := validateActiveChild(ctx, state, opts)
	if err != nil {
		if shouldRepromptAfterValidationError(state, opts, err) {
			if repromptErr := continueReprompt(
				ctx,
				state,
				opts,
				d,
				out,
				err,
			); repromptErr != nil {
				return repromptErr
			}
			result.Reprompted = true
			result.StopReason = "invalid result; reprompted active child"
			return writeContinueOutput(out, opts, result)
		}
		if isRetryExhaustedValidationError(state, opts, err) {
			result.StopReason = err.Error()
			card := buildContinueActionCard(state, opts, err)
			if card != nil {
				state.LastActionCard = card
				_ = store.Save(opts.StateFile, state)
			}
			return writeRetryExhaustedNotice(out, opts, state, err)
		}
		card := buildContinueActionCard(state, opts, err)
		if card != nil {
			state.LastActionCard = card
			_ = store.Save(opts.StateFile, state)
			return writeManagerActionCard(out, *card, opts.Output)
		}
		return err
	}
	result.Validated = &parsed
	result.PrimaryArtifact = parsed.Result.PrimaryArtifact
	var launchIntent *ChildLaunchIntent
	if parsed.Result.Status == wruntime.StatusHandoff && parsed.Decision.StartNext {
		intent, intentErr := deriveChildLaunchIntent(
			state,
			*state.ActiveChild,
			parsed.Result,
			parsed.Decision,
		)
		if intentErr != nil {
			card := buildInvalidHandoffArtifactCard(
				opts.StateFile,
				*state.ActiveChild,
				parsed.Result,
				intentErr,
			)
			state.ActiveChild.LifecycleStatus = "awaiting_manager"
			state.LastActionCard = card
			if saveErr := store.Save(opts.StateFile, state); saveErr != nil {
				return saveErr
			}
			return writeManagerActionCard(out, *card, opts.Output)
		}
		if cwd := strings.TrimSpace(opts.Cwd); cwd != "" {
			intent.Cwd = cwd
		}
		launchIntent = &intent
	}

	nextState, err := decideValidatedResult(ctx, state, parsed, opts, store)
	if err != nil {
		return err
	}
	result.Decided = true
	result.NextNodeID = parsed.Decision.NextNodeID
	result.Policy = activePolicySummary(nextState)
	result.NextChild = nextChildInfo(nextState, parsed.Decision.NextNodeID)
	result.WaitingHuman = parsed.Decision.WaitingHuman
	result.StopReason = parsed.Decision.StopReason
	if result.WaitingHuman {
		result.HumanPrompt = humanPromptContext(state, opts.StateFile, parsed)
		card := humanGateActionCard(state, opts.StateFile, result.HumanPrompt)
		result.ActionCard = &card
		nextState.LastActionCard = &card
		_ = store.Save(opts.StateFile, nextState)
	}

	if parsed.Decision.StartNext {
		var launched ManagerState
		if launchIntent != nil {
			launched, err = startChildFromIntent(
				ctx,
				nextState,
				*launchIntent,
				opts,
				false,
				d,
				out,
			)
		} else {
			launched, err = startNextChildFromDecision(
				ctx,
				nextState,
				parsed.Decision,
				opts,
				d,
				out,
			)
		}
		if err != nil {
			if launchIntent != nil && state.ActiveChild != nil {
				durable := nextState
				if loaded, loadErr := store.Load(opts.StateFile); loadErr == nil {
					durable = loaded
				}
				card := buildHandoffContinuationFailureCard(
					opts.StateFile,
					*state.ActiveChild,
					parsed.Result,
					err,
				)
				if durable.ActiveChild != nil && durable.ActiveChild.ID == state.ActiveChild.ID {
					durable.ActiveChild.LifecycleStatus = "awaiting_manager"
				}
				durable.LastActionCard = card
				if saveErr := store.Save(opts.StateFile, durable); saveErr != nil {
					return saveErr
				}
				return writeManagerActionCard(out, *card, opts.Output)
			}
			return err
		}
		launched, _, err = maybeStartManagerCompaction(
			ctx,
			launched,
			opts.StateFile,
			opts.Usage,
			d,
			out,
		)
		if err != nil {
			return err
		}
		result.StartedChild = launched.ActiveChild
	}
	return writeContinueOutput(out, opts, result)
}

func BuildChildContextExhaustedCard(
	health ActiveChildHealth,
	state ManagerState,
	stateFile string,
) *ManagerActionCard {
	if health.Status != ActiveChildContextExhausted &&
		health.Status != ActiveChildProviderContextError {
		return nil
	}
	evidence := []string{
		fmt.Sprintf(
			"child: %s stage=%s",
			health.ChildID,
			firstNonEmpty(health.Stage, string(state.Workflow.CurrentNodeID)),
		),
		fmt.Sprintf("session: %s", firstNonEmpty(health.SessionPath, health.SessionDir)),
		fmt.Sprintf("status: %s", health.Status),
	}
	evidence = append(evidence, health.Evidence...)
	for _, line := range health.OutputTail {
		evidence = append(evidence, "output tail: "+line)
	}
	summary := "child ended without valid qrspi_result after context-limit evidence"
	if IsTerminalProviderContextError(health) {
		summary = "child ended with terminal provider context-window evidence; latest session outranks stale qrspi_result"
	}
	if health.TerminalEvidence != nil {
		if command := providerContextRecoverySummaryCommand(
			stateFile,
			*health.TerminalEvidence,
		); command != "" {
			evidence = append(evidence, "recovery summary: "+command)
		}
	}
	return &ManagerActionCard{
		Kind:              ActionChildContextExhausted,
		Severity:          "warning",
		Summary:           summary,
		Evidence:          evidence,
		RecommendedAction: "inspect latest session evidence, then recover/relaunch the same graph node without inventing a qrspi_result",
		SafeCommand:       providerContextRecoverySafeCommand(stateFile),
		ContinueCommand:   providerContextRecoveryContinueCommand(stateFile),
		RequiresHuman:     false,
	}
}

func validateActiveChild(
	ctx context.Context,
	state ManagerState,
	opts ContinueOptions,
) (ParsedDecision, error) {
	_ = ctx
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{})
	if err != nil {
		return ParsedDecision{}, err
	}
	parseCtx.ExpectedNodeID = wruntime.NodeID(opts.Stage)
	return ParseNormalizeValidateDecide(text, state, parseCtx)
}

func invalidResultRetryLimit(state ManagerState) int {
	limit := 1
	if len(state.Workflow.Policy) == 0 {
		return limit
	}
	var cfg struct {
		InvalidResultRetryLimit *int `json:"invalidResultRetryLimit"`
	}
	if json.Unmarshal(state.Workflow.Policy, &cfg) == nil &&
		cfg.InvalidResultRetryLimit != nil {
		return *cfg.InvalidResultRetryLimit
	}
	return limit
}

func shouldRepromptAfterValidationError(
	state ManagerState,
	opts ContinueOptions,
	err error,
) bool {
	if err == nil || state.ActiveChild == nil {
		return false
	}
	if state.ActiveChild.Stage != opts.Stage {
		return false
	}
	if strings.TrimSpace(state.ActiveChild.TmuxPaneID) == "" {
		return false
	}
	return state.ActiveChild.ValidationRetryCount < invalidResultRetryLimit(state)
}

func isRetryExhaustedValidationError(
	state ManagerState,
	opts ContinueOptions,
	err error,
) bool {
	if err == nil || state.ActiveChild == nil {
		return false
	}
	if state.ActiveChild.Stage != opts.Stage {
		return false
	}
	return state.ActiveChild.ValidationRetryCount >= invalidResultRetryLimit(state)
}

func writeRetryExhaustedNotice(
	out io.Writer,
	opts ContinueOptions,
	state ManagerState,
	validationErr error,
) error {
	stage := strings.TrimSpace(opts.Stage)
	if stage == "" && state.ActiveChild != nil {
		stage = state.ActiveChild.Stage
	}
	pane := ""
	attempt := invalidResultRetryLimit(state)
	if state.ActiveChild != nil {
		pane = state.ActiveChild.TmuxPaneID
		attempt = state.ActiveChild.ValidationRetryCount
	}
	guidance := "Inspect child output/artifacts; recover or steer deterministically before asking human."
	notice := ManagerNotice{
		Kind:           "retry_exhausted",
		Validated:      false,
		ManagerNeeded:  true,
		RetryExhausted: true,
		StateFile:      opts.StateFile,
		Stage:          stage,
		Status:         "invalid_result",
		ChildPane:      pane,
		Summary: fmt.Sprintf(
			"invalid result after retry limit (%d): %s",
			attempt,
			validationErr.Error(),
		),
		ManagerGuidance: guidance,
		NextCommand:     debugCommandForState(opts.StateFile, "continue"),
		FeedbackCommand: feedbackCommand(opts.StateFile),
	}
	_ = appendValidationRecoveryLog(opts.StateFile, ValidationRecoveryLog{
		StateFile:        opts.StateFile,
		PlanDir:          opts.PlanDir,
		CurrentNode:      string(state.Workflow.CurrentNodeID),
		ActiveChildStage: stage,
		Recovered:        false,
		Reason:           validationErr.Error(),
	})
	return writeManagerNotice(out, notice, opts.Output)
}

func continueReprompt(
	ctx context.Context,
	state ManagerState,
	opts ContinueOptions,
	d deps,
	out io.Writer,
	validationErr error,
) error {
	_ = out
	attempt := state.ActiveChild.ValidationRetryCount + 1
	if state.ActiveChild.LastRepromptAttempt >= attempt {
		return fmt.Errorf(
			"reprompt attempt %d already sent for active child %s",
			attempt,
			state.ActiveChild.ID,
		)
	}
	if strings.TrimSpace(state.ActiveChild.DonePath) != "" {
		if err := os.Remove(
			state.ActiveChild.DonePath,
		); err != nil &&
			!os.IsNotExist(err) {
			return err
		}
	}
	if err := RunRepromptChild(ctx, RepromptChildOptions{
		StateFile: opts.StateFile,
		PlanDir:   opts.PlanDir,
		Stage:     opts.Stage,
		Attempt:   attempt,
		ErrorText: validationErr.Error(),
	}, d, io.Discard); err != nil {
		return err
	}
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	latest, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if latest.ActiveChild == nil || latest.ActiveChild.ID != state.ActiveChild.ID {
		return errors.New("active child changed during reprompt")
	}
	latest.ActiveChild.ValidationRetryCount = attempt
	latest.ActiveChild.LastRepromptAttempt = attempt
	return store.Save(opts.StateFile, latest)
}

func decideValidatedResult(
	ctx context.Context,
	state ManagerState,
	parsed ParsedDecision,
	opts ContinueOptions,
	store StateStore,
) (ManagerState, error) {
	_ = ctx
	state.Workflow = parsed.Decision.State
	state = UpdateImplementationCwd(state, parsed.Result)
	if parsed.Decision.StartNext {
		state = markPendingCleanup(state)
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return ManagerState{}, err
	}
	return state, nil
}

func startNextChildFromDecision(
	ctx context.Context,
	state ManagerState,
	decision wruntime.TransitionDecision,
	opts ContinueOptions,
	d deps,
	out io.Writer,
) (ManagerState, error) {
	nodeID := decision.NextNodeID
	if nodeID == "" {
		return state, errors.New("transition has no next node")
	}
	def, err := Definition()
	if err != nil {
		return state, err
	}
	node, ok := def.Nodes[nodeID]
	if !ok {
		return state, fmt.Errorf("node %q is not in QRSPI definition", nodeID)
	}
	cwd, err := defaultChildCwd(state, nodeID, opts.Cwd)
	if err != nil {
		return state, err
	}
	return startChildFromIntent(
		ctx,
		state,
		ChildLaunchIntent{
			Kind:            ChildLaunchNormal,
			NodeID:          nodeID,
			SkillPath:       node.Prompt.SkillPath,
			PrimaryArtifact: latestPrimaryArtifact(state.Workflow.LastResult),
			Cwd:             cwd,
		},
		opts,
		false,
		d,
		out,
	)
}

func startChildFromIntent(
	ctx context.Context,
	state ManagerState,
	intent ChildLaunchIntent,
	opts ContinueOptions,
	deferPendingCleanup bool,
	d deps,
	out io.Writer,
) (ManagerState, error) {
	if intent.NodeID == "" {
		return state, errors.New("child launch intent has no node")
	}
	if strings.TrimSpace(intent.Cwd) == "" {
		return state, errors.New("child launch intent has no cwd")
	}
	promptFile, err := renderLaunchPromptFile(ctx, state, intent, opts)
	if err != nil {
		return state, err
	}
	planDir := strings.TrimSpace(opts.PlanDir)
	if planDir == "" {
		planDir = state.CanonicalPlanDir
	}
	runOut := io.Writer(io.Discard)
	if strings.EqualFold(opts.Output, "ndjson") {
		runOut = out
	}
	if err := RunChild(ctx, RunChildOptions{
		PlanDir:             planDir,
		Stage:               string(intent.NodeID),
		Cwd:                 intent.Cwd,
		PromptFile:          promptFile,
		StateFile:           opts.StateFile,
		Split:               opts.Split,
		PiModel:             resolvePiModel(opts.PiModel, state.PiModel),
		ManagerPane:         opts.ManagerPane,
		Timeout:             opts.Timeout,
		Launch:              &intent,
		DeferPendingCleanup: deferPendingCleanup,
	}, d, runOut); err != nil {
		return state, err
	}
	store := stateStore(d, "", time.Now)
	return store.Load(opts.StateFile)
}

func defaultContinueCwd(state ManagerState, node wruntime.NodeID) string {
	cwd, err := defaultChildCwd(state, node, "")
	if err != nil {
		return ""
	}
	return cwd
}

func renderLaunchPromptFile(
	ctx context.Context,
	state ManagerState,
	intent ChildLaunchIntent,
	opts ContinueOptions,
) (string, error) {
	def, err := Definition()
	if err != nil {
		return "", err
	}
	node, ok := def.Nodes[intent.NodeID]
	if !ok {
		return "", fmt.Errorf("node %q is not in QRSPI definition", intent.NodeID)
	}
	return WriteStagePromptFile(
		ctx,
		state,
		node,
		PromptFileOptions{
			StateFile: opts.StateFile,
			NodeID:    string(intent.NodeID),
			Timestamp: time.Now(),
			Launch:    &intent,
		},
	)
}

func humanPromptContext(
	state ManagerState,
	stateFile string,
	parsed ParsedDecision,
) HumanPromptContext {
	result := parsed.Result
	return HumanPromptContext{
		Stage:                    string(result.SourceNodeID),
		Status:                   string(result.Status),
		Summary:                  strings.TrimSpace(result.Summary),
		Artifact:                 result.PrimaryArtifact,
		SuggestedFeedbackCommand: feedbackCommand(stateFile),
	}
}

func buildContinueActionCard(
	state ManagerState,
	opts ContinueOptions,
	err error,
) *ManagerActionCard {
	if state.ActiveChild == nil {
		return &ManagerActionCard{
			Kind:     ActionActiveChildConflict,
			Severity: "warning",
			Summary:  "no active child to continue",
			Evidence: []string{
				fmt.Sprintf("current node: %s", state.Workflow.CurrentNodeID),
			},
			RecommendedAction: "start or inspect the graph-selected child",
			SafeCommand: fmt.Sprintf(
				"vamos qrspi start-next --state-file %s",
				opts.StateFile,
			),
			ContinueCommand: continueCommand(opts.StateFile),
			RequiresHuman:   false,
		}
	}
	if err == nil {
		return nil
	}
	if state.ActiveChild.Stage != string(state.Workflow.CurrentNodeID) {
		card := buildStateDesyncActionCard(state, opts.StateFile, err)
		return &card
	}
	if looksWorkspaceMoved(state, err) {
		return &ManagerActionCard{
			Kind:              ActionWorkspaceMoved,
			Severity:          "warning",
			Summary:           "implementation workspace differs from current child cwd",
			Evidence:          workspaceMovedEvidence(state),
			RecommendedAction: "run q-manager continue from the recorded implementation workspace",
			SafeCommand: fmt.Sprintf(
				"cd %q && vamos qrspi continue --state-file %s",
				state.ImplementationCwd,
				opts.StateFile,
			),
			ContinueCommand: continueCommand(opts.StateFile),
			RequiresHuman:   false,
		}
	}
	kind := ActionInvalidChildYAML
	if strings.Contains(strings.ToLower(err.Error()), "canonical qrspi graph rejected") ||
		strings.Contains(strings.ToLower(err.Error()), "outcome") {
		kind = ActionGraphOutcomeMismatch
	}
	return &ManagerActionCard{
		Kind:     kind,
		Severity: "warning",
		Summary:  "child result needs deterministic repair",
		Evidence: []string{
			err.Error(),
			fmt.Sprintf("active child stage: %s", state.ActiveChild.Stage),
			fmt.Sprintf(
				"retry: %d/%d",
				state.ActiveChild.ValidationRetryCount,
				invalidResultRetryLimit(state),
			),
		},
		RecommendedAction: "reprompt or steer the active child with canonical YAML",
		SafeCommand: fmt.Sprintf(
			"vamos qrspi reprompt-child --state-file %s --plan-dir %s --stage %s --attempt %d",
			opts.StateFile,
			opts.PlanDir,
			state.ActiveChild.Stage,
			state.ActiveChild.ValidationRetryCount+1,
		),
		ContinueCommand: continueCommand(opts.StateFile),
		RequiresHuman:   false,
	}
}

func buildStateDesyncActionCard(
	state ManagerState,
	stateFile string,
	err error,
) ManagerActionCard {
	evidence := []string{fmt.Sprintf("current node: %s", state.Workflow.CurrentNodeID)}
	if state.ActiveChild != nil {
		evidence = append(
			evidence,
			fmt.Sprintf("active child stage: %s", state.ActiveChild.Stage),
			fmt.Sprintf("active child id: %s", state.ActiveChild.ID),
		)
		if state.ActiveChild.SessionPath != "" {
			evidence = append(
				evidence,
				fmt.Sprintf("session: %s", state.ActiveChild.SessionPath),
			)
		}
	}
	if err != nil {
		evidence = append(evidence, err.Error())
	}
	return ManagerActionCard{
		Kind:              ActionStateDesync,
		Severity:          "warning",
		Summary:           "workflow cursor and active child are out of sync",
		Evidence:          evidence,
		RecommendedAction: "align active child, then continue",
		SafeCommand: fmt.Sprintf(
			"vamos qrspi repair-state --state-file %s --align-active-child && vamos qrspi continue --state-file %s",
			stateFile,
			stateFile,
		),
		ContinueCommand: continueCommand(stateFile),
		RequiresHuman:   false,
	}
}

func humanGateActionCard(
	state ManagerState,
	stateFile string,
	prompt HumanPromptContext,
) ManagerActionCard {
	review := strings.TrimSpace(prompt.Summary)
	if review == "" {
		review = fmt.Sprintf("review %s artifact before steering feedback", prompt.Stage)
	}
	action := semantic.NextAction{
		Kind:            semantic.NextActionWaitHuman,
		Severity:        "info",
		CurrentNodeID:   wruntime.NodeID(prompt.Stage),
		Status:          wruntime.StatusNeedsHuman,
		PrimaryArtifact: prompt.Artifact,
		RecoveryReason:  "child requested human input",
	}
	card := ProjectManagerActionCard(action, state, stateFile)
	if card == nil {
		return ManagerActionCard{}
	}
	card.ReviewSummary = review
	return *card
}

func manualChildSteerActionCard(state ManagerState, stateFile string) ManagerActionCard {
	evidence := []string{}
	if state.ActiveChild != nil {
		evidence = append(
			evidence,
			fmt.Sprintf("active child: %s", state.ActiveChild.ID),
			fmt.Sprintf("generation: %d", activeChildGeneration(state)),
		)
	}
	return ManagerActionCard{
		Kind:              ActionManualChildSteer,
		Severity:          "info",
		Summary:           "child marked active after manual steering",
		Evidence:          evidence,
		RecommendedAction: "wait for newer completion before flushing queued wakes",
		SafeCommand:       continueCommand(stateFile),
		ContinueCommand:   continueCommand(stateFile),
		RequiresHuman:     false,
	}
}

func looksWorkspaceMoved(state ManagerState, err error) bool {
	if strings.TrimSpace(state.ImplementationCwd) == "" || state.ActiveChild == nil {
		return false
	}
	if state.ActiveChild.Cwd != "" &&
		filepath.Clean(state.ActiveChild.Cwd) != filepath.Clean(state.ImplementationCwd) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "workspace") ||
		strings.Contains(strings.ToLower(err.Error()), "cwd")
}

func workspaceMovedEvidence(state ManagerState) []string {
	evidence := []string{
		fmt.Sprintf("implementation workspace: %s", state.ImplementationCwd),
	}
	if state.ActiveChild != nil {
		evidence = append(
			evidence,
			fmt.Sprintf("active child cwd: %s", state.ActiveChild.Cwd),
		)
	}
	return evidence
}

func writeManagerActionCard(out io.Writer, card ManagerActionCard, mode string) error {
	if strings.EqualFold(mode, "ndjson") || strings.EqualFold(mode, "json") {
		return WriteNDJSON(out, Event{Type: "manager_action", ActionCard: &card})
	}
	if card.Kind != "" {
		fmt.Fprintf(out, "action: %s\n", card.Kind)
	}
	if card.Summary != "" {
		fmt.Fprintf(out, "summary: %s\n", card.Summary)
	}
	for _, evidence := range card.Evidence {
		fmt.Fprintf(out, "evidence: %s\n", evidence)
	}
	if card.ReviewSummary != "" {
		fmt.Fprintf(out, "review: %s\n", card.ReviewSummary)
	}
	if card.RecommendedAction != "" {
		fmt.Fprintf(out, "recommended: %s\n", card.RecommendedAction)
	}
	if card.SafeCommand != "" {
		fmt.Fprintf(out, "safe command: %s\n", card.SafeCommand)
	}
	if card.ContinueCommand != "" {
		fmt.Fprintf(out, "continue: %s\n", card.ContinueCommand)
	}
	return nil
}

func appendRecoveryIncident(
	stateFile string,
	card ManagerActionCard,
	recovered bool,
) error {
	if strings.TrimSpace(stateFile) == "" {
		return nil
	}
	entry := ValidationRecoveryLog{
		Timestamp:      time.Now(),
		StateFile:      stateFile,
		Recovered:      recovered,
		RecoveryAction: card.Kind,
		Reason:         card.Summary,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(stateFile), "validation-recoveries.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func writeContinueOutput(
	out io.Writer,
	opts ContinueOptions,
	result ContinueResult,
) error {
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(
			out,
			Event{
				Type:       "continued",
				Decision:   result.Validated,
				ActionCard: result.ActionCard,
				Ref:        continueRef(result),
			},
		)
	}
	if err := writeContinueText(out, result); err != nil {
		return err
	}
	if result.ActionCard != nil {
		return writeManagerActionCard(out, *result.ActionCard, opts.Output)
	}
	return nil
}

func writeContinueText(out io.Writer, result ContinueResult) error {
	if result.Validated != nil {
		fmt.Fprintf(
			out,
			"validated: %s %s\n",
			result.Validated.Result.SourceNodeID,
			result.Validated.Result.Status,
		)
		if result.Validated.Result.Outcome != "" {
			fmt.Fprintf(out, "outcome: %s\n", result.Validated.Result.Outcome)
		}
	}
	if result.PrimaryArtifact != "" {
		fmt.Fprintf(out, "artifact: %s\n", result.PrimaryArtifact)
	}
	if result.Validated != nil {
		summary := childCompletionResult(result.Validated.Result)
		if summary.PlanGoal != "" {
			fmt.Fprintf(out, "plan goal: %s\n", summary.PlanGoal)
		}
		if summary.StageCompleted != "" {
			fmt.Fprintf(out, "stage completed: %s\n", summary.StageCompleted)
		}
		if summary.KeyDecisions != "" {
			fmt.Fprintf(out, "key decisions: %s\n", summary.KeyDecisions)
		}
	}
	if result.Reprompted {
		_, err := fmt.Fprintln(out, "retry: reprompted active child")
		return err
	}
	if result.Policy.AdvanceMode != "" {
		fmt.Fprintf(
			out,
			"policy: %s, plan reviews %s, retries %d\n",
			result.Policy.AdvanceMode,
			onOff(result.Policy.EnablePlanReviews),
			result.Policy.InvalidResultRetryLimit,
		)
	}
	if result.NextNodeID != "" {
		fmt.Fprintf(out, "next: %s\n", result.NextNodeID)
	}
	if result.NextChild.Stage != "" {
		fmt.Fprintf(out, "next child: %s\n", result.NextChild.Stage)
		if result.NextChild.WorkingOn != "" {
			fmt.Fprintf(out, "working on: %s\n", result.NextChild.WorkingOn)
		}
		if result.NextChild.Cwd != "" {
			fmt.Fprintf(out, "cwd: %s\n", result.NextChild.Cwd)
		}
	}
	if result.StartedChild != nil {
		fmt.Fprintf(
			out,
			"started child: %s (%s)\n",
			result.StartedChild.Stage,
			result.StartedChild.TmuxPaneID,
		)
	}
	if result.WaitingHuman {
		if _, err := fmt.Fprintln(out, "stop: waiting human"); err != nil {
			return err
		}
		if result.HumanPrompt.Summary != "" {
			fmt.Fprintf(out, "question: %s\n", result.HumanPrompt.Summary)
		}
		if result.HumanPrompt.SuggestedFeedbackCommand != "" {
			fmt.Fprintf(
				out,
				"feedback: %s\n",
				result.HumanPrompt.SuggestedFeedbackCommand,
			)
		}
		return nil
	}
	if result.StopReason != "" && result.StartedChild == nil {
		fmt.Fprintf(out, "stop: %s\n", result.StopReason)
		if result.Validated != nil &&
			managerGuidanceForStatus(string(result.Validated.Result.Status)) != "" {
			fmt.Fprintf(
				out,
				"guidance: %s\n",
				managerGuidanceForStatus(string(result.Validated.Result.Status)),
			)
		}
	}
	return nil
}

func managerGuidanceForStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "blocked", "error":
		return "diagnose child artifact/session first; steer or continue if deterministic; ask human only for judgment or authority."
	case "invalid_result":
		return "inspect child output/artifacts; recover or steer deterministically before asking human."
	default:
		return ""
	}
}

func continueRef(result ContinueResult) map[string]any {
	ref := map[string]any{
		"reprompted":   result.Reprompted,
		"waitingHuman": result.WaitingHuman,
	}
	if result.Validated != nil {
		ref["validated"] = true
		ref["stage"] = result.Validated.Result.SourceNodeID
		ref["status"] = result.Validated.Result.Status
		if guidance := managerGuidanceForStatus(
			string(result.Validated.Result.Status),
		); guidance != "" {
			ref["managerNeeded"] = true
			ref["managerGuidance"] = guidance
		}
	}
	if result.WaitingHuman {
		ref["managerNeeded"] = true
		ref["humanPrompt"] = result.HumanPrompt
	}
	if result.NextNodeID != "" {
		ref["nextNode"] = result.NextNodeID
	}
	if result.Policy.AdvanceMode != "" {
		ref["policy"] = result.Policy
	}
	if result.NextChild.Stage != "" {
		ref["nextChild"] = result.NextChild
	}
	if result.PrimaryArtifact != "" {
		ref["artifact"] = result.PrimaryArtifact
	}
	if result.StopReason != "" {
		ref["stopReason"] = result.StopReason
	}
	if result.ActionCard != nil {
		ref["actionCardKind"] = result.ActionCard.Kind
	}
	if result.StartedChild != nil {
		ref["startedChild"] = childRef(result.StartedChild)
	}
	return ref
}

func RunDecideNext(
	ctx context.Context,
	opts DecideNextOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	text, parseCtx, err := ReadChildResultText(
		state,
		ResultSourceOptions{ResultFile: opts.ResultFile, SessionFile: opts.SessionFile},
	)
	if err != nil {
		return err
	}
	parseCtx.ExpectedNodeID = state.Workflow.CurrentNodeID
	parsed, err := ParseNormalizeValidateDecide(text, state, parseCtx)
	if err != nil {
		return err
	}
	state.Workflow = parsed.Decision.State
	state = UpdateImplementationCwd(state, parsed.Result)
	if parsed.Decision.StartNext {
		state = markPendingCleanup(state)
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	return WriteNDJSON(out, Event{
		Type:     "decided",
		Decision: &parsed,
		Ref: map[string]any{
			"nextNode":          parsed.Decision.NextNodeID,
			"startNext":         parsed.Decision.StartNext,
			"waitingHuman":      parsed.Decision.WaitingHuman,
			"stopReason":        parsed.Decision.StopReason,
			"implementationCwd": state.ImplementationCwd,
		},
	})
}

func ReadChildResultText(
	state ManagerState,
	opts ResultSourceOptions,
) (string, wruntime.ParseContext, error) {
	ctx := wruntime.ParseContext{RunID: opts.RunID, SessionID: opts.SessionID}
	if strings.TrimSpace(opts.SessionFile) != "" {
		text, err := ExtractFinalAssistantTextFromSession(opts.SessionFile)
		if err != nil {
			return "", ctx, err
		}
		if ctx.SessionID == "" && state.ActiveChild != nil {
			ctx.SessionID = state.ActiveChild.SessionID
		}
		return text, ctx, nil
	}
	if state.ActiveChild != nil {
		ref := state.ActiveChild
		sessionPath := strings.TrimSpace(ref.SessionPath)
		if sessionPath == "" {
			resolved, err := ResolveSessionPath(ref.SessionDir, ref.SessionID, ref.Cwd)
			if err != nil {
				return "", ctx, err
			}
			sessionPath = resolved
		}
		text, err := ExtractFinalAssistantTextFromSession(sessionPath)
		if err != nil {
			return "", ctx, err
		}
		if ctx.RunID == "" {
			ctx.RunID = ref.ID
		}
		if ctx.SessionID == "" {
			ctx.SessionID = ref.SessionID
		}
		return text, ctx, nil
	}
	if strings.TrimSpace(opts.ResultFile) != "" {
		data, err := os.ReadFile(opts.ResultFile)
		if err != nil {
			return "", ctx, err
		}
		return string(data), ctx, nil
	}
	return "", ctx, errors.New(
		"no child result source: keep active child session refs, use latest-session recovery commands, pass --session-file for a specific JSONL, or use deprecated --result-file only as a debug fallback",
	)
}

func RunRenderPrompt(
	ctx context.Context,
	opts RenderPromptOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.NodeID) == "" {
		return errors.New("node is required")
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	_ = ctx
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	def, err := Definition()
	if err != nil {
		return err
	}
	node, ok := def.Nodes[wruntime.NodeID(opts.NodeID)]
	if !ok {
		return fmt.Errorf("node %q is not in QRSPI definition", opts.NodeID)
	}
	manifest, err := LoadManifest(state.SourceCwd)
	if err != nil {
		return err
	}
	prompt, err := RenderStagePrompt(PromptContext{
		Node:       node,
		State:      state,
		PlanDir:    opts.PlanDir,
		Manifest:   manifest,
		LastResult: state.Workflow.LastResult,
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, prompt)
	return err
}

func ensureWriter(out io.Writer) io.Writer {
	if out == nil {
		return os.Stdout
	}
	return out
}

func initialPolicy(path, preset string) (json.RawMessage, error) {
	if strings.TrimSpace(path) != "" && strings.TrimSpace(preset) != "" {
		return nil, errors.New("use either --policy-file or --policy-preset, not both")
	}
	if strings.TrimSpace(preset) != "" {
		policy, err := policyFromSetOptions(
			SetPolicyOptions{Preset: preset},
			qrspi.DefaultPolicy(),
		)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(policy)
		if err != nil {
			return nil, err
		}
		return encoded, nil
	}
	return readPolicyFile(path)
}

func readPolicyFile(path string) (json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func managerRunID(t time.Time) string {
	return fmt.Sprintf("%s-%09d", t.Format("20060102150405"), t.Nanosecond())
}

func stateRoot(d deps) (string, error) {
	if d.StateRoot != nil {
		return d.StateRoot()
	}
	return DefaultStateRoot()
}

func stateStore(d deps, root string, clock func() time.Time) StateStore {
	if d.StateStore != nil {
		return d.StateStore
	}
	return FileStateStore{Root: root, Clock: clock}
}

func childRunner(d deps) ChildRunner {
	if d.Runner != nil {
		return d.Runner
	}
	return TmuxChildRunner{Tmux: tmuxClient(d)}
}

func tmuxClient(d deps) TmuxClient {
	if d.Tmux != nil {
		return d.Tmux
	}
	return ShellTmuxClient{}
}

func childRunID(stage string, t time.Time) string {
	clean := strings.NewReplacer("/", "-", " ", "-", "_", "-").
		Replace(strings.TrimSpace(stage))
	if clean == "" {
		clean = "child"
	}
	return fmt.Sprintf("%s-%s-%09d", clean, t.Format("20060102150405"), t.Nanosecond())
}

func nextChildRunID(state ManagerState, stage string, when time.Time) string {
	base := childRunID(stage, when)
	used := func(id string) bool {
		return (state.ActiveChild != nil && state.ActiveChild.ID == id) ||
			(state.PendingCleanupChild != nil && state.PendingCleanupChild.ID == id)
	}
	if !used(base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !used(candidate) {
			return candidate
		}
	}
}

func CaptureManagerPaneID(explicit string) string {
	if pane := strings.TrimSpace(explicit); pane != "" {
		return pane
	}
	return strings.TrimSpace(os.Getenv("TMUX_PANE"))
}

func markPendingCleanup(state ManagerState) ManagerState {
	if state.ActiveChild != nil {
		state.PendingCleanupChild = state.ActiveChild
	}
	return state
}

func cleanupPendingChildAfterNextStart(
	ctx context.Context,
	state ManagerState,
	tmux TmuxClient,
) (ManagerState, error) {
	return cleanupPendingChildAfterNotification(ctx, state, tmux)
}

func cleanupPendingChildAfterNotification(
	ctx context.Context,
	state ManagerState,
	tmux TmuxClient,
) (ManagerState, error) {
	ref := state.PendingCleanupChild
	if ref == nil {
		return state, nil
	}
	if strings.TrimSpace(ref.TmuxPaneID) == "" {
		state.PendingCleanupChild = nil
		return state, nil
	}
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	pane := TmuxPane{ID: ref.TmuxPaneID}
	exists, err := tmux.PaneExists(ctx, pane)
	if err != nil {
		return state, err
	}
	if !exists {
		state.PendingCleanupChild = nil
		return state, nil
	}
	if err := tmux.KillPane(ctx, pane); err != nil {
		return state, err
	}
	if state.ActiveChild != nil && strings.TrimSpace(state.ActiveChild.TmuxPaneID) != "" {
		if err := tmux.SelectLayout(
			ctx,
			TmuxPane{ID: state.ActiveChild.TmuxPaneID},
			"even-horizontal",
		); err != nil {
			return state, err
		}
	}
	state.PendingCleanupChild = nil
	return state, nil
}

func normalizeSplit(split string) string {
	switch strings.TrimSpace(split) {
	case "", "right":
		return "right"
	case "down":
		return "down"
	default:
		return split
	}
}

func waitForDone(ctx context.Context, path string, interval time.Duration) error {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func childRef(ref *ChildRunRef) map[string]any {
	if ref == nil {
		return nil
	}
	out := map[string]any{
		"childId":    ref.ID,
		"stage":      ref.Stage,
		"cwd":        ref.Cwd,
		"tmuxPaneId": ref.TmuxPaneID,
		"outputPath": ref.OutputPath,
		"sessionId":  ref.SessionID,
		"sessionDir": ref.SessionDir,
		"donePath":   ref.DonePath,
		"statusPath": ref.StatusPath,
	}
	if strings.TrimSpace(ref.SessionPath) != "" {
		out["sessionPath"] = ref.SessionPath
	}
	if strings.TrimSpace(ref.ResultPath) != "" {
		out["resultPath"] = ref.ResultPath
	}
	if strings.TrimSpace(ref.ValidationStatusPath) != "" {
		out["validationStatusPath"] = ref.ValidationStatusPath
	}
	if strings.TrimSpace(ref.LifecycleStatus) != "" {
		out["lifecycleStatus"] = ref.LifecycleStatus
	}
	if ref.Generation != 0 {
		out["generation"] = ref.Generation
	}
	return out
}
