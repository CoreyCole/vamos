package agentchat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/CoreyCole/vamos/pkg/safecomponent"
)

const maxHermesTranscriptRecordBytes = 1 << 20

const hermesTranscriptScannerCapacity = maxHermesTranscriptRecordBytes + 2

type HermesTranscriptEvent struct {
	ID              string                 `json:"id"`
	At              time.Time              `json:"at"`
	Type            string                 `json:"type"`
	ThreadID        string                 `json:"thread_id"`
	PlanDir         HermesPlanIdentity     `json:"plan_dir"`
	CreatorEmail    string                 `json:"creator_email,omitempty"`
	PromptAuthority *HermesPromptAuthority `json:"prompt_authority,omitempty"`
	Title           string                 `json:"title,omitempty"`
	Content         string                 `json:"content,omitempty"`
	Tool            *HermesToolCard        `json:"tool,omitempty"`
	PiSessionID     string                 `json:"pi_session_id,omitempty"`
	CommandID       string                 `json:"command_id,omitempty"`
	CommandDigest   string                 `json:"command_digest,omitempty"`
	ContextPaths    []string               `json:"context_paths,omitempty"`
	DeliveryStatus  string                 `json:"delivery_status,omitempty"`
	DeliveryReason  string                 `json:"delivery_reason,omitempty"`
}

type HermesToolCard struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

type hermesTranscriptLockEntry struct {
	mu   sync.RWMutex
	refs int
}

type hermesTranscriptLockRegistry struct {
	mu      sync.Mutex
	entries map[string]*hermesTranscriptLockEntry
}

var transcriptLocks = hermesTranscriptLockRegistry{
	entries: make(map[string]*hermesTranscriptLockEntry),
}

var (
	hermesTranscriptBeforeKernelLockHook func(*sync.RWMutex)
	hermesTranscriptLocalContentionHook  func(bool)
	hermesTranscriptLockFileOpenedHook   func(int)
	hermesTranscriptKernelContentionHook func()
	hermesTranscriptLockAcquiredHook     func(string, string, bool)
	writeHermesTranscriptPayload         = func(file *os.File, payload []byte) (int, error) {
		return file.Write(payload)
	}
)

type hermesTranscriptLock struct {
	registry *hermesTranscriptLockRegistry
	key      string
	entry    *hermesTranscriptLockEntry
	file     *os.File
	shared   bool
	once     sync.Once
}

func HermesTranscriptPath(planDir, threadID string) (string, error) {
	if strings.TrimSpace(planDir) == "" {
		return "", errors.New("plan directory is required")
	}
	if err := safecomponent.ValidateBounded(threadID); err != nil {
		return "", err
	}
	return filepath.Join(planDir, ".vamos", "sessions", "hermes", threadID+".jsonl"), nil
}

func acquireHermesTranscriptLock(
	ctx context.Context, planDir, threadID string, shared bool,
) (*hermesTranscriptLock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safecomponent.ValidateBounded(threadID); err != nil {
		return nil, err
	}
	resolvedPlan, err := resolveExistingDirectory(planDir)
	if err != nil {
		return nil, err
	}
	lockDir, err := ensureContainedDirectory(resolvedPlan, ".vamos", "sessions", "hermes", ".locks")
	if err != nil {
		return nil, err
	}
	key := resolvedPlan + "\x00" + threadID
	entry := transcriptLocks.retain(key)
	locked := false
	defer func() {
		if !locked {
			transcriptLocks.releaseReference(key, entry)
		}
	}()
	if err := acquireLocalTranscriptLock(ctx, &entry.mu, shared); err != nil {
		return nil, err
	}
	locked = true
	if hermesTranscriptBeforeKernelLockHook != nil {
		hermesTranscriptBeforeKernelLockHook(&entry.mu)
	}
	lockPath := filepath.Join(lockDir, threadID+".lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		releaseLocalTranscriptLock(&entry.mu, shared)
		locked = false
		return nil, err
	}
	if hermesTranscriptLockFileOpenedHook != nil {
		hermesTranscriptLockFileOpenedHook(int(file.Fd()))
	}
	operation := unix.LOCK_EX
	if shared {
		operation = unix.LOCK_SH
	}
	for {
		err = unix.Flock(int(file.Fd()), operation|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			_ = file.Close()
			releaseLocalTranscriptLock(&entry.mu, shared)
			locked = false
			return nil, err
		}
		if hermesTranscriptKernelContentionHook != nil {
			hermesTranscriptKernelContentionHook()
		}
		if err := waitForHermesLock(ctx); err != nil {
			_ = file.Close()
			releaseLocalTranscriptLock(&entry.mu, shared)
			locked = false
			return nil, err
		}
	}
	if hermesTranscriptLockAcquiredHook != nil {
		hermesTranscriptLockAcquiredHook(resolvedPlan, threadID, shared)
	}
	return &hermesTranscriptLock{
		registry: &transcriptLocks,
		key:      key,
		entry:    entry,
		file:     file,
		shared:   shared,
	}, nil
}

func (r *hermesTranscriptLockRegistry) retain(key string) *hermesTranscriptLockEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[key]
	if entry == nil {
		entry = &hermesTranscriptLockEntry{}
		r.entries[key] = entry
	}
	entry.refs++
	return entry
}

func (r *hermesTranscriptLockRegistry) releaseReference(
	key string, entry *hermesTranscriptLockEntry,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && r.entries[key] == entry {
		delete(r.entries, key)
	}
}

func acquireLocalTranscriptLock(ctx context.Context, lock *sync.RWMutex, shared bool) error {
	for {
		var acquired bool
		if shared {
			acquired = lock.TryRLock()
		} else {
			acquired = lock.TryLock()
		}
		if acquired {
			return nil
		}
		if hermesTranscriptLocalContentionHook != nil {
			hermesTranscriptLocalContentionHook(shared)
		}
		if err := waitForHermesLock(ctx); err != nil {
			return err
		}
	}
}

func waitForHermesLock(ctx context.Context) error {
	timer := time.NewTimer(5 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func releaseLocalTranscriptLock(lock *sync.RWMutex, shared bool) {
	if shared {
		lock.RUnlock()
		return
	}
	lock.Unlock()
}

func (lock *hermesTranscriptLock) Close() error {
	var result error
	lock.once.Do(func() {
		if err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN); err != nil {
			result = err
		}
		if err := lock.file.Close(); result == nil && err != nil {
			result = err
		}
		releaseLocalTranscriptLock(&lock.entry.mu, lock.shared)
		lock.registry.releaseReference(lock.key, lock.entry)
	})
	return result
}

func AppendHermesTranscript(planDir string, event HermesTranscriptEvent) error {
	return appendHermesTranscript(context.Background(), planDir, event)
}

func appendHermesTranscript(
	ctx context.Context, planDir string, event HermesTranscriptEvent,
) error {
	if err := validateHermesEventShape(event); err != nil {
		return err
	}
	path, err := hermesTranscriptWritePath(planDir, event.ThreadID)
	if err != nil {
		return err
	}
	if event.Tool != nil {
		event.Tool = &HermesToolCard{
			Name: strings.TrimSpace(event.Tool.Name), Status: strings.TrimSpace(event.Tool.Status),
		}
	}
	lock, err := acquireHermesTranscriptLock(ctx, planDir, event.ThreadID, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	return appendHermesTranscriptUnlocked(path, event)
}

func appendHermesTranscriptUnlocked(path string, event HermesTranscriptEvent) error {
	if err := validateHermesEventShape(event); err != nil {
		return err
	}
	existing, err := readHermesTranscriptFile(path, event.PlanDir, event.ThreadID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(existing.events) == 0 && event.Type != "thread_metadata" {
		return errors.New("Hermes transcript metadata must be first")
	}
	if old, ok := existing.canonicalByID[event.ID]; ok {
		candidate := event
		if candidate.At.IsZero() {
			var persisted HermesTranscriptEvent
			if err := json.Unmarshal(old, &persisted); err != nil {
				return err
			}
			candidate.At = persisted.At
		}
		data, err := json.Marshal(candidate)
		if err != nil {
			return err
		}
		if bytes.Equal(old, data) {
			return nil
		}
		if event.Type == "thread_metadata" {
			return errors.New("Hermes transcript metadata is immutable")
		}
		return errors.New("Hermes transcript event ID conflicts")
	}
	if len(existing.events) > 0 && event.Type == "thread_metadata" {
		return errors.New("Hermes transcript metadata is immutable")
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if len(data) > maxHermesTranscriptRecordBytes {
		return errors.New("Hermes transcript record exceeds semantic limit")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	preAppendLength := info.Size()
	payload := append(append([]byte(nil), data...), '\n')
	written, writeErr := writeHermesTranscriptPayload(file, payload)
	if writeErr == nil && written != len(payload) {
		writeErr = ioErrShortWrite
	}
	if writeErr != nil {
		rollbackErr := file.Truncate(preAppendLength)
		if rollbackErr == nil {
			rollbackErr = file.Sync()
		}
		return errors.Join(writeErr, rollbackErr)
	}
	return file.Sync()
}

var ioErrShortWrite = errors.New("short Hermes transcript write")

func validateHermesEventShape(event HermesTranscriptEvent) error {
	if err := safecomponent.ValidateBounded(event.ID); err != nil {
		return err
	}
	if err := safecomponent.ValidateBounded(event.ThreadID); err != nil {
		return err
	}
	if err := ValidateHermesPlanIdentity(event.PlanDir); err != nil {
		return err
	}
	if !validHermesEventType(event.Type) {
		return errors.New("invalid Hermes transcript event type")
	}
	if event.Type == "thread_metadata" {
		if event.PromptAuthority == nil || event.PromptAuthority.PrincipalType == "" ||
			event.PromptAuthority.PrincipalValue == "" || event.CreatorEmail == "" {
			return errors.New("Hermes thread metadata is incomplete")
		}
	} else if event.PromptAuthority != nil || event.CreatorEmail != "" || event.Title != "" {
		return errors.New("Hermes thread metadata fields are header-only")
	}
	return nil
}

func hermesTranscriptWritePath(planDir, threadID string) (string, error) {
	path, err := HermesTranscriptPath(planDir, threadID)
	if err != nil {
		return "", err
	}
	dir, err := ensureContainedDirectory(planDir, ".vamos", "sessions", "hermes")
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, filepath.Base(path))
	if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Hermes transcript file may not be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return candidate, nil
}

func ensureContainedDirectory(planDir string, parts ...string) (string, error) {
	current, err := resolveExistingDirectory(planDir)
	if err != nil {
		return "", err
	}
	planRoot := current
	for _, part := range parts {
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(next, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
		} else if err != nil {
			return "", err
		} else if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("Hermes transcript component %q is not a directory", part)
		}
		resolved, err := filepath.EvalSymlinks(next)
		if err != nil {
			return "", err
		}
		if !pathWithinRoot(resolved, planRoot) {
			return "", errors.New("Hermes transcript path escapes plan directory")
		}
		current = resolved
	}
	return current, nil
}

func hermesTranscriptReadPath(planDir, threadID string) (string, error) {
	path, err := HermesTranscriptPath(planDir, threadID)
	if err != nil {
		return "", err
	}
	return containedResolvedPath(planDir, path, "")
}

func containedResolvedPath(planDir, target, name string) (string, error) {
	resolvedPlan, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && name == "" {
			return target, nil
		}
		return "", err
	}
	if !pathWithinRoot(resolvedTarget, resolvedPlan) {
		return "", errors.New("Hermes transcript path escapes plan directory")
	}
	if name != "" {
		return filepath.Join(resolvedTarget, name), nil
	}
	return resolvedTarget, nil
}

func validHermesEventType(kind string) bool {
	switch kind {
	case "thread_metadata", "user", "lifecycle", "tool", "final", "pi_run",
		"prompt_requested", "prompt_delivery_started", "prompt_delivery", "settlement_delivering":
		return true
	}
	return false
}

type parsedHermesTranscript struct {
	events        []HermesTranscriptEvent
	canonicalByID map[string][]byte
}

func readHermesTranscript(planDir, threadID string) ([]HermesTranscriptEvent, error) {
	return readHermesTranscriptContext(context.Background(), planDir, "", threadID)
}

func readHermesTranscriptContext(
	ctx context.Context, planDir string, expectedPlan HermesPlanIdentity, threadID string,
) ([]HermesTranscriptEvent, error) {
	lock, err := acquireHermesTranscriptLock(ctx, planDir, threadID, true)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	path, err := hermesTranscriptReadPath(planDir, threadID)
	if err != nil {
		return nil, err
	}
	parsed, err := readHermesTranscriptFile(path, expectedPlan, threadID)
	return parsed.events, err
}

func readHermesTranscriptFile(
	path string, expectedPlan HermesPlanIdentity, threadID string,
) (parsedHermesTranscript, error) {
	file, err := os.Open(path)
	if err != nil {
		return parsedHermesTranscript{}, err
	}
	defer file.Close()
	result := parsedHermesTranscript{canonicalByID: make(map[string][]byte)}
	scanner := newHermesTranscriptScanner(file)
	for scanner.Scan() {
		token := append([]byte(nil), scanner.Bytes()...)
		if len(token) > maxHermesTranscriptRecordBytes {
			return parsedHermesTranscript{}, errors.New("Hermes transcript record exceeds semantic limit")
		}
		var event HermesTranscriptEvent
		if err := json.Unmarshal(token, &event); err != nil {
			return parsedHermesTranscript{}, fmt.Errorf("read Hermes transcript: %w", err)
		}
		canonical, err := json.Marshal(event)
		if err != nil {
			return parsedHermesTranscript{}, err
		}
		if !bytes.Equal(token, canonical) {
			return parsedHermesTranscript{}, errors.New("Hermes transcript record is not canonical JSON")
		}
		if err := validateHermesEventShape(event); err != nil {
			return parsedHermesTranscript{}, err
		}
		if len(result.events) == 0 && event.Type != "thread_metadata" {
			return parsedHermesTranscript{}, errors.New("Hermes transcript metadata must be first")
		}
		if len(result.events) > 0 && event.Type == "thread_metadata" {
			return parsedHermesTranscript{}, errors.New("conflicting Hermes transcript metadata")
		}
		if event.ThreadID != threadID {
			return parsedHermesTranscript{}, errors.New("Hermes transcript filename and event thread differ")
		}
		if len(result.events) > 0 && event.PlanDir != result.events[0].PlanDir {
			return parsedHermesTranscript{}, errors.New("Hermes transcript event plan differs from metadata")
		}
		if expectedPlan != "" && event.PlanDir != expectedPlan {
			return parsedHermesTranscript{}, errors.New("Hermes transcript plan differs from requested plan")
		}
		if old, exists := result.canonicalByID[event.ID]; exists {
			if !bytes.Equal(old, canonical) {
				return parsedHermesTranscript{}, errors.New("Hermes transcript event ID conflicts")
			}
			continue
		}
		result.canonicalByID[event.ID] = canonical
		result.events = append(result.events, event)
	}
	if err := scanner.Err(); err != nil {
		return parsedHermesTranscript{}, fmt.Errorf("read Hermes transcript framing: %w", err)
	}
	if len(result.events) == 0 {
		return parsedHermesTranscript{}, errors.New("empty Hermes transcript")
	}
	return result, nil
}

func newHermesTranscriptScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, hermesTranscriptScannerCapacity), hermesTranscriptScannerCapacity)
	return scanner
}
