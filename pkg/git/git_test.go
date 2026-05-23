package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetChangedFiles_IncludesRebasedLocalCodeChanges(t *testing.T) {
	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	remote := filepath.Join(base, "remote.git")
	worktree := filepath.Join(base, "worktree")
	updater := filepath.Join(base, "updater")

	runGit(t, "", "init", "-b", "main", seed)
	mustWriteFile(
		t,
		filepath.Join(seed, "pkg/agents/workflows/runtime/worker.go"),
		"package runtime\n\nvar Worker = \"old\"\n",
	)
	mustWriteFile(t, filepath.Join(seed, "thoughts/status.md"), "base\n")
	runGit(t, seed, "add", ".")
	commitGit(t, seed, "base")

	runGit(t, "", "clone", "--bare", seed, remote)
	runGit(t, "", "clone", remote, worktree)
	runGit(t, "", "clone", remote, updater)

	mustWriteFile(
		t,
		filepath.Join(worktree, "pkg/agents/workflows/runtime/worker.go"),
		"package runtime\n\nvar Worker = \"local-code-change\"\n",
	)
	runGit(t, worktree, "add", "pkg/agents/workflows/runtime/worker.go")
	commitGit(t, worktree, "local code change")
	beforeCommit := strings.TrimSpace(runGit(t, worktree, "rev-parse", "HEAD"))

	mustWriteFile(
		t,
		filepath.Join(updater, "thoughts/status.md"),
		"remote-thought-change\n",
	)
	runGit(t, updater, "add", "thoughts/status.md")
	commitGit(t, updater, "remote thoughts change")
	runGit(t, updater, "push", "origin", "main")

	runGit(t, worktree, "pull", "--rebase")
	afterCommit := strings.TrimSpace(runGit(t, worktree, "rev-parse", "HEAD"))

	changed, err := GetChangedFiles(
		context.Background(),
		worktree,
		beforeCommit,
		afterCommit,
	)
	if err != nil {
		t.Fatalf("GetChangedFiles: %v", err)
	}

	assertContains(t, changed, "pkg/agents/workflows/runtime/worker.go")
	assertContains(t, changed, "thoughts/status.md")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(Binary(), args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func commitGit(t *testing.T, dir, message string) {
	t.Helper()
	runGit(t, dir, "commit", "-m", message)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("expected %q in changed files, got %v", want, items)
}
