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
	"github.com/CoreyCole/vamos/server/services/appletruntime"
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
	AIGenerator     AIGenerator
	AppletEditor    AppletEditor
	AppletRuntime   appletruntime.Manager
	FilesRoot       string
	CurrentAppDir   string
	IterationsDir   string
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
	if opts.FilesRoot == "" {
		opts.FilesRoot = filepath.Join("examples", "pickleball", "files")
	}
	if opts.FilesRoot, err = filepath.Abs(opts.FilesRoot); err != nil {
		return nil, fmt.Errorf("resolve pickleball files root: %w", err)
	}
	if opts.CurrentAppDir == "" {
		opts.CurrentAppDir = filepath.Join(opts.FilesRoot, "apps", "current")
	}
	if opts.CurrentAppDir, err = filepath.Abs(opts.CurrentAppDir); err != nil {
		return nil, fmt.Errorf("resolve current app dir: %w", err)
	}
	if opts.IterationsDir == "" {
		opts.IterationsDir = filepath.Join(opts.FilesRoot, "apps", "iterations")
	}
	if opts.IterationsDir, err = filepath.Abs(opts.IterationsDir); err != nil {
		return nil, fmt.Errorf("resolve iterations dir: %w", err)
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
		CurrentApplet:  session.CurrentIterationID,
		UserMessage:    session.UserMessage,
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
		vm.Share = shareModelForSnapshot(snapshot)
	}
	if session.LastGoodBuildID != "" {
		snapshot, err := s.store.LoadSnapshot(ctx, session.ID, session.LastGoodBuildID)
		if err != nil {
			return PickleballViewModel{}, err
		}
		vm.LastGood = &snapshot
		if vm.Current == nil {
			vm.Share = shareModelForSnapshot(snapshot)
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
	if s.opts.WorkflowStarter == nil {
		return PromptAccepted{}, fmt.Errorf("pickleball self-modify workflow is disabled")
	}
	runID, err := s.opts.WorkflowStarter.StartPickleballSelfModify(ctx, req)
	if err != nil {
		return PromptAccepted{}, err
	}
	session.State = AppStateGenerating
	session.ActiveRunID = runID
	session.UserMessage = "I'm working on that change. Your current app stays available."
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
	session.UserMessage = "Done — I updated the app and files."
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
	session.UserMessage = "I couldn't make that change safely. Your app is unchanged."
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

func (s *Service) ShareModel(ctx context.Context, sessionID string) (ShareModel, error) {
	session, err := s.store.LoadSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return ShareModel{}, err
	}
	buildID := session.CurrentBuildID
	if buildID == "" {
		buildID = session.LastGoodBuildID
	}
	if buildID == "" {
		return ShareModel{}, fmt.Errorf("no build is available to share")
	}
	snapshot, err := s.store.LoadSnapshot(ctx, session.ID, buildID)
	if err != nil {
		return ShareModel{}, err
	}
	return shareModelForSnapshot(snapshot), nil
}

func (s *Service) RestoreSnapshotForAI(ctx context.Context, sessionID, buildID string) error {
	session, err := s.store.LoadSession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	snapshot, err := s.store.LoadSnapshot(ctx, session.ID, strings.TrimSpace(buildID))
	if err != nil {
		return err
	}
	sourceDir := filepath.Join(s.opts.ThoughtsRoot, filepath.FromSlash(cleanRelativePath(snapshot.SnapshotPath)), "source")
	if !pathWithinRoot(sourceDir, s.store.Root()) {
		return fmt.Errorf("snapshot source escapes example root")
	}
	if info, err := os.Stat(sourceDir); err != nil {
		return fmt.Errorf("restore source: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("restore source is not a directory")
	}
	if strings.TrimSpace(session.WorkspacePath) == "" || !pathWithinRoot(session.WorkspacePath, s.store.Root()) {
		return fmt.Errorf("session workspace path is invalid")
	}
	if err := os.RemoveAll(session.WorkspacePath); err != nil {
		return fmt.Errorf("clear workspace: %w", err)
	}
	if err := copyDir(sourceDir, session.WorkspacePath); err != nil {
		return fmt.Errorf("restore workspace: %w", err)
	}
	session.State = AppStateIdle
	session.ActiveRunID = ""
	session.ErrorMessage = ""
	session.LogTail = ""
	if err := s.store.SaveSession(ctx, session); err != nil {
		return err
	}
	s.notify(session.ID)
	return nil
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

func shareModelForSnapshot(snapshot BuildSnapshot) ShareModel {
	return ShareModel{
		PreviewURL:     LatestPreviewURL(snapshot),
		CSVDownloadURL: CSVDownloadURL(snapshot),
	}
}

func LatestPreviewURL(snapshot BuildSnapshot) string {
	return thoughtsURL(snapshot.HTMLThoughtsPath)
}

func CSVDownloadURL(snapshot BuildSnapshot) string {
	return thoughtsURL(snapshot.CSVThoughtsPath)
}

func thoughtsURL(path string) string {
	path = strings.TrimPrefix(cleanRelativePath(path), "thoughts/")
	if path == "." || path == "" {
		return ""
	}
	return "/thoughts/" + path
}

func pathWithinRoot(path, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	return p == r || strings.HasPrefix(p, r+string(os.PathSeparator))
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
