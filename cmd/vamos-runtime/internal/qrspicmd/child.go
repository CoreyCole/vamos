package qrspicmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ChildRunRequest struct {
	ID          string
	Stage       string
	Cwd         string
	PromptFile  string
	OutputPath  string
	SessionID   string
	SessionDir  string
	SessionName string
	DonePath    string
	StatusPath  string
	Split       string
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
func ChildSessionID(childID string) string {
	clean := strings.NewReplacer("/", "-", " ", "-", "_", "-").Replace(strings.TrimSpace(childID))
	clean = strings.Trim(clean, "-.")
	if clean == "" {
		return "q-manager-child"
	}
	return clean
}

func BuildChildCommand(req ChildRunRequest) []string {
	script := `set -o pipefail
pi --print --session-id "$SESSION_ID" --session-dir "$SESSION_DIR" --name "$SESSION_NAME" < "$PROMPT_FILE" 2>&1 | tee "$OUTPUT_PATH"
status=${PIPESTATUS[0]}
printf '{"exitCode":%d,"finishedAt":"%s"}\n' "$status" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$STATUS_PATH"
touch "$DONE_PATH"
exit "$status"`
	return []string{
		"env",
		"PROMPT_FILE=" + req.PromptFile,
		"OUTPUT_PATH=" + req.OutputPath,
		"SESSION_ID=" + req.SessionID,
		"SESSION_DIR=" + req.SessionDir,
		"SESSION_NAME=" + req.SessionName,
		"DONE_PATH=" + req.DonePath,
		"STATUS_PATH=" + req.StatusPath,
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
	for _, dir := range []string{filepath.Dir(req.OutputPath), req.SessionDir, filepath.Dir(req.DonePath), filepath.Dir(req.StatusPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
