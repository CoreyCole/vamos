package workspaces

import "sync"

type WorkspaceLifecycleNotifier interface {
	Notify(slug string)
	Subscribe() (<-chan struct{}, func())
}

type LifecycleNotifier struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func NewLifecycleNotifier() *LifecycleNotifier {
	return &LifecycleNotifier{subscribers: map[chan struct{}]struct{}{}}
}

func (n *LifecycleNotifier) Notify(_ string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (n *LifecycleNotifier) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	n.subscribers[ch] = struct{}{}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		if _, ok := n.subscribers[ch]; ok {
			delete(n.subscribers, ch)
			close(ch)
		}
		n.mu.Unlock()
	}
}

func (n *LifecycleNotifier) SubscriberCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.subscribers)
}
