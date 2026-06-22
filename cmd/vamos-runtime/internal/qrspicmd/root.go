package qrspicmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
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
		newStartNextCommand(d),
		newSteerChildCommand(d),
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
		newValidateResultCommand(d),
		newDecideNextCommand(d),
		newRepromptChildCommand(d),
		newContinueCommand(d),
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
	cmd.Flags().StringVar(&opts.NodeID, "node", "", "initial QRSPI node ID (defaults to graph start)")
	cmd.Flags().StringVar(&opts.NodeID, "stage", "", "alias for --node")
	cmd.Flags().StringVar(&opts.ImplementationCwd, "implementation-cwd", "", "implementation workspace cwd for implementation/review/verify stages")
	cmd.Flags().StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "replace existing expired/inactive state")
	return cmd
}

func newStartNextCommand(d deps) *cobra.Command {
	opts := StartNextOptions{Split: "right", Timeout: 0, Output: "text"}
	var usagePercent float64
	var usageTokens int
	var usageWindow int
	cmd := &cobra.Command{
		Use:   "start-next --plan-dir <path> --project-root <path>",
		Short: "Initialize or resume q-manager state and launch the graph-selected child",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Usage = usageFromChangedFlags(cmd, usagePercent, usageTokens, usageWindow)
			_, err := RunStartNext(cmd.Context(), opts, d, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory")
	cmd.Flags().StringVar(&opts.ProjectRoot, "project-root", "", "project repository root")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.PolicyFile, "policy-file", "", "optional policy JSON file")
	cmd.Flags().StringVar(&opts.NodeID, "node", "", "QRSPI node ID override")
	cmd.Flags().StringVar(&opts.NodeID, "stage", "", "alias for --node")
	cmd.Flags().StringVar(&opts.ImplementationCwd, "implementation-cwd", "", "implementation workspace cwd for implementation/review/verify stages")
	cmd.Flags().StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().StringVar(&opts.LatestResultFile, "latest-result-file", "", "file containing latest fenced qrspi_result YAML")
	cmd.Flags().BoolVar(&opts.LatestResultStdin, "latest-result-stdin", false, "read latest fenced qrspi_result YAML from stdin")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "child cwd override")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 0, "maximum time to wait for child; 0 returns after launch")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "replace existing expired/inactive state")
	cmd.Flags().Float64Var(&usagePercent, "manager-usage-percent", 0, "parent manager context usage percent for optional compaction")
	cmd.Flags().IntVar(&usageTokens, "manager-usage-tokens", 0, "parent manager context token count for optional compaction")
	cmd.Flags().IntVar(&usageWindow, "manager-usage-window", 0, "parent manager context window size for optional compaction")
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
	cmd.Flags().StringVar(&opts.FeedbackFile, "feedback-file", "", "file containing feedback for the active child")
	cmd.Flags().StringVar(&opts.Feedback, "feedback", "", "inline feedback for the active child")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected active child node")
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
	cmd.Flags().StringVar(&opts.PromptFile, "prompt-file", "", "rendered child prompt file")
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction")
	cmd.Flags().StringVar(&opts.ManagerRunID, "manager-run-id", "", "manager run ID")
	cmd.Flags().StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the parent q-manager session")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 12*time.Hour, "maximum time to wait for child done marker")
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
	cmd.Flags().StringVar(&opts.ManagerPane, "manager-pane", "", "tmux pane ID for the resumed parent q-manager session")
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
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text, ndjson, or json")
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
	cmd.Flags().BoolVar(&opts.AlignActiveChild, "align-active-child", false, "align current workflow node to the active child stage when safe")
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
	cmd.Flags().StringVar(&opts.Reason, "reason", "manual-reprompt", "reason for marking child active")
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
	cmd.Flags().BoolVar(&opts.Sessions, "sessions", false, "include active child session refs")
	cmd.Flags().BoolVar(&opts.Latest, "latest", false, "include latest relevant session candidate")
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
	cmd.Flags().StringVar(&opts.SessionFile, "session-file", "", "child Pi session JSONL file")
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
	cmd.Flags().BoolVar(&opts.ApplyRebind, "apply-rebind", false, "rebind active child before validating")
	cmd.Flags().BoolVar(&opts.Continue, "continue", false, "continue graph after validation")
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
	cmd.Flags().BoolVar(&opts.Continue, "continue", false, "continue graph after rebind/validation")
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
	cmd.Flags().StringVar(&opts.SessionFile, "session-file", "", "explicit child Pi session JSONL file")
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback only when session/latest-session recovery is unavailable")
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
	cmd.Flags().StringVar(&opts.SessionFile, "session-file", "", "explicit child Pi session JSONL file")
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback only when session/latest-session recovery is unavailable")
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
	cmd.Flags().StringVar(&opts.ErrorFile, "error-file", "", "file containing validation error text")
	return cmd
}

func newContinueCommand(d deps) *cobra.Command {
	opts := ContinueOptions{Split: "right", Timeout: 0, Output: "text"}
	var usagePercent float64
	var usageTokens int
	var usageWindow int
	cmd := &cobra.Command{
		Use:   "continue --state-file <file>",
		Short: "Validate active child and continue the QRSPI graph when safe",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Usage = usageFromChangedFlags(cmd, usagePercent, usageTokens, usageWindow)
			return RunContinue(cmd.Context(), opts, d, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.StateFile, "state-file", "", "q-manager state file")
	cmd.Flags().StringVar(&opts.PlanDir, "plan-dir", "", "QRSPI plan directory; defaults from state")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "expected active child node; defaults from state")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "next child cwd override")
	cmd.Flags().StringVar(&opts.Split, "split", "right", "tmux split direction for next child")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 0, "maximum time to wait for next child; 0 returns after launch")
	cmd.Flags().StringVar(&opts.Output, "output", "text", "output format: text or ndjson")
	cmd.Flags().Float64Var(&usagePercent, "manager-usage-percent", 0, "parent manager context usage percent for optional compaction")
	cmd.Flags().IntVar(&usageTokens, "manager-usage-tokens", 0, "parent manager context token count for optional compaction")
	cmd.Flags().IntVar(&usageWindow, "manager-usage-window", 0, "parent manager context window size for optional compaction")
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
	policy, err := readPolicyFile(opts.PolicyFile)
	if err != nil {
		return err
	}
	state, err := InitialManagerState(opts.PlanDir, projectRoot, policy)
	if err != nil {
		return err
	}
	if err := ApplyInitOverrides(&state, InitOverrides{NodeID: opts.NodeID, ImplementationCwd: opts.ImplementationCwd}); err != nil {
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

func RunStartNext(ctx context.Context, opts StartNextOptions, d deps, out io.Writer) (*StartNextResult, error) {
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
		preflight, err := CheckQRSPIPreflight(ctx, ManagerState{}, PreflightOptions{StateRootPath: root, UsesExtension: true}, d)
		if err != nil {
			return nil, err
		}
		if card := BuildPreflightFailedCard(preflight.Pi, ""); card != nil {
			return nil, writeManagerActionCard(out, *card, opts.Output)
		}
		if !preflight.StateRoot.OK || !preflight.Tmux.OK {
			card := ManagerActionCard{Kind: ActionPiCompatibilityFailed, Severity: "error", Summary: "q-manager preflight failed", Evidence: append(preflight.StateRoot.Evidence, preflight.Tmux.Evidence...), RecommendedAction: "fix q-manager runtime dependencies before launching child", SafeCommand: "vamos qrspi doctor", RequiresHuman: false}
			return nil, writeManagerActionCard(out, card, opts.Output)
		}
	}
	state, stateFile, err := resolveOrInitStartState(ctx, opts, d)
	if err != nil {
		return nil, err
	}
	result := StartNextResult{StateFile: stateFile}
	if strings.TrimSpace(opts.StateFile) != "" {
		preflight, err := CheckQRSPIPreflight(ctx, state, PreflightOptions{StateFile: stateFile, ManagerPaneID: state.ManagerPaneID, UsesExtension: true}, d)
		if err != nil {
			return nil, err
		}
		if card := BuildPreflightFailedCard(preflight.Pi, stateFile); card != nil {
			state.LastActionCard = card
			_ = stateStore(d, "", clock).Save(stateFile, state)
			return nil, writeManagerActionCard(out, *card, opts.Output)
		}
	}
	if state.ActiveChild != nil {
		result.ActiveChild = state.ActiveChild
		result.CurrentNode = state.ActiveChild.Stage
		result.StopReason = "active child already running"
		result.NextCommand = continueCommand(stateFile)
		result.FeedbackCommand = feedbackCommand(stateFile)
		return &result, writeStartNextOutput(out, result, opts.Output)
	}
	store := stateStore(d, "", clock)
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
			result.NextCommand = fmt.Sprintf("vamos qrspi start-next --state-file %s", stateFile)
			return &result, writeStartNextOutput(out, result, opts.Output)
		}
	}
	node, err := selectLaunchNode(state, opts)
	if err != nil {
		return nil, err
	}
	result.CurrentNode = string(node.ID)
	promptFile, err := WriteStagePromptFile(ctx, state, node, PromptFileOptions{StateFile: stateFile, NodeID: string(node.ID), Timestamp: clock()})
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
		PlanDir:    planDirForStart(state, opts),
		Stage:      string(node.ID),
		Cwd:        cwd,
		PromptFile: promptFile,
		StateFile:  stateFile,
		Split:      opts.Split,
		Timeout:    opts.Timeout,
	}, d, runOut); err != nil {
		return nil, err
	}
	launched, err := store.Load(stateFile)
	if err != nil {
		return nil, err
	}
	launched, _, err = maybeStartManagerCompaction(ctx, launched, stateFile, opts.Usage, d, out)
	if err != nil {
		return nil, err
	}
	result.ActiveChild = launched.ActiveChild
	result.NextCommand = continueCommand(stateFile)
	result.FeedbackCommand = feedbackCommand(stateFile)
	return &result, writeStartNextOutput(out, result, opts.Output)
}

func readLatestResultSeed(opts StartNextOptions) (string, error) {
	if opts.LatestResultStdin && strings.TrimSpace(opts.LatestResultFile) != "" {
		return "", errors.New("use only one of --latest-result-stdin or --latest-result-file")
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

func applyLatestResultSeed(state *ManagerState, resultText string) (*ParsedDecision, error) {
	if state == nil {
		return nil, errors.New("state is required")
	}
	parsed, err := ParseNormalizeValidateDecide(resultText, *state, wruntime.ParseContext{ExpectedNodeID: state.Workflow.CurrentNodeID})
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
		if _, err := fmt.Fprintf(out, "active child: %s (%s), session %s\n", result.ActiveChild.Stage, result.ActiveChild.TmuxPaneID, emptyAsPending(result.ActiveChild.SessionID)); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "next: wait for validated wake or inspect active child; do not launch duplicate child")
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
		if _, err := fmt.Fprintf(out, "started child: %s (%s)\n", result.ActiveChild.Stage, result.ActiveChild.TmuxPaneID); err != nil {
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
		if _, err := fmt.Fprintf(out, "feedback: %s\n", result.FeedbackCommand); err != nil {
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
	return fmt.Sprintf("vamos qrspi steer-child --state-file %s --feedback-file <file>", stateFile)
}

func debugCommandForState(stateFile string, action string) string {
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
		if _, err := fmt.Fprintf(out, "validated: %s %s\n", notice.Stage, notice.Status); err != nil {
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
	if notice.ChildPane != "" && notice.Kind == "active_child" {
		if _, err := fmt.Fprintf(out, "active child: %s (%s)\n", notice.Stage, notice.ChildPane); err != nil {
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
	if notice.ManagerGuidance != "" {
		if _, err := fmt.Fprintf(out, "guidance: %s\n", notice.ManagerGuidance); err != nil {
			return err
		}
	}
	if notice.NextCommand != "" {
		if _, err := fmt.Fprintf(out, "next: %s\n", notice.NextCommand); err != nil {
			return err
		}
	}
	if notice.FeedbackCommand != "" {
		if _, err := fmt.Fprintf(out, "feedback: %s\n", notice.FeedbackCommand); err != nil {
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
	childID := childRunID(opts.Stage, clock())
	runRoot := filepath.Dir(opts.StateFile)
	extensionPath, err := ResolveChildExtensionPath(runRoot)
	if err != nil {
		return err
	}
	managerRunID := strings.TrimSpace(opts.ManagerRunID)
	if managerRunID == "" {
		managerRunID = state.ManagerRunID
	}
	req := ChildRunRequest{
		ID:                   childID,
		Stage:                opts.Stage,
		Cwd:                  opts.Cwd,
		ManagerRunID:         managerRunID,
		PromptFile:           opts.PromptFile,
		OutputPath:           OutputPath(runRoot, childID),
		SessionID:            ChildSessionID(childID),
		SessionDir:           SessionDir(runRoot, childID),
		SessionName:          fmt.Sprintf("q-manager %s %s", opts.Stage, childID),
		DonePath:             DonePath(runRoot, childID),
		StatusPath:           StatusPath(runRoot, childID),
		ValidationStatusPath: ValidationStatusPath(runRoot, childID),
		Split:                normalizeSplit(opts.Split),
		ParentPaneID:         parentPaneID,
		StateFile:            opts.StateFile,
		PlanDir:              opts.PlanDir,
		ExtensionPath:        extensionPath,
	}
	if err := ensureRunFiles(req); err != nil {
		return err
	}
	runner := childRunner(d)
	run, err := runner.Start(ctx, req)
	if err != nil {
		return err
	}
	state.ActiveChild = &ChildRunRef{ID: childID, Stage: opts.Stage, Cwd: opts.Cwd, TmuxPaneID: run.Pane.ID, OutputPath: req.OutputPath, SessionID: req.SessionID, SessionDir: req.SessionDir, DonePath: req.DonePath, StatusPath: req.StatusPath, ValidationStatusPath: req.ValidationStatusPath, LifecycleStatus: "running", Generation: 1}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if err := WriteNDJSON(out, Event{Type: "child_started", Ref: childRef(state.ActiveChild)}); err != nil {
		return err
	}
	if state.PendingCleanupChild != nil {
		pending := state.PendingCleanupChild
		cleaned, cleanupErr := cleanupPendingChildAfterNextStart(ctx, state, d.Tmux)
		if cleanupErr != nil {
			if err := WriteNDJSON(out, Event{Type: "child_cleanup_failed", Ref: childRef(pending), Error: cleanupErr.Error()}); err != nil {
				return err
			}
		} else {
			state = cleaned
			if err := store.Save(opts.StateFile, state); err != nil {
				return err
			}
			if err := WriteNDJSON(out, Event{Type: "child_cleaned", Ref: childRef(pending)}); err != nil {
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
		return fmt.Errorf("timed out waiting for child done marker %s (pane %s, output %s, sessionDir %s, sessionID %s): %w", req.DonePath, run.Pane.ID, req.OutputPath, req.SessionDir, req.SessionID, err)
	}
	sessionPath, err := ResolveSessionPath(req.SessionDir, req.SessionID, req.Cwd)
	if err != nil {
		return fmt.Errorf("resolve child session path after done marker %s (pane %s, output %s, sessionDir %s, sessionID %s): %w", req.DonePath, run.Pane.ID, req.OutputPath, req.SessionDir, req.SessionID, err)
	}
	state.ActiveChild.SessionPath = sessionPath
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	return WriteNDJSON(out, Event{Type: "child_finished", Ref: childRef(state.ActiveChild)})
}

func RunChildComplete(ctx context.Context, opts ChildCompletionOptions, d deps, out io.Writer) (*ChildCompletionStatus, error) {
	if strings.TrimSpace(opts.StateFile) == "" {
		return nil, errors.New("state-file is required")
	}
	out = ensureWriter(out)
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return nil, err
	}
	if state.ActiveChild == nil {
		return nil, errors.New("no active child to complete")
	}
	if strings.TrimSpace(opts.ChildID) != "" && state.ActiveChild.ID != strings.TrimSpace(opts.ChildID) {
		return nil, fmt.Errorf("active child %q does not match requested child %q", state.ActiveChild.ID, opts.ChildID)
	}
	child := state.ActiveChild
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{})
	status := ChildCompletionStatus{ChildID: child.ID, Attempt: child.ValidationRetryCount, RetryLimit: invalidResultRetryLimit(state)}
	if err == nil {
		parseCtx.ExpectedNodeID = wruntime.NodeID(child.Stage)
		parsed, parseErr := ParseNormalizeValidateDecide(text, state, parseCtx)
		if parseErr == nil {
			status.Validated = true
			status.DeliveryID = childCompletionDeliveryID(*child, &parsed, false)
			status.Result = childCompletionResult(parsed.Result)
			status.ManagerNeeded = childCompletionManagerNeeded(status.Result.Status)
			status.Normalizations = parsed.Normalizations
			state.ActiveChild.LifecycleStatus = "completed"
			state.ActiveChild.LastDeliveryID = status.DeliveryID
			state, status.Wake, err = queueOrDeliverWake(ctx, opts.StateFile, state, status, d)
			if err != nil {
				return nil, err
			}
		} else {
			err = parseErr
		}
	}
	if err != nil {
		if shouldRepromptAfterValidationError(state, ContinueOptions{StateFile: opts.StateFile, PlanDir: state.CanonicalPlanDir, Stage: child.Stage}, err) {
			attempt := child.ValidationRetryCount + 1
			if repromptErr := continueReprompt(ctx, state, ContinueOptions{StateFile: opts.StateFile, PlanDir: state.CanonicalPlanDir, Stage: child.Stage}, d, io.Discard, err); repromptErr != nil {
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
			status.Wake = WakeDeliveryInstruction{Mode: "suppress", Reason: "retryable_invalid_result"}
		} else if isRetryExhaustedValidationError(state, ContinueOptions{StateFile: opts.StateFile, PlanDir: state.CanonicalPlanDir, Stage: child.Stage}, err) {
			status.Validated = false
			status.ManagerNeeded = true
			status.RetryExhausted = true
			status.DeliveryID = childCompletionDeliveryID(*child, nil, true)
			status.Result = ChildCompletionResult{Stage: child.Stage, Status: "invalid_result", Summary: err.Error()}
			status.Reason = err.Error()
			state.ActiveChild.LifecycleStatus = "awaiting_manager"
			state.ActiveChild.LastDeliveryID = status.DeliveryID
			state, status.Wake, err = queueOrDeliverWake(ctx, opts.StateFile, state, status, d)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if state.ActiveChild != nil && strings.TrimSpace(state.ActiveChild.ValidationStatusPath) != "" {
		if writeErr := writeValidationStatus(state.ActiveChild.ValidationStatusPath, status); writeErr != nil {
			return nil, writeErr
		}
	}
	if saveErr := store.Save(opts.StateFile, state); saveErr != nil {
		return nil, saveErr
	}
	if strings.EqualFold(opts.Output, "json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(status); err != nil {
			return nil, err
		}
	} else {
		if _, err := fmt.Fprintf(out, "child completion: %s\nwake: %s\n", status.Result.Status, status.Wake.Mode); err != nil {
			return nil, err
		}
	}
	return &status, nil
}

func childCompletionResult(result wruntime.WorkflowResult) ChildCompletionResult {
	return ChildCompletionResult{Stage: string(result.SourceNodeID), Status: string(result.Status), Outcome: string(result.Outcome), Artifact: result.PrimaryArtifact, Summary: result.Summary}
}

func childCompletionDeliveryID(child ChildRunRef, parsed *ParsedDecision, exhausted bool) string {
	parts := []string{child.ID, fmt.Sprintf("%d", child.Generation)}
	if exhausted || parsed == nil {
		parts = append(parts, "invalid_result")
	} else {
		parts = append(parts, string(parsed.Result.SourceNodeID), string(parsed.Result.Status), string(parsed.Result.Outcome), parsed.Result.PrimaryArtifact)
	}
	return strings.Join(parts, ":")
}

func childCompletionManagerNeeded(status string) bool {
	return status == string(wruntime.StatusNeedsHuman) || status == string(wruntime.StatusBlocked) || status == string(wruntime.StatusError) || status == "invalid_result"
}

func queueOrDeliverWake(ctx context.Context, stateFile string, state ManagerState, status ChildCompletionStatus, d deps) (ManagerState, WakeDeliveryInstruction, error) {
	if status.Wake.Mode == "suppress" {
		return state, status.Wake, nil
	}
	if strings.TrimSpace(status.DeliveryID) == "" {
		return state, WakeDeliveryInstruction{Mode: "suppress", Reason: "missing_delivery_id"}, nil
	}
	if state.Delivery.LastDeliveryID == status.DeliveryID {
		return state, WakeDeliveryInstruction{Mode: "suppress", Reason: "duplicate_delivery"}, nil
	}
	payload := childCompletionWakePayload(stateFile, status)
	if strings.EqualFold(state.Delivery.Status, "compacting") {
		state.Delivery.QueuedWake = &QueuedWake{DeliveryID: status.DeliveryID, ChildID: status.ChildID, ChildGeneration: activeChildGeneration(state), Payload: payload, QueuedAt: time.Now().Format(time.RFC3339)}
		return state, WakeDeliveryInstruction{Mode: "queue", Payload: payload, Reason: "manager_compacting"}, nil
	}
	paneID := managerDeliveryPane(state)
	if paneID == "" {
		state.Delivery.QueuedWake = &QueuedWake{DeliveryID: status.DeliveryID, ChildID: status.ChildID, ChildGeneration: activeChildGeneration(state), Payload: payload, QueuedAt: time.Now().Format(time.RFC3339)}
		return state, WakeDeliveryInstruction{Mode: "queue", Payload: payload, Reason: "manager_pane_missing"}, nil
	}
	if err := pasteWake(ctx, d, paneID, payload); err != nil {
		return state, WakeDeliveryInstruction{}, err
	}
	state.Delivery.LastDeliveryID = status.DeliveryID
	return state, WakeDeliveryInstruction{Mode: "deliver", Payload: payload}, nil
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

func pasteWake(ctx context.Context, d deps, paneID string, payload string) error {
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

func childCompletionWake(stateFile string, state ManagerState, status ChildCompletionStatus) WakeDeliveryInstruction {
	if status.DeliveryID != "" && state.Delivery.LastDeliveryID == status.DeliveryID {
		return WakeDeliveryInstruction{Mode: "suppress", Reason: "duplicate_delivery"}
	}
	return WakeDeliveryInstruction{Mode: "deliver", Payload: childCompletionWakePayload(stateFile, status)}
}

func childCompletionWakePayload(stateFile string, status ChildCompletionStatus) string {
	managerNeeded := status.ManagerNeeded || status.RetryExhausted || childCompletionManagerNeeded(status.Result.Status)
	return fmt.Sprintf("```yaml\nq_manager_child_wake:\n  validated: %t\n  manager_needed: %t\n  retry_exhausted: %t\n  stage: %q\n  status: %q\n  outcome: %q\n  artifact: %q\n  child_id: %q\n  state_file: %q\n  reason: %q\n  next:\n    - action: run_command\n      param: %q\n```", status.Validated, managerNeeded, status.RetryExhausted, status.Result.Stage, status.Result.Status, status.Result.Outcome, status.Result.Artifact, status.ChildID, stateFile, status.Reason, continueCommand(stateFile))
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

func usageFromChangedFlags(cmd *cobra.Command, usagePercent float64, usageTokens int, usageWindow int) ManagerUsageInput {
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

func maybeStartManagerCompaction(ctx context.Context, state ManagerState, stateFile string, usage ManagerUsageInput, d deps, out io.Writer) (ManagerState, bool, error) {
	_ = ctx
	out = ensureWriter(out)
	percent, ok := managerUsagePercent(usage)
	if !ok {
		return state, false, writeCompactionDiagnostic(out, "manager compaction: skipped; no explicit usage input", stateFile, "", false)
	}
	if percent <= 80 {
		return state, false, writeCompactionDiagnostic(out, fmt.Sprintf("manager compaction: skipped; usage %.1f%% <= 80%%", percent), stateFile, "", false)
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock()
	}
	handoffPath, err := writeManagerOperationalHandoff(state, stateFile, now)
	if err != nil {
		return state, false, err
	}
	state.Delivery.Status = "compacting"
	if strings.TrimSpace(state.Delivery.ManagerPaneID) == "" {
		state.Delivery.ManagerPaneID = strings.TrimSpace(state.ManagerPaneID)
	}
	if err := stateStore(d, "", func() time.Time { return now }).Save(stateFile, state); err != nil {
		return state, false, err
	}
	return state, true, writeCompactionDiagnostic(out, fmt.Sprintf("manager compaction: usage %.1f%% > 80%%; handoff written", percent), stateFile, handoffPath, true)
}

func writeCompactionDiagnostic(out io.Writer, summary string, stateFile string, handoffPath string, compacting bool) error {
	if out == nil {
		return nil
	}
	if compacting {
		fmt.Fprintf(out, "%s\n", summary)
		fmt.Fprintf(out, "handoff: %s\n", handoffPath)
		fmt.Fprintf(out, "resume: pi @%s\n", handoffPath)
		fmt.Fprintf(out, "ready: vamos qrspi manager-ready --state-file %s --manager-pane $TMUX_PANE\n", stateFile)
		return nil
	}
	_, err := fmt.Fprintln(out, summary)
	return err
}

func writeManagerOperationalHandoff(state ManagerState, stateFile string, now time.Time) (string, error) {
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

func buildManagerOperationalHandoff(state ManagerState, stateFile string, now time.Time, path string) string {
	child := "none"
	if state.ActiveChild != nil {
		child = fmt.Sprintf("stage=%s childID=%s pane=%s sessionID=%s sessionPath=%s statusPath=%s donePath=%s", state.ActiveChild.Stage, state.ActiveChild.ID, state.ActiveChild.TmuxPaneID, state.ActiveChild.SessionID, state.ActiveChild.SessionPath, state.ActiveChild.StatusPath, state.ActiveChild.DonePath)
	}
	return fmt.Sprintf(`---
date: %s
stage: q-manager
artifact: manager-operational-handoff
---

# q-manager operational handoff

Done: parent manager usage exceeded compaction threshold after launching child; delivery marked compacting so child wake queues safely.

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

func RunManagerReady(ctx context.Context, opts ManagerReadyOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	pane := strings.TrimSpace(opts.ManagerPane)
	if pane == "" {
		pane = CaptureManagerPaneID("")
	}
	state.Delivery.Status = "ready"
	if pane != "" {
		state.Delivery.ManagerPaneID = pane
		state.ManagerPaneID = pane
	}
	var flushed bool
	state, flushed, err = flushQueuedWake(ctx, state, pane, d)
	if err != nil {
		return err
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(out, Event{Type: "manager_ready", Ref: map[string]any{"flushed": flushed, "stateFile": opts.StateFile}})
	}
	if flushed {
		_, err = fmt.Fprintln(out, "manager ready: flushed queued wake")
	} else {
		_, err = fmt.Fprintln(out, "manager ready: no queued wake")
	}
	return err
}

func flushQueuedWake(ctx context.Context, state ManagerState, pane string, d deps) (ManagerState, bool, error) {
	queued := state.Delivery.QueuedWake
	if queued == nil {
		return state, false, nil
	}
	if queued.DeliveryID == "" || queued.DeliveryID == state.Delivery.LastDeliveryID {
		state.Delivery.QueuedWake = nil
		return state, false, nil
	}
	if state.ActiveChild != nil {
		if queued.ChildGeneration != activeChildGeneration(state) || state.ActiveChild.LifecycleStatus == "running" || state.ActiveChild.LifecycleStatus == "manual_reprompt" {
			state.LastActionCard = &ManagerActionCard{Kind: ActionSupersededQueuedWake, Severity: "info", Summary: "queued child wake superseded by active child generation", RecommendedAction: "wait for newer child completion", RequiresHuman: false}
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
	if err := pasteWake(ctx, d, paneID, queued.Payload); err != nil {
		return state, false, err
	}
	now := time.Now().Format(time.RFC3339)
	queued.DeliveredAt = now
	state.Delivery.LastDeliveryID = queued.DeliveryID
	state.Delivery.QueuedWake = nil
	return state, true, nil
}

func supersedeQueuedWakeForActiveChild(state ManagerState, childID string, reason string) ManagerState {
	if state.Delivery.QueuedWake == nil || state.Delivery.QueuedWake.ChildID != childID {
		return state
	}
	state.Delivery.QueuedWake = nil
	state.LastActionCard = &ManagerActionCard{Kind: ActionSupersededQueuedWake, Severity: "info", Summary: reason, RecommendedAction: "wait for newer child completion", RequiresHuman: false}
	return state
}

func RunRepairState(ctx context.Context, opts RepairStateOptions, d deps, out io.Writer) error {
	_ = ctx
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if !opts.AlignActiveChild {
		return errors.New("--align-active-child is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if state.ActiveChild == nil || strings.TrimSpace(state.ActiveChild.Stage) == "" {
		return errors.New("no active child evidence to align")
	}
	card := buildStateDesyncActionCard(state, opts.StateFile, fmt.Errorf("workflow cursor %s differs from active child %s", state.Workflow.CurrentNodeID, state.ActiveChild.Stage))
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
	fmt.Fprintf(out, "repaired: aligned current node to active child %s\n", state.ActiveChild.Stage)
	return writeManagerActionCard(out, card, opts.Output)
}

func RunMarkChildActive(ctx context.Context, opts MarkChildActiveOptions, d deps, out io.Writer) error {
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
	if state.ActiveChild == nil || state.ActiveChild.ID != strings.TrimSpace(opts.ChildID) {
		return fmt.Errorf("active child does not match requested child %q", opts.ChildID)
	}
	state.ActiveChild.LifecycleStatus = "manual_reprompt"
	state.ActiveChild.Generation = activeChildGeneration(state) + 1
	state = supersedeQueuedWakeForActiveChild(state, state.ActiveChild.ID, markChildActiveReason(opts.Reason))
	if state.LastActionCard == nil {
		card := manualChildSteerActionCard(state, opts.StateFile)
		state.LastActionCard = &card
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(out, Event{Type: "child_marked_active", ActionCard: state.LastActionCard, Ref: childRef(state.ActiveChild)})
	}
	fmt.Fprintf(out, "child active: %s generation %d\n", state.ActiveChild.ID, state.ActiveChild.Generation)
	return writeManagerActionCard(out, *state.LastActionCard, opts.Output)
}

func markChildActiveReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual reprompt marked child active"
	}
	return reason
}

func RunValidateResult(ctx context.Context, opts ValidateResultOptions, d deps, out io.Writer) error {
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
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{ResultFile: opts.ResultFile, SessionFile: opts.SessionFile, SessionID: opts.SessionID, RunID: opts.RunID})
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

func RunRepromptChild(ctx context.Context, opts RepromptChildOptions, d deps, out io.Writer) error {
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
		return fmt.Errorf("active child stage %q does not match requested stage %q", state.ActiveChild.Stage, opts.Stage)
	}
	if strings.TrimSpace(state.ActiveChild.TmuxPaneID) == "" {
		return errors.New("active child has no tmux pane ID")
	}
	errText, err := repromptErrorText(opts)
	if err != nil {
		return err
	}
	prompt := CorrectionPrompt(errors.New(errText), opts.Attempt)
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
	return WriteNDJSON(out, Event{Type: "child_reprompted", Ref: childRef(state.ActiveChild)})
}

func RunSteerChild(ctx context.Context, opts SteerChildOptions, d deps, out io.Writer) (*SteerChildResult, error) {
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
		return nil, fmt.Errorf("no active child to steer; next: %s or %s", fmt.Sprintf("vamos qrspi start-next --state-file %s", opts.StateFile), continueCommand(opts.StateFile))
	}
	child := *state.ActiveChild
	if strings.TrimSpace(opts.Stage) != "" && child.Stage != opts.Stage {
		return nil, fmt.Errorf("active child stage %q does not match requested stage %q", child.Stage, opts.Stage)
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
	result := &SteerChildResult{StateFile: opts.StateFile, Stage: child.Stage, PaneID: child.TmuxPaneID, FeedbackPath: feedbackPath, NextCommand: continueCommand(opts.StateFile)}
	return result, writeSteerChildOutput(out, *result, opts.Output)
}

func readFeedback(opts SteerChildOptions) (string, string, error) {
	if strings.TrimSpace(opts.FeedbackFile) != "" && strings.TrimSpace(opts.Feedback) != "" {
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

func buildChildSteerPrompt(state ManagerState, child ChildRunRef, feedback string, feedbackPath string) string {
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
	b.WriteString("Instruction: incorporate this feedback into your current QRSPI stage. Update artifacts if needed. Then emit the required fenced YAML result when complete or ask one concise follow-up if still blocked.\n")
	return b.String()
}

func writeSteerChildOutput(out io.Writer, result SteerChildResult, mode string) error {
	if strings.EqualFold(mode, "ndjson") {
		return WriteNDJSON(out, Event{Type: "child_steered", Ref: steerChildRef(result)})
	}
	if _, err := fmt.Fprintf(out, "steered child: %s (%s)\n", result.Stage, result.PaneID); err != nil {
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
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		opts.PlanDir = state.CanonicalPlanDir
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	if state.ActiveChild == nil {
		card := buildContinueActionCard(state, opts, errors.New("no active child to continue"))
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

	result := ContinueResult{}
	parsed, err := validateActiveChild(ctx, state, opts)
	if err != nil {
		if shouldRepromptAfterValidationError(state, opts, err) {
			if repromptErr := continueReprompt(ctx, state, opts, d, out, err); repromptErr != nil {
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

	nextState, err := decideValidatedResult(ctx, state, parsed, opts, store)
	if err != nil {
		return err
	}
	result.Decided = true
	result.NextNodeID = parsed.Decision.NextNodeID
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
		launched, err := startNextChildFromDecision(ctx, nextState, parsed.Decision, opts, d, out)
		if err != nil {
			return err
		}
		launched, _, err = maybeStartManagerCompaction(ctx, launched, opts.StateFile, opts.Usage, d, out)
		if err != nil {
			return err
		}
		result.StartedChild = launched.ActiveChild
	}
	return writeContinueOutput(out, opts, result)
}

func validateActiveChild(ctx context.Context, state ManagerState, opts ContinueOptions) (ParsedDecision, error) {
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
	if json.Unmarshal(state.Workflow.Policy, &cfg) == nil && cfg.InvalidResultRetryLimit != nil {
		return *cfg.InvalidResultRetryLimit
	}
	return limit
}

func shouldRepromptAfterValidationError(state ManagerState, opts ContinueOptions, err error) bool {
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

func isRetryExhaustedValidationError(state ManagerState, opts ContinueOptions, err error) bool {
	if err == nil || state.ActiveChild == nil {
		return false
	}
	if state.ActiveChild.Stage != opts.Stage {
		return false
	}
	return state.ActiveChild.ValidationRetryCount >= invalidResultRetryLimit(state)
}

func writeRetryExhaustedNotice(out io.Writer, opts ContinueOptions, state ManagerState, validationErr error) error {
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
		Kind:            "retry_exhausted",
		Validated:       false,
		ManagerNeeded:   true,
		RetryExhausted:  true,
		StateFile:       opts.StateFile,
		Stage:           stage,
		Status:          "invalid_result",
		ChildPane:       pane,
		Summary:         fmt.Sprintf("invalid result after retry limit (%d): %s", attempt, validationErr.Error()),
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

func continueReprompt(ctx context.Context, state ManagerState, opts ContinueOptions, d deps, out io.Writer, validationErr error) error {
	_ = out
	attempt := state.ActiveChild.ValidationRetryCount + 1
	if state.ActiveChild.LastRepromptAttempt >= attempt {
		return fmt.Errorf("reprompt attempt %d already sent for active child %s", attempt, state.ActiveChild.ID)
	}
	if strings.TrimSpace(state.ActiveChild.DonePath) != "" {
		if err := os.Remove(state.ActiveChild.DonePath); err != nil && !os.IsNotExist(err) {
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

func decideValidatedResult(ctx context.Context, state ManagerState, parsed ParsedDecision, opts ContinueOptions, store StateStore) (ManagerState, error) {
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

func startNextChildFromDecision(ctx context.Context, state ManagerState, decision wruntime.TransitionDecision, opts ContinueOptions, d deps, out io.Writer) (ManagerState, error) {
	nodeID := decision.NextNodeID
	if nodeID == "" {
		return state, errors.New("transition has no next node")
	}
	promptFile, err := renderContinuePromptFile(ctx, state, nodeID, opts)
	if err != nil {
		return state, err
	}
	cwd, err := defaultChildCwd(state, nodeID, opts.Cwd)
	if err != nil {
		return state, err
	}
	runOut := io.Writer(io.Discard)
	if strings.EqualFold(opts.Output, "ndjson") {
		runOut = out
	}
	if err := RunChild(ctx, RunChildOptions{
		PlanDir:    opts.PlanDir,
		Stage:      string(nodeID),
		Cwd:        cwd,
		PromptFile: promptFile,
		StateFile:  opts.StateFile,
		Split:      opts.Split,
		Timeout:    opts.Timeout,
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

func renderContinuePromptFile(ctx context.Context, state ManagerState, nodeID wruntime.NodeID, opts ContinueOptions) (string, error) {
	def, err := Definition()
	if err != nil {
		return "", err
	}
	node, ok := def.Nodes[nodeID]
	if !ok {
		return "", fmt.Errorf("node %q is not in QRSPI definition", nodeID)
	}
	return WriteStagePromptFile(ctx, state, node, PromptFileOptions{StateFile: opts.StateFile, NodeID: string(nodeID), Timestamp: time.Now()})
}

func humanPromptContext(state ManagerState, stateFile string, parsed ParsedDecision) HumanPromptContext {
	result := parsed.Result
	return HumanPromptContext{
		Stage:                    string(result.SourceNodeID),
		Status:                   string(result.Status),
		Summary:                  strings.TrimSpace(result.Summary),
		Artifact:                 result.PrimaryArtifact,
		SuggestedFeedbackCommand: feedbackCommand(stateFile),
	}
}

func buildContinueActionCard(state ManagerState, opts ContinueOptions, err error) *ManagerActionCard {
	if state.ActiveChild == nil {
		return &ManagerActionCard{Kind: ActionActiveChildConflict, Severity: "warning", Summary: "no active child to continue", Evidence: []string{fmt.Sprintf("current node: %s", state.Workflow.CurrentNodeID)}, RecommendedAction: "start or inspect the graph-selected child", SafeCommand: fmt.Sprintf("vamos qrspi start-next --state-file %s", opts.StateFile), ContinueCommand: continueCommand(opts.StateFile), RequiresHuman: false}
	}
	if err == nil {
		return nil
	}
	if state.ActiveChild.Stage != string(state.Workflow.CurrentNodeID) {
		card := buildStateDesyncActionCard(state, opts.StateFile, err)
		return &card
	}
	if looksWorkspaceMoved(state, err) {
		return &ManagerActionCard{Kind: ActionWorkspaceMoved, Severity: "warning", Summary: "implementation workspace differs from current child cwd", Evidence: workspaceMovedEvidence(state), RecommendedAction: "run q-manager continue from the recorded implementation workspace", SafeCommand: fmt.Sprintf("cd %q && vamos qrspi continue --state-file %s", state.ImplementationCwd, opts.StateFile), ContinueCommand: continueCommand(opts.StateFile), RequiresHuman: false}
	}
	kind := ActionInvalidChildYAML
	if strings.Contains(strings.ToLower(err.Error()), "canonical qrspi graph rejected") || strings.Contains(strings.ToLower(err.Error()), "outcome") {
		kind = ActionGraphOutcomeMismatch
	}
	return &ManagerActionCard{Kind: kind, Severity: "warning", Summary: "child result needs deterministic repair", Evidence: []string{err.Error(), fmt.Sprintf("active child stage: %s", state.ActiveChild.Stage), fmt.Sprintf("retry: %d/%d", state.ActiveChild.ValidationRetryCount, invalidResultRetryLimit(state))}, RecommendedAction: "reprompt or steer the active child with canonical YAML", SafeCommand: fmt.Sprintf("vamos qrspi reprompt-child --state-file %s --plan-dir %s --stage %s --attempt %d", opts.StateFile, opts.PlanDir, state.ActiveChild.Stage, state.ActiveChild.ValidationRetryCount+1), ContinueCommand: continueCommand(opts.StateFile), RequiresHuman: false}
}

func buildStateDesyncActionCard(state ManagerState, stateFile string, err error) ManagerActionCard {
	evidence := []string{fmt.Sprintf("current node: %s", state.Workflow.CurrentNodeID)}
	if state.ActiveChild != nil {
		evidence = append(evidence, fmt.Sprintf("active child stage: %s", state.ActiveChild.Stage), fmt.Sprintf("active child id: %s", state.ActiveChild.ID))
		if state.ActiveChild.SessionPath != "" {
			evidence = append(evidence, fmt.Sprintf("session: %s", state.ActiveChild.SessionPath))
		}
	}
	if err != nil {
		evidence = append(evidence, err.Error())
	}
	return ManagerActionCard{Kind: ActionStateDesync, Severity: "warning", Summary: "workflow cursor and active child are out of sync", Evidence: evidence, RecommendedAction: "align active child, then continue", SafeCommand: fmt.Sprintf("vamos qrspi repair-state --state-file %s --align-active-child && vamos qrspi continue --state-file %s", stateFile, stateFile), ContinueCommand: continueCommand(stateFile), RequiresHuman: false}
}

func humanGateActionCard(state ManagerState, stateFile string, prompt HumanPromptContext) ManagerActionCard {
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
		evidence = append(evidence, fmt.Sprintf("active child: %s", state.ActiveChild.ID), fmt.Sprintf("generation: %d", activeChildGeneration(state)))
	}
	return ManagerActionCard{Kind: ActionManualChildSteer, Severity: "info", Summary: "child marked active after manual steering", Evidence: evidence, RecommendedAction: "wait for newer completion before flushing queued wakes", SafeCommand: continueCommand(stateFile), ContinueCommand: continueCommand(stateFile), RequiresHuman: false}
}

func looksWorkspaceMoved(state ManagerState, err error) bool {
	if strings.TrimSpace(state.ImplementationCwd) == "" || state.ActiveChild == nil {
		return false
	}
	if state.ActiveChild.Cwd != "" && filepath.Clean(state.ActiveChild.Cwd) != filepath.Clean(state.ImplementationCwd) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "workspace") || strings.Contains(strings.ToLower(err.Error()), "cwd")
}

func workspaceMovedEvidence(state ManagerState) []string {
	evidence := []string{fmt.Sprintf("implementation workspace: %s", state.ImplementationCwd)}
	if state.ActiveChild != nil {
		evidence = append(evidence, fmt.Sprintf("active child cwd: %s", state.ActiveChild.Cwd))
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

func appendRecoveryIncident(stateFile string, card ManagerActionCard, recovered bool) error {
	if strings.TrimSpace(stateFile) == "" {
		return nil
	}
	entry := ValidationRecoveryLog{Timestamp: time.Now(), StateFile: stateFile, Recovered: recovered, RecoveryAction: card.Kind, Reason: card.Summary}
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

func writeContinueOutput(out io.Writer, opts ContinueOptions, result ContinueResult) error {
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(out, Event{Type: "continued", Decision: result.Validated, ActionCard: result.ActionCard, Ref: continueRef(result)})
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
		fmt.Fprintf(out, "validated: %s %s\n", result.Validated.Result.SourceNodeID, result.Validated.Result.Status)
		if result.Validated.Result.Outcome != "" {
			fmt.Fprintf(out, "outcome: %s\n", result.Validated.Result.Outcome)
		}
	}
	if result.PrimaryArtifact != "" {
		fmt.Fprintf(out, "artifact: %s\n", result.PrimaryArtifact)
	}
	if result.Reprompted {
		_, err := fmt.Fprintln(out, "retry: reprompted active child")
		return err
	}
	if result.NextNodeID != "" {
		fmt.Fprintf(out, "next: %s\n", result.NextNodeID)
	}
	if result.StartedChild != nil {
		fmt.Fprintf(out, "started child: %s (%s)\n", result.StartedChild.Stage, result.StartedChild.TmuxPaneID)
	}
	if result.WaitingHuman {
		if _, err := fmt.Fprintln(out, "stop: waiting human"); err != nil {
			return err
		}
		if result.HumanPrompt.Summary != "" {
			fmt.Fprintf(out, "question: %s\n", result.HumanPrompt.Summary)
		}
		if result.HumanPrompt.SuggestedFeedbackCommand != "" {
			fmt.Fprintf(out, "feedback: %s\n", result.HumanPrompt.SuggestedFeedbackCommand)
		}
		return nil
	}
	if result.StopReason != "" && result.StartedChild == nil {
		fmt.Fprintf(out, "stop: %s\n", result.StopReason)
		if result.Validated != nil && managerGuidanceForStatus(string(result.Validated.Result.Status)) != "" {
			fmt.Fprintf(out, "guidance: %s\n", managerGuidanceForStatus(string(result.Validated.Result.Status)))
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
	ref := map[string]any{"reprompted": result.Reprompted, "waitingHuman": result.WaitingHuman}
	if result.Validated != nil {
		ref["validated"] = true
		ref["stage"] = result.Validated.Result.SourceNodeID
		ref["status"] = result.Validated.Result.Status
		if guidance := managerGuidanceForStatus(string(result.Validated.Result.Status)); guidance != "" {
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

func RunDecideNext(ctx context.Context, opts DecideNextOptions, d deps, out io.Writer) error {
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
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{ResultFile: opts.ResultFile, SessionFile: opts.SessionFile})
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

func ReadChildResultText(state ManagerState, opts ResultSourceOptions) (string, wruntime.ParseContext, error) {
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
	return "", ctx, errors.New("no child result source: keep active child session refs, use latest-session recovery commands, pass --session-file for a specific JSONL, or use deprecated --result-file only as a debug fallback")
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
	clean := strings.NewReplacer("/", "-", " ", "-", "_", "-").Replace(strings.TrimSpace(stage))
	if clean == "" {
		clean = "child"
	}
	return fmt.Sprintf("%s-%s-%09d", clean, t.Format("20060102150405"), t.Nanosecond())
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

func cleanupPendingChildAfterNextStart(ctx context.Context, state ManagerState, tmux TmuxClient) (ManagerState, error) {
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
	if err := tmux.KillPane(ctx, TmuxPane{ID: ref.TmuxPaneID}); err != nil {
		return state, err
	}
	if state.ActiveChild != nil && strings.TrimSpace(state.ActiveChild.TmuxPaneID) != "" {
		if err := tmux.SelectLayout(ctx, TmuxPane{ID: state.ActiveChild.TmuxPaneID}, "even-horizontal"); err != nil {
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
