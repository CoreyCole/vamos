package e2ecmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/e2e/goldens"
)

type GoldensConfig struct {
	RunDir        string
	GoldenRoot    string
	HumanApproved bool
}

func RunGoldensCapture(ctx context.Context, cfg GoldensConfig) error {
	if strings.TrimSpace(cfg.RunDir) == "" {
		return fmt.Errorf("--run is required")
	}
	manifest, err := goldens.LoadManifest(cfg.RunDir)
	if err != nil {
		return err
	}
	return goldens.Capture(
		ctx,
		manifest,
		goldens.CaptureOptions{GoldenRoot: cfg.GoldenRoot},
	)
}

func RunGoldensAccept(ctx context.Context, cfg GoldensConfig) error {
	if strings.TrimSpace(cfg.RunDir) == "" {
		return fmt.Errorf("--run is required")
	}
	manifest, err := goldens.LoadManifest(cfg.RunDir)
	if err != nil {
		return err
	}
	return goldens.Accept(ctx, manifest, goldens.AcceptOptions{
		GoldenRoot:    cfg.GoldenRoot,
		HumanApproved: cfg.HumanApproved,
	})
}
