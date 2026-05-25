package workspaces

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type childProcess struct {
	cmd  *exec.Cmd
	done chan error

	once   sync.Once
	exited chan struct{}
	err    error
}

func newChildProcess(cmd *exec.Cmd) *childProcess {
	return &childProcess{cmd: cmd, done: make(chan error, 1), exited: make(chan struct{})}
}

func (cp *childProcess) pid() int {
	if cp == nil || cp.cmd == nil || cp.cmd.Process == nil {
		return 0
	}
	return cp.cmd.Process.Pid
}

func (cp *childProcess) finish(err error) {
	if cp == nil {
		return
	}
	cp.once.Do(func() {
		cp.err = err
		if cp.done != nil {
			cp.done <- err
		}
		close(cp.exited)
	})
}

func (cp *childProcess) wait(ctx context.Context) error {
	if cp == nil {
		return nil
	}
	if cp.exited == nil {
		select {
		case err := <-cp.done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case <-cp.exited:
		return cp.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func ChildEnv(
	parent map[string]string,
	ws Workspace,
	ports map[BundleComponent]int,
	rt RuntimeConfig,
) []string {
	env := os.Environ()
	for key, value := range parent {
		env = appendEnv(env, key, value)
	}
	env = removeEnv(
		env,
		"VAMOS_DEV_AUTH_SIGNING_KEY",
		"CN_AGENTS_DEV_AUTH_SIGNING_KEY",
		"CN_AGENTS_DEV_AUTH_SECRET",
	)
	webPort := ports[ComponentWeb]
	temporalPort := ports[ComponentTemporal]
	temporalUIPort := ports[ComponentTemporalUI]
	paths := RuntimePaths(ws.CheckoutPath, rt.MetadataDirName)
	listenAddress := "127.0.0.1:" + strconv.Itoa(webPort)
	env = appendEnv(env, "VAMOS_LISTEN_ADDRESS", listenAddress)
	env = appendEnv(env, "VAMOS_PUBLIC_BASE_URL", strings.TrimRight(ws.URL, "/"))
	env = appendEnv(env, "VAMOS_INTERNAL_CALLBACK_BASE_URL", "http://"+listenAddress)
	env = appendEnv(env, "VAMOS_THOUGHTS_REPO", rt.ThoughtsRepo)
	env = appendEnv(env, "VAMOS_THOUGHTS_ROOT", rt.ThoughtsRoot)
	env = appendEnv(env, "VAMOS_DEFAULT_CWD", ws.CheckoutPath)
	env = appendEnv(env, "CN_TEMPORAL", "true")
	env = appendEnv(env, "TEMPORAL_ADDRESS", "127.0.0.1:"+strconv.Itoa(temporalPort))
	env = appendEnv(
		env,
		"TEMPORAL_UI_BASE_URL",
		"http://127.0.0.1:"+strconv.Itoa(temporalUIPort),
	)
	env = appendEnv(env, "VAMOS_DATABASE_PATH", paths.AgentsDB)
	env = appendEnv(env, "OPENCLAW_STATE_DIR", paths.OpenClawDir)
	env = appendEnv(env, "VAMOS_WORKSPACE_MODE", "child")
	env = appendEnv(env, "VAMOS_WORKSPACE_SLUG", ws.Slug)
	env = appendEnv(env, "VAMOS_WORKSPACE_MANAGER_URL", rt.ManagerURL)
	env = appendEnv(env, "VAMOS_WORKSPACE_RESTART_TOKEN", rt.RestartToken)
	env = appendEnv(env, "VAMOS_DEV_AUTH_VERIFY_KEY", rt.DevAuthVerifyKey)
	return env
}

func removeEnv(env []string, keys ...string) []string {
	blocked := map[string]struct{}{}
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	out := env[:0]
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := blocked[key]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func appendEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return append(out, prefix+value)
}

func startChild(
	ctx context.Context,
	ws Workspace,
	port int,
	rt RuntimeConfig,
) (*childProcess, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if err := os.MkdirAll(ws.StateDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(ws.LogPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(ws.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(filepath.Join(ws.PackagePath, "agents-server"))
	cmd.Dir = ws.PackagePath
	cmd.Env = ChildEnv(rt.BaseEnv, ws, map[BundleComponent]int{ComponentWeb: port}, rt)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", ws.Slug, err)
	}

	cp := newChildProcess(cmd)
	go func() {
		cp.finish(cmd.Wait())
		_ = logFile.Close()
	}()
	return cp, nil
}

func stopChild(ctx context.Context, cp *childProcess) error {
	if cp == nil || cp.cmd == nil || cp.cmd.Process == nil {
		return nil
	}
	_ = syscall.Kill(-cp.cmd.Process.Pid, syscall.SIGTERM)
	if err := cp.wait(ctx); err != nil {
		_ = syscall.Kill(-cp.cmd.Process.Pid, syscall.SIGKILL)
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func processMatchesWorkspace(ws Workspace, pid int) bool {
	if pid <= 0 {
		return false
	}
	env, err := processEnv(pid)
	if err == nil {
		if slug := strings.TrimSpace(env["VAMOS_WORKSPACE_SLUG"]); slug != "" && slug != ws.Slug {
			return false
		}
		if cwd := strings.TrimSpace(env["VAMOS_DEFAULT_CWD"]); cwd != "" {
			return samePath(cwd, ws.CheckoutPath)
		}
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	if err != nil {
		return true
	}
	return samePath(cwd, ws.PackagePath)
}

func processEnv(pid int) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, item := range strings.Split(string(data), "\x00") {
		if item == "" {
			continue
		}
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out, nil
}
