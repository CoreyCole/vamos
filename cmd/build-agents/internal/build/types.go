package build

import "context"

type Step struct {
	Name           StepName
	ForceTarget    ForceTarget
	Inputs         HashSpec
	Outputs        HashSpec
	RestartInputs  HashSpec
	RestartOutputs HashSpec
	Required       []RequiredPath
	Prerequisites  []RequiredPath
	Command        CommandSpec
	DependsOn      []StepName
}

type StepResult struct {
	Name                 StepName
	Decision             StepDecision
	PreviousInput        string
	CurrentInput         string
	PreviousOutput       string
	BeforeOutput         string
	AfterOutput          string
	OutputChanged        bool
	RestartInputChanged  bool
	RestartOutputChanged bool
}

type StepDecision string

const (
	DecisionRunForced            StepDecision = "run_forced"
	DecisionRunInputChanged      StepDecision = "run_input_changed"
	DecisionRunMissingOutput     StepDecision = "run_missing_output"
	DecisionRunDependencyChanged StepDecision = "run_dependency_changed"
	DecisionSkippedUnavailable   StepDecision = "skipped_unavailable"
	DecisionSkipped              StepDecision = "skipped"
)

type HashSpec struct {
	Roots    []string
	Includes []string
	Excludes []string
	Optional bool
}

type RequiredPath struct {
	Path string
	Glob bool
}

type Hasher interface {
	Hash(ctx context.Context, spec HashSpec) (string, error)
	Exists(ctx context.Context, required []RequiredPath) (bool, error)
}

type CommandFunc func(ctx context.Context, repoRoot string, runner Runner) error

type CommandSpec struct {
	Dir   string
	Env   map[string]string
	Args  []string
	Func  CommandFunc
	Quiet bool
}

type Runner interface {
	Run(ctx context.Context, spec CommandSpec) error
}
