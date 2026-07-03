package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlePush_RefusesDirtyWorktreeWithoutStashing(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	remote := filepath.Join(base, "remote.git")
	worktree := filepath.Join(base, "worktree")

	runGit(t, "", "init", "-b", "main", seed)
	mustWriteFile(t, filepath.Join(seed, "README.md"), "hello\n")
	runGitWithEnv(t, seed, []string{
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}, "add", "README.md")
	runGitWithEnv(t, seed, []string{
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}, "commit", "-m", "initial")
	runGit(t, "", "clone", "--bare", seed, remote)
	runGit(t, "", "clone", remote, worktree)

	mustWriteFile(t, filepath.Join(worktree, "README.md"), "hello\ndirty\n")

	svc := NewService("", worktree, filepath.Join(t.TempDir(), "noop.sh"))
	payload := []byte(
		`{"ref":"refs/heads/main","repository":{"full_name":"premiumlabs/cn-agents"}}`,
	)
	err := svc.HandlePush(t.Context(), payload, RequestMeta{EventType: "push"})
	if err == nil {
		t.Fatal("HandlePush() error = nil, want dirty-worktree refusal")
	}
	if !strings.Contains(err.Error(), "refusing webhook pull") {
		t.Fatalf("HandlePush() error = %v, want refusing webhook pull", err)
	}

	status := runGit(t, worktree, "status", "--porcelain")
	if !strings.Contains(status, " M README.md") {
		t.Fatalf("expected README.md to remain dirty, status=%q", status)
	}

	stashList := runGit(t, worktree, "stash", "list")
	if strings.TrimSpace(stashList) != "" {
		t.Fatalf("expected no webhook stash, stash list=%q", stashList)
	}
}

func TestHandlePushForwardsUnknownLocalRepoWhenConfigured(t *testing.T) {
	t.Parallel()

	var gotBody []byte
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer downstream.Close()

	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  true,
	}})

	err := svc.HandlePush(t.Context(), payload, RequestMeta{EventType: "push", Repository: "CoreyCole/vamos"})
	if err != nil {
		t.Fatalf("HandlePush() error = %v", err)
	}
	if string(gotBody) != string(payload) {
		t.Fatalf("forwarded body = %s, want %s", gotBody, payload)
	}
}

func TestHandlePushBestEffortForwardFailureDoesNotFail(t *testing.T) {
	t.Parallel()

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer downstream.Close()

	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  true,
	}})

	if err := svc.HandlePush(t.Context(), payload, RequestMeta{EventType: "push"}); err != nil {
		t.Fatalf("HandlePush() error = %v, want nil for best-effort forward failure", err)
	}
}

func TestHandlePushRequiredForwardFailureFails(t *testing.T) {
	t.Parallel()

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer downstream.Close()

	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"CoreyCole/vamos"}}`)
	svc := NewServiceWithRoutes("secret", RepoRoute{}, nil, []ForwardRoute{{
		URL:         downstream.URL,
		GitHubRepos: map[string]bool{"coreycole/vamos": true},
		Events:      map[string]bool{"push": true},
		BestEffort:  false,
	}})

	err := svc.HandlePush(t.Context(), payload, RequestMeta{EventType: "push"})
	if err == nil {
		t.Fatal("HandlePush() error = nil, want required forward failure")
	}
	if !strings.Contains(err.Error(), "webhook forward failed") {
		t.Fatalf("HandlePush() error = %v, want webhook forward failed", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitWithEnv(t, dir, nil, args...)
}

func runGitWithEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
