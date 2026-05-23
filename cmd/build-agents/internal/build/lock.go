package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Lock interface {
	Acquire(ctx context.Context) (ReleaseFunc, error)
}
type ReleaseFunc func() error

type LockMetadata struct {
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	StartedAt time.Time `json:"started_at"`
}

type FileLock struct{ lockPath, metadataPath string }

func NewFileLock(lockPath, metadataPath string) *FileLock {
	return &FileLock{lockPath: lockPath, metadataPath: metadataPath}
}

func (l *FileLock) Acquire(ctx context.Context) (ReleaseFunc, error) {
	if err := os.MkdirAll(filepath.Dir(l.lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir lock dir: %w", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		file, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = file.Close()
			if err := l.writeMetadata(); err != nil {
				_ = os.Remove(l.lockPath)
				return nil, err
			}
			return func() error {
				metaErr := os.Remove(l.metadataPath)
				lockErr := os.Remove(l.lockPath)
				if lockErr != nil && !os.IsNotExist(lockErr) {
					return lockErr
				}
				if metaErr != nil && !os.IsNotExist(metaErr) {
					return metaErr
				}
				return nil
			}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (l *FileLock) writeMetadata() error {
	hostname, _ := os.Hostname()
	metadata := LockMetadata{PID: os.Getpid(), Hostname: hostname, StartedAt: time.Now()}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(l.metadataPath, data, 0o644); err != nil {
		return fmt.Errorf("write lock metadata: %w", err)
	}
	return nil
}
