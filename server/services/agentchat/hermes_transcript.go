package agentchat

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HermesTranscriptEvent is the durable, secret-free record received from Hermes.
type HermesTranscriptEvent struct {
	ID          string          `json:"id"`
	At          time.Time       `json:"at"`
	Type        string          `json:"type"`
	ThreadID    string          `json:"thread_id"`
	Content     string          `json:"content,omitempty"`
	Tool        *HermesToolCard `json:"tool,omitempty"`
	PiSessionID string          `json:"pi_session_id,omitempty"`
}

type HermesToolCard struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

var hermesTranscriptMu sync.Mutex

func HermesTranscriptPath(planDir, threadID string) (string, error) {
	if strings.TrimSpace(planDir) == "" || strings.TrimSpace(threadID) == "" ||
		filepath.Base(threadID) != threadID {
		return "", errors.New("plan directory and safe thread ID are required")
	}
	return filepath.Join(planDir, ".vamos", "sessions", "hermes", threadID+".jsonl"), nil
}

func AppendHermesTranscript(planDir string, event HermesTranscriptEvent) error {
	if event.ID == "" || event.ThreadID == "" || !validHermesEventType(event.Type) {
		return errors.New("invalid Hermes transcript event")
	}
	path, err := hermesTranscriptWritePath(planDir, event.ThreadID)
	if err != nil {
		return err
	}
	// Tool arguments are deliberately absent from the durable format.
	if event.Tool != nil {
		event.Tool = &HermesToolCard{
			Name:   strings.TrimSpace(event.Tool.Name),
			Status: strings.TrimSpace(event.Tool.Status),
		}
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	hermesTranscriptMu.Lock()
	defer hermesTranscriptMu.Unlock()
	if existing, err := os.ReadFile(path); err == nil {
		s := bufio.NewScanner(strings.NewReader(string(existing)))
		for s.Scan() {
			var old HermesTranscriptEvent
			if json.Unmarshal(s.Bytes(), &old) == nil && old.ID == event.ID {
				return nil
			}
		}
		if err := s.Err(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

// hermesTranscriptWritePath creates each canonical transcript directory only
// after checking the preceding component. This avoids MkdirAll following a
// plan-owned symlink before containment has been established.
func hermesTranscriptWritePath(planDir, threadID string) (string, error) {
	path, err := HermesTranscriptPath(planDir, threadID)
	if err != nil {
		return "", err
	}
	dir, err := ensureContainedDirectory(planDir, ".vamos", "sessions", "hermes")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(path)), nil
}

func ensureContainedDirectory(planDir string, parts ...string) (string, error) {
	// AppendHermesTranscript historically creates the plan-local transcript
	// hierarchy on first use. Preserve that behavior before resolving the plan;
	// Service callers have already verified plan containment against thoughts.
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		return "", err
	}
	current, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", fmt.Errorf("resolve plan directory: %w", err)
	}
	for _, part := range parts {
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(next, 0o700); err != nil {
				return "", err
			}
			current = next
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf(
				"Hermes transcript path component %q is not a directory",
				part,
			)
		}
		resolved, err := filepath.EvalSymlinks(next)
		if err != nil {
			return "", err
		}
		if !pathWithinRoot(resolved, current) {
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

// containedResolvedPath rejects any existing symlink chain that escapes planDir.
// name is appended after resolving target when writing a new file; an empty name
// resolves and validates the complete existing read path.
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
	case "user", "lifecycle", "tool", "final", "pi_run":
		return true
	}
	return false
}

func readHermesTranscript(planDir, threadID string) ([]HermesTranscriptEvent, error) {
	path, err := hermesTranscriptReadPath(planDir, threadID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []HermesTranscriptEvent
	s := bufio.NewScanner(f)
	for s.Scan() {
		var e HermesTranscriptEvent
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("read Hermes transcript: %w", err)
		}
		events = append(events, e)
	}
	return events, s.Err()
}
