package build

import (
	"context"
	"path/filepath"
	"time"
)

func ShouldRunStep(
	state State,
	step Step,
	inputHash string,
	missingOutput bool,
	outputDrift bool,
	dependencyChanged bool,
	force ForceSet,
) StepDecision {
	if force.Has(step.ForceTarget) {
		return DecisionRunForced
	}
	if missingOutput || outputDrift {
		return DecisionRunMissingOutput
	}
	if dependencyChanged {
		return DecisionRunDependencyChanged
	}
	if state.Steps[string(step.Name)].InputHash != inputHash {
		return DecisionRunInputChanged
	}
	return DecisionSkipped
}

func ExecuteStep(
	ctx context.Context,
	state *State,
	step Step,
	dirtyDeps map[StepName]bool,
	opts Options,
	hasher Hasher,
	runner Runner,
) (StepResult, error) {
	prereqOK, err := hasher.Exists(ctx, step.Prerequisites)
	if err != nil {
		return StepResult{}, err
	}
	if len(step.Prerequisites) > 0 && !prereqOK {
		return StepResult{Name: step.Name, Decision: DecisionSkippedUnavailable}, nil
	}

	inputHash, err := hasher.Hash(ctx, step.Inputs)
	if err != nil {
		return StepResult{}, err
	}
	beforeOutput, err := hasher.Hash(ctx, step.Outputs)
	if err != nil {
		return StepResult{}, err
	}
	missingOutput, err := missingRequiredOutput(ctx, hasher, step.Required)
	if err != nil {
		return StepResult{}, err
	}

	beforeRestartInput, err := hashWhenConfigured(ctx, hasher, step.RestartInputs)
	if err != nil {
		return StepResult{}, err
	}
	beforeRestartOutput, err := hashWhenConfigured(ctx, hasher, step.RestartOutputs)
	if err != nil {
		return StepResult{}, err
	}

	dependencyChanged := false
	for _, dep := range step.DependsOn {
		dependencyChanged = dependencyChanged || dirtyDeps[dep]
	}

	previous := state.Steps[string(step.Name)]
	outputDrift := previous.OutputHash != "" && previous.OutputHash != beforeOutput
	decision := ShouldRunStep(
		*state,
		step,
		inputHash,
		missingOutput,
		outputDrift,
		dependencyChanged,
		opts.Force,
	)
	result := StepResult{
		Name:           step.Name,
		Decision:       decision,
		PreviousInput:  previous.InputHash,
		CurrentInput:   inputHash,
		PreviousOutput: previous.OutputHash,
		BeforeOutput:   beforeOutput,
		AfterOutput:    beforeOutput,
	}
	if decision == DecisionSkipped {
		if len(step.RestartInputs.Roots) > 0 && previous.RestartInputHash == "" {
			state.Steps[string(step.Name)] = StepState{
				InputHash:        previous.InputHash,
				OutputHash:       previous.OutputHash,
				RestartInputHash: beforeRestartInput,
			}
		}
		return result, nil
	}

	if err := runner.Run(ctx, step.Command); err != nil {
		return result, err
	}
	afterOutput, err := hasher.Hash(ctx, step.Outputs)
	if err != nil {
		return StepResult{}, err
	}
	afterRestartInput, err := hashWhenConfigured(ctx, hasher, step.RestartInputs)
	if err != nil {
		return StepResult{}, err
	}
	afterRestartOutput, err := hashWhenConfigured(ctx, hasher, step.RestartOutputs)
	if err != nil {
		return StepResult{}, err
	}

	result.AfterOutput = afterOutput
	result.OutputChanged = beforeOutput != afterOutput
	result.RestartInputChanged = len(step.RestartInputs.Roots) > 0 &&
		((previous.RestartInputHash != "" && previous.RestartInputHash != afterRestartInput) ||
			beforeRestartInput != afterRestartInput)
	result.RestartOutputChanged = len(step.RestartOutputs.Roots) > 0 &&
		beforeRestartOutput != afterRestartOutput
	state.Steps[string(step.Name)] = StepState{
		InputHash:        inputHash,
		OutputHash:       afterOutput,
		RestartInputHash: afterRestartInput,
	}
	return result, nil
}

func missingRequiredOutput(
	ctx context.Context,
	hasher Hasher,
	required []RequiredPath,
) (bool, error) {
	if len(required) == 0 {
		return false, nil
	}
	exists, err := hasher.Exists(ctx, required)
	return !exists, err
}

func hashWhenConfigured(
	ctx context.Context,
	hasher Hasher,
	spec HashSpec,
) (string, error) {
	if len(spec.Roots) == 0 {
		return "", nil
	}
	return hasher.Hash(ctx, spec)
}

func Run(ctx context.Context, opts Options) error {
	if opts.StateDir == "" {
		opts.StateDir = ".build-agents"
	}
	stateDir := filepath.Join(opts.RepoRoot, opts.StateDir)
	lock := NewFileLock(
		filepath.Join(stateDir, "build.lock"),
		filepath.Join(stateDir, "build.lock.json"),
	)
	store := NewFileStateStore(filepath.Join(stateDir, "state.json"))
	hasher := NewTreeHasher(opts.RepoRoot)
	runner := NewExecRunner(opts.RepoRoot, opts.Stdout, opts.Stderr)
	return runWithDeps(ctx, opts, lock, store, hasher, runner)
}

func runWithDeps(
	ctx context.Context,
	opts Options,
	lock Lock,
	store StateStore,
	hasher Hasher,
	runner Runner,
) error {
	release, err := lock.Acquire(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	reporter := opts.Reporter
	if reporter == nil {
		reporter = NewTextReporter(opts.Stdout)
	}
	reporter.LockAcquired()
	services, err := NewPlatformServiceManager(
		ctx,
		"",
		runner,
		nil,
		opts.Stdout,
		opts.Stderr,
	)
	if err != nil {
		return err
	}
	_, _, err = runLocked(ctx, opts, store, hasher, runner, reporter, services)
	return err
}

func runLocked(
	ctx context.Context,
	opts Options,
	store StateStore,
	hasher Hasher,
	runner Runner,
	reporter Reporter,
	services ServiceManager,
) (State, []StepResult, error) {
	state, err := store.Load(ctx)
	if err != nil {
		return State{}, nil, err
	}
	if opts.Clean {
		if err := store.Clean(ctx); err != nil {
			return State{}, nil, err
		}
		pending := state.PendingRestarts
		state = DefaultState(opts.RepoRoot)
		state.PendingRestarts = pending
	}

	state.LastRun.StartedAt = time.Now()
	state.LastRun.FinishedAt = time.Time{}
	state.LastRun.Success = false

	dirtyDeps := map[StepName]bool{}
	results := make([]StepResult, 0, len(BuildSteps(opts)))
	checkoutPath := findCheckoutRoot(opts.RepoRoot)
	for _, step := range BuildSteps(opts) {
		result, err := ExecuteStep(ctx, &state, step, dirtyDeps, opts, hasher, runner)
		if err != nil {
			logPath, logErr := WriteWorkspaceBuildLog(checkoutPath, err.Error()+"\n")
			if logErr == nil {
				_ = WriteWorkspaceBuildStatus(ctx, checkoutPath, WorkspaceBuildStatus{
					LastFailedAt: time.Now(),
					Error:        err.Error(),
					LogPath:      logPath,
				})
			}
			_ = store.Save(ctx, state)
			return state, results, err
		}
		dirtyDeps[step.Name] = result.OutputChanged
		results = append(results, result)
		if reporter != nil {
			reporter.Step(result)
		}
		if err := store.Save(ctx, state); err != nil {
			return state, results, err
		}
	}

	restartPlan := ComputeRestartPlan(state, results)
	if err := ApplyRestartPlanWithOptions(
		ctx,
		&state,
		restartPlan,
		RestartOptions{
			NoRestart:    opts.NoRestart,
			Services:     services,
			Save:         store.Save,
			CheckoutPath: checkoutPath,
		},
	); err != nil {
		_ = store.Save(ctx, state)
		return state, results, err
	}
	if reporter != nil {
		reporter.Restart(restartPlan, opts.NoRestart)
	}

	finishedAt := time.Now()
	state.LastRun.FinishedAt = finishedAt
	state.LastRun.Success = true
	_ = WriteWorkspaceBuildStatus(ctx, checkoutPath, WorkspaceBuildStatus{
		LastSuccessAt: finishedAt,
		LogPath:       workspaceRuntimePaths(checkoutPath).buildLog,
	})
	if err := store.Save(ctx, state); err != nil {
		return state, results, err
	}
	if reporter != nil {
		reporter.Done(state.LastRun)
	}
	return state, results, nil
}
