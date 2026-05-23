package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const globSplitParts = 2

type TreeHasher struct{ repoRoot string }

func NewTreeHasher(repoRoot string) *TreeHasher { return &TreeHasher{repoRoot: repoRoot} }

func (h *TreeHasher) Hash(ctx context.Context, spec HashSpec) (string, error) {
	files, err := h.files(ctx, spec)
	if err != nil {
		return "", err
	}
	if len(files) == 0 && spec.Optional {
		return "", nil
	}

	sum := sha256.New()
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		abs := filepath.Join(h.repoRoot, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", rel, err)
		}
		fmt.Fprintf(sum, "%s\x00%d\x00", rel, info.Size())
		file, err := os.Open(abs)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", rel, err)
		}
		_, copyErr := io.Copy(sum, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", fmt.Errorf("hash %s: %w", rel, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close %s: %w", rel, closeErr)
		}
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (h *TreeHasher) Exists(ctx context.Context, required []RequiredPath) (bool, error) {
	for _, req := range required {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		abs := filepath.Join(h.repoRoot, filepath.FromSlash(req.Path))
		if req.Glob {
			matches, err := filepath.Glob(abs)
			if err != nil {
				return false, fmt.Errorf("glob %s: %w", req.Path, err)
			}
			if len(matches) == 0 {
				return false, nil
			}
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("stat required %s: %w", req.Path, err)
		}
	}
	return true, nil
}

func (h *TreeHasher) files(ctx context.Context, spec HashSpec) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range spec.Roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cleanRoot := filepath.ToSlash(filepath.Clean(root))
		if cleanRoot == "." {
			cleanRoot = "."
		}
		absRoot := filepath.Join(h.repoRoot, filepath.FromSlash(cleanRoot))
		info, err := os.Stat(absRoot)
		if os.IsNotExist(err) {
			if spec.Optional {
				continue
			}
			return nil, fmt.Errorf("hash root missing: %s", root)
		}
		if err != nil {
			return nil, fmt.Errorf("stat hash root %s: %w", root, err)
		}
		if !info.IsDir() {
			rel := cleanRoot
			if rel == "." {
				rel = filepath.ToSlash(filepath.Base(absRoot))
			}
			if shouldInclude(rel, spec) {
				seen[rel] = true
			}
			continue
		}
		if err := filepath.WalkDir(
			absRoot,
			func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				rel, err := filepath.Rel(h.repoRoot, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if d.IsDir() {
					if rel != "." &&
						(rel == ".build-agents" || matchAny(rel, spec.Excludes)) {
						return filepath.SkipDir
					}
					return nil
				}
				if shouldInclude(rel, spec) {
					seen[rel] = true
				}
				return nil
			},
		); err != nil {
			return nil, fmt.Errorf("walk %s: %w", root, err)
		}
	}
	files := make([]string, 0, len(seen))
	for rel := range seen {
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}

func shouldInclude(path string, spec HashSpec) bool {
	path = filepath.ToSlash(path)
	if len(spec.Includes) > 0 && !matchAny(path, spec.Includes) {
		return false
	}
	return !matchAny(path, spec.Excludes)
}

func matchAny(path string, patterns []string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, pattern := range patterns {
		if matchPattern(path, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(path, pattern string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	path = strings.TrimSuffix(path, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return path == ""
	}
	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		if ok, _ := filepath.Match(suffix, path); ok {
			return true
		}
		if ok, _ := filepath.Match(suffix, filepath.Base(path)); ok {
			return true
		}
		if strings.HasSuffix(path, "/"+suffix) {
			return true
		}
	}
	if strings.Contains(pattern, "/**/") {
		parts := strings.SplitN(pattern, "/**/", globSplitParts)
		prefix, suffix := parts[0], parts[1]
		if strings.HasPrefix(path, prefix+"/") {
			remainder := strings.TrimPrefix(path, prefix+"/")
			if ok, _ := filepath.Match(suffix, remainder); ok {
				return true
			}
			if matchPattern(remainder, "**/"+suffix) {
				return true
			}
		}
	}
	return false
}
