package review

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func WriteMarkdown(
	path string,
	input VisualReviewInput,
	result VisualReviewResult,
) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`---
date: %s
tool: vamos e2e review
review_type: e2e_visual
status: complete
verdict: %s
baseline: %s
baseline_commit: %s
workspace_commit: %s
suite_run: %s
skill: %s
---

# E2E Visual Review

## Summary

Verdict: %s

## Inputs

- Run manifest: %s
- Plan dir: %s

## Classifications
`, time.Now().Format(time.RFC3339), result.Verdict, input.BaselineRef, input.BaselineCommit, input.WorkspaceCommit, input.RunManifestPath, input.SkillName, result.Verdict, input.RunManifestPath, input.PlanDir)
	for _, diff := range result.Classifications {
		label := diff.Classification
		if diff.Story != "" || diff.Scenario != "" || diff.Viewport != "" {
			label = fmt.Sprintf(
				"%s/%s/%s: %s",
				diff.Story,
				diff.Scenario,
				diff.Viewport,
				diff.Classification,
			)
		}
		body += fmt.Sprintf("- %s — %s\n", label, diff.Rationale)
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
