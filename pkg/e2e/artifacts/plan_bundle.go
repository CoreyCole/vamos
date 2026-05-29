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
	if opts.RunDir != "" {
		staticIndexPath := filepath.Join(opts.RunDir, "index.html")
		if _, err := os.Stat(staticIndexPath); err == nil {
			if err := copyFile(staticIndexPath, filepath.Join(target, "index.html")); err != nil {
				return PlanBundle{}, err
			}
			fmt.Fprintf(&b, "- Static index: [`index.html`](index.html)\n")
		}
	}
	fmt.Fprintln(&b)

	copiedScreenshots := []string{}
	if len(manifest.Artifacts) > 0 {
		copied, err := copyStructuredArtifacts(opts.RunDir, target, manifest.Artifacts)
		if err != nil {
			return PlanBundle{}, err
		}
		for _, dst := range copied {
			if strings.EqualFold(filepath.Ext(dst), ".png") || strings.EqualFold(filepath.Ext(dst), ".jpg") || strings.EqualFold(filepath.Ext(dst), ".jpeg") || strings.EqualFold(filepath.Ext(dst), ".webp") {
				copiedScreenshots = append(copiedScreenshots, dst)
			}
		}
		fmt.Fprintln(&b, "## Artifacts")
		fmt.Fprintln(&b)
		for _, entry := range manifest.Artifacts {
			label := strings.TrimSpace(fmt.Sprintf("%s / %s / %s / %s", entry.FeatureSlug, entry.ScenarioSlug, entry.Viewport, entry.Kind))
			fmt.Fprintf(&b, "- %s: [`%s`](%s)\n", label, filepath.Base(entry.Path), entry.Path)
			if entry.Kind == ArtifactKindScreenshot {
				fmt.Fprintf(&b, "\n![%s / %s / %s](%s)\n\n", entry.FeatureSlug, entry.ScenarioSlug, entry.Viewport, entry.Path)
			}
		}
	} else {
		for _, shot := range manifest.Screenshots {
			rel, ok := preserveRunRelativePath(opts.RunDir, shot)
			if !ok {
				rel = filepath.ToSlash(filepath.Join("screenshots", filepath.Base(shot)))
			}
			dst, err := copyRunArtifactOrAbsolute(opts.RunDir, target, shot, rel)
			if err != nil {
				return PlanBundle{}, err
			}
			copiedScreenshots = append(copiedScreenshots, dst)
			fmt.Fprintf(&b, "![%s](%s)\n\n", filepath.Base(shot), rel)
		}
		if opts.IncludeHTML {
			for _, snap := range manifest.HTMLSnapshots {
				rel, ok := preserveRunRelativePath(opts.RunDir, snap)
				if !ok {
					rel = filepath.ToSlash(filepath.Join("html", filepath.Base(snap)))
				}
				if _, err := copyRunArtifactOrAbsolute(opts.RunDir, target, snap, rel); err != nil {
					return PlanBundle{}, err
				}
				fmt.Fprintf(&b, "- HTML snapshot: [`%s`](%s)\n", filepath.Base(snap), rel)
			}
		}
		if opts.IncludeTrace {
			for _, trace := range manifest.Traces {
				fmt.Fprintf(&b, "- Trace: `%s`\n", trace)
			}
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
	if !planDirHasThoughtsRoot(parts) {
		return "", fmt.Errorf("plan dir must be under thoughts/: %s", planDir)
	}
	return clean, nil
}

func planDirHasThoughtsRoot(parts []string) bool {
	for _, part := range parts {
		if part == "thoughts" {
			return true
		}
	}
	return false
}

func preserveRunRelativePath(runDir, artifactPath string) (string, bool) {
	if runDir == "" || artifactPath == "" {
		return "", false
	}
	rel, err := filepath.Rel(runDir, artifactPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func copyRunArtifact(runDir, target, rel string) (string, error) {
	if runDir == "" {
		return "", fmt.Errorf("run dir is required")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return "", fmt.Errorf("unsafe run artifact path: %s", rel)
	}
	src := filepath.Join(runDir, cleanRel)
	dst := filepath.Join(target, cleanRel)
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyStructuredArtifacts(runDir, target string, entries []ArtifactEntry) ([]string, error) {
	copied := []string{}
	for _, entry := range entries {
		if entry.Path == "" {
			continue
		}
		dst, err := copyRunArtifact(runDir, target, entry.Path)
		if err != nil {
			return nil, err
		}
		copied = append(copied, dst)
	}
	return copied, nil
}

func copyRunArtifactOrAbsolute(runDir, target, absolutePath, rel string) (string, error) {
	if runDir != "" {
		if dst, err := copyRunArtifact(runDir, target, rel); err == nil {
			return dst, nil
		}
	}
	dst := filepath.Join(target, filepath.Clean(filepath.FromSlash(rel)))
	if err := copyFile(absolutePath, dst); err != nil {
		return "", err
	}
	return dst, nil
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
