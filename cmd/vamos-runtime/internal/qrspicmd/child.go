package qrspicmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ChildRunRequest struct {
	ID                   string
	Stage                string
	Cwd                  string
	PromptFile           string
	OutputPath           string
	SessionID            string
	SessionDir           string
	SessionName          string
	DonePath             string
	StatusPath           string
	ValidationStatusPath string
	Split                string
	ParentPaneID         string
	StateFile            string
	PlanDir              string
	ExtensionPath        string
}

type ChildRun struct {
	ID         string
	Pane       TmuxPane
	OutputPath string
	SessionID  string
	SessionDir string
	DonePath   string
	StatusPath string
}

type ChildRunResult struct {
	ID          string
	ExitCode    int
	OutputPath  string
	SessionID   string
	SessionDir  string
	SessionPath string
	DonePath    string
	StatusPath  string
}

type ChildStatus struct {
	ExitCode   int    `json:"exitCode"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

type ChildTurnStatus struct {
	Event      string `json:"event"`
	Stage      string `json:"stage"`
	ChildID    string `json:"childId"`
	StateFile  string `json:"stateFile"`
	PlanDir    string `json:"planDir"`
	SessionID  string `json:"sessionId"`
	SessionDir string `json:"sessionDir"`
	FinishedAt string `json:"finishedAt"`
	WakeTarget string `json:"wakeTarget"`
}

type TmuxSplitRequest struct {
	Cwd       string
	Direction string
	Command   []string
}

type TmuxPane struct{ ID string }

func ResultPath(root string, childID string) string {
	return filepath.Join(root, "runs", childID, "result.txt")
}
func OutputPath(root string, childID string) string {
	return filepath.Join(root, "runs", childID, "output.txt")
}
func SessionDir(root, childID string) string {
	return filepath.Join(root, "runs", childID, "sessions")
}
func DonePath(root, childID string) string {
	return filepath.Join(root, "runs", childID, "done")
}
func StatusPath(root, childID string) string {
	return filepath.Join(root, "runs", childID, "status.json")
}
func ValidationStatusPath(root, childID string) string {
	return filepath.Join(root, "runs", childID, "validation-status.json")
}
func ChildSessionID(childID string) string {
	clean := strings.NewReplacer("/", "-", " ", "-", "_", "-").Replace(strings.TrimSpace(childID))
	clean = strings.Trim(clean, "-.")
	if clean == "" {
		return "q-manager-child"
	}
	return clean
}

func BuildChildCommand(req ChildRunRequest) []string {
	piCommand := `pi --session-id "$SESSION_ID" --session-dir "$SESSION_DIR" --name "$SESSION_NAME" "@$PROMPT_FILE"`
	if strings.TrimSpace(req.ExtensionPath) != "" {
		piCommand = `pi --extension "$Q_MANAGER_CHILD_EXTENSION" --session-id "$SESSION_ID" --session-dir "$SESSION_DIR" --name "$SESSION_NAME" "@$PROMPT_FILE"`
	}
	script := `set -o pipefail
printf 'q-manager child starting\n'
printf 'stage: %s\n' "$STAGE"
printf 'cwd: %s\n' "$PWD"
printf 'session id: %s\n' "$SESSION_ID"
printf 'session dir: %s\n' "$SESSION_DIR"
printf 'prompt: %s\n' "$PROMPT_FILE"
printf 'output: %s\n' "$OUTPUT_PATH"
printf 'mode: interactive Pi; exit child Pi after final qrspi_result so q-manager can validate\n\n'
: > "$OUTPUT_PATH"
` + piCommand + `
status=$?
if [ -n "${TMUX_PANE:-}" ]; then
  tmux capture-pane -p -t "$TMUX_PANE" -S - > "$OUTPUT_PATH" 2>/dev/null || true
fi
printf '{"exitCode":%d,"finishedAt":"%s"}\n' "$status" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$STATUS_PATH"
touch "$DONE_PATH"
exit "$status"`
	return []string{
		"env",
		"STAGE=" + req.Stage,
		"PROMPT_FILE=" + req.PromptFile,
		"OUTPUT_PATH=" + req.OutputPath,
		"SESSION_ID=" + req.SessionID,
		"SESSION_DIR=" + req.SessionDir,
		"SESSION_NAME=" + req.SessionName,
		"DONE_PATH=" + req.DonePath,
		"STATUS_PATH=" + req.StatusPath,
		"Q_MANAGER_PARENT_PANE=" + req.ParentPaneID,
		"Q_MANAGER_STATE_FILE=" + req.StateFile,
		"Q_MANAGER_PLAN_DIR=" + req.PlanDir,
		"Q_MANAGER_STAGE=" + req.Stage,
		"Q_MANAGER_CHILD_ID=" + req.ID,
		"Q_MANAGER_DONE_PATH=" + req.DonePath,
		"Q_MANAGER_STATUS_PATH=" + req.StatusPath,
		"Q_MANAGER_VALIDATED_STATUS_PATH=" + req.ValidationStatusPath,
		"Q_MANAGER_WAKE_MODE=validated-only",
		"Q_MANAGER_SESSION_ID=" + req.SessionID,
		"Q_MANAGER_SESSION_DIR=" + req.SessionDir,
		"Q_MANAGER_CHILD_EXTENSION=" + req.ExtensionPath,
		"bash",
		"-lc",
		script,
	}
}

type TmuxChildRunner struct{ Tmux TmuxClient }

func (r TmuxChildRunner) Start(ctx context.Context, req ChildRunRequest) (ChildRun, error) {
	if r.Tmux == nil {
		return ChildRun{}, errors.New("tmux client is required")
	}
	pane, err := r.Tmux.SplitPane(ctx, TmuxSplitRequest{Cwd: req.Cwd, Direction: req.Split, Command: BuildChildCommand(req)})
	if err != nil {
		return ChildRun{}, err
	}
	return ChildRun{ID: req.ID, Pane: pane, OutputPath: req.OutputPath, SessionID: req.SessionID, SessionDir: req.SessionDir, DonePath: req.DonePath, StatusPath: req.StatusPath}, nil
}

func (r TmuxChildRunner) Wait(ctx context.Context, run ChildRun) (ChildRunResult, error) {
	select {
	case <-ctx.Done():
		return ChildRunResult{}, ctx.Err()
	default:
	}
	return ChildRunResult{ID: run.ID, OutputPath: run.OutputPath, SessionID: run.SessionID, SessionDir: run.SessionDir, DonePath: run.DonePath, StatusPath: run.StatusPath}, nil
}

func ensureRunFiles(req ChildRunRequest) error {
	for _, dir := range []string{filepath.Dir(req.OutputPath), req.SessionDir, filepath.Dir(req.DonePath), filepath.Dir(req.StatusPath), filepath.Dir(req.ValidationStatusPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
