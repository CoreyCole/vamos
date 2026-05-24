package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WriteManifest(root string, manifest RunManifest) (string, error) {
	manifest.CompletedAt = time.Now().UTC()
	dir := RunDir(root, manifest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func WriteFailures(
	root string,
	manifest RunManifest,
	failures []Failure,
) (string, error) {
	dir := RunDir(root, manifest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(failures, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "failures.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func WriteMarkdownReport(
	root string,
	manifest RunManifest,
	failures []Failure,
) (string, error) {
	dir := RunDir(root, manifest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# E2E Run %s\n\n", manifest.ID)
	fmt.Fprintf(&b, "- Started: %s\n", manifest.StartedAt.Format(time.RFC3339))
	if !manifest.CompletedAt.IsZero() {
		fmt.Fprintf(&b, "- Completed: %s\n", manifest.CompletedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "- Stories: %d\n", len(manifest.Stories))
	fmt.Fprintf(&b, "- Screenshots: %d\n", len(manifest.Screenshots))
	fmt.Fprintf(&b, "- HTML snapshots: %d\n", len(manifest.HTMLSnapshots))
	fmt.Fprintf(&b, "- Failures: %d\n\n", len(failures))
	for _, failure := range failures {
		fmt.Fprintf(
			&b,
			"## Failure: %s / %s\n\n%s\n\n",
			failure.Story,
			failure.Scenario,
			failure.Error,
		)
	}
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
