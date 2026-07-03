package appletruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHealthPath = "/healthz"
	startTimeout      = 10 * time.Second
	healthPollEvery   = 50 * time.Millisecond
)

type activeProcess struct {
	state      AppletProcessState
	cmd        *exec.Cmd
	log        *os.File
	healthPath string
	ready      chan struct{}
	done       chan struct{}
	once       sync.Once
	readyOnce  sync.Once
	waitErr    error
	startErr   error
}

// ManagerImpl supervises local generated applet processes.
type ManagerImpl struct {
	mu     sync.Mutex
	logDir string
	active map[string]*activeProcess
}

func NewManager(logDir string) *ManagerImpl {
	return &ManagerImpl{logDir: logDir, active: make(map[string]*activeProcess)}
}

func (m *ManagerImpl) EnsureStarted(ctx context.Context, cfg RuntimeConfig) (AppletProcessState, error) {
	appID := strings.TrimSpace(cfg.AppID)
	m.mu.Lock()
	proc := m.active[appID]
	m.mu.Unlock()
	if proc != nil {
		return waitForProcessReady(ctx, proc)
	}
	return m.Start(ctx, cfg)
}

func (m *ManagerImpl) Start(ctx context.Context, cfg RuntimeConfig) (ProcessState, error) {
	if err := validateConfig(cfg); err != nil {
		return ProcessState{}, err
	}
	if cfg.HealthPath == "" {
		cfg.HealthPath = defaultHealthPath
	}
	appID := strings.TrimSpace(cfg.AppID)

	m.mu.Lock()
	previous := m.active[appID]
	candidate := &activeProcess{state: AppletProcessState{
		AppID:       appID,
		SourceDir:   cfg.SourceDir,
		Status:      ProcessStatusStarting,
		Healthy:     false,
		LastSeenAt:  time.Now(),
		IdleTimeout: cfg.IdleTimeout,
	}, healthPath: cfg.HealthPath, ready: make(chan struct{}), done: make(chan struct{})}
	reserved := previous == nil
	if previous != nil && previous.state.Status == ProcessStatusStarting {
		m.mu.Unlock()
		return waitForProcessReady(ctx, previous)
	}
	if reserved {
		m.active[appID] = candidate
	}
	m.mu.Unlock()

	failCandidate := func(stage string, err error) (ProcessState, error) {
		wrapped := fmt.Errorf("%s applet %q: %w", stage, appID, err)
		candidate.state.Status = ProcessStatusUnhealthy
		candidate.state.Healthy = false
		candidate.finishStart(wrapped)
		if reserved {
			m.mu.Lock()
			if m.active[appID] == candidate {
				delete(m.active, appID)
			}
			m.mu.Unlock()
		}
		_ = stopProcess(candidate)
		return ProcessState{}, wrapped
	}

	port, err := allocateLocalPort()
	if err != nil {
		return failCandidate("allocate port for", err)
	}
	env := appletEnv(cfg, port)
	logPath := m.logPath(appID)

	if len(cfg.BuildCommand) > 0 {
		if err := runCommand(ctx, cfg.SourceDir, cfg.BuildCommand, env, nil); err != nil {
			return failCandidate("build", err)
		}
	}
	if err := candidate.startCanceled(); err != nil {
		return ProcessState{}, err
	}

	logFile, err := openLog(logPath)
	if err != nil {
		return failCandidate("open log for", err)
	}
	if err := candidate.startCanceled(); err != nil {
		_ = logFile.Close()
		return ProcessState{}, err
	}
	cmd := exec.Command(cfg.StartCommand[0], cfg.StartCommand[1:]...)
	cmd.Dir = cfg.SourceDir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return failCandidate("start", err)
	}

	candidate.cmd = cmd
	candidate.log = logFile
	candidate.state.Port = port
	candidate.state.PID = cmd.Process.Pid
	candidate.state.BaseURL = "http://127.0.0.1:" + strconv.Itoa(port)
	candidate.state.LogPath = logPath
	candidate.state.LastSeenAt = time.Now()
	go m.reap(appID, candidate)
	if err := candidate.startCanceled(); err != nil {
		_ = stopProcess(candidate)
		return ProcessState{}, err
	}

	if err := waitHealthy(ctx, candidate.state.BaseURL+cfg.HealthPath); err != nil {
		return failCandidate("health check", err)
	}
	candidate.state.Status = ProcessStatusHealthy
	candidate.state.Healthy = true
	candidate.state.LastSeenAt = time.Now()
	candidate.finishStart(nil)

	m.mu.Lock()
	if reserved {
		if m.active[appID] == candidate {
			m.active[appID] = candidate
		}
	} else if m.active[appID] == previous {
		m.active[appID] = candidate
	}
	m.mu.Unlock()
	if previous != nil && previous != candidate {
		stopProcess(previous)
	}
	return candidate.state, nil
}

func (m *ManagerImpl) Stop(_ context.Context, appID string) error {
	appID = strings.TrimSpace(appID)
	m.mu.Lock()
	proc := m.active[appID]
	delete(m.active, appID)
	m.mu.Unlock()
	if proc == nil {
		return nil
	}
	return stopProcess(proc)
}

func (m *ManagerImpl) Health(ctx context.Context, appID string) (AppletProcessState, error) {
	m.mu.Lock()
	proc := m.active[strings.TrimSpace(appID)]
	if proc == nil {
		m.mu.Unlock()
		return AppletProcessState{}, fmt.Errorf("unknown applet %q", appID)
	}
	if proc.state.Status == ProcessStatusStarting {
		state := proc.state
		m.mu.Unlock()
		return state, nil
	}
	m.mu.Unlock()
	if err := waitHealthy(ctx, proc.state.BaseURL+proc.healthPath); err != nil {
		m.mu.Lock()
		if m.active[strings.TrimSpace(appID)] == proc {
			proc.state.Status = ProcessStatusUnhealthy
			proc.state.Healthy = false
			state := proc.state
			m.mu.Unlock()
			return state, err
		}
		m.mu.Unlock()
		state := proc.state
		state.Status = ProcessStatusUnhealthy
		state.Healthy = false
		return state, err
	}
	m.mu.Lock()
	if m.active[strings.TrimSpace(appID)] == proc {
		proc.state.Status = ProcessStatusHealthy
		proc.state.Healthy = true
		proc.state.LastSeenAt = time.Now()
		state := proc.state
		m.mu.Unlock()
		return state, nil
	}
	m.mu.Unlock()
	state := proc.state
	state.Status = ProcessStatusHealthy
	state.Healthy = true
	return state, nil
}

func (m *ManagerImpl) ProxyTarget(appID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc := m.active[strings.TrimSpace(appID)]
	if proc == nil || proc.state.Status != ProcessStatusHealthy || proc.state.BaseURL == "" {
		return "", false
	}
	return proc.state.BaseURL, true
}

func (m *ManagerImpl) Touch(appID string, activeDelta int) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	proc := m.active[appID]
	if proc == nil {
		return
	}
	proc.state.LastSeenAt = time.Now()
	proc.state.ActiveConnections += activeDelta
	if proc.state.ActiveConnections < 0 {
		proc.state.ActiveConnections = 0
	}
}

func (m *ManagerImpl) SweepInactive(ctx context.Context, now time.Time) ([]AppletProcessState, error) {
	m.mu.Lock()
	var idle []*activeProcess
	for appID, proc := range m.active {
		state := proc.state
		if state.IdleTimeout <= 0 || state.ActiveConnections > 0 || state.Status != ProcessStatusHealthy {
			continue
		}
		if now.Sub(state.LastSeenAt) >= state.IdleTimeout {
			delete(m.active, appID)
			idle = append(idle, proc)
		}
	}
	m.mu.Unlock()

	stopped := make([]AppletProcessState, 0, len(idle))
	var errs []error
	for _, proc := range idle {
		state := proc.state
		state.Status = ProcessStatusStopped
		state.Healthy = false
		stopped = append(stopped, state)
		if err := stopProcess(proc); err != nil {
			errs = append(errs, err)
		}
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return stopped, errors.Join(errs...)
}

func (m *ManagerImpl) reap(appID string, proc *activeProcess) {
	err := waitProcess(proc)
	m.mu.Lock()
	if m.active[appID] == proc && err != nil {
		delete(m.active, appID)
	}
	m.mu.Unlock()
	if err == nil && proc.log != nil {
		_ = proc.log.Close()
	}
}

func validateConfig(cfg RuntimeConfig) error {
	if strings.TrimSpace(cfg.AppID) == "" {
		return errors.New("app id is required")
	}
	if len(cfg.StartCommand) == 0 || strings.TrimSpace(cfg.StartCommand[0]) == "" {
		return errors.New("start command is required")
	}
	filesRoot, err := filepath.Abs(filepath.Clean(cfg.FilesRoot))
	if err != nil || filesRoot == "" {
		return fmt.Errorf("files root is required")
	}
	sourceDir, err := filepath.Abs(filepath.Clean(cfg.SourceDir))
	if err != nil || sourceDir == "" {
		return fmt.Errorf("source dir is required")
	}
	if err := os.MkdirAll(filesRoot, 0o755); err != nil {
		return fmt.Errorf("files root: %w", err)
	}
	if info, err := os.Stat(filesRoot); err != nil {
		return fmt.Errorf("files root: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("files root is not a directory")
	}
	if info, err := os.Stat(sourceDir); err != nil {
		return fmt.Errorf("source dir: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("source dir is not a directory")
	}
	return nil
}

func runCommand(ctx context.Context, dir string, args, env []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	return cmd.Run()
}

func appletEnv(cfg RuntimeConfig, port int) []string {
	keep := map[string]bool{"PATH": true, "HOME": true, "GOCACHE": true, "TMPDIR": true, "TEMP": true, "TMP": true}
	env := make([]string, 0, len(keep)+len(cfg.Env)+2)
	for _, pair := range os.Environ() {
		key, _, ok := strings.Cut(pair, "=")
		if ok && keep[key] {
			env = append(env, pair)
		}
	}
	env = append(env, "PORT="+strconv.Itoa(port), "VAMOS_APP_FILES_ROOT="+filepath.Clean(cfg.FilesRoot))
	for key, value := range cfg.Env {
		if strings.TrimSpace(key) != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func waitHealthy(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	return healthCheck(ctx, endpoint)
}

func healthCheck(ctx context.Context, endpoint string) error {
	ticker := time.NewTicker(healthPollEvery)
	defer ticker.Stop()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(endpoint)
			if err != nil {
				lastErr = err
				continue
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("health returned %s", resp.Status)
		}
	}
}

func stopProcess(proc *activeProcess) error {
	if proc == nil {
		return nil
	}
	if proc.state.Status == ProcessStatusStarting {
		proc.finishStart(errors.New("applet stopped while starting"))
	}
	if proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		_ = proc.cmd.Process.Kill()
	} else {
		_ = proc.cmd.Process.Signal(os.Interrupt)
	}
	proc.startWait()
	select {
	case <-time.After(time.Second):
		_ = proc.cmd.Process.Kill()
		<-proc.done
	case <-proc.done:
	}
	if proc.log != nil {
		_ = proc.log.Close()
	}
	return nil
}

func waitProcess(proc *activeProcess) error {
	if proc == nil || proc.cmd == nil {
		return nil
	}
	proc.startWait()
	<-proc.done
	return proc.waitErr
}

func (p *activeProcess) startWait() {
	p.once.Do(func() {
		go func() {
			p.waitErr = p.cmd.Wait()
			close(p.done)
		}()
	})
}

func (p *activeProcess) finishStart(err error) {
	if p == nil || p.ready == nil {
		return
	}
	p.readyOnce.Do(func() {
		p.startErr = err
		close(p.ready)
	})
}

func (p *activeProcess) startCanceled() error {
	if p == nil || p.ready == nil {
		return nil
	}
	select {
	case <-p.ready:
		return p.startErr
	default:
		return nil
	}
}

func waitForProcessReady(ctx context.Context, proc *activeProcess) (AppletProcessState, error) {
	if proc == nil {
		return AppletProcessState{}, errors.New("applet process is unavailable")
	}
	state := proc.state
	if state.Status == ProcessStatusHealthy {
		return state, nil
	}
	if proc.ready == nil {
		return state, nil
	}
	select {
	case <-proc.ready:
		if proc.startErr != nil {
			return proc.state, proc.startErr
		}
		return proc.state, nil
	case <-ctx.Done():
		return state, ctx.Err()
	}
}

func openLog(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return os.CreateTemp("", "vamos-applet-*.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func (m *ManagerImpl) logPath(appID string) string {
	if strings.TrimSpace(m.logDir) == "" {
		return ""
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, appID)
	return filepath.Join(m.logDir, safe+".log")
}

func pathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
