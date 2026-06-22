package rootcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRootCommandContainsExpectedSubcommands(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"auth", "chat", "ctl", "e2e", "qrspi"} {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing command %s", name)
		}
	}
}

func TestRootCommandSuppressesUsageAndErrors(t *testing.T) {
	cmd := NewCommand()
	if !cmd.SilenceUsage || !cmd.SilenceErrors {
		t.Fatalf("root silence flags = usage:%v errors:%v", cmd.SilenceUsage, cmd.SilenceErrors)
	}
}

func TestNoLegacyCLIReferences(t *testing.T) {
	root := repoRoot(t)
	legacyName := "agents" + "ctl"
	hits := grepRepo(t, root, []string{"cmd", "pkg", "docs", "justfile", "scripts"}, legacyName)
	if len(hits) > 0 {
		t.Fatalf("legacy CLI references:\n%s", strings.Join(hits, "\n"))
	}
}

func TestNoStaticPlaywrightTokenInstructions(t *testing.T) {
	root := repoRoot(t)
	for _, needle := range []string{"playwright-auth.sh", "workspace-verify-playwright"} {
		if hits := grepRepo(t, root, []string{"docs", "justfile", "scripts"}, needle); len(hits) > 0 {
			t.Fatalf("legacy Playwright helper references for %q:\n%s", needle, strings.Join(hits, "\n"))
		}
	}

	hits := grepRepo(t, root, []string{"docs", "justfile", "scripts"}, "VAMOS_PLAYWRIGHT_AUTH_TOKEN")
	var bad []string
	for _, hit := range hits {
		if !strings.Contains(hit, "vamos auth playwright-env") {
			bad = append(bad, hit)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("static Playwright token instructions:\n%s", strings.Join(bad, "\n"))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func grepRepo(t *testing.T, root string, paths []string, needle string) []string {
	t.Helper()
	var hits []string
	for _, rel := range paths {
		start := filepath.Join(root, rel)
		info, err := os.Stat(start)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		if !info.IsDir() {
			appendFileHits(t, root, start, needle, &hits)
			continue
		}
		if err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "thoughts" {
					return filepath.SkipDir
				}
				return nil
			}
			appendFileHits(t, root, path, needle, &hits)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return hits
}

func appendFileHits(t *testing.T, root, path, needle string, hits *[]string) {
	t.Helper()
	rel, _ := filepath.Rel(root, path)
	if rel == "cmd/vamos-runtime/internal/rootcmd/root_test.go" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return
	}
	for idx, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, needle) {
			*hits = append(*hits, rel+":"+strconv.Itoa(idx+1)+":"+line)
		}
	}
}
