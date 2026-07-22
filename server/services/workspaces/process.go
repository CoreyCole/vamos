package workspaces

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
	env = appendEnv(env, "VAMOS_WORKSPACE_PROJECT_ID", ws.ProjectID)
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

func processCmdline(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(strings.TrimRight(string(data), "\x00"), "\x00", " "), nil
}

func workspaceRuntimePIDs(ws Workspace, paths WorkspaceRuntimePaths) map[BundleComponent][]int {
	out := map[BundleComponent][]int{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	self := os.Getpid()
	temporalDB := filepath.Clean(paths.TemporalDB)
	webBinary := filepath.Clean(filepath.Join(ws.PackagePath, "agents-server"))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == self {
			continue
		}
		cmdline, err := processCmdline(pid)
		if err != nil || cmdline == "" {
			continue
		}
		switch {
		case temporalDB != "." && strings.Contains(cmdline, temporalDB):
			out[ComponentTemporal] = append(out[ComponentTemporal], pid)
		case webBinary != "." && strings.Contains(cmdline, webBinary):
			out[ComponentWeb] = append(out[ComponentWeb], pid)
		case strings.Contains(cmdline, "dist/pkg/agents/temporal/workers/ts/worker.js") &&
			processMatchesWorkspace(ws, pid):
			out[ComponentTSWorker] = append(out[ComponentTSWorker], pid)
		}
	}
	return out
}

func terminateProcessGroup(ctx context.Context, component BundleComponent, pid int) error {
	if pid <= 0 || !processAlive(pid) {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if waitForPIDExit(ctx, pid, componentStopGracePeriod) {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if waitForPIDExit(ctx, pid, componentKillWaitPeriod) {
		return nil
	}
	return fmt.Errorf("stop %s: process %d did not exit", component, pid)
}

func waitForPIDExit(ctx context.Context, pid int, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !processAlive(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return !processAlive(pid)
		case <-ticker.C:
		}
	}
}
