package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LauncherConfig struct {
	RuntimeSourceRoot string `json:"runtime_source_root"`
}

type RuntimeSource struct {
	Root       string
	SourceKey  string
	SourceFrom string
}

func resolveRuntimeSource(ctx context.Context) (RuntimeSource, error) {
	_ = ctx
	if root := strings.TrimSpace(os.Getenv("VAMOS_PACKAGE_ROOT")); root != "" {
		validated, err := validateRuntimeSourceRoot(root)
		if err != nil {
			return RuntimeSource{}, fmt.Errorf("validate VAMOS_PACKAGE_ROOT runtime source root: %w", err)
		}
		return RuntimeSource{Root: validated, SourceKey: sourceRootKey(validated), SourceFrom: "VAMOS_PACKAGE_ROOT"}, nil
	}

	configPath, err := defaultLauncherConfigPath()
	if err != nil {
		return RuntimeSource{}, err
	}
	cfg, err := loadLauncherConfig(configPath)
	if err != nil {
		return RuntimeSource{}, fmt.Errorf("load launcher config %q: %w; set VAMOS_PACKAGE_ROOT or run vamos launcher configure --runtime-source-root /path/to/vamos", configPath, err)
	}
	validated, err := validateRuntimeSourceRoot(cfg.RuntimeSourceRoot)
	if err != nil {
		return RuntimeSource{}, fmt.Errorf("validate runtime_source_root from %q: %w", configPath, err)
	}
	return RuntimeSource{Root: validated, SourceKey: sourceRootKey(validated), SourceFrom: configPath}, nil
}

func loadLauncherConfig(path string) (LauncherConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LauncherConfig{}, err
	}
	var cfg LauncherConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LauncherConfig{}, fmt.Errorf("parse launcher config: %w", err)
	}
	if strings.TrimSpace(cfg.RuntimeSourceRoot) == "" {
		return LauncherConfig{}, fmt.Errorf("runtime_source_root is required")
	}
	return cfg, nil
}

func defaultLauncherConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("VAMOS_LAUNCHER_CONFIG")); path != "" {
		return path, nil
	}
	if stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); stateHome != "" {
		return filepath.Join(stateHome, "vamos", "launcher.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for launcher config: %w", err)
	}
	return filepath.Join(home, ".local", "state", "vamos", "launcher.json"), nil
}

func validateRuntimeSourceRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("runtime source root is required")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("runtime source root %q must be absolute", root)
	}
	cleaned := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("stat runtime source root %q: %w", cleaned, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("runtime source root %q is not a directory", cleaned)
	}
	if _, err := os.Stat(filepath.Join(cleaned, "go.mod")); err != nil {
		return "", fmt.Errorf("runtime source root %q missing go.mod: %w", cleaned, err)
	}
	runtimeDir := filepath.Join(cleaned, "cmd", "vamos-runtime")
	info, err = os.Stat(runtimeDir)
	if err != nil {
		return "", fmt.Errorf("runtime source root %q missing cmd/vamos-runtime: %w", cleaned, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("runtime source root %q cmd/vamos-runtime is not a directory", cleaned)
	}
	return cleaned, nil
}

func sourceRootKey(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:])[:16]
}
