package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type FingerprintSpec struct {
	Version  string
	Roots    []string
	Includes []string
	Excludes []string
}

type Fingerprint struct {
	Value      string
	SourceRoot string
	GOOS       string
	GOARCH     string
	GoVersion  string
}

func computeRuntimeFingerprint(ctx context.Context, source RuntimeSource) (Fingerprint, error) {
	spec := runtimeFingerprintSpec()
	goVersion := goVersion(ctx, source.Root)
	treeHash, err := hashRuntimeTree(ctx, source.Root, spec)
	if err != nil {
		return Fingerprint{}, err
	}

	h := sha256.New()
	writeHashField(h, "version", spec.Version)
	writeHashField(h, "source_root", filepath.Clean(source.Root))
	writeHashField(h, "source_key", source.SourceKey)
	writeHashField(h, "goos", runtime.GOOS)
	writeHashField(h, "goarch", runtime.GOARCH)
	writeHashField(h, "go_version", goVersion)
	writeHashField(h, "tree", treeHash)

	return Fingerprint{
		Value:      hex.EncodeToString(h.Sum(nil))[:24],
		SourceRoot: filepath.Clean(source.Root),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		GoVersion:  goVersion,
	}, nil
}

func runtimeFingerprintSpec() FingerprintSpec {
	return FingerprintSpec{
		Version: "vamos-runtime-fingerprint-v1",
		Roots: []string{
			"go.mod",
			"go.sum",
			"cmd/vamos-runtime",
			"pkg",
			"buf.yaml",
			"buf.gen.yaml",
			"sqlc.yaml",
			"pkg/proto/source",
			"pkg/db/queries",
			"pkg/db/migrations/schema.sql",
		},
		Includes: []string{
			"go.mod", "go.sum",
			"*.yaml", "*.yml",
			"**/*.go", "**/*.templ", "**/*.sql", "**/*.proto", "**/*.js",
		},
		Excludes: []string{
			".git/**", ".vamos/**", ".build-agents/**",
			"node_modules/**", "**/node_modules/**", "dist/**", "**/dist/**",
			"thoughts/**", "docs/**", "static/**", "**/*_test.go",
		},
	}
}

func hashRuntimeTree(ctx context.Context, root string, spec FingerprintSpec) (string, error) {
	_ = ctx
	cleanRoot := filepath.Clean(root)
	required := map[string]bool{"go.mod": true, "cmd/vamos-runtime": true, "pkg": true}
	var files []string

	for _, relRoot := range spec.Roots {
		relRoot = filepath.Clean(relRoot)
		absRoot := filepath.Join(cleanRoot, relRoot)
		info, err := os.Stat(absRoot)
		if err != nil {
			if os.IsNotExist(err) && !required[filepath.ToSlash(relRoot)] {
				continue
			}
			return "", fmt.Errorf("stat fingerprint root %q: %w", relRoot, err)
		}
		if !info.IsDir() {
			rel := filepath.ToSlash(relRoot)
			if shouldHashRuntimeFile(rel, spec) {
				files = append(files, rel)
			}
			continue
		}
		if err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(cleanRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if rel != "." && matchesAnyRuntimePattern(rel+"/", spec.Excludes) {
					return filepath.SkipDir
				}
				return nil
			}
			if shouldHashRuntimeFile(rel, spec) {
				files = append(files, rel)
			}
			return nil
		}); err != nil {
			return "", fmt.Errorf("walk fingerprint root %q: %w", relRoot, err)
		}
	}

	sort.Strings(files)
	h := sha256.New()
	writeHashField(h, "spec", spec.Version)
	for _, rel := range files {
		abs := filepath.Join(cleanRoot, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("stat fingerprint file %q: %w", rel, err)
		}
		writeHashField(h, "path", rel)
		writeHashField(h, "size", fmt.Sprintf("%d", info.Size()))
		f, err := os.Open(abs)
		if err != nil {
			return "", fmt.Errorf("open fingerprint file %q: %w", rel, err)
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return "", fmt.Errorf("hash fingerprint file %q: %w", rel, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close fingerprint file %q: %w", rel, closeErr)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func goVersion(ctx context.Context, sourceRoot string) string {
	cmd := exec.CommandContext(ctx, "go", "version")
	cmd.Dir = sourceRoot
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func shouldHashRuntimeFile(rel string, spec FingerprintSpec) bool {
	if matchesAnyRuntimePattern(rel, spec.Excludes) {
		return false
	}
	return matchesAnyRuntimePattern(rel, spec.Includes)
}

func matchesAnyRuntimePattern(rel string, patterns []string) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	for _, pattern := range patterns {
		if matchesRuntimePattern(rel, pattern) {
			return true
		}
	}
	return false
}

func matchesRuntimePattern(rel, pattern string) bool {
	pattern = filepath.ToSlash(pattern)
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "**")
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(pattern, "/**") {
		part := strings.TrimSuffix(strings.TrimPrefix(pattern, "**/"), "/**")
		return rel == part || strings.HasPrefix(rel, part+"/") || strings.Contains(rel, "/"+part+"/")
	}
	if strings.HasPrefix(pattern, "**/*") {
		return strings.HasSuffix(rel, strings.TrimPrefix(pattern, "**/*"))
	}
	if strings.HasPrefix(pattern, "**/.") {
		return strings.HasSuffix(rel, strings.TrimPrefix(pattern, "**/"))
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		return rel == suffix || strings.HasSuffix(rel, "/"+suffix)
	}
	if ok, _ := filepath.Match(pattern, rel); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		base := filepath.Base(rel)
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}
	return rel == pattern
}

func writeHashField(w io.Writer, name, value string) {
	_, _ = fmt.Fprintf(w, "%s\x00%s\x00", name, value)
}
