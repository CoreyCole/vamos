package pickleball

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/generatedgo"
)

const (
	defaultExampleRoot = "creative-mode-agent/examples/pickleball"
	defaultSeedDir     = "examples/pickleball/seed-bundle"
	maxLogTailBytes    = 8 * 1024
)

type Options struct {
	ThoughtsRoot    string
	ExampleRoot     string
	SeedBundleDir   string
	Runner          Runner
	WorkflowStarter WorkflowStarter
	Notifier        Notifier
}

type Runner interface {
	BuildAndRun(context.Context, generatedgo.RunnerInput) (generatedgo.RunnerResult, error)
}

type WorkflowStarter interface {
	StartPickleballSelfModify(context.Context, PromptRequest) (string, error)
}

type Notifier interface {
	NotifyPickleballSession(sessionID string)
}

type Service struct {
	store  *StateStore
	opts   Options
	runner Runner

	subscribersMu sync.Mutex
	subscribers   map[string]map[chan struct{}]struct{}
}

func NewService(opts Options) (*Service, error) {
	if strings.TrimSpace(opts.ThoughtsRoot) == "" {
		return nil, fmt.Errorf("thoughts root is required")
	}
	thoughtsRoot, err := filepath.Abs(opts.ThoughtsRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve thoughts root: %w", err)
	}
	opts.ThoughtsRoot = thoughtsRoot
	if strings.TrimSpace(opts.ExampleRoot) == "" {
		opts.ExampleRoot = defaultExampleRoot
	}
	opts.ExampleRoot = cleanRelativePath(opts.ExampleRoot)
	if strings.TrimSpace(opts.SeedBundleDir) == "" {
		opts.SeedBundleDir = defaultSeedDir
	}
	root := filepath.Join(opts.ThoughtsRoot, filepath.FromSlash(opts.ExampleRoot))
	return &Service{
		store:       NewStateStore(root),
		opts:        opts,
		runner:      opts.Runner,
		subscribers: make(map[string]map[chan struct{}]struct{}),
	}, nil
}

func (s *Service) EnsureSession(ctx context.Context, userEmail string) (PickleballSession, error) {
	id := sessionIDForUser(userEmail)
	session, err := s.store.EnsureSession(ctx, id, strings.TrimSpace(userEmail))
	if err != nil {
		return PickleballSession{}, err
	}
	if err := SeedOrUpdateGeneratedWorkspace(ctx, session, s.opts.SeedBundleDir); err != nil {
		return PickleballSession{}, err
	}
	return session, nil
}

func (s *Service) GetState(ctx context.Context, sessionID string) (PickleballViewModel, error) {
	session, err := s.store.LoadSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return PickleballViewModel{}, err
	}
	vm := PickleballViewModel{
		SessionID:      session.ID,
		State:          session.State,
		ErrorMessage:   session.ErrorMessage,
		LogTail:        session.LogTail,
		PromptExamples: promptExamples(),
	}
	if session.CurrentBuildID != "" {
		snapshot, err := s.store.LoadSnapshot(ctx, session.ID, session.CurrentBuildID)
		if err != nil {
			return PickleballViewModel{}, err
		}
		vm.Current = &snapshot
		vm.Share = shareModel(snapshot)
	}
	if session.LastGoodBuildID != "" {
		snapshot, err := s.store.LoadSnapshot(ctx, session.ID, session.LastGoodBuildID)
		if err != nil {
			return PickleballViewModel{}, err
		}
		vm.LastGood = &snapshot
		if vm.Current == nil {
			vm.Share = shareModel(snapshot)
		}
	}
	return vm, nil
}

func (s *Service) SubmitPrompt(ctx context.Context, req PromptRequest) (PromptAccepted, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return PromptAccepted{}, fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(req.SessionID) == "" {
		session, err := s.EnsureSession(ctx, req.UserEmail)
		if err != nil {
			return PromptAccepted{}, err
		}
		req.SessionID = session.ID
	}
	session, err := s.store.LoadSession(ctx, req.SessionID)
	if err != nil {
		return PromptAccepted{}, err
	}
	if isActive(session.State) && session.ActiveRunID != "" {
		return PromptAccepted{SessionID: session.ID, RunID: session.ActiveRunID, State: session.State}, nil
	}
	runID := ""
	if s.opts.WorkflowStarter != nil {
		runID, err = s.opts.WorkflowStarter.StartPickleballSelfModify(ctx, req)
		if err != nil {
			return PromptAccepted{}, err
		}
	}
	session.State = AppStateGenerating
	session.ActiveRunID = runID
	session.ErrorMessage = ""
	session.LogTail = ""
	if err := s.store.SaveSession(ctx, session); err != nil {
		return PromptAccepted{}, err
	}
	s.notify(session.ID)
	return PromptAccepted{SessionID: session.ID, RunID: runID, State: session.State}, nil
}

func (s *Service) PromoteSnapshot(ctx context.Context, sessionID string, snapshot BuildSnapshot) error {
	if snapshot.BuildID == "" {
		return fmt.Errorf("snapshot build_id is required")
	}
	session, err := s.store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	if err := s.store.SaveSnapshot(ctx, session.ID, snapshot); err != nil {
		return err
	}
	session.CurrentBuildID = snapshot.BuildID
	session.LastGoodBuildID = snapshot.BuildID
	session.State = AppStateSucceeded
	session.ActiveRunID = ""
	session.ErrorMessage = ""
	session.LogTail = ""
	if err := s.store.SaveSession(ctx, session); err != nil {
		return err
	}
	s.notify(session.ID)
	return nil
}

func (s *Service) MarkFailed(ctx context.Context, sessionID string, cause error, logTail string) error {
	session, err := s.store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	session.State = AppStateFailed
	session.ActiveRunID = ""
	if cause != nil {
		session.ErrorMessage = cause.Error()
	}
	session.LogTail = tailString(logTail, maxLogTailBytes)
	if err := s.store.SaveSession(ctx, session); err != nil {
		return err
	}
	s.notify(session.ID)
	return nil
}

func (s *Service) SnapshotHistoryForPrompt(ctx context.Context, sessionID string) ([]BuildSnapshot, error) {
	return s.store.ListSnapshots(ctx, sessionID)
}

func (s *Service) notify(sessionID string) {
	if s.opts.Notifier != nil {
		s.opts.Notifier.NotifyPickleballSession(sessionID)
	}
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	for ch := range s.subscribers[sessionID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Service) subscribe(sessionID string) chan struct{} {
	ch := make(chan struct{}, 1)
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	if s.subscribers[sessionID] == nil {
		s.subscribers[sessionID] = make(map[chan struct{}]struct{})
	}
	s.subscribers[sessionID][ch] = struct{}{}
	return ch
}

func (s *Service) unsubscribe(sessionID string, ch chan struct{}) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	delete(s.subscribers[sessionID], ch)
	if len(s.subscribers[sessionID]) == 0 {
		delete(s.subscribers, sessionID)
	}
	close(ch)
}

func isActive(state AppState) bool {
	return state == AppStateGenerating || state == AppStateBuilding
}

func promptExamples() []string {
	return []string{
		"Prioritize new partner pairings over skill balance.",
		"Make the preview more colorful and mobile-friendly.",
		"Add a CSV column explaining skill totals.",
	}
}

func shareModel(snapshot BuildSnapshot) ShareModel {
	return ShareModel{
		PreviewURL:     thoughtsURL(snapshot.HTMLThoughtsPath),
		CSVDownloadURL: thoughtsURL(snapshot.CSVThoughtsPath),
	}
}

func thoughtsURL(path string) string {
	path = strings.TrimPrefix(cleanRelativePath(path), "thoughts/")
	if path == "." || path == "" {
		return ""
	}
	return "/thoughts/" + path
}

func sessionIDForUser(userEmail string) string {
	value := strings.ToLower(strings.TrimSpace(userEmail))
	if value == "" {
		return "default"
	}
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}
	return value
}

func cleanRelativePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "../")
	if path == "." {
		return ""
	}
	return path
}

func tailString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func copyDirIfMissing(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	return filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("seed bundle symlink not allowed: %s", path)
		}
		if d.Type().IsRegular() {
			return copyFile(path, target)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
