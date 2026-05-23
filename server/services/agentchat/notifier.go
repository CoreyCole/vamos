package agentchat

import (
	"strings"
	"sync"
)

type Notifier struct {
	mu      sync.RWMutex
	subs    map[string][]chan WorkspaceStreamSignal
	cursors map[string]int64
}

func NewNotifier() *Notifier {
	return &Notifier{
		subs:    make(map[string][]chan WorkspaceStreamSignal),
		cursors: make(map[string]int64),
	}
}

func (n *Notifier) Subscribe(workspaceID string) chan WorkspaceStreamSignal {
	n.mu.Lock()
	defer n.mu.Unlock()

	ch := make(chan WorkspaceStreamSignal, 32)
	n.subs[workspaceID] = append(n.subs[workspaceID], ch)
	return ch
}

func (n *Notifier) Unsubscribe(workspaceID string, ch chan WorkspaceStreamSignal) {
	n.mu.Lock()
	defer n.mu.Unlock()

	subs := n.subs[workspaceID]
	for i, sub := range subs {
		if sub == ch {
			n.subs[workspaceID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(n.subs[workspaceID]) == 0 {
		delete(n.subs, workspaceID)
	}
}

func (n *Notifier) SubscriberCount(workspaceID string) int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.subs[workspaceID])
}

func (n *Notifier) CurrentCursor(workspaceID string) int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cursors[workspaceID]
}

func (n *Notifier) Notify(
	workspaceID string,
	signal WorkspaceStreamSignal,
) WorkspaceStreamSignal {
	n.mu.Lock()
	cursor := n.cursors[workspaceID] + 1
	n.cursors[workspaceID] = cursor
	signal.Cursor = cursor
	subs := append([]chan WorkspaceStreamSignal(nil), n.subs[workspaceID]...)
	n.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- signal:
		default:
		}
	}

	return signal
}

func (n *Notifier) NotifyWorkspaceResource(workspaceID string) WorkspaceStreamSignal {
	return n.Notify(workspaceID, WorkspaceStreamSignal{Scope: PatchWorkspaceResource})
}

func (n *Notifier) NotifyLiveTranscript(workspaceID string) WorkspaceStreamSignal {
	return n.Notify(workspaceID, WorkspaceStreamSignal{Scope: PatchLiveTranscript})
}

func (n *Notifier) NotifyScopes(workspaceID string, scopes ...WorkspacePatchScope) {
	for _, scope := range scopes {
		n.Notify(workspaceID, WorkspaceStreamSignal{Scope: scope})
	}
}

func projectPlanSidebarNotifyKey(projectName string) string {
	projectName = strings.TrimSpace(strings.ToLower(projectName))
	if projectName == "" {
		projectName = "project"
	}
	return "plan-sidebar:project:" + projectName
}

func (n *Notifier) NotifyProjectPlanSidebar(projectName string) WorkspaceStreamSignal {
	return n.Notify(
		projectPlanSidebarNotifyKey(projectName),
		WorkspaceStreamSignal{Scope: PatchWorkspaceSidebar},
	)
}
