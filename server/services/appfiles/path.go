package appfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CleanRelPath(rel string) string {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	rel = strings.TrimPrefix(rel, "/")
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func SafeOpenPath(root, relPath string) (string, error) {
	root = filepath.Clean(root)
	rel := CleanRelPath(relPath)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("file is outside this app")
	}
	candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if !PathWithinRoot(root, candidate) {
		return "", fmt.Errorf("file is outside this app")
	}
	if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if !PathWithinRoot(root, target) {
			return "", fmt.Errorf("file is outside this app")
		}
	}
	return candidate, nil
}

func PathWithinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../")
}

func IsHidden(relPath string, hidden []string) bool {
	rel := CleanRelPath(relPath)
	for _, pattern := range hidden {
		p := CleanRelPath(pattern)
		if p == "" {
			continue
		}
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}
