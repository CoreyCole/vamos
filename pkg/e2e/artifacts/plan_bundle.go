package artifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PlanBundleOptions struct {
	PlanDir      string
	RunDir       string
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
	if opts.PlanDir == "" {
		return PlanBundle{}, fmt.Errorf("plan dir is required")
	}
	target := filepath.Join(opts.PlanDir, "context", "implement", "e2e-runs", manifest.ID)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return PlanBundle{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# E2E Run %s\n\n", manifest.ID)
	fmt.Fprintf(&b, "- Stories: %d\n", len(manifest.Stories))
	fmt.Fprintf(&b, "- Screenshots: %d\n", len(manifest.Screenshots))
	fmt.Fprintf(&b, "- HTML snapshots: %d\n\n", len(manifest.HTMLSnapshots))
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
			if err := copyFile(
				snap,
				filepath.Join(target, "html", filepath.Base(snap)),
			); err != nil {
				return PlanBundle{}, err
			}
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
