package agentchat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/CoreyCole/vamos/pkg/safecomponent"
)

var ErrHermesPromptInProgress = errors.New("Hermes prompt command is in progress")

type hermesCommandLockEntry struct {
	mu   sync.Mutex
	refs int
}

type hermesCommandLockRegistry struct {
	mu      sync.Mutex
	entries map[string]*hermesCommandLockEntry
}

var commandLocks = hermesCommandLockRegistry{entries: make(map[string]*hermesCommandLockEntry)}

var (
	hermesCommandBeforeKernelLockHook func(*sync.Mutex)
	hermesCommandLockAcquiredHook     func(string, string, string)
)

type hermesCommandLock struct {
	registry *hermesCommandLockRegistry
	key      string
	entry    *hermesCommandLockEntry
	file     *os.File
	once     sync.Once
}

func tryAcquireHermesCommandLock(
	ctx context.Context,
	planDir, threadID, commandID string,
) (*hermesCommandLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := safecomponent.ValidateBounded(threadID); err != nil {
		return nil, err
	}
	if err := safecomponent.ValidateBounded(commandID); err != nil {
		return nil, err
	}
	resolvedPlan, err := resolveExistingDirectory(planDir)
	if err != nil {
		return nil, err
	}
	lockDir, err := ensureContainedDirectory(
		resolvedPlan,
		".vamos", "sessions", "hermes", ".locks", "commands", threadID,
	)
	if err != nil {
		return nil, err
	}
	key := resolvedPlan + "\x00" + threadID + "\x00" + commandID
	entry := commandLocks.retain(key)
	if !entry.mu.TryLock() {
		commandLocks.releaseReference(key, entry)
		return nil, ErrHermesPromptInProgress
	}
	if hermesCommandBeforeKernelLockHook != nil {
		hermesCommandBeforeKernelLockHook(&entry.mu)
	}
	file, err := os.OpenFile(filepath.Join(lockDir, commandID+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		entry.mu.Unlock()
		commandLocks.releaseReference(key, entry)
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		entry.mu.Unlock()
		commandLocks.releaseReference(key, entry)
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrHermesPromptInProgress
		}
		return nil, err
	}
	if hermesCommandLockAcquiredHook != nil {
		hermesCommandLockAcquiredHook(resolvedPlan, threadID, commandID)
	}
	return &hermesCommandLock{
		registry: &commandLocks,
		key:      key,
		entry:    entry,
		file:     file,
	}, nil
}

func (r *hermesCommandLockRegistry) retain(key string) *hermesCommandLockEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[key]
	if entry == nil {
		entry = &hermesCommandLockEntry{}
		r.entries[key] = entry
	}
	entry.refs++
	return entry
}

func (r *hermesCommandLockRegistry) releaseReference(key string, entry *hermesCommandLockEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && r.entries[key] == entry {
		delete(r.entries, key)
	}
}

func (lock *hermesCommandLock) Close() error {
	var result error
	lock.once.Do(func() {
		if err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN); err != nil {
			result = err
		}
		if err := lock.file.Close(); result == nil && err != nil {
			result = err
		}
		lock.entry.mu.Unlock()
		lock.registry.releaseReference(lock.key, lock.entry)
	})
	return result
}
