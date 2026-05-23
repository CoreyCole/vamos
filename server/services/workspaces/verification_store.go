package workspaces

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type MemoryVerifyRunStore struct {
	mu          sync.Mutex
	nextID      int64
	runs        map[string]VerifyWorkspaceRun
	subscribers map[string][]chan VerifyWorkspaceRun
}

func NewMemoryVerifyRunStore() *MemoryVerifyRunStore {
	return &MemoryVerifyRunStore{
		runs:        map[string]VerifyWorkspaceRun{},
		subscribers: map[string][]chan VerifyWorkspaceRun{},
	}
}

func (s *MemoryVerifyRunStore) Create(
	ctx context.Context,
	req VerifyWorkspaceRequest,
) (VerifyWorkspaceRun, error) {
	if err := ctx.Err(); err != nil {
		return VerifyWorkspaceRun{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	run := VerifyWorkspaceRun{
		ID: now.Format(
			"20060102T150405.000000000",
		) + "-" + strconv.FormatInt(
			s.nextID,
			10,
		),
		Slug:      req.Slug,
		Status:    VerifyRunPending,
		StartedAt: now,
	}
	s.runs[run.ID] = run
	return run, nil
}

func (s *MemoryVerifyRunStore) Update(ctx context.Context, run VerifyWorkspaceRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; !ok {
		return fmt.Errorf("unknown verification run %q", run.ID)
	}
	s.runs[run.ID] = run
	for _, ch := range s.subscribers[run.ID] {
		select {
		case ch <- run:
		default:
		}
	}
	return nil
}

func (s *MemoryVerifyRunStore) Get(
	ctx context.Context,
	id string,
) (VerifyWorkspaceRun, error) {
	if err := ctx.Err(); err != nil {
		return VerifyWorkspaceRun{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return VerifyWorkspaceRun{}, fmt.Errorf("unknown verification run %q", id)
	}
	return run, nil
}

func (s *MemoryVerifyRunStore) Subscribe(
	ctx context.Context,
	id string,
) (<-chan VerifyWorkspaceRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := make(chan VerifyWorkspaceRun, 8)
	s.mu.Lock()
	run, ok := s.runs[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("unknown verification run %q", id)
	}
	s.subscribers[id] = append(s.subscribers[id], ch)
	s.mu.Unlock()
	ch <- run
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.subscribers[id]
		for i, sub := range subs {
			if sub == ch {
				s.subscribers[id] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}
