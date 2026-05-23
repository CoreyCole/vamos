package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var datastarAssetsWarningWriter io.Writer = os.Stderr

type DatastarAssetsOptions struct {
	RuntimeAsset string
	HostAsset    string
}

func SyncDatastarAssets(opts DatastarAssetsOptions) CommandFunc {
	return func(ctx context.Context, repoRoot string, runner Runner) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		runtimeAsset := filepath.Join(repoRoot, filepath.FromSlash(opts.RuntimeAsset))
		hostAsset := filepath.Join(repoRoot, filepath.FromSlash(opts.HostAsset))

		hostBytes, hostErr := os.ReadFile(hostAsset)
		if hostErr != nil && !os.IsNotExist(hostErr) {
			return fmt.Errorf("read Datastar host asset %s: %w", opts.HostAsset, hostErr)
		}

		runtimeBytes, runtimeErr := os.ReadFile(runtimeAsset)
		if runtimeErr != nil && !os.IsNotExist(runtimeErr) {
			return fmt.Errorf(
				"read Datastar runtime asset %s: %w",
				opts.RuntimeAsset,
				runtimeErr,
			)
		}

		if hostErr == nil {
			if runtimeErr == nil && bytes.Equal(runtimeBytes, hostBytes) {
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(runtimeAsset), 0o755); err != nil {
				return fmt.Errorf("create Datastar runtime asset dir: %w", err)
			}
			if err := os.WriteFile(runtimeAsset, hostBytes, 0o644); err != nil {
				return fmt.Errorf(
					"write Datastar runtime asset %s: %w",
					opts.RuntimeAsset,
					err,
				)
			}
			return nil
		}

		if runtimeErr == nil {
			return nil
		}

		_, _ = fmt.Fprintf(
			datastarAssetsWarningWriter,
			"WARNING: missing Datastar Pro JS asset; install licensed asset at %s or provide host asset at %s\n",
			opts.RuntimeAsset,
			opts.HostAsset,
		)
		return nil
	}
}
