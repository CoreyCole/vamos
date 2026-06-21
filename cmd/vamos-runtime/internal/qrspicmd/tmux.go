package qrspicmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

type ShellTmuxClient struct{}

func (ShellTmuxClient) SplitPane(ctx context.Context, req TmuxSplitRequest) (TmuxPane, error) {
	out, err := exec.CommandContext(ctx, "tmux", splitPaneArgs(req, os.Getenv("TMUX_PANE"))...).Output()
	if err != nil {
		return TmuxPane{}, err
	}
	return TmuxPane{ID: strings.TrimSpace(string(out))}, nil
}

func splitPaneArgs(req TmuxSplitRequest, targetPane string) []string {
	args := []string{"split-window", "-P", "-F", "#{pane_id}"}
	if strings.TrimSpace(targetPane) != "" {
		args = append(args, "-t", targetPane)
	}
	if req.Direction == "down" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	return append(args, "-c", req.Cwd, strings.Join(shellquote(req.Command), " "))
}

func (ShellTmuxClient) SendKeys(ctx context.Context, pane TmuxPane, keys []string) error {
	args := append([]string{"send-keys", "-t", pane.ID}, keys...)
	return exec.CommandContext(ctx, "tmux", args...).Run()
}

func (ShellTmuxClient) PasteText(ctx context.Context, pane TmuxPane, text string) error {
	if strings.TrimSpace(pane.ID) == "" {
		return errors.New("tmux pane ID is required")
	}
	if err := exec.CommandContext(ctx, "tmux", setBufferArgs("q-manager-wake", text)...).Run(); err != nil {
		return err
	}
	return exec.CommandContext(ctx, "tmux", pasteBufferArgs("q-manager-wake", pane.ID)...).Run()
}

func (ShellTmuxClient) KillPane(ctx context.Context, pane TmuxPane) error {
	if strings.TrimSpace(pane.ID) == "" {
		return errors.New("tmux pane ID is required")
	}
	return exec.CommandContext(ctx, "tmux", killPaneArgs(pane.ID)...).Run()
}

func (ShellTmuxClient) SelectLayout(ctx context.Context, pane TmuxPane, layout string) error {
	if strings.TrimSpace(pane.ID) == "" {
		return errors.New("tmux pane ID is required")
	}
	if strings.TrimSpace(layout) == "" {
		return errors.New("tmux layout is required")
	}
	return exec.CommandContext(ctx, "tmux", selectLayoutArgs(pane.ID, layout)...).Run()
}

func setBufferArgs(name, text string) []string {
	return []string{"set-buffer", "-b", name, text}
}

func pasteBufferArgs(name, paneID string) []string {
	return []string{"paste-buffer", "-b", name, "-t", paneID}
}

func killPaneArgs(paneID string) []string {
	return []string{"kill-pane", "-t", paneID}
}

func selectLayoutArgs(paneID, layout string) []string {
	return []string{"select-layout", "-t", paneID, layout}
}

func shellquote(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return quoted
}
