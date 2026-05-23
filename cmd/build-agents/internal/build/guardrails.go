package build

import (
	"context"
	"fmt"
)

func RequireLinuxSystemd(ctx context.Context, runner Runner) error {
	if err := runner.Run(
		ctx,
		CommandSpec{
			Args:  []string{"systemctl", "--user", "show-environment"},
			Quiet: true,
		},
	); err != nil {
		return fmt.Errorf(
			"systemd user manager unavailable; enable user systemd before running smart build: %w",
			err,
		)
	}
	return nil
}

func RequireLaunchd(ctx context.Context, runner Runner, uid int) error {
	if err := runner.Run(
		ctx,
		CommandSpec{
			Args:  []string{"launchctl", "print", fmt.Sprintf("gui/%d", uid)},
			Quiet: true,
		},
	); err != nil {
		return fmt.Errorf(
			"launchd user manager unavailable; ensure a graphical user launchd session exists before running smart build: %w",
			err,
		)
	}
	return nil
}
