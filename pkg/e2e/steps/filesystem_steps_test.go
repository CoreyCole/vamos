package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	e2e "github.com/CoreyCole/vamos/pkg/e2e/runtime"
)

func TestFileHashChangedPiSectionsAndOnlyFileChanged(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	path := filepath.Join(dir, "review.md")
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "review.md")
	runGit(t, dir, "commit", "-m", "seed")
	ctx := &e2e.Context{Config: e2e.Config{RepoRoot: dir}, Memory: map[string]string{}}
	RememberFileHash(t, ctx, path)
	content := `# E2E Pi Plan Docs Review

## Latest E2E Pi Review
Marker: VAMOS_E2E_PLAN_DOCS_REVIEW_OK
Run nonce: 123

### Potential E2E user story updates
- Keep durable chat story strict.

### Potential test implementation updates
- Regenerate tests.

### Potential docs additions/updates/simplifications
- Document q-verify evidence.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ExpectFileHashChanged(t, ctx, path)
	ExpectPiReviewFileSections(t, ctx, path)
	ExpectOnlyFileChanged(t, ctx, path)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
