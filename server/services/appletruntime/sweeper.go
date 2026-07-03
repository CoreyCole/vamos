package appletruntime

import (
	"context"
	"time"
)

// SweepOptions configures the periodic idle-app sweep.
type SweepOptions struct {
	Interval time.Duration
	Logger   interface{ Printf(string, ...any) }
}

// StartAppletSweeper periodically stops idle healthy applets until ctx is done.
func StartAppletSweeper(ctx context.Context, manager Manager, opts SweepOptions) {
	if manager == nil {
		return
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			stopped, err := manager.SweepInactive(ctx, now)
			if opts.Logger != nil && (len(stopped) > 0 || err != nil) {
				opts.Logger.Printf("applet_sweeper stopped=%d err=%v", len(stopped), err)
			}
		}
	}
}
