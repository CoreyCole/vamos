package build

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type WorkspaceBuildStatus struct {
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastFailedAt  time.Time `json:"last_failed_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	LogPath       string    `json:"log_path,omitempty"`
}

type workspaceRuntimeStatusFile struct {
	Status string               `json:"status"`
	Phase  string               `json:"phase,omitempty"`
	Error  string               `json:"error,omitempty"`
	Logs   map[string]string    `json:"logs,omitempty"`
	Ports  map[string]int       `json:"ports,omitempty"`
	PIDs   map[string]int       `json:"pids,omitempty"`
	Build  WorkspaceBuildStatus `json:"build,omitempty"`
}

func WriteWorkspaceBuildStatus(
	ctx context.Context,
	checkoutPath string,
	build WorkspaceBuildStatus,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(workspaceEnvPath(checkoutPath)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	paths := workspaceRuntimePaths(checkoutPath)
	status := workspaceRuntimeStatusFile{Status: "stopped"}
	data, err := os.ReadFile(paths.statusJSON)
	if err == nil {
		if err := json.Unmarshal(data, &status); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	status.Build = build
	if status.Logs == nil {
		status.Logs = map[string]string{}
	}
	status.Logs["build"] = build.LogPath
	return writeJSONFile(paths.statusJSON, status, 0o644)
}

func WriteWorkspaceBuildLog(checkoutPath, text string) (string, error) {
	paths := workspaceRuntimePaths(checkoutPath)
	if err := os.MkdirAll(filepath.Dir(paths.buildLog), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(paths.buildLog, []byte(text), 0o644); err != nil {
		return "", err
	}
	return paths.buildLog, nil
}

type workspaceRuntimeFilePaths struct {
	statusJSON string
	buildLog   string
}

func workspaceRuntimePaths(checkoutPath string) workspaceRuntimeFilePaths {
	root := filepath.Join(checkoutPath, ".vamos")
	return workspaceRuntimeFilePaths{
		statusJSON: filepath.Join(root, "run", "status.json"),
		buildLog:   filepath.Join(root, "log", "build.log"),
	}
}

func writeJSONFile(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
