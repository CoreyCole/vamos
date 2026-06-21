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
	childID := childRunID(opts.Stage, clock())
	runRoot := filepath.Dir(opts.StateFile)
	req := ChildRunRequest{
		ID:          childID,
		Stage:       opts.Stage,
		Cwd:         opts.Cwd,
		PromptFile:  opts.PromptFile,
		OutputPath:  OutputPath(runRoot, childID),
		SessionID:   ChildSessionID(childID),
		SessionDir:  SessionDir(runRoot, childID),
		SessionName: fmt.Sprintf("q-manager %s %s", opts.Stage, childID),
		DonePath:    DonePath(runRoot, childID),
		StatusPath:  StatusPath(runRoot, childID),
		Split:       normalizeSplit(opts.Split),
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
	state.ActiveChild = nil
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
