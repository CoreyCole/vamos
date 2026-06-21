package qrspicmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

type ChildRunRequest struct {
	ID         string
	Stage      string
	Cwd        string
	PromptFile string
	OutputPath string
	ResultPath string
	Split      string
}

type ChildRun struct {
	ID         string
	Pane       TmuxPane
	OutputPath string
	ResultPath string
}

type ChildRunResult struct {
	ID         string
	ExitCode   int
	OutputPath string
	ResultPath string
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

func BuildChildCommand(req ChildRunRequest) []string {
	script := `set -o pipefail; pi --print < "$PROMPT_FILE" 2>&1 | tee "$OUTPUT_PATH"; status=${PIPESTATUS[0]}; cp "$OUTPUT_PATH" "$RESULT_PATH"; exit $status`
	return []string{
		"env",
		"PROMPT_FILE=" + req.PromptFile,
		"OUTPUT_PATH=" + req.OutputPath,
		"RESULT_PATH=" + req.ResultPath,
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
	return ChildRun{ID: req.ID, Pane: pane, OutputPath: req.OutputPath, ResultPath: req.ResultPath}, nil
}

func (r TmuxChildRunner) Wait(ctx context.Context, run ChildRun) (ChildRunResult, error) {
	select {
	case <-ctx.Done():
		return ChildRunResult{}, ctx.Err()
	default:
	}
	return ChildRunResult{ID: run.ID, OutputPath: run.OutputPath, ResultPath: run.ResultPath}, nil
}

func ensureRunFiles(req ChildRunRequest) error {
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(req.ResultPath), 0o755)
}
