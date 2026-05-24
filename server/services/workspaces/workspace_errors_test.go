package workspaces

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeWorkspaceErrorTailer struct {
	mu      sync.Mutex
	tails   map[string]string
	paths   []string
	started chan struct{}
	release chan struct{}
}

func (f *fakeWorkspaceErrorTailer) Tail(path string, lines int) (string, error) {
	f.mu.Lock()
	f.paths = append(f.paths, path)
	if f.started != nil && len(f.paths) == 1 {
		close(f.started)
	}
	f.mu.Unlock()
	if f.release != nil {
		<-f.release
	}
	return f.tails[path], nil
}

func (f *fakeWorkspaceErrorTailer) recordedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

type safeWorkspaceErrorEventStore struct {
	mu     sync.Mutex
	events []UpsertWorkspaceErrorEventParams
}

func (s *safeWorkspaceErrorEventStore) UpsertWorkspaceErrorEvent(_ context.Context, arg UpsertWorkspaceErrorEventParams) (WorkspaceErrorEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, arg)
	return WorkspaceErrorEvent{ID: int64(len(s.events)), WorkspaceSlug: arg.WorkspaceSlug, Source: string(arg.Source), Severity: string(arg.Severity), Message: arg.Message, Detail: arg.Detail, DedupeKey: arg.DedupeKey, OccurrenceCount: 1}, nil
}

func (s *safeWorkspaceErrorEventStore) ListRecentWorkspaceErrorEvents(context.Context, int64) ([]WorkspaceErrorEvent, error) {
	return nil, nil
}

func (s *safeWorkspaceErrorEventStore) ListRecentWorkspaceErrorEventsForWorkspace(context.Context, ListRecentWorkspaceErrorEventsForWorkspaceParams) ([]WorkspaceErrorEvent, error) {
	return nil, nil
}

func (s *safeWorkspaceErrorEventStore) recordedEvents() []UpsertWorkspaceErrorEventParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]UpsertWorkspaceErrorEventParams(nil), s.events...)
}

func TestWorkspaceErrorScannerRecordsNotableLogLines(t *testing.T) {
	store := &safeWorkspaceErrorEventStore{}
	tailer := &fakeWorkspaceErrorTailer{tails: map[string]string{"web.log": "hello\npanic: boom\nfatal server exit\nrequest failed\nplain error\nworkspace_child_crashed pid=1\nworkspace_error code=1"}}
	scanner := &WorkspaceErrorScanner{Store: store, Tailer: tailer}
	if err := scanner.ScanWorkspace(t.Context(), Workspace{Slug: "feature", LogPath: "web.log"}); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	events := store.recordedEvents()
	if got, want := len(events), 6; got != want {
		t.Fatalf("events = %d, want %d: %#v", got, want, events)
	}
	for _, event := range events {
		if event.WorkspaceSlug != "feature" || event.Source != WorkspaceErrorSourceLog || event.Severity != WorkspaceErrorSeverityError {
			t.Fatalf("unexpected event: %#v", event)
		}
		if event.DedupeKey == "" {
			t.Fatalf("empty dedupe key for event: %#v", event)
		}
	}
}

func TestWorkspaceErrorScannerScansSelectedFirst(t *testing.T) {
	store := &safeWorkspaceErrorEventStore{}
	tailer := &fakeWorkspaceErrorTailer{tails: map[string]string{"selected.log": "panic: selected", "other.log": "panic: other"}}
	manager := &fakeLifecycleManager{workspaces: []Workspace{
		{Slug: "other", LogPath: "other.log"},
		{Slug: "selected", LogPath: "selected.log"},
	}}
	scanner := &WorkspaceErrorScanner{Manager: manager, Store: store, Tailer: tailer}
	if err := scanner.ScanSelectedThenAll(t.Context(), "selected"); err != nil {
		t.Fatalf("ScanSelectedThenAll() error = %v", err)
	}
	paths := tailer.recordedPaths()
	if len(paths) < 2 || paths[0] != "selected.log" || paths[1] != "other.log" {
		t.Fatalf("paths = %#v, want selected first then other", paths)
	}
}

func TestTriggerWorkspaceErrorScanDedupesInFlight(t *testing.T) {
	store := &safeWorkspaceErrorEventStore{}
	tailer := &fakeWorkspaceErrorTailer{
		tails:   map[string]string{"web.log": "panic: boom"},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := &fakeLifecycleManager{workspaces: []Workspace{{Slug: "feature", LogPath: "web.log"}}}
	handler := NewHandler(manager, "https://main.cn-agents.test", "main", WithWorkspaceErrorStore(store), WithWorkspaceErrorScanner(&WorkspaceErrorScanner{Tailer: tailer}))
	handler.triggerWorkspaceErrorScan("feature")
	handler.triggerWorkspaceErrorScan("feature")
	select {
	case <-tailer.started:
	case <-time.After(time.Second):
		t.Fatal("scan did not start")
	}
	if !handler.isWorkspaceErrorScanInFlight("feature") {
		t.Fatal("scan not marked in flight")
	}
	paths := tailer.recordedPaths()
	if got := len(paths); got != 1 {
		t.Fatalf("tail calls while in flight = %d, want 1", got)
	}
	close(tailer.release)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if !handler.isWorkspaceErrorScanInFlight("feature") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan remained in flight after tailer release")
}

func TestNormalizeWorkspaceLogDedupeStableNonEmpty(t *testing.T) {
	first := normalizeWorkspaceLogDedupe(" Feature ", ComponentWeb, "panic:   BOOM")
	second := normalizeWorkspaceLogDedupe("Feature", ComponentWeb, "panic: BOOM")
	if first == "" || second == "" {
		t.Fatal("empty dedupe key")
	}
	if first != second {
		t.Fatalf("dedupe mismatch: %q != %q", first, second)
	}
}
