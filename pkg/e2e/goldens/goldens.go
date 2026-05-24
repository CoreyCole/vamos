package goldens

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CoreyCole/vamos/pkg/e2e/artifacts"
)

type CaptureOptions struct {
	GoldenRoot string
}

type AcceptOptions struct {
	GoldenRoot    string
	HumanApproved bool
}

type GoldenLocator interface {
	MainCheckout(ctx context.Context) (string, error)
	GoldenPath(story, scenario, viewport string) string
}

type Locator struct {
	Root string
}

func (l Locator) MainCheckout(ctx context.Context) (string, error) {
	if l.Root == "" {
		return "", fmt.Errorf("golden root is empty")
	}
	return l.Root, nil
}

func (l Locator) GoldenPath(story, scenario, viewport string) string {
	return filepath.Join(
		l.Root,
		"pkg",
		"e2e",
		"goldens",
		story,
		scenario+"."+viewport+".png",
	)
}

func LoadManifest(runDir string) (artifacts.RunManifest, error) {
	for _, name := range []string{"manifest.json", "run.json"} {
		path := filepath.Join(runDir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			var manifest artifacts.RunManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return artifacts.RunManifest{}, fmt.Errorf("read %s: %w", path, err)
			}
			return manifest, nil
		}
		if !os.IsNotExist(err) {
			return artifacts.RunManifest{}, err
		}
	}
	return artifacts.RunManifest{}, fmt.Errorf("run manifest missing in %s", runDir)
}

func Capture(ctx context.Context, run artifacts.RunManifest, opts CaptureOptions) error {
	return copyScreenshots(run, opts.GoldenRoot, false)
}

func Accept(ctx context.Context, run artifacts.RunManifest, opts AcceptOptions) error {
	if !opts.HumanApproved {
		return fmt.Errorf("goldens accept requires --human-approved")
	}
	return copyScreenshots(run, opts.GoldenRoot, true)
}

func copyScreenshots(run artifacts.RunManifest, root string, overwrite bool) error {
	if root == "" {
		return fmt.Errorf("golden root is empty")
	}
	for _, src := range run.Screenshots {
		dst := goldenDestination(root, src, run.ID)
		if !overwrite {
			if _, err := os.Stat(dst); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func goldenDestination(root, src, runID string) string {
	parts := splitPath(src)
	for i, part := range parts {
		if part == runID && len(parts) >= i+5 {
			story := parts[i+1]
			scenario := parts[i+2]
			viewport := parts[i+3]
			return filepath.Join(root, story, scenario+"."+viewport+filepath.Ext(src))
		}
	}
	return filepath.Join(root, filepath.Base(src))
}

func splitPath(path string) []string {
	clean := filepath.Clean(path)
	parts := []string{}
	for {
		dir, file := filepath.Split(clean)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		dir = filepath.Clean(dir)
		if dir == "." || dir == string(filepath.Separator) || dir == "" {
			break
		}
		clean = dir
	}
	return parts
}
