package qrspicmd

import (
	"context"
	"os/exec"
	"strings"
)

type ShellTmuxClient struct{}

func (ShellTmuxClient) SplitPane(ctx context.Context, req TmuxSplitRequest) (TmuxPane, error) {
	args := []string{"split-window", "-P", "-F", "#{pane_id}", "-h", "-c", req.Cwd}
	if req.Direction == "down" {
		args = []string{"split-window", "-P", "-F", "#{pane_id}", "-v", "-c", req.Cwd}
	}
	args = append(args, strings.Join(shellquote(req.Command), " "))
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		return TmuxPane{}, err
	}
	return TmuxPane{ID: strings.TrimSpace(string(out))}, nil
}

func (ShellTmuxClient) SendKeys(ctx context.Context, pane TmuxPane, keys []string) error {
	args := append([]string{"send-keys", "-t", pane.ID}, keys...)
	return exec.CommandContext(ctx, "tmux", args...).Run()
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
