package workspace

import (
	"context"
	"strings"
	"sync"
	"time"
)

type LiveKey struct {
	WorkspaceID string
	ThreadID    string
}

type LiveFlushPolicy struct {
	Interval time.Duration
}

func (p LiveFlushPolicy) EffectiveInterval() time.Duration {
	if p.Interval <= 0 {
		return 100 * time.Millisecond
	}
	return p.Interval
}

type LiveNotifyFunc func(workspaceID string)

type LiveFlushLoop struct {
	mu     sync.Mutex
	policy LiveFlushPolicy
	notify LiveNotifyFunc
	dirty  map[LiveKey]struct{}
}

func NewLiveFlushLoop(policy LiveFlushPolicy, notify LiveNotifyFunc) *LiveFlushLoop {
	return &LiveFlushLoop{
		policy: policy,
		notify: notify,
		dirty:  make(map[LiveKey]struct{}),
	}
}

func (l *LiveFlushLoop) MarkDirty(workspaceID, threadID string) {
	if l == nil {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	threadID = strings.TrimSpace(threadID)
	if workspaceID == "" {
		return
	}
	l.mu.Lock()
	l.dirty[LiveKey{WorkspaceID: workspaceID, ThreadID: threadID}] = struct{}{}
	l.mu.Unlock()
}

func (l *LiveFlushLoop) FlushOnce(context.Context) int {
	if l == nil || l.notify == nil {
		return 0
	}
	l.mu.Lock()
	keys := make([]LiveKey, 0, len(l.dirty))
	for key := range l.dirty {
		keys = append(keys, key)
	}
	l.dirty = make(map[LiveKey]struct{})
	l.mu.Unlock()

	for _, key := range keys {
		l.notify(key.WorkspaceID)
	}
	return len(keys)
}

func (l *LiveFlushLoop) Run(ctx context.Context) {
	if l == nil {
		return
	}
	ticker := time.NewTicker(l.policy.EffectiveInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.FlushOnce(ctx)
		}
	}
}
