package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Binary() string {
	if path := strings.TrimSpace(os.Getenv("GIT_BIN")); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err == nil {
		preferred := filepath.Join(home, ".nix-profile", "bin", "git")
		if _, statErr := os.Stat(preferred); statErr == nil {
			return preferred
		}
	}

	if path, err := exec.LookPath("git"); err == nil {
		return path
	}

	return "git"
}

// GetCurrentCommit returns the current HEAD commit hash
// This should be called once at server startup and cached
func GetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	fmt.Printf("Getting git commit at %s\n", repoPath)
	cmd := exec.CommandContext(ctx, Binary(), "-C", repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git commit: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Pull performs a git pull --rebase in the specified directory
// Uses rebase to handle local commits (e.g. from self-improving automation)
// being replayed on top of incoming remote changes
func Pull(ctx context.Context, repoPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary(), "-C", repoPath, "pull", "--rebase")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git pull failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetChangedFiles returns the list of files introduced on the path to toCommit,
// using the merge-base with fromCommit so rebased local commits are preserved.
func GetChangedFiles(ctx context.Context, repoPath, fromCommit, toCommit string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary(), "-C", repoPath, "diff", "--name-only", fromCommit+"..."+toCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	lines := strings.TrimSpace(string(output))
	if lines == "" {
		return []string{}, nil
	}

	return strings.Split(lines, "\n"), nil
}
