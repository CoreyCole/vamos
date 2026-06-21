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
		newRunChildCommand(d),
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
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback: plaintext child result file")
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
	cmd.Flags().StringVar(&opts.ResultFile, "result-file", "", "deprecated debug fallback: plaintext child result file")
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
	cmd := &cobra.Command{
		Use:   "continue --state-file <file>",
		Short: "Validate active child and continue the QRSPI graph when safe",
		RunE: func(cmd *cobra.Command, args []string) error {
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
	req := ChildRunRequest{
		ID:            childID,
		Stage:         opts.Stage,
		Cwd:           opts.Cwd,
		PromptFile:    opts.PromptFile,
		OutputPath:    OutputPath(runRoot, childID),
		SessionID:     ChildSessionID(childID),
		SessionDir:    SessionDir(runRoot, childID),
		SessionName:   fmt.Sprintf("q-manager %s %s", opts.Stage, childID),
		DonePath:      DonePath(runRoot, childID),
		StatusPath:    StatusPath(runRoot, childID),
		Split:         normalizeSplit(opts.Split),
		ParentPaneID:  parentPaneID,
		StateFile:     opts.StateFile,
		PlanDir:       opts.PlanDir,
		ExtensionPath: extensionPath,
	}
	if err := ensureRunFiles(req); err != nil {
		return err
	}
	runner := childRunner(d)
	run, err := runner.Start(ctx, req)
	if err != nil {
		return err
	}
	state.ActiveChild = &ChildRunRef{ID: childID, Stage: opts.Stage, Cwd: opts.Cwd, TmuxPaneID: run.Pane.ID, OutputPath: req.OutputPath, SessionID: req.SessionID, SessionDir: req.SessionDir, DonePath: req.DonePath, StatusPath: req.StatusPath}
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
	parsed, err := ParseValidateDecide(text, state.Workflow, parseCtx)
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

	if parsed.Decision.StartNext {
		launched, err := startNextChildFromDecision(ctx, nextState, parsed.Decision, opts, d, out)
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
	return ParseValidateDecide(text, state.Workflow, parseCtx)
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
	cwd := strings.TrimSpace(opts.Cwd)
	if cwd == "" {
		cwd = defaultContinueCwd(state, nodeID)
	}
	if cwd == "" {
		return state, errors.New("next child cwd is required")
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
	switch node {
	case "implement", "review-implementation", "verify":
		if strings.TrimSpace(state.ImplementationCwd) != "" {
			return state.ImplementationCwd
		}
	}
	return state.SourceCwd
}

func renderContinuePromptFile(ctx context.Context, state ManagerState, nodeID wruntime.NodeID, opts ContinueOptions) (string, error) {
	_ = ctx
	def, err := Definition()
	if err != nil {
		return "", err
	}
	node, ok := def.Nodes[nodeID]
	if !ok {
		return "", fmt.Errorf("node %q is not in QRSPI definition", nodeID)
	}
	manifest, err := LoadManifest(state.SourceCwd)
	if err != nil {
		return "", err
	}
	prompt, err := RenderStagePrompt(PromptContext{Node: node, State: state, PlanDir: opts.PlanDir, Manifest: manifest, LastResult: state.Workflow.LastResult})
	if err != nil {
		return "", err
	}
	path := ""
	if state.ActiveChild != nil && strings.TrimSpace(state.ActiveChild.OutputPath) != "" {
		path = filepath.Join(filepath.Dir(state.ActiveChild.OutputPath), "next-"+string(nodeID)+"-prompt.md")
	} else {
		path = filepath.Join(filepath.Dir(opts.StateFile), "prompts", string(nodeID)+".md")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func writeContinueOutput(out io.Writer, opts ContinueOptions, result ContinueResult) error {
	if strings.EqualFold(opts.Output, "ndjson") {
		return WriteNDJSON(out, Event{Type: "continued", Decision: result.Validated, Ref: continueRef(result)})
	}
	return writeContinueText(out, result)
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
	if result.NextNodeID != "" {
		fmt.Fprintf(out, "next: %s\n", result.NextNodeID)
	}
	if result.StartedChild != nil {
		fmt.Fprintf(out, "started child: %s (%s)\n", result.StartedChild.Stage, result.StartedChild.TmuxPaneID)
	}
	if result.WaitingHuman {
		_, err := fmt.Fprintln(out, "stop: waiting human")
		return err
	}
	if result.StopReason != "" && result.StartedChild == nil {
		fmt.Fprintf(out, "stop: %s\n", result.StopReason)
	}
	return nil
}

func continueRef(result ContinueResult) map[string]any {
	ref := map[string]any{"reprompted": result.Reprompted, "waitingHuman": result.WaitingHuman}
	if result.NextNodeID != "" {
		ref["nextNode"] = result.NextNodeID
	}
	if result.PrimaryArtifact != "" {
		ref["artifact"] = result.PrimaryArtifact
	}
	if result.StopReason != "" {
		ref["stopReason"] = result.StopReason
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
	parsed, err := ParseValidateDecide(text, state.Workflow, parseCtx)
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
	return "", ctx, errors.New("no child result source: provide --session-file, keep active child session refs, or pass deprecated --result-file")
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
	tmux := d.Tmux
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	return TmuxChildRunner{Tmux: tmux}
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
	return out
}
