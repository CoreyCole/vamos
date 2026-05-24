package artifacts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PlanBundleOptions struct {
	PlanDir      string
	RunDir       string
	Command      string
	IncludeHTML  bool
	IncludeTrace bool
}

type PlanBundle struct {
	SummaryPath     string
	IndexPath       string
	ScreenshotPaths []string
}

func ExportPlanBundle(
	ctx context.Context,
	manifest RunManifest,
	opts PlanBundleOptions,
) (PlanBundle, error) {
	_ = ctx
	planDir, err := safePlanDir(opts.PlanDir)
	if err != nil {
		return PlanBundle{}, err
	}
	target := filepath.Join(planDir, "context", "implement", "e2e-runs", manifest.ID)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return PlanBundle{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# E2E Run %s\n\n", manifest.ID)
	if opts.Command != "" {
		fmt.Fprintf(&b, "- Command: `%s`\n", opts.Command)
	}
	fmt.Fprintf(&b, "- Run dir: `%s`\n", opts.RunDir)
	fmt.Fprintf(&b, "- Manifest: [`manifest.json`](manifest.json)\n")
	if manifest.FailuresPath != "" {
		fmt.Fprintf(&b, "- Failures: [`failures.json`](failures.json)\n")
	}
	fmt.Fprintf(&b, "- Stories: %d\n", len(manifest.Stories))
	fmt.Fprintf(&b, "- Screenshots: %d\n", len(manifest.Screenshots))
	fmt.Fprintf(&b, "- HTML snapshots: %d\n", len(manifest.HTMLSnapshots))
	fmt.Fprintf(&b, "- Traces: %d\n\n", len(manifest.Traces))
	copiedScreenshots := []string{}
	for _, shot := range manifest.Screenshots {
		base := filepath.Base(shot)
		dst := filepath.Join(target, "screenshots", base)
		if err := copyFile(shot, dst); err != nil {
			return PlanBundle{}, err
		}
		copiedScreenshots = append(copiedScreenshots, dst)
		fmt.Fprintf(&b, "![%s](screenshots/%s)\n\n", base, base)
	}
	if opts.IncludeHTML {
		for _, snap := range manifest.HTMLSnapshots {
			base := filepath.Base(snap)
			if err := copyFile(snap, filepath.Join(target, "html", base)); err != nil {
				return PlanBundle{}, err
			}
			fmt.Fprintf(&b, "- HTML snapshot: [`%s`](html/%s)\n", base, base)
		}
	}
	if opts.IncludeTrace {
		for _, trace := range manifest.Traces {
			fmt.Fprintf(&b, "- Trace: `%s`\n", trace)
		}
	}
	if err := writeJSONFile(filepath.Join(target, "manifest.json"), manifest); err != nil {
		return PlanBundle{}, err
	}
	if manifest.FailuresPath != "" {
		if err := copyFile(manifest.FailuresPath, filepath.Join(target, "failures.json")); err != nil {
			return PlanBundle{}, err
		}
	}
	if opts.RunDir != "" {
		reportPath := filepath.Join(opts.RunDir, "report.md")
		if _, err := os.Stat(reportPath); err == nil {
			if err := copyFile(reportPath, filepath.Join(target, "report.md")); err != nil {
				return PlanBundle{}, err
			}
			fmt.Fprintln(&b, "- Report: [`report.md`](report.md)")
		}
	}
	indexPath := filepath.Join(target, "index.md")
	if err := os.WriteFile(indexPath, []byte(b.String()), 0o644); err != nil {
		return PlanBundle{}, err
	}
	return PlanBundle{
		IndexPath:       indexPath,
		SummaryPath:     filepath.Join(target, "report.md"),
		ScreenshotPaths: copiedScreenshots,
	}, nil
}

func safePlanDir(planDir string) (string, error) {
	if planDir == "" {
		return "", fmt.Errorf("plan dir is required")
	}
	clean := filepath.Clean(planDir)
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("plan dir must point at a QRSPI plan")
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("plan dir must not contain '..': %s", planDir)
		}
	}
	if !filepath.IsAbs(clean) && parts[0] != "thoughts" {
		return "", fmt.Errorf("plan dir must be under thoughts/: %s", planDir)
	}
	return clean, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
