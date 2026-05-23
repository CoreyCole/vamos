package workspaces

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestLocalComponentRunnerStartComponentSurvivesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	runner := LocalComponentRunner{}
	handle, err := runner.StartComponent(ctx, ComponentSpec{
		Component: ComponentWeb,
		Args:      []string{"sh", "-c", "sleep 30"},
		Dir:       t.TempDir(),
		LogPath:   t.TempDir() + "/web.log",
		PIDPath:   t.TempDir() + "/web.pid",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = runner.StopComponent(t.Context(), handle)
	})

	cancel()
	time.Sleep(100 * time.Millisecond)
	if !processAlive(handle.pid()) {
		t.Fatal("process exited after start context was canceled")
	}
}

func TestLocalComponentRunnerStopComponentTreatsTerminatedProcessAsStopped(t *testing.T) {
	oldGrace := componentStopGracePeriod
	oldKill := componentKillWaitPeriod
	componentStopGracePeriod = 100 * time.Millisecond
	componentKillWaitPeriod = time.Second
	t.Cleanup(func() {
		componentStopGracePeriod = oldGrace
		componentKillWaitPeriod = oldKill
	})

	runner := LocalComponentRunner{}
	handle, err := runner.StartComponent(context.Background(), ComponentSpec{
		Component: ComponentWeb,
		Args:      []string{"sh", "-c", "sleep 30"},
		Dir:       t.TempDir(),
		LogPath:   t.TempDir() + "/web.log",
		PIDPath:   t.TempDir() + "/web.pid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.StopComponent(context.Background(), handle); err != nil {
		t.Fatalf("StopComponent returned error for terminated process: %v", err)
	}
}

func TestLocalComponentRunnerStopComponentKillsAfterGracePeriod(t *testing.T) {
	oldGrace := componentStopGracePeriod
	oldKill := componentKillWaitPeriod
	componentStopGracePeriod = 100 * time.Millisecond
	componentKillWaitPeriod = time.Second
	t.Cleanup(func() {
		componentStopGracePeriod = oldGrace
		componentKillWaitPeriod = oldKill
	})

	runner := LocalComponentRunner{}
	handle, err := runner.StartComponent(context.Background(), ComponentSpec{
		Component: ComponentTSWorker,
		Args:      []string{"sh", "-c", "trap '' TERM; sleep 30"},
		Dir:       t.TempDir(),
		LogPath:   t.TempDir() + "/ts.log",
		PIDPath:   t.TempDir() + "/ts.pid",
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := runner.StopComponent(context.Background(), handle); err != nil {
		t.Fatalf("StopComponent returned error after SIGKILL: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("StopComponent took %s, want bounded stop", elapsed)
	}
}

func TestWaitForProcessExitTimesOutForUnfinishedHandle(t *testing.T) {
	t.Parallel()

	handle := &ProcessHandle{
		Component: ComponentWeb,
		Command:   &exec.Cmd{},
		exited:    make(chan struct{}),
	}
	if waitForProcessExit(handle, 10*time.Millisecond) {
		t.Fatal("waitForProcessExit returned true for unfinished handle")
	}
}
