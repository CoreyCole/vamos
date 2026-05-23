package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

type PlatformID string

const (
	PlatformLinux  PlatformID = "linux"
	PlatformDarwin PlatformID = "darwin"
)

type CommandOutput func(ctx context.Context, spec CommandSpec) (string, error)

type PlatformServiceManager struct {
	ID      PlatformID
	Manager ServiceManager
}

func NewPlatformServiceManager(
	ctx context.Context,
	goos string,
	runner Runner,
	output CommandOutput,
	stdout, stderr io.Writer,
) (ServiceManager, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	if output == nil {
		output = defaultCommandOutput
	}

	switch goos {
	case string(PlatformLinux):
		if err := RequireLinuxSystemd(ctx, runner); err != nil {
			return nil, err
		}
		return NewSystemdServiceManager(runner, output, stdout, stderr), nil
	case string(PlatformDarwin):
		uid := os.Getuid()
		if err := RequireLaunchd(ctx, runner, uid); err != nil {
			return nil, err
		}
		return NewLaunchdServiceManager(runner, output, stdout, stderr, uid), nil
	default:
		return nil, fmt.Errorf(
			"unsupported smart build platform %q; supported platforms: linux/systemd, darwin/launchd",
			goos,
		)
	}
}

func defaultCommandOutput(ctx context.Context, spec CommandSpec) (string, error) {
	if spec.Func != nil {
		return "", errors.New("command output does not support function commands")
	}
	if len(spec.Args) == 0 {
		return "", errors.New("empty command")
	}
	// #nosec G204 -- service probe commands are fixed specs defined by this package.
	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Env = os.Environ()
	for key, value := range spec.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("run %v: %w", spec.Args, err)
	}
	return string(output), nil
}
