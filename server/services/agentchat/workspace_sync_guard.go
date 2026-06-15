package agentchat

import (
	"context"
	"errors"
	"sync"
)

type WorkspaceSyncRunKind string

const (
	WorkspaceSyncRunScheduled  WorkspaceSyncRunKind = "scheduled"
	WorkspaceSyncRunManual     WorkspaceSyncRunKind = "manual"
	WorkspaceSyncRunDeprecated WorkspaceSyncRunKind = "deprecated-plan-discovery"
)

var ErrWorkspaceSyncInProgress = errors.New("workspace sync already in progress")

type WorkspaceSyncGuardResult struct {
	Acquired bool
	Reason   string
	Kind     WorkspaceSyncRunKind
}

type WorkspaceSyncGuard struct {
	mu          sync.Mutex
	running     bool
	runningKind WorkspaceSyncRunKind
}

func NewWorkspaceSyncGuard() *WorkspaceSyncGuard { return &WorkspaceSyncGuard{} }

func (g *WorkspaceSyncGuard) InProgress() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running
}

func (g *WorkspaceSyncGuard) TryRun(
	ctx context.Context,
	kind WorkspaceSyncRunKind,
	fn func(context.Context) error,
) (WorkspaceSyncGuardResult, error) {
	if g == nil {
		if fn == nil {
			return WorkspaceSyncGuardResult{Acquired: true, Kind: kind}, nil
		}
		return WorkspaceSyncGuardResult{Acquired: true, Kind: kind}, fn(ctx)
	}

	g.mu.Lock()
	if g.running {
		reason := "workspace sync already running"
		if g.runningKind != "" {
			reason = "workspace sync already running: " + string(g.runningKind)
		}
		g.mu.Unlock()
		return WorkspaceSyncGuardResult{Acquired: false, Reason: reason, Kind: kind}, ErrWorkspaceSyncInProgress
	}
	g.running = true
	g.runningKind = kind
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.running = false
		g.runningKind = ""
		g.mu.Unlock()
	}()

	if fn == nil {
		return WorkspaceSyncGuardResult{Acquired: true, Kind: kind}, nil
	}
	return WorkspaceSyncGuardResult{Acquired: true, Kind: kind}, fn(ctx)
}
