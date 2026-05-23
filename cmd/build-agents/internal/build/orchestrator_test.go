package build

import (
	"context"
	"testing"
)

func TestShouldRunStep(t *testing.T) {
	t.Parallel()

	step := Step{Name: StepTempl, ForceTarget: ForceTempl}
	state := DefaultState("")
	state.Steps[string(StepTempl)] = StepState{InputHash: "same"}

	tests := []struct {
		name              string
		inputHash         string
		missingOutput     bool
		dependencyChanged bool
		force             ForceSet
		want              StepDecision
	}{
		{
			name:              "forced first",
			inputHash:         "same",
			missingOutput:     true,
			dependencyChanged: true,
			force:             newForceSet(ForceTempl),
			want:              DecisionRunForced,
		},
		{
			name:              "missing output before dependency",
			inputHash:         "same",
			missingOutput:     true,
			dependencyChanged: true,
			want:              DecisionRunMissingOutput,
		},
		{
			name:              "dependency before input",
			inputHash:         "changed",
			dependencyChanged: true,
			want:              DecisionRunDependencyChanged,
		},
		{name: "input changed", inputHash: "changed", want: DecisionRunInputChanged},
		{name: "skipped", inputHash: "same", want: DecisionSkipped},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldRunStep(
				state,
				step,
				test.inputHash,
				test.missingOutput,
				false,
				test.dependencyChanged,
				test.force,
			)
			if got != test.want {
				t.Fatalf("ShouldRunStep = %s, want %s", got, test.want)
			}
		})
	}
}

func TestExecuteStepTargetedForceOnlyForcesNamedStep(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.Steps[string(StepTempl)] = StepState{InputHash: "in"}
	state.Steps[string(StepGo)] = StepState{InputHash: "in"}
	hasher := &fakeHasher{
		hashes: map[string]string{
			"templ-in":  "in",
			"templ-out": "out",
			"go-in":     "in",
			"go-out":    "out",
		},
	}
	runner := &fakeRunner{}
	templStep := Step{
		Name:        StepTempl,
		ForceTarget: ForceTempl,
		Inputs:      namedHashSpec("templ-in"),
		Outputs:     namedHashSpec("templ-out"),
		Command:     CommandSpec{Args: []string{"templ"}},
	}
	goStep := Step{
		Name:        StepGo,
		ForceTarget: ForceGo,
		Inputs:      namedHashSpec("go-in"),
		Outputs:     namedHashSpec("go-out"),
		Command:     CommandSpec{Args: []string{"go"}},
	}
	opts := Options{Force: newForceSet(ForceTempl)}

	templResult, err := ExecuteStep(
		t.Context(),
		&state,
		templStep,
		nil,
		opts,
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep templ: %v", err)
	}
	goResult, err := ExecuteStep(
		t.Context(),
		&state,
		goStep,
		nil,
		opts,
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep go: %v", err)
	}
	if templResult.Decision != DecisionRunForced {
		t.Fatalf("templ decision = %s, want forced", templResult.Decision)
	}
	if goResult.Decision != DecisionSkipped {
		t.Fatalf("go decision = %s, want skipped", goResult.Decision)
	}
	if got, want := len(runner.calls), 1; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
}

func TestExecuteStepOutputDriftRunsStep(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.Steps[string(StepTempl)] = StepState{
		InputHash:  "same-in",
		OutputHash: "old-out",
	}
	hasher := &fakeHasher{hashes: map[string]string{"in": "same-in", "out": "new-out"}}
	runner := &fakeRunner{}
	step := Step{
		Name:        StepTempl,
		ForceTarget: ForceTempl,
		Inputs:      namedHashSpec("in"),
		Outputs:     namedHashSpec("out"),
		Command:     CommandSpec{Args: []string{"templ"}},
	}

	result, err := ExecuteStep(
		t.Context(),
		&state,
		step,
		nil,
		Options{},
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionRunMissingOutput {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionRunMissingOutput)
	}
	if got, want := len(runner.calls), 1; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
}

func TestExecuteStepSkippedUnavailableWhenPrerequisiteAbsent(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	hasher := &fakeHasher{exists: map[string]bool{"node_modules": false}}
	runner := &fakeRunner{}
	result, err := ExecuteStep(
		t.Context(),
		&state,
		TSWorkerStep(),
		nil,
		Options{},
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionSkippedUnavailable {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionSkippedUnavailable)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner was called for unavailable step")
	}
}

func TestExecuteStepByteIdenticalOutputsNotDirty(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.Steps[string(StepTempl)] = StepState{InputHash: "old", OutputHash: "same-out"}
	hasher := &fakeHasher{hashes: map[string]string{"in": "new", "out": "same-out"}}
	runner := &fakeRunner{}
	step := Step{
		Name:        StepTempl,
		ForceTarget: ForceTempl,
		Inputs:      namedHashSpec("in"),
		Outputs:     namedHashSpec("out"),
		Command:     CommandSpec{Args: []string{"templ"}},
	}

	result, err := ExecuteStep(
		t.Context(),
		&state,
		step,
		nil,
		Options{},
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionRunInputChanged {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionRunInputChanged)
	}
	if result.OutputChanged {
		t.Fatal("byte-identical outputs marked dirty")
	}
}

func TestExecuteStepBackfillsRestartInputHashWhenSkipped(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.Steps[string(StepTSWorker)] = StepState{
		InputHash:  "same-input",
		OutputHash: "same-output",
	}
	hasher := &fakeHasher{hashes: map[string]string{
		"input":         "same-input",
		"output":        "same-output",
		"runtime-input": "current-runtime-input",
	}}
	step := Step{
		Name:          StepTSWorker,
		ForceTarget:   ForceTSWorker,
		Inputs:        namedHashSpec("input"),
		Outputs:       namedHashSpec("output"),
		RestartInputs: namedHashSpec("runtime-input"),
	}

	result, err := ExecuteStep(
		t.Context(),
		&state,
		step,
		nil,
		Options{},
		hasher,
		&fakeRunner{},
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionSkipped {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionSkipped)
	}
	if state.Steps[string(StepTSWorker)].RestartInputHash != "current-runtime-input" {
		t.Fatalf(
			"stored restart input hash = %q, want current-runtime-input",
			state.Steps[string(StepTSWorker)].RestartInputHash,
		)
	}
}

func TestExecuteStepDetectsRestartInputChangeFromPreviousState(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.Steps[string(StepTSWorker)] = StepState{
		InputHash:        "old-input",
		OutputHash:       "same-output",
		RestartInputHash: "old-runtime-input",
	}
	hasher := &fakeHasher{hashes: map[string]string{
		"input":         "new-input",
		"output":        "same-output",
		"runtime-input": "new-runtime-input",
	}}
	runner := &fakeRunner{}
	step := Step{
		Name:          StepTSWorker,
		ForceTarget:   ForceTSWorker,
		Inputs:        namedHashSpec("input"),
		Outputs:       namedHashSpec("output"),
		RestartInputs: namedHashSpec("runtime-input"),
		Command:       CommandSpec{Args: []string{"tsc"}},
	}

	result, err := ExecuteStep(
		t.Context(),
		&state,
		step,
		nil,
		Options{},
		hasher,
		runner,
	)
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if result.Decision != DecisionRunInputChanged {
		t.Fatalf("decision = %s, want %s", result.Decision, DecisionRunInputChanged)
	}
	if !result.RestartInputChanged {
		t.Fatal("restart input change was not detected from previous state")
	}
	if state.Steps[string(StepTSWorker)].RestartInputHash != "new-runtime-input" {
		t.Fatalf(
			"stored restart input hash = %q, want new-runtime-input",
			state.Steps[string(StepTSWorker)].RestartInputHash,
		)
	}
}

func TestRunOrdering(t *testing.T) {
	t.Parallel()

	recorder := &eventRecorder{}
	lock := &recordingLock{events: recorder}
	store := &recordingStore{events: recorder, state: DefaultState("")}
	hasher := &recordingHasher{events: recorder}
	runner := &fakeRunner{}

	err := runWithDeps(t.Context(), Options{}, lock, store, hasher, runner)
	if err != nil {
		t.Fatalf("runWithDeps: %v", err)
	}
	events := recorder.events
	if len(events) < 3 {
		t.Fatalf("events too short: %#v", events)
	}
	if events[0] != "lock" {
		t.Fatalf("first event = %q, want lock; events=%#v", events[0], events)
	}
	lockIndex := indexOf(events, "lock")
	loadIndex := indexOf(events, "load")
	hashIndex := indexOf(events, "hash")
	if lockIndex >= loadIndex || loadIndex >= hashIndex {
		t.Fatalf("ordering mismatch: %#v", events)
	}
}

type fakeHasher struct {
	hashes map[string]string
	exists map[string]bool
}

func (h *fakeHasher) Hash(_ context.Context, spec HashSpec) (string, error) {
	if h.hashes == nil {
		return "", nil
	}
	return h.hashes[spec.Roots[0]], nil
}

func (h *fakeHasher) Exists(_ context.Context, required []RequiredPath) (bool, error) {
	for _, req := range required {
		if h.exists == nil {
			continue
		}
		exists, ok := h.exists[req.Path]
		if !ok {
			continue
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

type fakeRunner struct{ calls []CommandSpec }

func (r *fakeRunner) Run(_ context.Context, spec CommandSpec) error {
	r.calls = append(r.calls, spec)
	return nil
}

func namedHashSpec(
	name string,
) HashSpec {
	return HashSpec{Roots: []string{name}, Optional: true}
}

func newForceSet(targets ...ForceTarget) ForceSet {
	set := ForceSet{}
	for _, target := range targets {
		set[target] = true
	}
	return set
}

type eventRecorder struct{ events []string }

func (r *eventRecorder) add(event string) { r.events = append(r.events, event) }

type recordingLock struct{ events *eventRecorder }

func (l *recordingLock) Acquire(context.Context) (ReleaseFunc, error) {
	l.events.add("lock")
	return func() error { l.events.add("release"); return nil }, nil
}

type recordingStore struct {
	events *eventRecorder
	state  State
}

func (s *recordingStore) Load(context.Context) (State, error) {
	s.events.add("load")
	return s.state, nil
}

func (s *recordingStore) Save(context.Context, State) error {
	s.events.add("save")
	return nil
}

func (s *recordingStore) Clean(context.Context) error {
	s.events.add("clean")
	return nil
}

type recordingHasher struct{ events *eventRecorder }

func (h *recordingHasher) Hash(context.Context, HashSpec) (string, error) {
	h.events.add("hash")
	return "hash", nil
}

func (h *recordingHasher) Exists(context.Context, []RequiredPath) (bool, error) {
	h.events.add("exists")
	return false, nil
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
