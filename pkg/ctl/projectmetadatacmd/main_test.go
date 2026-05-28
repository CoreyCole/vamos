package projectmetadatacmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateMarkdownDryRunDoesNotWrite(t *testing.T) {
	path := writeTempMarkdown(t, `---
stage: plan
repository: vamos
plan_dir: thoughts/example/plans/demo
---
# Body
`)
	changed, err := migrateMarkdown(path, options{
		fromRepository: "vamos",
		toProject:      "github.com/CoreyCole/vamos",
		write:          false,
	})
	if err != nil {
		t.Fatalf("migrateMarkdown() error = %v", err)
	}
	if !changed {
		t.Fatal("migrateMarkdown() changed = false, want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "repository: vamos") {
		t.Fatalf("dry-run rewrote file: %s", data)
	}
}

func TestMigrateMarkdownWritesQRSPIProject(t *testing.T) {
	path := writeTempMarkdown(t, `---
stage: plan
repository: vamos
plan_dir: thoughts/example/plans/demo
---
# Body

Keep me.
`)
	changed, err := migrateMarkdown(path, options{
		fromRepository: "vamos",
		toProject:      "github.com/CoreyCole/vamos",
		write:          true,
	})
	if err != nil {
		t.Fatalf("migrateMarkdown() error = %v", err)
	}
	if !changed {
		t.Fatal("migrateMarkdown() changed = false, want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "repository:") {
		t.Fatalf("repository was not removed: %s", text)
	}
	if !strings.Contains(text, "project: github.com/CoreyCole/vamos") {
		t.Fatalf("project was not added: %s", text)
	}
	if !strings.Contains(text, "# Body\n\nKeep me.\n") {
		t.Fatalf("body was not preserved: %s", text)
	}
}

func TestMigrateMarkdownSkipsNonQRSPI(t *testing.T) {
	path := writeTempMarkdown(t, `---
repository: vamos
topic: docs
---
# Body
`)
	changed, err := migrateMarkdown(path, options{
		fromRepository: "vamos",
		toProject:      "github.com/CoreyCole/vamos",
		write:          true,
	})
	if err != nil {
		t.Fatalf("migrateMarkdown() error = %v", err)
	}
	if changed {
		t.Fatal("migrateMarkdown() changed = true, want false")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "repository: vamos") || strings.Contains(string(data), "project:") {
		t.Fatalf("non-QRSPI file was rewritten: %s", data)
	}
}

func writeTempMarkdown(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
