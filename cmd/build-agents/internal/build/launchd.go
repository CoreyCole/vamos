package build

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type LaunchdServiceManager struct {
	runner         Runner
	uid            int
	labels         map[ServiceName]string
	output         CommandOutput
	stdout, stderr io.Writer
}

func NewLaunchdServiceManager(
	runner Runner,
	output CommandOutput,
	stdout, stderr io.Writer,
	uid int,
) *LaunchdServiceManager {
	if output == nil {
		output = defaultCommandOutput
	}
	return &LaunchdServiceManager{
		runner: runner,
		uid:    uid,
		labels: map[ServiceName]string{
			ServiceWeb:      "dev.vamos",
			ServiceTemporal: "dev.chestnut.temporal-server",
			ServiceTSWorker: "dev.vamos-ts-worker",
		},
		output: output,
		stdout: stdout,
		stderr: stderr,
	}
}

func (m *LaunchdServiceManager) label(service ServiceName) string {
	if label, ok := m.labels[service]; ok {
		return label
	}
	return string(service)
}

func launchdTarget(uid int, label string) string {
	return fmt.Sprintf("gui/%d/%s", uid, label)
}

func (m *LaunchdServiceManager) IsInstalled(
	ctx context.Context,
	service ServiceName,
) (bool, error) {
	output, err := m.output(ctx, CommandSpec{
		Args:  []string{"launchctl", "print", launchdTarget(m.uid, m.label(service))},
		Quiet: true,
	})
	if err != nil {
		if strings.Contains(output, "Could not find service") {
			return false, nil
		}
		return false, fmt.Errorf("probe launchd service %s: %w", service, err)
	}
	return true, nil
}

func (m *LaunchdServiceManager) Restart(ctx context.Context, service ServiceName) error {
	return m.runner.Run(ctx, CommandSpec{
		Args: []string{
			"launchctl",
			"kickstart",
			"-k",
			launchdTarget(m.uid, m.label(service)),
		},
	})
}

func (m *LaunchdServiceManager) EnsureRunning(
	ctx context.Context,
	service ServiceName,
) error {
	return m.runner.Run(ctx, CommandSpec{
		Args: []string{"launchctl", "kickstart", launchdTarget(m.uid, m.label(service))},
	})
}

func (m *LaunchdServiceManager) MissingServiceHint(service ServiceName) string {
	label := m.label(service)
	return fmt.Sprintf(
		"%s launchd service is not installed; install/update the LaunchAgent for this checkout, then load it with `launchctl bootstrap gui/%d ~/Library/LaunchAgents/%s.plist`",
		label,
		m.uid,
		label,
	)
}
