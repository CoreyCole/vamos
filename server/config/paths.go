package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PathPolicy int

const (
	PathPolicyHost PathPolicy = iota
	PathPolicyModule
	PathPolicyState
)

func DefaultStateDir() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "vamos"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "vamos"), nil
}

func ExpandHostPath(name, value string) (string, error) {
	return expandPathWithPolicy(name, value, PathPolicyHost, "")
}

func ExpandOptionalHostPath(name, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return ExpandHostPath(name, value)
}

func ExpandStatePath(stateDir, name, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return expandPathWithPolicy(name, value, PathPolicyState, stateDir)
}

func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func expandPathWithPolicy(
	name, value string,
	policy PathPolicy,
	stateDir string,
) (string, error) {
	expanded, err := ExpandPath(value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	switch policy {
	case PathPolicyState:
		if strings.TrimSpace(stateDir) == "" {
			stateDir, err = DefaultStateDir()
			if err != nil {
				return "", err
			}
		}
		return filepath.Join(stateDir, expanded), nil
	case PathPolicyModule:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, expanded), nil
	default:
		return "", fmt.Errorf("%s must be absolute or ~/ relative; got %q", name, value)
	}
}
