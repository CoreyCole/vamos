package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func BuildTailwindHashedAsset(ctx context.Context, repoRoot string, runner Runner) error {
	if err := runner.Run(ctx, CommandSpec{Args: []string{
		"pnpm",
		"exec",
		"tailwindcss",
		"-i",
		"static/css/index.css",
		"-o",
		"static/css/out.css",
	}}); err != nil {
		return err
	}

	outPath := filepath.Join(repoRoot, "static", "css", "out.css")
	hash, err := fileHash8(outPath)
	if err != nil {
		return err
	}

	matches, err := filepath.Glob(filepath.Join(repoRoot, "static", "css", "out.*.css"))
	if err != nil {
		return fmt.Errorf("glob generated css: %w", err)
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			return fmt.Errorf("remove old css %s: %w", match, err)
		}
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return fmt.Errorf("read out.css: %w", err)
	}
	target := filepath.Join(repoRoot, "static", "css", "out."+hash+".css")
	// #nosec G306 G703 -- generated CSS assets must be web-readable; target is derived
	// from repoRoot plus content hash.
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write hashed css: %w", err)
	}
	return nil
}

func ActiveCSSHashPath(repoRoot string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "static", "css", "out.*.css"))
	if err != nil {
		return "", fmt.Errorf("glob generated css: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}

	latest := matches[0]
	latestInfo, err := os.Stat(latest)
	if err != nil {
		return "", fmt.Errorf("stat css %s: %w", latest, err)
	}
	for _, match := range matches[1:] {
		info, err := os.Stat(match)
		if err != nil {
			return "", fmt.Errorf("stat css %s: %w", match, err)
		}
		if info.ModTime().After(latestInfo.ModTime()) {
			latest = match
			latestInfo = info
		}
	}

	rel, err := filepath.Rel(repoRoot, latest)
	if err != nil {
		return "", fmt.Errorf("rel css path: %w", err)
	}
	return filepath.ToSlash(rel), nil
}

func fileHash8(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(sum.Sum(nil))[:8], nil
}

func isCompiledTestArtifact(path string) bool {
	slash := filepath.ToSlash(path)
	return strings.Contains(slash, ".test.") ||
		strings.HasSuffix(slash, "_test.js") ||
		strings.HasSuffix(slash, "_test.d.ts")
}
