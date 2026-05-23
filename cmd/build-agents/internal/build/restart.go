package build

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type ServiceName string

const (
	ServiceWeb      ServiceName = "vamos"
	ServiceTemporal ServiceName = "temporal-server"
	ServiceTSWorker ServiceName = "vamos-ts-worker"
)

type RestartPlan struct {
	Web      RestartReason
	TSWorker RestartReason
}

type RestartReason struct {
	Needed        bool
	OutputChanged bool
	InputChanged  bool
	Pending       bool
}

type ServiceManager interface {
	IsInstalled(ctx context.Context, service ServiceName) (bool, error)
	Restart(ctx context.Context, service ServiceName) error
	EnsureRunning(ctx context.Context, service ServiceName) error
	MissingServiceHint(service ServiceName) string
}

type SystemdServiceManager struct {
	runner         Runner
	stdout, stderr io.Writer
	output         CommandOutput
}

func NewSystemdServiceManager(
	runner Runner,
	output CommandOutput,
	stdout, stderr io.Writer,
) *SystemdServiceManager {
	if output == nil {
		output = defaultCommandOutput
	}
	return &SystemdServiceManager{
		runner: runner,
		stdout: stdout,
		stderr: stderr,
		output: output,
	}
}

func ComputeRestartPlan(state State, results []StepResult) RestartPlan {
	plan := RestartPlan{
		Web:      RestartReason{Pending: state.PendingRestarts.Web},
		TSWorker: RestartReason{Pending: state.PendingRestarts.TSWorker},
	}
	for _, result := range results {
		switch result.Name {
		case StepGo, StepTailwind:
			if result.RestartOutputChanged {
				plan.Web.OutputChanged = true
			}
		case StepTSWorker:
			if result.RestartInputChanged {
				plan.TSWorker.InputChanged = true
			}
			if result.RestartOutputChanged {
				plan.TSWorker.OutputChanged = true
			}
		case StepSQLC, StepTempl:
			// Generated-source steps only affect restarts through downstream outputs.
		default:
			// Unknown steps are ignored by restart planning.
		}
	}
	plan.Web.Needed = plan.Web.OutputChanged || plan.Web.Pending
	plan.TSWorker.Needed = plan.TSWorker.OutputChanged ||
		plan.TSWorker.InputChanged ||
		plan.TSWorker.Pending
	return plan
}

type RestartStateSaver func(ctx context.Context, state State) error

func ComponentsFromRestartPlan(plan RestartPlan) []string {
	components := []string{}
	if plan.Web.Needed {
		components = append(components, "web")
	}
	if plan.TSWorker.Needed {
		components = append(components, "ts_worker")
	}
	return components
}

func ApplyRestartPlan(
	ctx context.Context,
	state *State,
	plan RestartPlan,
	noRestart bool,
	services ServiceManager,
	save RestartStateSaver,
) error {
	return ApplyRestartPlanWithOptions(ctx, state, plan, RestartOptions{
		NoRestart: noRestart,
		Services:  services,
		Save:      save,
	})
}

type RestartOptions struct {
	NoRestart    bool
	Services     ServiceManager
	Save         RestartStateSaver
	CheckoutPath string
}

func ApplyRestartPlanWithOptions(
	ctx context.Context,
	state *State,
	plan RestartPlan,
	opts RestartOptions,
) error {
	save := opts.Save
	if save == nil {
		save = func(context.Context, State) error { return nil }
	}

	checkoutPath := opts.CheckoutPath
	if checkoutPath == "" {
		if cwd, err := os.Getwd(); err == nil {
			checkoutPath = findCheckoutRoot(cwd)
		}
	}
	services := opts.Services
	if services == nil {
		services = NewSystemdServiceManager(
			NewExecRunner(checkoutPath, os.Stdout, os.Stderr),
			nil,
			os.Stdout,
			os.Stderr,
		)
	}

	if opts.NoRestart {
		if plan.TSWorker.Needed {
			state.PendingRestarts.TSWorker = true
			if err := save(ctx, *state); err != nil {
				return err
			}
		}
		if plan.Web.Needed {
			state.PendingRestarts.Web = true
			if err := save(ctx, *state); err != nil {
				return err
			}
		}
		return nil
	}

	if plan.Web.Needed || plan.TSWorker.Needed {
		if _, err := os.Stat(workspaceEnvPath(checkoutPath)); err == nil {
			if plan.TSWorker.Needed {
				state.PendingRestarts.TSWorker = true
				if err := save(ctx, *state); err != nil {
					return err
				}
			}
			if plan.Web.Needed {
				state.PendingRestarts.Web = true
				if err := save(ctx, *state); err != nil {
					return err
				}
			}
			result, err := TryWorkspaceRestartWithRecovery(ctx, WorkspaceRestartOptions{
				CheckoutPath: checkoutPath,
				Components:   ComponentsFromRestartPlan(plan),
				Client:       http.DefaultClient,
				Stdout:       os.Stdout,
			})
			if result.Handled {
				if err != nil {
					return err
				}
				if plan.TSWorker.Needed {
					state.PendingRestarts.TSWorker = false
				}
				if plan.Web.Needed {
					state.PendingRestarts.Web = false
				}
				return save(ctx, *state)
			}
		}
	}

	if plan.TSWorker.Needed {
		state.PendingRestarts.TSWorker = true
		if err := save(ctx, *state); err != nil {
			return err
		}
		if err := ensureIfInstalled(ctx, services, ServiceTemporal); err != nil {
			return err
		}
		if err := restartIfInstalled(ctx, services, ServiceTSWorker); err != nil {
			return err
		}
		state.PendingRestarts.TSWorker = false
		if err := save(ctx, *state); err != nil {
			return err
		}
	}
	if plan.Web.Needed {
		state.PendingRestarts.Web = true
		if err := save(ctx, *state); err != nil {
			return err
		}
		if err := restartIfInstalled(ctx, services, ServiceWeb); err != nil {
			return err
		}
		state.PendingRestarts.Web = false
		if err := save(ctx, *state); err != nil {
			return err
		}
	}
	return nil
}

func restartIfInstalled(
	ctx context.Context,
	services ServiceManager,
	service ServiceName,
) error {
	installed, err := services.IsInstalled(ctx, service)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("%s", services.MissingServiceHint(service))
	}
	return services.Restart(ctx, service)
}

func ensureIfInstalled(
	ctx context.Context,
	services ServiceManager,
	service ServiceName,
) error {
	installed, err := services.IsInstalled(ctx, service)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("%s", services.MissingServiceHint(service))
	}
	return services.EnsureRunning(ctx, service)
}

func (m *SystemdServiceManager) label(service ServiceName) string {
	return string(service)
}

func (m *SystemdServiceManager) IsInstalled(
	ctx context.Context,
	service ServiceName,
) (bool, error) {
	loadState, err := m.output(ctx, CommandSpec{
		Args: []string{
			"systemctl",
			"--user",
			"show",
			"-p",
			"LoadState",
			"--value",
			m.label(service),
		},
		Quiet: true,
	})
	if err != nil {
		return false, fmt.Errorf("probe systemd service %s: %w", service, err)
	}
	return strings.TrimSpace(loadState) == "loaded", nil
}

func (m *SystemdServiceManager) Restart(ctx context.Context, service ServiceName) error {
	args := []string{"systemctl", "--user", "restart", m.label(service)}
	if service == ServiceTSWorker {
		args = []string{"systemctl", "--user", "restart", "--no-block", m.label(service)}
	}
	return m.runner.Run(ctx, CommandSpec{Args: args})
}

func (m *SystemdServiceManager) EnsureRunning(
	ctx context.Context,
	service ServiceName,
) error {
	return m.runner.Run(ctx, CommandSpec{
		Args: []string{"systemctl", "--user", "start", m.label(service)},
	})
}

func (m *SystemdServiceManager) MissingServiceHint(service ServiceName) string {
	switch service {
	case ServiceTemporal:
		return "temporal-server user service is not installed; from the host root run `just install-systemd`, then `systemctl --user enable --now temporal-server`"
	case ServiceTSWorker:
		return "vamos-ts-worker user service is not installed; from the host root run `just install-systemd`, then `systemctl --user enable --now temporal-server vamos-ts-worker`"
	case ServiceWeb:
		return "vamos user service is not installed; from the host root run `just install-systemd`, then `systemctl --user enable --now vamos`"
	default:
		return fmt.Sprintf(
			"%s user service is not installed; from the host root run `just install-systemd`",
			service,
		)
	}
}
