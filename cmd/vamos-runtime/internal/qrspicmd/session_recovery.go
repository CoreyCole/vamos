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
)

type LatestChildQuery struct {
	ManagerRunID string
	PlanDir      string
	Stage        wruntime.NodeID
	WorkspaceCwd string
	SessionDir   string
	ActiveChild  *ChildRunRef
}

type LatestChildCandidate struct {
	Child          ChildRunRef `json:"child"`
	SessionPath    string      `json:"sessionPath"`
	SessionID      string      `json:"sessionId"`
	Classification string      `json:"classification"`
	Reason         string      `json:"reason"`
	ModTime        time.Time   `json:"modTime"`
}

func RunInspect(ctx context.Context, opts InspectOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	state, err := stateStore(d, "", time.Now).Load(opts.StateFile)
	if err != nil {
		return err
	}
	var latest *LatestChildCandidate
	if opts.Latest {
		candidate, ok, err := FindLatestRelevantChildSession(latestQueryFromState(state, ""))
		if err != nil {
			return err
		}
		if ok {
			latest = &candidate
		}
	}
	if strings.EqualFold(opts.Output, "json") {
		return json.NewEncoder(out).Encode(map[string]any{"stateFile": opts.StateFile, "currentNode": state.Workflow.CurrentNodeID, "activeChild": state.ActiveChild, "latest": latest, "lastActionCard": state.LastActionCard})
	}
	fmt.Fprintf(out, "state: %s\n", opts.StateFile)
	fmt.Fprintf(out, "current node: %s\n", state.Workflow.CurrentNodeID)
	fmt.Fprintf(out, "plan: %s\n", state.CanonicalPlanDir)
	fmt.Fprintf(out, "implementation cwd: %s\n", state.ImplementationCwd)
	failedChildSafeCommand := ""
	if state.ActiveChild != nil {
		fmt.Fprintf(out, "active child: %s stage=%s pane=%s session=%s\n", state.ActiveChild.ID, state.ActiveChild.Stage, state.ActiveChild.TmuxPaneID, firstNonEmpty(state.ActiveChild.SessionPath, state.ActiveChild.SessionID))
		if health, err := InspectActiveChildHealth(ctx, state, opts.StateFile, d); err == nil {
			fmt.Fprintf(out, "active child health: %s\n", health.Status)
			if IsTerminalFailedChild(health) {
				failedChildSafeCommand = health.SafeCommand
			}
		}
		if opts.Sessions {
			fmt.Fprintf(out, "session dir: %s\n", state.ActiveChild.SessionDir)
			fmt.Fprintf(out, "validation: %s\n", state.ActiveChild.ValidationStatusPath)
		}
	} else {
		fmt.Fprintln(out, "active child: none")
	}
	if latest != nil {
		fmt.Fprintf(out, "latest: %s (%s)\n", latest.SessionPath, latest.Classification)
	}
	if failedChildSafeCommand != "" {
		fmt.Fprintf(out, "safe command: %s\n", failedChildSafeCommand)
	} else {
		fmt.Fprintf(out, "safe command: %s\n", continueCommand(opts.StateFile))
	}
	return nil
}

func RunFindLatestChild(ctx context.Context, opts FindLatestChildOptions, d deps, out io.Writer) error {
	_ = ctx
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	state, err := stateStore(d, "", time.Now).Load(opts.StateFile)
	if err != nil {
		return err
	}
	candidate, ok, err := FindLatestRelevantChildSession(latestQueryFromState(state, opts.Stage))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no relevant child sessions found")
	}
	return writeLatestCandidate(out, candidate, opts.Output)
}

func RunRebindChild(ctx context.Context, opts RebindChildOptions, d deps, out io.Writer) error {
	_ = ctx
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if strings.TrimSpace(opts.SessionFile) == "" {
		return errors.New("session-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	child, err := childRefFromSessionFile(state, opts.SessionFile, opts.Stage)
	if err != nil {
		return err
	}
	state, err = RebindActiveChild(state, child, opts.Reason)
	if err != nil {
		return err
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "json") {
		return json.NewEncoder(out).Encode(state.ActiveChild)
	}
	fmt.Fprintf(out, "rebound child: %s stage=%s session=%s\n", state.ActiveChild.ID, state.ActiveChild.Stage, state.ActiveChild.SessionPath)
	if state.LastActionCard != nil {
		return writeManagerActionCard(out, *state.LastActionCard, opts.Output)
	}
	return nil
}

func RunValidateLatest(ctx context.Context, opts ValidateLatestOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	candidate, ok, err := FindLatestRelevantChildSession(latestQueryFromState(state, opts.Stage))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no relevant child sessions found")
	}
	if opts.ApplyRebind {
		state, err = RebindActiveChild(state, candidate.Child, "validate-latest")
		if err != nil {
			return err
		}
		if err := store.Save(opts.StateFile, state); err != nil {
			return err
		}
	}
	if opts.Continue {
		if !opts.ApplyRebind && (state.ActiveChild == nil || filepath.Clean(strings.TrimSpace(state.ActiveChild.SessionPath)) != filepath.Clean(candidate.SessionPath)) {
			return fmt.Errorf("latest session differs from active child; rerun with --apply-rebind or use rebind-child --session-file %s", candidate.SessionPath)
		}
		return RunContinue(ctx, ContinueOptions{StateFile: opts.StateFile, PlanDir: state.CanonicalPlanDir, Stage: string(wruntime.NodeID(candidate.Child.Stage)), Output: opts.Output}, d, out)
	}
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{SessionFile: candidate.SessionPath, SessionID: candidate.SessionID, RunID: candidate.Child.ID})
	if err != nil {
		return err
	}
	stage := strings.TrimSpace(opts.Stage)
	if stage == "" {
		stage = candidate.Child.Stage
	}
	parseCtx.ExpectedNodeID = wruntime.NodeID(stage)
	parsed, err := ParseNormalizeValidateDecide(text, state, parseCtx)
	if err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "json") {
		return json.NewEncoder(out).Encode(map[string]any{"candidate": candidate, "decision": parsed})
	}
	fmt.Fprintf(out, "validated latest: %s %s\n", parsed.Result.SourceNodeID, parsed.Result.Status)
	if parsed.Decision.NextNodeID != "" {
		fmt.Fprintf(out, "next: %s\n", parsed.Decision.NextNodeID)
	}
	fmt.Fprintf(out, "session: %s\n", candidate.SessionPath)
	if candidate.SessionPath != strings.TrimSpace(activeSessionPath(state)) {
		fmt.Fprintf(out, "safe command: vamos qrspi rebind-child --state-file %s --session-file %s --stage %s\n", opts.StateFile, candidate.SessionPath, stage)
	}
	return nil
}

func RunRecoverManual(ctx context.Context, opts RecoverManualOptions, d deps, out io.Writer) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	if opts.Mode != "" && opts.Mode != "latest-session" {
		return fmt.Errorf("unsupported recovery mode %q", opts.Mode)
	}
	out = ensureWriter(out)
	store := stateStore(d, "", time.Now)
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	candidate, ok, err := FindLatestRelevantChildSession(latestQueryFromState(state, ""))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no relevant child sessions found")
	}
	state, err = RebindActiveChild(state, candidate.Child, "recover-manual")
	if err != nil {
		return err
	}
	if err := store.Save(opts.StateFile, state); err != nil {
		return err
	}
	if opts.Continue {
		return RunContinue(ctx, ContinueOptions{StateFile: opts.StateFile, PlanDir: state.CanonicalPlanDir, Stage: candidate.Child.Stage, Output: opts.Output}, d, out)
	}
	if strings.EqualFold(opts.Output, "json") {
		return json.NewEncoder(out).Encode(candidate)
	}
	fmt.Fprintf(out, "recovered manual child: %s (%s)\n", candidate.SessionPath, candidate.Classification)
	fmt.Fprintf(out, "safe command: vamos qrspi validate-latest --state-file %s --apply-rebind --continue\n", opts.StateFile)
	return nil
}

func FindLatestRelevantChildSession(query LatestChildQuery) (LatestChildCandidate, bool, error) {
	sessionDir := strings.TrimSpace(query.SessionDir)
	if sessionDir == "" && query.ActiveChild != nil {
		sessionDir = strings.TrimSpace(query.ActiveChild.SessionDir)
	}
	if sessionDir == "" {
		return LatestChildCandidate{}, false, nil
	}
	var latest LatestChildCandidate
	walkErr := filepath.WalkDir(sessionDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			return nil
		}
		header, err := readSessionHeader(path)
		if err != nil || header.Type != "session" {
			return nil
		}
		if strings.TrimSpace(query.WorkspaceCwd) != "" && strings.TrimSpace(header.Cwd) != "" && filepath.Clean(header.Cwd) != filepath.Clean(query.WorkspaceCwd) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		child := childFromQuery(query, header, path)
		candidate := LatestChildCandidate{Child: child, SessionPath: path, SessionID: header.ID, ModTime: info.ModTime()}
		candidate.Classification, candidate.Reason = classifyLatestChild(query, candidate)
		if latest.SessionPath == "" || candidate.ModTime.After(latest.ModTime) || (candidate.ModTime.Equal(latest.ModTime) && candidate.SessionPath > latest.SessionPath) {
			latest = candidate
		}
		return nil
	})
	if walkErr != nil {
		return LatestChildCandidate{}, false, walkErr
	}
	return latest, latest.SessionPath != "", nil
}

func RebindActiveChild(state ManagerState, child ChildRunRef, reason string) (ManagerState, error) {
	if strings.TrimSpace(child.SessionPath) == "" {
		return state, errors.New("child session path is required")
	}
	if strings.TrimSpace(child.Stage) == "" {
		child.Stage = string(state.Workflow.CurrentNodeID)
	}
	if strings.TrimSpace(child.ID) == "" {
		child.ID = "manual-" + strings.Trim(strings.ReplaceAll(child.SessionID, "/", "-"), "-")
	}
	if state.ActiveChild != nil {
		child.TmuxPaneID = firstNonEmpty(child.TmuxPaneID, state.ActiveChild.TmuxPaneID)
		child.OutputPath = firstNonEmpty(child.OutputPath, state.ActiveChild.OutputPath)
		child.DonePath = firstNonEmpty(child.DonePath, state.ActiveChild.DonePath)
		child.StatusPath = firstNonEmpty(child.StatusPath, state.ActiveChild.StatusPath)
		child.ValidationStatusPath = firstNonEmpty(child.ValidationStatusPath, state.ActiveChild.ValidationStatusPath)
		child.Generation = activeChildGeneration(state) + 1
	} else {
		child.Generation = 1
	}
	child.LifecycleStatus = "manual_rebound"
	child.ValidationRetryCount = 0
	child.LastRepromptAttempt = 0
	state.ActiveChild = &child
	state = SupersedeStaleQueuedWake(state, child)
	action := semantic.NextAction{Kind: semantic.NextActionManualRecovery, Severity: "info", CurrentNodeID: wruntime.NodeID(child.Stage), RecoveryReason: fmt.Sprintf("active child rebound: %s", firstNonEmpty(reason, "manual recovery")), Evidence: []string{fmt.Sprintf("session: %s", child.SessionPath)}}
	state.LastActionCard = ProjectManagerActionCard(action, state, "")
	return state, nil
}

func ValidateLatestChildSession(opts ValidateResultOptions) (ChildCompletionStatus, error) {
	state, err := FileStateStore{}.Load(opts.StateFile)
	if err != nil {
		return ChildCompletionStatus{}, err
	}
	candidate, ok, err := FindLatestRelevantChildSession(latestQueryFromState(state, opts.Stage))
	if err != nil {
		return ChildCompletionStatus{}, err
	}
	if !ok {
		return ChildCompletionStatus{}, errors.New("no relevant child sessions found")
	}
	text, parseCtx, err := ReadChildResultText(state, ResultSourceOptions{SessionFile: candidate.SessionPath, SessionID: candidate.SessionID, RunID: candidate.Child.ID})
	if err != nil {
		return ChildCompletionStatus{}, err
	}
	parseCtx.ExpectedNodeID = wruntime.NodeID(firstNonEmpty(opts.Stage, candidate.Child.Stage))
	parsed, err := ParseNormalizeValidateDecide(text, state, parseCtx)
	if err != nil {
		return ChildCompletionStatus{}, err
	}
	return ChildCompletionStatus{Validated: true, ChildID: candidate.Child.ID, Result: childCompletionResult(parsed.Result), Normalizations: parsed.Normalizations}, nil
}

func SupersedeStaleQueuedWake(state ManagerState, child ChildRunRef) ManagerState {
	queued := state.Delivery.QueuedWake
	if queued == nil {
		return state
	}
	if queued.ChildID != child.ID || queued.ChildGeneration != child.Generation {
		state.Delivery.QueuedWake = nil
		state.LastActionCard = &ManagerActionCard{Kind: ActionSupersededQueuedWake, Severity: "info", Summary: "queued child wake superseded by recovered child session", Evidence: []string{fmt.Sprintf("session: %s", child.SessionPath)}, RecommendedAction: "continue from latest validated child session", SafeCommand: "vamos qrspi recover-manual --mode latest-session --continue", RequiresHuman: false}
	}
	return state
}

func latestQueryFromState(state ManagerState, stage string) LatestChildQuery {
	active := state.ActiveChild
	workspace := strings.TrimSpace(state.ImplementationCwd)
	if workspace == "" && active != nil {
		workspace = active.Cwd
	}
	if strings.TrimSpace(stage) == "" && active != nil {
		stage = active.Stage
	}
	return LatestChildQuery{ManagerRunID: state.ManagerRunID, PlanDir: state.CanonicalPlanDir, Stage: wruntime.NodeID(stage), WorkspaceCwd: workspace, ActiveChild: active}
}

func childFromQuery(query LatestChildQuery, header sessionEntry, path string) ChildRunRef {
	child := ChildRunRef{Stage: string(query.Stage), Cwd: header.Cwd, SessionID: header.ID, SessionDir: filepath.Dir(path), SessionPath: path}
	if query.ActiveChild != nil {
		base := *query.ActiveChild
		base.SessionID = header.ID
		base.SessionDir = filepath.Dir(path)
		base.SessionPath = path
		if header.Cwd != "" {
			base.Cwd = header.Cwd
		}
		if query.Stage != "" {
			base.Stage = string(query.Stage)
		}
		return base
	}
	if child.Stage == "" {
		child.Stage = "manual"
	}
	child.ID = "manual-" + strings.Trim(strings.ReplaceAll(header.ID, "/", "-"), "-")
	return child
}

func classifyLatestChild(query LatestChildQuery, candidate LatestChildCandidate) (string, string) {
	if query.ActiveChild == nil {
		return "manual_new", "no active child; session can seed manual recovery"
	}
	activePath := strings.TrimSpace(query.ActiveChild.SessionPath)
	if activePath != "" && filepath.Clean(activePath) == filepath.Clean(candidate.SessionPath) {
		return "managed_same_session", "session matches active child"
	}
	if query.ActiveChild.SessionID != "" && query.ActiveChild.SessionID == candidate.SessionID {
		return "manual_same_child_chat", "same session id at newer path"
	}
	return "manual_new", "newer child session in active child session directory"
}

func childRefFromSessionFile(state ManagerState, sessionFile, stage string) (ChildRunRef, error) {
	header, err := readSessionHeader(sessionFile)
	if err != nil {
		return ChildRunRef{}, err
	}
	if header.Type != "session" {
		return ChildRunRef{}, fmt.Errorf("%s is not a Pi session JSONL", sessionFile)
	}
	query := latestQueryFromState(state, stage)
	return childFromQuery(query, header, sessionFile), nil
}

func writeLatestCandidate(out io.Writer, candidate LatestChildCandidate, mode string) error {
	if strings.EqualFold(mode, "json") {
		return json.NewEncoder(out).Encode(candidate)
	}
	fmt.Fprintf(out, "latest child: %s\n", candidate.SessionPath)
	fmt.Fprintf(out, "classification: %s\n", candidate.Classification)
	fmt.Fprintf(out, "reason: %s\n", candidate.Reason)
	fmt.Fprintf(out, "stage: %s\n", candidate.Child.Stage)
	return nil
}

func activeSessionPath(state ManagerState) string {
	if state.ActiveChild == nil {
		return ""
	}
	return state.ActiveChild.SessionPath
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
