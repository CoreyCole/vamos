package build

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRestartPlanIgnoresForcedByteIdenticalTempl(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	got := ComputeRestartPlan(state, []StepResult{{
		Name:     StepTempl,
		Decision: DecisionRunForced,
	}})
	want := RestartPlan{}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("restart plan mismatch (-want +got):\n%s", diff)
	}
}

func TestRestartPlanWebRestartReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result StepResult
	}{
		{
			name:   "go binary changed",
			result: StepResult{Name: StepGo, RestartOutputChanged: true},
		},
		{
			name:   "active css changed",
			result: StepResult{Name: StepTailwind, RestartOutputChanged: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := ComputeRestartPlan(DefaultState(""), []StepResult{test.result})
			if !plan.Web.Needed || !plan.Web.OutputChanged {
				t.Fatalf(
					"web restart plan = %#v, want needed for changed output",
					plan.Web,
				)
			}
			if plan.TSWorker.Needed {
				t.Fatalf("TS worker restart unexpectedly needed: %#v", plan.TSWorker)
			}
		})
	}
}

func TestRestartPlanTSWorkerRestartReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step StepResult
		want RestartReason
	}{
		{
			name: "runtime dist changed",
			step: StepResult{Name: StepTSWorker, RestartOutputChanged: true},
			want: RestartReason{Needed: true, OutputChanged: true},
		},
		{
			name: "runtime dependency manifest changed",
			step: StepResult{Name: StepTSWorker, RestartInputChanged: true},
			want: RestartReason{Needed: true, InputChanged: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := ComputeRestartPlan(DefaultState(""), []StepResult{test.step})
			if diff := cmp.Diff(test.want, plan.TSWorker); diff != "" {
				t.Fatalf("TS worker restart mismatch (-want +got):\n%s", diff)
			}
			if plan.Web.Needed {
				t.Fatalf("web restart unexpectedly needed: %#v", plan.Web)
			}
		})
	}
}

func TestTSWorkerRestartOutputsExcludeTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	testOnly := filepath.Join(
		repoRoot,
		"pkg",
		"agents",
		"dist",
		"temporal",
		"workers",
		"ts",
		"worker.test.js",
	)
	writeFile(t, testOnly, "before")
	hasher := NewTreeHasher(repoRoot)
	before, err := hasher.Hash(t.Context(), TSWorkerStep().RestartOutputs)
	if err != nil {
		t.Fatalf("hash before: %v", err)
	}
	writeFile(t, testOnly, "after")
	after, err := hasher.Hash(t.Context(), TSWorkerStep().RestartOutputs)
	if err != nil {
		t.Fatalf("hash after: %v", err)
	}
	if before != after {
		t.Fatalf("test-only dist output changed restart hash: %s != %s", before, after)
	}

	runtimeFile := filepath.Join(
		repoRoot,
		"dist",
		"pkg",
		"agents",
		"temporal",
		"workers",
		"ts",
		"worker.js",
	)
	writeFile(t, runtimeFile, "runtime")
	withRuntime, err := hasher.Hash(t.Context(), TSWorkerStep().RestartOutputs)
	if err != nil {
		t.Fatalf("hash runtime: %v", err)
	}
	if withRuntime == after {
		t.Fatal("runtime worker output did not affect restart hash")
	}
}

func TestApplyRestartPlanNoRestartSetsPending(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: allServicesInstalled()}
	plan := RestartPlan{
		Web:      RestartReason{Needed: true, OutputChanged: true},
		TSWorker: RestartReason{Needed: true, InputChanged: true},
	}
	if err := ApplyRestartPlan(
		t.Context(),
		&state,
		plan,
		true,
		services,
		nil,
	); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	want := PendingRestartState{Web: true, TSWorker: true}
	if diff := cmp.Diff(want, state.PendingRestarts); diff != "" {
		t.Fatalf("pending mismatch (-want +got):\n%s", diff)
	}
	if len(services.events) != 0 {
		t.Fatalf("service calls with --no-restart: %#v", services.events)
	}
}

func TestApplyRestartPlanPersistsPendingSetAndClear(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: allServicesInstalled()}
	saves := &restartSaveRecorder{}
	plan := RestartPlan{Web: RestartReason{Needed: true, OutputChanged: true}}

	if err := ApplyRestartPlanWithOptions(t.Context(), &state, plan, RestartOptions{
		Services:     services,
		Save:         saves.Save,
		CheckoutPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	want := []PendingRestartState{{Web: true}, {Web: false}}
	if diff := cmp.Diff(want, saves.states); diff != "" {
		t.Fatalf("save sequence mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRestartPlanEnsuresTemporalAndTSBeforeWeb(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: allServicesInstalled()}
	saves := &restartSaveRecorder{}
	plan := RestartPlan{
		Web:      RestartReason{Needed: true, OutputChanged: true},
		TSWorker: RestartReason{Needed: true, InputChanged: true},
	}

	if err := ApplyRestartPlanWithOptions(t.Context(), &state, plan, RestartOptions{
		Services:     services,
		Save:         saves.Save,
		CheckoutPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	wantEvents := []string{
		"is-installed:temporal-server",
		"ensure:temporal-server",
		"is-installed:vamos-ts-worker",
		"restart:vamos-ts-worker",
		"is-installed:vamos",
		"restart:vamos",
	}
	if diff := cmp.Diff(wantEvents, services.events); diff != "" {
		t.Fatalf("event sequence mismatch (-want +got):\n%s", diff)
	}
	wantSaves := []PendingRestartState{
		{TSWorker: true},
		{},
		{Web: true},
		{},
	}
	if diff := cmp.Diff(wantSaves, saves.states); diff != "" {
		t.Fatalf("save sequence mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRestartPlanNoRestartDoesNotProbeServices(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: allServicesInstalled()}
	saves := &restartSaveRecorder{}
	plan := RestartPlan{
		Web:      RestartReason{Needed: true, OutputChanged: true},
		TSWorker: RestartReason{Needed: true, InputChanged: true},
	}

	if err := ApplyRestartPlan(
		t.Context(),
		&state,
		plan,
		true,
		services,
		saves.Save,
	); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	if len(services.events) != 0 {
		t.Fatalf("service manager was called with --no-restart: %#v", services.events)
	}
	wantPending := PendingRestartState{Web: true, TSWorker: true}
	if diff := cmp.Diff(wantPending, state.PendingRestarts); diff != "" {
		t.Fatalf("pending mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRestartPlanMissingTemporalLeavesTSPending(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: map[ServiceName]bool{
		ServiceTemporal: false,
		ServiceTSWorker: true,
		ServiceWeb:      true,
	}}
	saves := &restartSaveRecorder{}
	plan := RestartPlan{TSWorker: RestartReason{Needed: true, InputChanged: true}}

	err := ApplyRestartPlanWithOptions(t.Context(), &state, plan, RestartOptions{
		Services:     services,
		Save:         saves.Save,
		CheckoutPath: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "install temporal-server") {
		t.Fatalf("error = %v, want missing temporal hint", err)
	}
	if !state.PendingRestarts.TSWorker {
		t.Fatal("missing Temporal did not leave TS worker pending")
	}
}

func TestApplyRestartPlanNoopMakesNoServiceCalls(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{installed: allServicesInstalled()}
	saves := &restartSaveRecorder{}
	if err := ApplyRestartPlan(
		t.Context(),
		&state,
		RestartPlan{},
		false,
		services,
		saves.Save,
	); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	if len(services.events) != 0 || len(saves.states) != 0 {
		t.Fatalf(
			"noop touched services/saves: events=%#v saves=%#v",
			services.events,
			saves.states,
		)
	}
}

func TestRunCleanNoRestartPreservesPendingRestarts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, ".build-agents")
	store := NewFileStateStore(filepath.Join(stateDir, "state.json"))
	state := DefaultState(repoRoot)
	state.PendingRestarts = PendingRestartState{Web: true, TSWorker: true}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeFile(t, filepath.Join(stateDir, "go-build-cache", "cachefile"), "cache")

	if err := runWithDeps(
		t.Context(),
		Options{
			RepoRoot:  repoRoot,
			StateDir:  ".build-agents",
			Clean:     true,
			NoRestart: true,
		},
		NewFileLock(
			filepath.Join(stateDir, "build.lock"),
			filepath.Join(stateDir, "build.lock.json"),
		),
		store,
		&fakeHasher{},
		&fakeRunner{},
	); err != nil {
		t.Fatalf("Run clean: %v", err)
	}
	got, err := store.Load(t.Context())
	if err != nil {
		t.Fatalf("Load after clean: %v", err)
	}
	if diff := cmp.Diff(state.PendingRestarts, got.PendingRestarts); diff != "" {
		t.Fatalf("pending restarts mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRestartPlanFailedRestartLeavesPending(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	services := &fakeServices{
		installed: allServicesInstalled(),
		fail: map[string]bool{
			"restart:" + string(ServiceWeb):      true,
			"restart:" + string(ServiceTSWorker): false,
		},
	}
	plan := RestartPlan{Web: RestartReason{Needed: true, OutputChanged: true}}
	if err := ApplyRestartPlanWithOptions(
		t.Context(),
		&state,
		plan,
		RestartOptions{
			Services:     services,
			CheckoutPath: t.TempDir(),
		},
	); err == nil {
		t.Fatal("ApplyRestartPlan succeeded, want restart error")
	}
	if !state.PendingRestarts.Web {
		t.Fatal("failed web restart did not leave pending flag set")
	}
}

func TestSystemdServiceManagerIsInstalledParsesLoadState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		loadState string
		want      bool
	}{
		{name: "loaded", loadState: "loaded\n", want: true},
		{name: "not found", loadState: "not-found\n", want: false},
		{name: "empty", loadState: "\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output := &fakeCommandOutput{out: test.loadState}
			manager := NewSystemdServiceManager(&fakeRunner{}, output.Output, nil, nil)
			got, err := manager.IsInstalled(t.Context(), ServiceWeb)
			if err != nil {
				t.Fatalf("IsInstalled: %v", err)
			}
			if got != test.want {
				t.Fatalf("IsInstalled = %t, want %t", got, test.want)
			}
			assertCommandArgs(t, output.calls, [][]string{{
				"systemctl", "--user", "show", "-p", "LoadState", "--value", "vamos",
			}})
		})
	}
}

func TestSystemdServiceManagerCommands(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	manager := NewSystemdServiceManager(
		runner,
		func(context.Context, CommandSpec) (string, error) {
			return "loaded\n", nil
		},
		nil,
		nil,
	)

	if err := manager.Restart(t.Context(), ServiceWeb); err != nil {
		t.Fatalf("restart web: %v", err)
	}
	if err := manager.Restart(t.Context(), ServiceTSWorker); err != nil {
		t.Fatalf("restart ts worker: %v", err)
	}
	if err := manager.EnsureRunning(t.Context(), ServiceTemporal); err != nil {
		t.Fatalf("ensure temporal: %v", err)
	}

	want := [][]string{
		{"systemctl", "--user", "restart", "vamos"},
		{"systemctl", "--user", "restart", "--no-block", "vamos-ts-worker"},
		{"systemctl", "--user", "start", "temporal-server"},
	}
	assertCommandArgs(t, runner.calls, want)
}

func TestLaunchdServiceManagerCommands(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	output := &fakeCommandOutput{out: "service = { }\n"}
	manager := NewLaunchdServiceManager(runner, output.Output, nil, nil, 501)

	installed, err := manager.IsInstalled(t.Context(), ServiceWeb)
	if err != nil || !installed {
		t.Fatalf("IsInstalled = %t, %v; want true, nil", installed, err)
	}
	if err := manager.EnsureRunning(t.Context(), ServiceTemporal); err != nil {
		t.Fatalf("ensure temporal: %v", err)
	}
	if err := manager.Restart(t.Context(), ServiceTSWorker); err != nil {
		t.Fatalf("restart ts worker: %v", err)
	}

	assertCommandArgs(
		t,
		output.calls,
		[][]string{{"launchctl", "print", "gui/501/dev.vamos"}},
	)
	assertCommandArgs(t, runner.calls, [][]string{
		{"launchctl", "kickstart", "gui/501/dev.chestnut.temporal-server"},
		{"launchctl", "kickstart", "-k", "gui/501/dev.vamos-ts-worker"},
	})
}

func TestSystemdServiceManagerProbeError(t *testing.T) {
	t.Parallel()

	manager := NewSystemdServiceManager(
		&fakeRunner{},
		func(context.Context, CommandSpec) (string, error) { return "boom", errors.New("exit 1") },
		nil,
		nil,
	)
	installed, err := manager.IsInstalled(t.Context(), ServiceWeb)
	if err == nil || !strings.Contains(err.Error(), "probe systemd service vamos") {
		t.Fatalf("IsInstalled = %t, %v; want probe error", installed, err)
	}
}

func TestLaunchdServiceManagerProbeErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing service", func(t *testing.T) {
		t.Parallel()

		manager := NewLaunchdServiceManager(
			&fakeRunner{},
			func(context.Context, CommandSpec) (string, error) {
				return "Could not find service \"dev.vamos\" in domain", errors.New(
					"exit 113",
				)
			},
			nil,
			nil,
			501,
		)
		installed, err := manager.IsInstalled(t.Context(), ServiceWeb)
		if err != nil || installed {
			t.Fatalf("IsInstalled = %t, %v; want false, nil", installed, err)
		}
	})

	t.Run("unexpected failure", func(t *testing.T) {
		t.Parallel()

		manager := NewLaunchdServiceManager(
			&fakeRunner{},
			func(context.Context, CommandSpec) (string, error) { return "permission denied", errors.New("exit 1") },
			nil,
			nil,
			501,
		)
		installed, err := manager.IsInstalled(t.Context(), ServiceWeb)
		if err == nil ||
			!strings.Contains(err.Error(), "probe launchd service vamos") {
			t.Fatalf("IsInstalled = %t, %v; want probe error", installed, err)
		}
	})
}

func TestApplyRestartPlanSuccessfulRestartClearsOnlySuccessfulService(t *testing.T) {
	t.Parallel()

	state := DefaultState("")
	state.PendingRestarts = PendingRestartState{Web: true, TSWorker: true}
	services := &fakeServices{installed: allServicesInstalled()}
	plan := RestartPlan{Web: RestartReason{Needed: true, Pending: true}}
	if err := ApplyRestartPlanWithOptions(t.Context(), &state, plan, RestartOptions{
		Services:     services,
		CheckoutPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("ApplyRestartPlan: %v", err)
	}
	wantPending := PendingRestartState{Web: false, TSWorker: true}
	if diff := cmp.Diff(wantPending, state.PendingRestarts); diff != "" {
		t.Fatalf("pending mismatch (-want +got):\n%s", diff)
	}
	wantEvents := []string{"is-installed:vamos", "restart:vamos"}
	if diff := cmp.Diff(wantEvents, services.events); diff != "" {
		t.Fatalf("service event mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRestartPlanUsesWorkspaceRestartBeforeSystemd(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
	)
	defer server.Close()
	writeWorkspaceMetadata(t, checkout, "feature", checkout, server.URL, "secret")
	state := DefaultState("")
	state.PendingRestarts = PendingRestartState{Web: true, TSWorker: true}
	services := &fakeServices{installed: allServicesInstalled(), failIfCalled: true}
	plan := RestartPlan{
		Web:      RestartReason{Needed: true, Pending: true},
		TSWorker: RestartReason{Needed: true, Pending: true},
	}
	if err := ApplyRestartPlanWithOptions(t.Context(), &state, plan, RestartOptions{
		Services:     services,
		CheckoutPath: checkout,
	}); err != nil {
		t.Fatalf("ApplyRestartPlanWithOptions: %v", err)
	}
	if diff := cmp.Diff(PendingRestartState{}, state.PendingRestarts); diff != "" {
		t.Fatalf("pending mismatch (-want +got):\n%s", diff)
	}
}

type fakeCommandOutput struct {
	calls []CommandSpec
	out   string
	err   error
}

func (o *fakeCommandOutput) Output(_ context.Context, spec CommandSpec) (string, error) {
	o.calls = append(o.calls, spec)
	return o.out, o.err
}

type restartSaveRecorder struct{ states []PendingRestartState }

func (r *restartSaveRecorder) Save(_ context.Context, state State) error {
	r.states = append(r.states, state.PendingRestarts)
	return nil
}

type fakeServices struct {
	installed    map[ServiceName]bool
	fail         map[string]bool
	failIfCalled bool
	events       []string
}

func (s *fakeServices) IsInstalled(_ context.Context, service ServiceName) (bool, error) {
	if s.failIfCalled {
		return false, fmt.Errorf("unexpected service probe %s", service)
	}
	s.events = append(s.events, "is-installed:"+string(service))
	return s.installed[service], nil
}

func (s *fakeServices) Restart(_ context.Context, service ServiceName) error {
	if s.failIfCalled {
		return fmt.Errorf("unexpected service restart %s", service)
	}
	s.events = append(s.events, "restart:"+string(service))
	if s.fail["restart:"+string(service)] {
		return fmt.Errorf("restart %s", service)
	}
	return nil
}

func (s *fakeServices) EnsureRunning(_ context.Context, service ServiceName) error {
	if s.failIfCalled {
		return fmt.Errorf("unexpected service ensure %s", service)
	}
	s.events = append(s.events, "ensure:"+string(service))
	if s.fail["ensure:"+string(service)] {
		return fmt.Errorf("ensure %s", service)
	}
	return nil
}

func (s *fakeServices) MissingServiceHint(service ServiceName) string {
	return fmt.Sprintf("install %s", service)
}

func allServicesInstalled() map[ServiceName]bool {
	return map[ServiceName]bool{
		ServiceWeb:      true,
		ServiceTemporal: true,
		ServiceTSWorker: true,
	}
}

func assertCommandArgs(t *testing.T, got []CommandSpec, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("command count = %d, want %d; calls=%#v", len(got), len(want), got)
	}
	for i := range want {
		if diff := cmp.Diff(want[i], got[i].Args); diff != "" {
			t.Fatalf("command %d mismatch (-want +got):\n%s", i, diff)
		}
	}
}

func TestComponentsFromRestartPlan(t *testing.T) {
	t.Parallel()

	got := ComponentsFromRestartPlan(RestartPlan{
		Web:      RestartReason{Needed: true},
		TSWorker: RestartReason{Needed: true},
	})
	want := []string{"web", "ts_worker"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ComponentsFromRestartPlan = %#v, want %#v", got, want)
	}
	if got := ComponentsFromRestartPlan(RestartPlan{}); len(got) != 0 {
		t.Fatalf("ComponentsFromRestartPlan(empty) = %#v, want empty", got)
	}
}
