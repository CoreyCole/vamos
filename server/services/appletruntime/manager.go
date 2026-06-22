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
	state      ProcessState
	cmd        *exec.Cmd
	log        *os.File
	healthPath string
	done       chan struct{}
	once       sync.Once
	waitErr    error
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

func (m *ManagerImpl) Start(ctx context.Context, cfg RuntimeConfig) (ProcessState, error) {
	if err := validateConfig(cfg); err != nil {
		return ProcessState{}, err
	}
	if cfg.HealthPath == "" {
		cfg.HealthPath = defaultHealthPath
	}

	port, err := allocateLocalPort()
	if err != nil {
		return ProcessState{}, err
	}
	env := appletEnv(cfg, port)
	logPath := m.logPath(cfg.AppID)

	if len(cfg.BuildCommand) > 0 {
		if err := runCommand(ctx, cfg.SourceDir, cfg.BuildCommand, env, nil); err != nil {
			return ProcessState{}, fmt.Errorf("build applet %q: %w", cfg.AppID, err)
		}
	}

	logFile, err := openLog(logPath)
	if err != nil {
		return ProcessState{}, err
	}
	cmd := exec.Command(cfg.StartCommand[0], cfg.StartCommand[1:]...)
	cmd.Dir = cfg.SourceDir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return ProcessState{}, fmt.Errorf("start applet %q: %w", cfg.AppID, err)
	}

	candidate := &activeProcess{state: ProcessState{
		AppID:     cfg.AppID,
		SourceDir: cfg.SourceDir,
		Port:      port,
		PID:       cmd.Process.Pid,
		BaseURL:   "http://127.0.0.1:" + strconv.Itoa(port),
		Healthy:   false,
		LogPath:   logPath,
	}, cmd: cmd, log: logFile, healthPath: cfg.HealthPath, done: make(chan struct{})}

	if err := waitHealthy(ctx, candidate.state.BaseURL+cfg.HealthPath); err != nil {
		stopProcess(candidate)
		return ProcessState{}, fmt.Errorf("health check applet %q: %w", cfg.AppID, err)
	}
	candidate.state.Healthy = true

	m.mu.Lock()
	previous := m.active[cfg.AppID]
	m.active[cfg.AppID] = candidate
	m.mu.Unlock()
	if previous != nil {
		stopProcess(previous)
	}
	go m.reap(cfg.AppID, candidate)
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

func (m *ManagerImpl) Health(ctx context.Context, appID string) (ProcessState, error) {
	m.mu.Lock()
	proc := m.active[strings.TrimSpace(appID)]
	m.mu.Unlock()
	if proc == nil {
		return ProcessState{}, fmt.Errorf("unknown applet %q", appID)
	}
	if err := waitHealthy(ctx, proc.state.BaseURL+proc.healthPath); err != nil {
		state := proc.state
		state.Healthy = false
		return state, err
	}
	state := proc.state
	state.Healthy = true
	return state, nil
}

func (m *ManagerImpl) ProxyTarget(appID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc := m.active[strings.TrimSpace(appID)]
	if proc == nil || !proc.state.Healthy || proc.state.BaseURL == "" {
		return "", false
	}
	return proc.state.BaseURL, true
}

func (m *ManagerImpl) reap(appID string, proc *activeProcess) {
	err := waitProcess(proc)
	m.mu.Lock()
	if m.active[appID] == proc {
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
	if !pathWithinRoot(filesRoot, sourceDir) {
		return fmt.Errorf("source dir must be inside files root")
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
	if proc == nil || proc.cmd == nil || proc.cmd.Process == nil {
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
