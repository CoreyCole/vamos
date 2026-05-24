package story

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileParsesScenarioSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thoughts-workbench.story.md")
	writeStory(t, path, `# Feature: Thoughts workbench

## User story
As an internal user, I want Thoughts.

## Business rules
- Chat is available by default.

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
`)

	feature, err := ParseFile(path, ParseOptions{Strict: true})
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := feature.Slug, "thoughts-workbench"; got != want {
		t.Fatalf("slug=%q want %q", got, want)
	}
	if got, want := feature.Scenarios[0].Then[0].Verb, StepVerb(
		"expect_region_visible",
	); got != want {
		t.Fatalf("verb=%s want %s", got, want)
	}
	if got, want := feature.Scenarios[0].When[0].Args["path"], "/"; got != want {
		t.Fatalf("path=%q want %q", got, want)
	}
}

func TestParseFileRejectsUnsupportedStep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.story.md")
	writeStory(t, path, `# Feature: Bad

## Scenario: Bad

### Then
- Something unsupported happens.
`)

	if _, err := ParseFile(path, ParseOptions{Strict: true}); err == nil {
		t.Fatal("ParseFile() error = nil, want unsupported step error")
	}
}

func writeStory(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write story: %v", err)
	}
}
