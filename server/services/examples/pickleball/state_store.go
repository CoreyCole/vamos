package pickleball

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type StateStore struct {
	root string
	mu   sync.Mutex
}

func NewStateStore(root string) *StateStore {
	return &StateStore{root: root}
}

func (s *StateStore) Root() string { return s.root }

func (s *StateStore) EnsureSession(ctx context.Context, id, userEmail string) (PickleballSession, error) {
	if err := ctx.Err(); err != nil {
		return PickleballSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, err := s.loadSessionLocked(id); err == nil {
		if userEmail != "" && session.UserEmail == "" {
			session.UserEmail = userEmail
			session.UpdatedAt = time.Now().UTC()
			if err := s.saveSessionLocked(session); err != nil {
				return PickleballSession{}, err
			}
		}
		return session, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PickleballSession{}, err
	}

	now := time.Now().UTC()
	session := PickleballSession{
		ID:            id,
		UserEmail:     userEmail,
		WorkspacePath: filepath.Join(s.sessionDir(id), "workspace"),
		State:         AppStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.saveSessionLocked(session); err != nil {
		return PickleballSession{}, err
	}
	return session, nil
}

func (s *StateStore) LoadSession(ctx context.Context, id string) (PickleballSession, error) {
	if err := ctx.Err(); err != nil {
		return PickleballSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSessionLocked(id)
}

func (s *StateStore) SaveSession(ctx context.Context, session PickleballSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session.UpdatedAt = time.Now().UTC()
	return s.saveSessionLocked(session)
}

func (s *StateStore) SaveSnapshot(ctx context.Context, sessionID string, snapshot BuildSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONAtomic(s.snapshotFile(sessionID, snapshot.BuildID), snapshot)
}

func (s *StateStore) LoadSnapshot(ctx context.Context, sessionID, buildID string) (BuildSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return BuildSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshot BuildSnapshot
	if err := readJSON(s.snapshotFile(sessionID, buildID), &snapshot); err != nil {
		return BuildSnapshot{}, err
	}
	return snapshot, nil
}

func (s *StateStore) ListSnapshots(ctx context.Context, sessionID string) ([]BuildSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.sessionDir(sessionID), "snapshots")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshots []BuildSnapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var snapshot BuildSnapshot
		if err := readJSON(filepath.Join(root, entry.Name(), "snapshot.json"), &snapshot); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})
	return snapshots, nil
}

func (s *StateStore) sessionDir(id string) string {
	return filepath.Join(s.root, "sessions", id)
}

func (s *StateStore) sessionFile(id string) string {
	return filepath.Join(s.sessionDir(id), "current.json")
}

func (s *StateStore) snapshotFile(sessionID, buildID string) string {
	return filepath.Join(s.sessionDir(sessionID), "snapshots", buildID, "snapshot.json")
}

func (s *StateStore) loadSessionLocked(id string) (PickleballSession, error) {
	var session PickleballSession
	if err := readJSON(s.sessionFile(id), &session); err != nil {
		return PickleballSession{}, err
	}
	return session, nil
}

func (s *StateStore) saveSessionLocked(session PickleballSession) error {
	return writeJSONAtomic(s.sessionFile(session.ID), session)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
