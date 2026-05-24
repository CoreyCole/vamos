package steps

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	e2e "github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

const (
	fileChangeTimeout  = 120 * time.Second
	fileChangePollTime = 500 * time.Millisecond
)

func RememberFileHash(t testing.TB, ctx *e2e.Context, path string) {
	t.Helper()
	resolved := resolveRepoPath(ctx, path)
	remembered, err := fileHash(resolved)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			t.Fatal(err)
		}
		seed := "# E2E Pi Plan Docs Review\n\nSeed content before Pi verification.\n"
		if err := os.WriteFile(resolved, []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		remembered, err = fileHash(resolved)
		if err != nil {
			t.Fatal(err)
		}
	}
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["file_hash:"+path] = remembered
	if status, err := changedRepoPaths(ctx.Config.RepoRoot); err == nil {
		ctx.Memory["repo_changed_paths_before"] = strings.Join(status, "\n")
	}
}

func ExpectFileHashChanged(t testing.TB, ctx *e2e.Context, path string) {
	t.Helper()
	remembered := ctx.Memory["file_hash:"+path]
	deadline := time.Now().Add(fileChangeTimeout)
	for {
		got, err := fileHash(resolveRepoPath(ctx, path))
		if err != nil {
			t.Fatal(err)
		}
		if remembered != got {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("file %s hash did not change", path)
		}
		time.Sleep(fileChangePollTime)
	}
}

func ExpectPiReviewFileSections(t testing.TB, ctx *e2e.Context, path string) {
	t.Helper()
	data, err := os.ReadFile(resolveRepoPath(ctx, path))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, section := range []string{
		"# E2E Pi Plan Docs Review",
		"## Latest E2E Pi Review",
		"### Potential E2E user story updates",
		"### Potential test implementation updates",
		"### Potential docs additions/updates/simplifications",
	} {
		if !strings.Contains(text, section) {
			t.Fatalf("file %s missing section %q", path, section)
		}
	}
	for _, section := range []string{
		"### Potential E2E user story updates",
		"### Potential test implementation updates",
		"### Potential docs additions/updates/simplifications",
	} {
		if !sectionHasBullet(text, section) {
			t.Fatalf("file %s section %q has no bullet", path, section)
		}
	}
}

func ExpectOnlyFileChanged(t testing.TB, ctx *e2e.Context, path string) {
	t.Helper()
	if _, err := os.Stat(resolveRepoPath(ctx, path)); err != nil {
		t.Fatal(err)
	}
	before := splitPathSet(ctx.Memory["repo_changed_paths_before"])
	after, err := changedRepoPaths(ctx.Config.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.ToSlash(
		strings.TrimPrefix(
			resolveRepoPath(ctx, path),
			ctx.Config.RepoRoot+string(filepath.Separator),
		),
	)
	for _, changed := range after {
		if changed == allowed || before[changed] {
			continue
		}
		t.Fatalf("unexpected changed file %s; only %s may change", changed, allowed)
	}
}

func resolveRepoPath(ctx *e2e.Context, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ctx.Config.RepoRoot, path)
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func changedRepoPaths(repoRoot string) ([]string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, nil
	}
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status changed files: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if renamed := strings.Split(path, " -> "); len(renamed) == 2 {
			path = renamed[1]
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	return paths, nil
}

func splitPathSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(value, "\n") {
		if path := strings.TrimSpace(line); path != "" {
			out[path] = true
		}
	}
	return out
}

func sectionHasBullet(text, heading string) bool {
	idx := strings.Index(text, heading)
	if idx < 0 {
		return false
	}
	rest := text[idx+len(heading):]
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			return true
		}
	}
	return false
}
