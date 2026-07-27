//go:build darwin

package qrspicmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func startLaneProcess(command []string, cwd string) (LaneProcess, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execLaneProcess{cmd: cmd, done: make(chan laneWaitResult, 1)}, nil
}

func terminateLaneProcess(
	ctx context.Context,
	process *execLaneProcess,
	grace time.Duration,
) error {
	if process == nil || process.cmd == nil || process.cmd.Process == nil {
		return errors.New("lane process is unavailable")
	}
	if grace <= 0 {
		return errors.New("lane termination grace must be positive")
	}
	pid := process.PID()
	if err := signalLaneProcessGroup(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("terminate lane process group %d: %w", pid, err)
	}
	graceCtx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if _, err := process.Wait(
		graceCtx,
	); err != nil &&
		!errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf(
			"reap lane process %d after termination: %w",
			pid,
			err,
		)
	}
	if err := waitForLaneProcessGroupExit(graceCtx, pid); err == nil {
		return nil
	} else if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf(
			"wait for lane process group %d after termination: %w",
			pid,
			err,
		)
	}
	if err := signalLaneProcessGroup(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill lane process group %d: %w", pid, err)
	}
	return nil
}

func waitForLaneProcessGroupExit(ctx context.Context, pid int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func signalLaneProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
