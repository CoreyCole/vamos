package qrspicmd

import (
	"fmt"
	"os"
	"path/filepath"
)

const qManagerChildExtensionRelativePath = ".pi/extensions/q-manager-child/index.js"

// ResolveChildExtensionPath copies the project-owned child extension into the
// manager's private runtime directory. Pi receives an immutable per-run copy,
// while source ownership remains in the project-local .pi tree.
func ResolveChildExtensionPath(runRoot, projectRoot string) (string, error) {
	source, err := findProjectChildExtension(projectRoot)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf(
			"read project q-manager child extension %s: %w",
			source,
			err,
		)
	}
	path := filepath.Join(runRoot, "assets", "q_manager_child_extension.js")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func findProjectChildExtension(projectRoot string) (string, error) {
	roots := []string{projectRoot, os.Getenv("VAMOS_PACKAGE_ROOT")}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			roots = append(roots, dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		path := filepath.Join(root, qManagerChildExtensionRelativePath)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf(
		"project q-manager child extension %s was not found",
		qManagerChildExtensionRelativePath,
	)
}
