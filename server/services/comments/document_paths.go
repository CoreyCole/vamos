package comments

import (
	"errors"
	"path/filepath"
	"strings"
)

func canonicalThoughtsPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", errors.New("document path is required")
	}
	if filepath.IsAbs(p) {
		return "", errors.New("document path must be relative")
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", errors.New("document path escapes thoughts root")
	}
	if !strings.HasPrefix(p, "thoughts/") {
		return "", errors.New("document path must start with thoughts/")
	}
	return p, nil
}
