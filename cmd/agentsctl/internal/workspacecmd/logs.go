package workspacecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceLogTarget string

const (
	WorkspaceLogWeb      WorkspaceLogTarget = "web"
	WorkspaceLogTemporal WorkspaceLogTarget = "temporal"
	WorkspaceLogTSWorker WorkspaceLogTarget = "ts-worker"
)

func RunLogs(
	ctx context.Context,
	cfg WorkspaceCLIConfig,
	target WorkspaceLogTarget,
	tail int,
	out io.Writer,
) error {
	_ = ctx
	key := string(target)
	if target == WorkspaceLogTSWorker {
		key = "ts_worker"
	}
	path := strings.TrimSpace(cfg.Status.Logs[key])
	if path == "" {
		return fmt.Errorf("no %s log path in %s", target, cfg.StatusPath)
	}
	if err := validateWorkspaceLogPath(cfg, path); err != nil {
		return err
	}
	fmt.Fprintf(out, "log: %s\n", path)
	text := tailTextFile(path, tail, 256*1024)
	if text != "" {
		fmt.Fprintln(out, text)
	}
	return nil
}

func validateWorkspaceLogPath(cfg WorkspaceCLIConfig, path string) error {
	logDir := filepath.Join(cfg.CheckoutPath, ".cn-agents", "log")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(
		absLogDir,
		absPath,
	); err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf(
			"log path %s is outside managed workspace log directory %s",
			path,
			logDir,
		)
	}
	if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
		realLogDir, err := filepath.EvalSymlinks(absLogDir)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(realLogDir, realPath)
		if err != nil || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf(
				"log path %s resolves outside managed workspace log directory %s",
				path,
				logDir,
			)
		}
	}
	return nil
}

func tailTextFile(path string, maxLines int, maxBytes int64) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
