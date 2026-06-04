package workspacecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func RunDoctor(
	ctx context.Context,
	cfg WorkspaceCLIConfig,
	tail int,
	out io.Writer,
) error {
	fmt.Fprintln(out, "workspace doctor")
	_ = RunStatus(ctx, cfg, out)
	if branch := gitOutput(
		ctx,
		cfg.CheckoutPath,
		"rev-parse",
		"--abbrev-ref",
		"HEAD",
	); branch != "" {
		fmt.Fprintf(out, "git_branch: %s\n", branch)
	}
	if commit := gitOutput(ctx, cfg.CheckoutPath, "rev-parse", "HEAD"); commit != "" {
		fmt.Fprintf(out, "git_commit: %s\n", commit)
	}
	for component, pid := range cfg.Status.PIDs {
		fmt.Fprintf(out, "pid_alive.%s: %t\n", component, processAlive(pid))
	}
	for _, target := range []WorkspaceLogTarget{WorkspaceLogWeb, WorkspaceLogTemporal, WorkspaceLogTSWorker} {
		fmt.Fprintf(out, "\n--- %s log ---\n", target)
		_ = RunLogs(ctx, cfg, target, tail, out)
	}
	fmt.Fprintln(out, "\nSuggested recovery:")
	fmt.Fprintln(out, "  vamos ctl workspace restart --force")
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		proc, err := os.FindProcess(pid)
		return err == nil && proc != nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return strings.Contains(err.Error(), "operation not permitted") ||
		strings.Contains(err.Error(), "permission denied") ||
		strings.Contains(err.Error(), strconv.Itoa(pid)) && os.IsPermission(err)
}
