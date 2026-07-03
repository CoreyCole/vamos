package appletruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartAppletSweeperTicksAndLogs(t *testing.T) {
	manager := &fakeSweepManager{
		stopped: []AppletProcessState{{AppID: "wordle", Status: ProcessStatusStopped}},
		err:     errors.New("boom"),
		called:  make(chan time.Time, 1),
	}
	logger := &fakeSweepLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go StartAppletSweeper(ctx, manager, SweepOptions{Interval: time.Millisecond, Logger: logger})

	select {
	case <-manager.called:
	case <-time.After(time.Second):
		t.Fatal("StartAppletSweeper did not call SweepInactive")
	}
	cancel()

	log := logger.String()
	if !strings.Contains(log, "applet_sweeper stopped=1 err=boom") {
		t.Fatalf("logger output = %q", log)
	}
}

func TestStartAppletSweeperStopsOnContextCancel(t *testing.T) {
	manager := &fakeSweepManager{called: make(chan time.Time, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		StartAppletSweeper(ctx, manager, SweepOptions{Interval: time.Hour})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StartAppletSweeper did not return after context cancellation")
	}
}

type fakeSweepManager struct {
	stopped []AppletProcessState
	err     error
	called  chan time.Time
}

func (m *fakeSweepManager) EnsureStarted(context.Context, RuntimeConfig) (AppletProcessState, error) {
	return AppletProcessState{}, nil
}

func (m *fakeSweepManager) Start(context.Context, RuntimeConfig) (ProcessState, error) {
	return ProcessState{}, nil
}

func (m *fakeSweepManager) Stop(context.Context, string) error { return nil }

func (m *fakeSweepManager) Health(context.Context, string) (AppletProcessState, error) {
	return AppletProcessState{}, nil
}

func (m *fakeSweepManager) ProxyTarget(string) (string, bool) { return "", false }

func (m *fakeSweepManager) Touch(string, int) {}

func (m *fakeSweepManager) SweepInactive(_ context.Context, now time.Time) ([]AppletProcessState, error) {
	select {
	case m.called <- now:
	default:
	}
	return m.stopped, m.err
}

type fakeSweepLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *fakeSweepLogger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *fakeSweepLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.entries, "\n")
}
