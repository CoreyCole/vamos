package workspaces

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type ProcessHandle struct {
	Component BundleComponent
	Command   *exec.Cmd
	LogFile   *os.File

	done   chan error
	exited chan struct{}
	once   sync.Once
	err    error
}

type ComponentSpec struct {
	Component BundleComponent
	Args      []string
	Dir       string
	Env       []string
	LogPath   string
	PIDPath   string
}

type BundleHandles map[BundleComponent]*ProcessHandle

type ComponentStarter interface {
	StartComponent(context.Context, ComponentSpec) (*ProcessHandle, error)
}

type ComponentStopper interface {
	StopComponent(context.Context, *ProcessHandle) error
}

type LocalComponentRunner struct{}

var (
	componentStopGracePeriod = 5 * time.Second
	componentKillWaitPeriod  = 2 * time.Second
)

func (LocalComponentRunner) StartComponent(
	ctx context.Context,
	spec ComponentSpec,
) (*ProcessHandle, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if len(spec.Args) == 0 || spec.Args[0] == "" {
		return nil, fmt.Errorf("start %s: empty command", spec.Component)
	}
	if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(spec.PIDPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// #nosec G204 -- component commands are server-defined, not user input.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), spec.Args[0], spec.Args[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", spec.Component, err)
	}
	if err := os.WriteFile(
		spec.PIDPath,
		[]byte(fmt.Sprintf("%d\n", cmd.Process.Pid)),
		0o644,
	); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = logFile.Close()
		return nil, err
	}
	h := &ProcessHandle{
		Component: spec.Component,
		Command:   cmd,
		LogFile:   logFile,
		done:      make(chan error, 1),
		exited:    make(chan struct{}),
	}
	go func() {
		h.finish(cmd.Wait())
		_ = os.Remove(spec.PIDPath)
		_ = logFile.Close()
	}()
	return h, nil
}

func (LocalComponentRunner) StopComponent(ctx context.Context, h *ProcessHandle) error {
	if h == nil || h.Command == nil || h.Command.Process == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_ = syscall.Kill(-h.Command.Process.Pid, syscall.SIGTERM)
	if waitForProcessExit(h, componentStopGracePeriod) {
		return nil
	}
	_ = syscall.Kill(-h.Command.Process.Pid, syscall.SIGKILL)
	if waitForProcessExit(h, componentKillWaitPeriod) {
		return nil
	}
	return fmt.Errorf(
		"stop %s: process %d did not exit",
		h.Component,
		h.Command.Process.Pid,
	)
}

func waitForProcessExit(h *ProcessHandle, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-h.exited:
		return true
	case <-timer.C:
		return false
	}
}

func (h *ProcessHandle) pid() int {
	if h == nil || h.Command == nil || h.Command.Process == nil {
		return 0
	}
	return h.Command.Process.Pid
}

func (h *ProcessHandle) finish(err error) {
	if h == nil {
		return
	}
	h.once.Do(func() {
		h.err = err
		if h.done != nil {
			h.done <- err
		}
		close(h.exited)
	})
}

func (h *ProcessHandle) wait(ctx context.Context) error {
	if h == nil {
		return nil
	}
	select {
	case <-h.exited:
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func bundlePIDs(handles BundleHandles) map[BundleComponent]int {
	pids := map[BundleComponent]int{}
	for component, handle := range handles {
		if pid := handle.pid(); pid != 0 {
			pids[component] = pid
		}
	}
	return pids
}
