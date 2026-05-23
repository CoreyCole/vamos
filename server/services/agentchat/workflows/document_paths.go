package workflows

import (
	"errors"
	"path/filepath"
	"strings"
)

func documentPathFromRoot(artifactRoot, relPath string) (string, error) {
	root := strings.TrimSpace(artifactRoot)
	if root == "" {
		return "", errors.New("artifact root is required")
	}
	root = filepath.ToSlash(filepath.Clean(root))
	if !strings.HasPrefix(root, "thoughts/") {
		return "", errors.New("artifact root must start with thoughts/")
	}
	rel := strings.TrimSpace(relPath)
	if rel == "" || filepath.IsAbs(rel) {
		return "", errors.New("artifact path must be relative")
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", errors.New("artifact path escapes workspace root")
	}
	return strings.TrimSuffix(root, "/") + "/" + rel, nil
}
