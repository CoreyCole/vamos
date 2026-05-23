package workspacecmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceCLIConfig struct {
	CheckoutPath string
	EnvPath      string
	StatusPath   string
	Metadata     WorkspaceMetadata
	Status       workspaceRuntimeStatusFile
	ManagerURL   string
	RestartToken string
}

type WorkspaceMetadata struct {
	Slug         string
	CheckoutPath string
	ManagerURL   string
	RestartToken string
}

type workspaceRuntimeStatusFile struct {
	Status string            `json:"status"`
	Phase  string            `json:"phase"`
	Error  string            `json:"error"`
	Logs   map[string]string `json:"logs"`
	Ports  map[string]int    `json:"ports"`
	PIDs   map[string]int    `json:"pids"`
	Build  map[string]string `json:"build"`
}

func LoadConfig(cwd string) (WorkspaceCLIConfig, error) {
	checkout := findCheckoutRoot(cwd)
	envPath := filepath.Join(checkout, ".vamos", "run", "workspace.env")
	meta, err := readWorkspaceMetadata(envPath)
	if errors.Is(err, os.ErrNotExist) {
		return WorkspaceCLIConfig{}, fmt.Errorf(
			"not a managed workspace checkout: missing %s",
			envPath,
		)
	}
	if err != nil {
		return WorkspaceCLIConfig{}, err
	}
	if strings.TrimSpace(meta.CheckoutPath) == "" {
		meta.CheckoutPath = checkout
	}
	statusPath := filepath.Join(checkout, ".vamos", "run", "status.json")
	status := workspaceRuntimeStatusFile{}
	if data, err := os.ReadFile(statusPath); err == nil {
		_ = json.Unmarshal(data, &status)
	}
	return WorkspaceCLIConfig{
		CheckoutPath: checkout,
		EnvPath:      envPath,
		StatusPath:   statusPath,
		Metadata:     meta,
		Status:       status,
		ManagerURL:   meta.ManagerURL,
		RestartToken: meta.RestartToken,
	}, nil
}

func findCheckoutRoot(cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if _, err := os.Stat(
			filepath.Join(current, ".vamos", "run", "workspace.env"),
		); err == nil {
			return current
		}
		if _, err := os.Stat(
			filepath.Join(current, "cmd", "server", "go.mod"),
		); err == nil {
			return current
		}
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd)
		}
		current = parent
	}
}

func readWorkspaceMetadata(path string) (WorkspaceMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return WorkspaceMetadata{}, err
	}
	defer f.Close()
	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(key)] = unshellWorkspaceValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return WorkspaceMetadata{}, err
	}
	return WorkspaceMetadata{
		Slug:         vals["VAMOS_WORKSPACE_SLUG"],
		CheckoutPath: vals["VAMOS_WORKSPACE_CHECKOUT"],
		ManagerURL:   vals["VAMOS_WORKSPACE_MANAGER_URL"],
		RestartToken: vals["VAMOS_WORKSPACE_RESTART_TOKEN"],
	}, nil
}

func unshellWorkspaceValue(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		inner := strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		return strings.ReplaceAll(inner, "'\\''", "'")
	}
	return value
}
