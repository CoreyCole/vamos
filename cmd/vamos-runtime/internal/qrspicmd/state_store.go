package qrspicmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const lockTTL = 12 * time.Hour

func DefaultStateRoot() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return filepath.Join(xdg, "vamos", "q-manager"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "vamos", "q-manager"), nil
}

func CanonicalPlanDir(projectRoot, planDir string) (string, error) {
	if strings.TrimSpace(planDir) == "" {
		return "", errors.New("plan-dir is required")
	}
	if !filepath.IsAbs(planDir) {
		planDir = filepath.Join(projectRoot, planDir)
	}
	abs, err := filepath.Abs(planDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func RepoID(projectRoot string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = "."
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func keyID(key LockKey) string {
	sum := sha256.Sum256([]byte(key.RepoID + "\x00" + key.CanonicalPlanDir))
	return hex.EncodeToString(sum[:])
}

func StatePath(root string, key LockKey, managerRunID string) string {
	return filepath.Join(root, keyID(key), managerRunID+".json")
}

func LockPath(root string, key LockKey) string {
	return filepath.Join(root, keyID(key), "lock.json")
}

type FileStateStore struct {
	Root  string
	Clock func() time.Time
}

func (s FileStateStore) Load(path string) (ManagerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagerState{}, err
	}
	var state ManagerState
	if err := json.Unmarshal(data, &state); err != nil {
		return ManagerState{}, err
	}
	return state, nil
}

func (s FileStateStore) Save(path string, state ManagerState) error {
	return writeJSONAtomically(path, state)
}

func (s FileStateStore) AcquireLock(ctx context.Context, key LockKey, owner string, ttl time.Duration) (Lock, error) {
	select {
	case <-ctx.Done():
		return Lock{}, ctx.Err()
	default:
	}
	if ttl <= 0 {
		ttl = lockTTL
	}
	if strings.TrimSpace(owner) == "" {
		return Lock{}, errors.New("lock owner is required")
	}
	path := LockPath(s.Root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Lock{}, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return Lock{}, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return Lock{}, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	now := s.now()
	data, err := io.ReadAll(file)
	if err != nil {
		return Lock{}, err
	}
	if len(strings.TrimSpace(string(data))) > 0 {
		var existing Lock
		if err := json.Unmarshal(data, &existing); err != nil {
			return Lock{}, err
		}
		if existing.ExpiresAt > now.Unix() && existing.Owner != owner {
			return Lock{}, LockConflictError{Existing: existing}
		}
	}
	lock := Lock{Key: key, Owner: owner, Path: path, ExpiresAt: now.Add(ttl).Unix()}
	if err := writeLockedJSON(file, lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func (s FileStateStore) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

type LockConflictError struct{ Existing Lock }

func (e LockConflictError) Error() string {
	return fmt.Sprintf("q-manager lock held by %s", e.Existing.Owner)
}

func writeJSONAtomically(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func writeLockedJSON(file *os.File, v any) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return file.Sync()
}
