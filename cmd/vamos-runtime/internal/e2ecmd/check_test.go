package e2ecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckValidatesStoryDir(t *testing.T) {
	dir := t.TempDir()
	storyPath := filepath.Join(dir, "thoughts-workbench.story.md")
	if err := os.WriteFile(storyPath, []byte(`# Feature: Thoughts workbench

## Scenario: Root opens document workbench with Chat

### Given
- I am authenticated as "tester@example.com".
- Fixture "thoughts-workbench.basic" is loaded.

### When
- I visit "/".
- I wait for feature "thoughts.workbench" to be ready.

### Then
- Region "thoughts.workbench.sidebar" is visible.
- Tab "thoughts.rightRail.chat" is selected.
- Text "Session history" is absent.
`), 0o644); err != nil {
		t.Fatalf("write story: %v", err)
	}

	var stdout bytes.Buffer
	err := RunCheck(context.Background(), CheckConfig{StoryDir: dir, Stdout: &stdout})
	if err != nil {
		t.Fatalf("RunCheck() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "validated 1 story features") {
		t.Fatalf("stdout=%q, want validated count", got)
	}
}

func TestCheckCommandRunsOnDefaultStories(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"check", "--story-dir", filepath.Join("..", "..", "..", "..", "docs", "features")})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
}
