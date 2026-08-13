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

const staleIndexLockAge = 30 * time.Minute

// Pull performs a git pull --rebase in the specified directory
// Uses rebase to handle local commits (e.g. from self-improving automation)
// being replayed on top of incoming remote changes
func Pull(ctx context.Context, repoPath string) (string, error) {
	output, err := pull(ctx, repoPath)
	if err == nil || !strings.Contains(output, "index.lock") {
		return output, err
	}

	recovered, recoverErr := recoverStaleIndexLock(repoPath)
	if recoverErr != nil || !recovered {
		return output, err
	}
	return pull(ctx, repoPath)
}

func pull(ctx context.Context, repoPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary(), "-C", repoPath, "pull", "--rebase")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git pull failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// recoverStaleIndexLock removes only an old index lock that lsof confirms no
// process owns. It deliberately leaves recent, owned, or unverifiable locks
// alone so a concurrent Git operation can never be disrupted.
func recoverStaleIndexLock(repoPath string) (bool, error) {
	lockPath := filepath.Join(repoPath, ".git", "index.lock")
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat git index lock: %w", err)
	}
	if time.Since(info.ModTime()) < staleIndexLockAge {
		return false, nil
	}

	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return false, fmt.Errorf("cannot verify git index lock ownership: lsof unavailable")
	}
	check := exec.Command(lsof, "-t", lockPath) //nolint:gosec // lsof comes from PATH and lockPath is configured locally.
	output, err := check.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return false, nil
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return false, fmt.Errorf("verify git index lock ownership: %w", err)
		}
	}

	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove verified stale git index lock: %w", err)
	}
	return true, nil
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
