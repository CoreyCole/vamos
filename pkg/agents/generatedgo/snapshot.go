package generatedgo

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SnapshotInput struct {
	SourceDir   string
	OutputDir   string
	SnapshotDir string
	Allowlist   []string
}

type SnapshotResult struct {
	SourceHash     string
	ArtifactHashes map[string]string
	SnapshotDir    string
}

func HashSource(root string) (string, error) {
	return hashTree(root, func(_ string, d fs.DirEntry) bool {
		name := d.Name()
		return name == ".git" || name == "tmp" || name == "node_modules"
	})
}

func HashArtifacts(outputDir string, allowlist []string) (map[string]string, error) {
	outputRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output dir: %w", err)
	}
	out := make(map[string]string, len(allowlist))
	for _, rel := range allowlist {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		abs := filepath.Join(outputRoot, filepath.FromSlash(rel))
		if !pathWithinRoot(abs, outputRoot) {
			return nil, fmt.Errorf("artifact %q escapes output", rel)
		}
		hash, err := hashFile(abs)
		if err != nil {
			return nil, err
		}
		out[filepath.ToSlash(filepath.Clean(rel))] = hash
	}
	return out, nil
}

func CopySnapshot(input SnapshotInput) (SnapshotResult, error) {
	if err := os.MkdirAll(input.SnapshotDir, 0o755); err != nil {
		return SnapshotResult{}, err
	}
	sourceHash, err := HashSource(input.SourceDir)
	if err != nil {
		return SnapshotResult{}, err
	}
	sourceTarget := filepath.Join(input.SnapshotDir, "source")
	if err := copyTree(input.SourceDir, sourceTarget); err != nil {
		return SnapshotResult{}, err
	}
	for _, rel := range input.Allowlist {
		rel = filepath.Clean(filepath.FromSlash(rel))
		if err := copyFile(filepath.Join(input.OutputDir, rel), filepath.Join(input.SnapshotDir, rel)); err != nil {
			return SnapshotResult{}, err
		}
	}
	hashes, err := HashArtifacts(input.SnapshotDir, input.Allowlist)
	if err != nil {
		return SnapshotResult{}, err
	}
	return SnapshotResult{SourceHash: sourceHash, ArtifactHashes: hashes, SnapshotDir: input.SnapshotDir}, nil
}

func pathWithinRoot(path, root string) bool {
	p, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	r, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	return p == r || strings.HasPrefix(p, r+string(os.PathSeparator))
}

func hashTree(root string, skip func(string, fs.DirEntry) bool) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	var files []string
	if err := filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip != nil && skip(path, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink not allowed: %s", path)
		}
		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, file := range files {
		rel, _ := filepath.Rel(rootAbs, file)
		_, _ = h.Write([]byte(filepath.ToSlash(rel)))
		fileHash, err := hashFile(file)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(fileHash))
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func copyTree(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	return filepath.WalkDir(srcAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink not allowed: %s", path)
		}
		if d.Type().IsRegular() {
			return copyFile(path, target)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
