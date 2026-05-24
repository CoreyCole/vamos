package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMarkdownContainsFrontmatterAndClassification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e-visual.md")
	input := VisualReviewInput{
		RunManifestPath: "runs/run-1/manifest.json",
		BaselineRef:     "main",
		BaselineCommit:  "abc123",
		WorkspaceCommit: "def456",
		PlanDir:         "thoughts/plan",
		SkillName:       "e2e-image-review",
	}
	result := VisualReviewResult{
		Verdict: "needs-human-review",
		Classifications: []VisualDifference{{
			Story:          "story",
			Scenario:       "scenario",
			Viewport:       "desktop-full",
			Classification: "needs human decision",
			Rationale:      "semantic review required",
		}},
	}
	if err := WriteMarkdown(path, input, result); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"tool: vamos e2e review",
		"review_type: e2e_visual",
		"verdict: needs-human-review",
		"baseline_commit: abc123",
		"workspace_commit: def456",
		"skill: e2e-image-review",
		"story/scenario/desktop-full: needs human decision",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown missing %q:\n%s", want, body)
		}
	}
}
