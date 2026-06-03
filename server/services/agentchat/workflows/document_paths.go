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
	if idx := strings.Index(root, "/thoughts/"); idx >= 0 {
		root = strings.TrimPrefix(root[idx+1:], "/")
	}
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
	if strings.HasPrefix(rel, "thoughts/") {
		return rel, nil
	}
	return strings.TrimSuffix(root, "/") + "/" + rel, nil
}
