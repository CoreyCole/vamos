package story

import (
	"path/filepath"
	"testing"
)

func TestParseAndExpandProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thoughts-workbench.story.md")
	writeStory(t, path, `# Feature: Thoughts workbench

## Scenario: Root opens

### Then
- Text "Session history" is absent.

## Properties

### Workbench regions remain usable across viewport classes
For each viewport:
- mobile
- desktop-half
- desktop-full

For each route:
- "/"
- "/thoughts/example.md?context=chat"

Then:
- Region "thoughts.workbench.sidebar" is reachable.
- Text "Session history" is absent.
`)
	feature, err := ParseFile(path, ParseOptions{Strict: true})
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := len(feature.Properties), 1; got != want {
		t.Fatalf("properties=%d want %d", got, want)
	}
	scenarios, err := ExpandProperties(feature)
	if err != nil {
		t.Fatalf("ExpandProperties() error = %v", err)
	}
	if got, want := len(scenarios), 6; got != want {
		t.Fatalf("expanded scenarios=%d want %d", got, want)
	}
	if got, want := scenarios[0].Viewport, "mobile"; got != want {
		t.Fatalf("viewport=%q want %q", got, want)
	}
	if got, want := scenarios[0].When[0].Args["path"], "/"; got != want {
		t.Fatalf("route=%q want %q", got, want)
	}
}
