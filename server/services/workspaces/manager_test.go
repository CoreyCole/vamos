package workspaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetadataRoundTrip(t *testing.T) {
	path := RuntimePaths(filepath.Join(t.TempDir(), "checkout with spaces")).WorkspaceEnv
	checkoutPath := filepath.Join(t.TempDir(), "checkout with spaces and 'quote'")
	want := WorkspaceMetadata{
		Slug:         "foo",
		CheckoutPath: checkoutPath,
		ManagerURL:   "https://main.cn-agents.test",
		RestartToken: "tok'en",
		DatabasePath: RuntimePaths(checkoutPath).AgentsDB,
		PID:          1234,
		Port:         4567,
	}
	if err := WriteMetadata(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v want 0600", info.Mode().Perm())
	}
	got, err := ReadMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("metadata=%#v want %#v", got, want)
	}
}

func TestChildEnv(t *testing.T) {
	t.Setenv("CN_AGENTS_DEV_AUTH_SIGNING_KEY", "must-not-leak-from-os-env")
	t.Setenv("CN_AGENTS_DEV_AUTH_SECRET", "old-secret-must-not-leak-from-os-env")

	ws := Workspace{
		Slug:         "foo",
		CheckoutPath: "/tmp/cn-agents-foo",
		URL:          "https://foo.cn-agents.test/",
		StateDir:     "/tmp/state/foo",
	}
	rt := RuntimeConfig{
		ManagerURL:       "https://main.cn-agents.test",
		RestartToken:     "restart-token",
		DevAuthVerifyKey: "verify-key",
		BaseEnv: map[string]string{
			"VAMOS_THOUGHTS_REPO":            "/wrong/repo",
			"VAMOS_THOUGHTS_ROOT":            "/wrong/thoughts",
			"GOOGLE_CREDENTIALS_FILE":        "/shared/google.json",
			"VAMOS_DEV_AUTH_SIGNING_KEY":     "must-not-leak",
			"CN_AGENTS_DEV_AUTH_SIGNING_KEY": "legacy-must-not-leak",
			"CN_AGENTS_DEV_AUTH_SECRET":      "old-secret-must-not-leak",
		},
		ThoughtsRepo: "/host/repo",
		ThoughtsRoot: "/host/repo/thoughts",
	}
	ports := map[BundleComponent]int{
		ComponentWeb:        4321,
		ComponentTemporal:   7234,
		ComponentTemporalUI: 8234,
	}
	env := envMap(ChildEnv(rt.BaseEnv, ws, ports, rt))
	assertEnv(t, env, "VAMOS_LISTEN_ADDRESS", "127.0.0.1:4321")
	assertEnv(t, env, "VAMOS_PUBLIC_BASE_URL", "https://foo.cn-agents.test")
	assertEnv(t, env, "VAMOS_INTERNAL_CALLBACK_BASE_URL", "http://127.0.0.1:4321")
	assertEnv(t, env, "VAMOS_DEFAULT_CWD", ws.CheckoutPath)
	assertEnv(t, env, "VAMOS_THOUGHTS_REPO", "/host/repo")
	assertEnv(t, env, "VAMOS_THOUGHTS_ROOT", "/host/repo/thoughts")
	assertEnvMissing(t, env, "PIPE"+"LINE_ARTIFACTS_DIR")
	assertEnv(t, env, "CN_TEMPORAL", "true")
	assertEnv(t, env, "TEMPORAL_ADDRESS", "127.0.0.1:7234")
	assertEnv(t, env, "TEMPORAL_UI_BASE_URL", "http://127.0.0.1:8234")
	assertEnv(t, env, "VAMOS_DATABASE_PATH", RuntimePaths(ws.CheckoutPath).AgentsDB)
	assertEnv(t, env, "OPENCLAW_STATE_DIR", RuntimePaths(ws.CheckoutPath).OpenClawDir)
	assertEnv(t, env, "VAMOS_WORKSPACE_MODE", "child")
	assertEnv(t, env, "VAMOS_WORKSPACE_SLUG", ws.Slug)
	assertEnv(t, env, "VAMOS_WORKSPACE_MANAGER_URL", rt.ManagerURL)
	assertEnv(t, env, "VAMOS_WORKSPACE_RESTART_TOKEN", rt.RestartToken)
	assertEnv(t, env, "VAMOS_DEV_AUTH_VERIFY_KEY", rt.DevAuthVerifyKey)
	assertEnvMissing(t, env, "VAMOS_DEV_AUTH_SIGNING_KEY")
	assertEnvMissing(t, env, "CN_AGENTS_DEV_AUTH_SIGNING_KEY")
	assertEnvMissing(t, env, "CN_AGENTS_DEV_AUTH_SECRET")
	assertEnv(t, env, "GOOGLE_CREDENTIALS_FILE", "/shared/google.json")
}

func TestSortedWorkspaceSlugsKeepsMainFirst(t *testing.T) {
	t.Parallel()

	got := sortedWorkspaceSlugs(map[string]Workspace{
		"2026-05-15-feature": {Slug: "2026-05-15-feature"},
		"main":               {Slug: "main", IsMain: true},
		"alpha":              {Slug: "alpha"},
	})
	want := []string{"main", "2026-05-15-feature", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("sortedWorkspaceSlugs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedWorkspaceSlugs() = %#v, want %#v", got, want)
		}
	}
}

func TestManagerStartStopRestart(t *testing.T) {
	m, checkout := newTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime

	startedWS, err := m.Start(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if startedWS.Status != StatusRunning || startedWS.PID == 0 || startedWS.Port == 0 {
		t.Fatalf("started workspace=%#v", startedWS)
	}
	if startedWS.LogPath != startedWS.Bundle.WebLog {
		t.Fatalf("log path=%q want %q", startedWS.LogPath, startedWS.Bundle.WebLog)
	}
	meta, err := ReadMetadata(RuntimePaths(checkout).WorkspaceEnv)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Slug != "foo" || meta.ManagerURL != m.runtime.ManagerURL {
		t.Fatalf("metadata=%#v started=%#v", meta, startedWS)
	}

	stoppedWS, err := m.Stop(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if stoppedWS.Status != StatusStopped || stoppedWS.PID != 0 {
		t.Fatalf("stopped workspace=%#v", stoppedWS)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("stop count=%d", runtime.stopCalls)
	}

	restartedWS, err := m.Restart(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if restartedWS.Status != StatusRunning || runtime.startCalls != 2 {
		t.Fatalf("restart workspace=%#v starts=%d", restartedWS, runtime.startCalls)
	}
}

func TestManagerStartWhileStartingIsNoop(t *testing.T) {
	t.Parallel()

	m, _ := newTestManager(t)
	runtime := &blockingStartRuntime{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	m.bundleRuntime = runtime

	done := make(chan error, 1)
	go func() {
		_, err := m.Start(t.Context(), "foo")
		done <- err
	}()
	<-runtime.started

	ws, err := m.Start(t.Context(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Status != StatusStarting {
		t.Fatalf("status = %q, want starting", ws.Status)
	}
	if runtime.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", runtime.startCalls)
	}

	close(runtime.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManagerRequestLifecycleStartPersistsStartingAndStartsOnce(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter

	snap, err := m.RequestLifecycle(context.Background(), WorkspaceLifecycleRequest{
		Slug: "foo",
		Kind: WorkspaceTransitionStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.ObservedState != WorkspaceObservedStarting ||
		snap.DesiredState != WorkspaceDesiredRunning ||
		snap.TransitionID != "transition-1" {
		t.Fatalf("snapshot=%#v", snap)
	}
	if len(starter.starts) != 1 || starter.starts[0].TransitionID != "transition-1" {
		t.Fatalf("starts=%#v", starter.starts)
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.ObservedState != WorkspaceObservedStarting ||
		state.TransitionID != "transition-1" {
		t.Fatalf("state=%#v", state)
	}
}

func TestManagerRequestLifecycleDuplicateSameDirectionIsNoop(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter

	first, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.TransitionID != first.TransitionID {
		t.Fatalf(
			"duplicate transition=%q want %q",
			second.TransitionID,
			first.TransitionID,
		)
	}
	if len(starter.starts) != 1 {
		t.Fatalf("starts=%#v", starter.starts)
	}
}

func TestManagerRequestLifecycleRestartStartsTransitionEvenWhenAlreadyRunning(
	t *testing.T,
) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	ws, ok := m.Lookup("foo")
	if !ok {
		t.Fatal("missing workspace")
	}
	state := WorkspaceLifecycleState{
		DesiredState:        WorkspaceDesiredRunning,
		ObservedState:       WorkspaceObservedRunning,
		TransitionUpdatedAt: m.lifecycleNow(),
	}
	if err := (FileBundleStore{}).WriteLifecycle(ws, state); err != nil {
		t.Fatal(err)
	}

	snap, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionRestart},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snap.TransitionKind != WorkspaceTransitionRestart ||
		snap.ObservedState != WorkspaceObservedStarting ||
		snap.TransitionID != "transition-1" {
		t.Fatalf("restart snapshot=%#v", snap)
	}
	if len(starter.starts) != 1 || starter.starts[0].Kind != WorkspaceTransitionRestart {
		t.Fatalf("starts=%#v", starter.starts)
	}
	got, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if got.TransitionKind != WorkspaceTransitionRestart ||
		got.TransitionID != "transition-1" {
		t.Fatalf("state=%#v", got)
	}
}

func TestManagerRequestLifecycleOppositeDirectionUpdatesDesiredOnly(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter

	first, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStop},
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.TransitionID != first.TransitionID ||
		second.DesiredState != WorkspaceDesiredStopped ||
		second.ObservedState != WorkspaceObservedStarting {
		t.Fatalf("opposite snapshot=%#v first=%#v", second, first)
	}
	if len(starter.starts) != 1 {
		t.Fatalf("starts=%#v", starter.starts)
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredState != WorkspaceDesiredStopped ||
		state.TransitionID != first.TransitionID {
		t.Fatalf("state=%#v", state)
	}
}

func TestManagerCompleteTransitionIgnoresStaleTransitionID(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	if _, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	); err != nil {
		t.Fatal(err)
	}
	ws, _ := m.Lookup("foo")
	newer := WorkspaceLifecycleState{
		DesiredState:        WorkspaceDesiredRunning,
		ObservedState:       WorkspaceObservedStarting,
		TransitionKind:      WorkspaceTransitionStart,
		TransitionID:        "transition-2",
		TransitionStartedAt: m.lifecycleNow(),
		TransitionUpdatedAt: m.lifecycleNow(),
	}
	if err := (FileBundleStore{}).WriteLifecycle(ws, newer); err != nil {
		t.Fatal(err)
	}

	if err := m.CompleteTransition(
		context.Background(),
		"foo",
		"transition-1",
		WorkspaceTransitionResult{ObservedState: WorkspaceObservedRunning},
	); err != nil {
		t.Fatal(err)
	}
	got, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if got.TransitionID != "transition-2" ||
		got.ObservedState != WorkspaceObservedStarting {
		t.Fatalf("state=%#v", got)
	}
}

func TestManagerCompleteTransitionQueuesOppositeDesiredFollowup(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter

	first, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStop},
	); err != nil {
		t.Fatal(err)
	}
	if err := m.CompleteTransition(
		context.Background(),
		"foo",
		first.TransitionID,
		WorkspaceTransitionResult{ObservedState: WorkspaceObservedRunning},
	); err != nil {
		t.Fatal(err)
	}
	if len(starter.starts) != 2 {
		t.Fatalf("starts=%#v", starter.starts)
	}
	if starter.starts[1].Kind != WorkspaceTransitionStop ||
		starter.starts[1].TransitionID != "transition-2" {
		t.Fatalf("followup=%#v", starter.starts[1])
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredState != WorkspaceDesiredStopped ||
		state.ObservedState != WorkspaceObservedStopping ||
		state.TransitionID != "transition-2" {
		t.Fatalf("state=%#v", state)
	}
}

func TestManagerCompleteTransitionQueuesStartFollowupAfterStop(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	m.mu.Lock()
	ws := m.workspaces["foo"]
	ws.Status = StatusRunning
	m.workspaces["foo"] = ws
	m.mu.Unlock()

	first, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStop},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	); err != nil {
		t.Fatal(err)
	}
	if err := m.CompleteTransition(
		context.Background(),
		"foo",
		first.TransitionID,
		WorkspaceTransitionResult{ObservedState: WorkspaceObservedStopped},
	); err != nil {
		t.Fatal(err)
	}
	if len(starter.starts) != 2 || starter.starts[1].Kind != WorkspaceTransitionStart {
		t.Fatalf("starts=%#v", starter.starts)
	}
}

func TestManagerRequestLifecycleFailsFastWithoutStarter(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	_, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "lifecycle starter is not configured") {
		t.Fatalf("err=%v", err)
	}
	_, readErr := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("ReadLifecycle err=%v want not exist", readErr)
	}
}

func TestLifecycleActivityWrongTransitionIDNoops(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	activities := &WorkspaceLifecycleActivities{Manager: m}

	if err := activities.StartWorkspace(
		context.Background(),
		WorkspaceLifecycleWorkflowInput{
			Slug:         "foo",
			TransitionID: "wrong-transition",
			Kind:         WorkspaceTransitionStart,
		},
	); err != nil {
		t.Fatal(err)
	}
	if runtime.startCalls != 0 {
		t.Fatalf("start calls=%d want 0", runtime.startCalls)
	}
}

func TestLifecycleActivityFailureCompletesFailed(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	m.bundleRuntime = &fakeBundleRuntime{startErr: errors.New("boom")}
	activities := &WorkspaceLifecycleActivities{Manager: m}
	started, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = activities.StartWorkspace(context.Background(), WorkspaceLifecycleWorkflowInput{
		Slug:         "foo",
		TransitionID: started.TransitionID,
		Kind:         WorkspaceTransitionStart,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v want boom", err)
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.ObservedState != WorkspaceObservedFailed || state.Error != "boom" {
		t.Fatalf("state=%#v", state)
	}
}

func TestLifecycleActivityStartUsesOwnedPathWhileObservedStarting(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	activities := &WorkspaceLifecycleActivities{Manager: m}
	started, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := activities.StartWorkspace(
		context.Background(),
		WorkspaceLifecycleWorkflowInput{
			Slug:         "foo",
			TransitionID: started.TransitionID,
			Kind:         WorkspaceTransitionStart,
		},
	); err != nil {
		t.Fatal(err)
	}
	if runtime.startCalls != 1 {
		t.Fatalf("start calls=%d want 1", runtime.startCalls)
	}
	ws, _ := m.Lookup("foo")
	if ws.Status != StatusRunning {
		t.Fatalf("workspace=%#v", ws)
	}
}

func TestLifecycleActivityRetryAdoptsExistingRunningRuntime(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	m.processAlive = func(pid int) bool { return pid == 4321 }
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	activities := &WorkspaceLifecycleActivities{Manager: m}
	started, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := m.Lookup("foo")
	if err := (FileBundleStore{}).WriteStatus(ws, RuntimeStatus{
		Status: StatusRunning,
		Ports:  map[BundleComponent]int{ComponentWeb: 9876},
		PIDs:   map[BundleComponent]int{ComponentWeb: 4321},
	}); err != nil {
		t.Fatal(err)
	}

	if err := activities.StartWorkspace(
		context.Background(),
		WorkspaceLifecycleWorkflowInput{
			Slug:         "foo",
			TransitionID: started.TransitionID,
			Kind:         WorkspaceTransitionStart,
		},
	); err != nil {
		t.Fatal(err)
	}
	if runtime.startCalls != 0 {
		t.Fatalf("start calls=%d want 0", runtime.startCalls)
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.ObservedState != WorkspaceObservedRunning {
		t.Fatalf("state=%#v", state)
	}
}

func TestManagerForceRestartComponentsUsesFullBundleForFailedWorkspace(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	ws, _ := m.Lookup("foo")
	ws.Status = StatusFailed
	ws.Ports = map[BundleComponent]int{
		ComponentWeb:        4100,
		ComponentTemporal:   4200,
		ComponentTemporalUI: 4300,
	}
	m.workspaces["foo"] = ws
	if err := (FileBundleStore{}).WriteWorkspaceEnv(ws, WorkspaceEnv{
		Slug:         "foo",
		CheckoutPath: checkout,
		ManagerURL:   m.runtime.ManagerURL,
		RestartToken: m.runtime.RestartToken,
	}); err != nil {
		t.Fatal(err)
	}
	if err := (FileBundleStore{}).WriteStatus(ws, RuntimeStatus{
		Status: StatusFailed,
		Phase:  PhaseStartingTemporal,
		Ports:  ws.Ports,
	}); err != nil {
		t.Fatal(err)
	}

	restarted, err := m.RestartComponents(
		context.Background(),
		"foo",
		[]BundleComponent{ComponentWeb, ComponentTSWorker},
		RestartComponentsOptions{Force: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status != StatusRunning {
		t.Fatalf("status = %q, want running", restarted.Status)
	}
	if runtime.stopCalls != 1 || runtime.startCalls != 1 {
		t.Fatalf(
			"stopCalls=%d startCalls=%d, want 1/1",
			runtime.stopCalls,
			runtime.startCalls,
		)
	}
	if restarted.Ports[ComponentWeb] == 4100 {
		t.Fatalf("web port = %d, want fresh bundle port", restarted.Ports[ComponentWeb])
	}
}

func TestRecoveryCanStartIncludesTerminalFailureStates(t *testing.T) {
	for _, status := range []Status{StatusStopped, StatusFailed, StatusCrashed, ""} {
		if !recoveryCanStart(status) {
			t.Fatalf("recoveryCanStart(%q) = false", status)
		}
	}
	for _, status := range []Status{StatusStarting, StatusRunning, StatusStopping, StatusInvalid} {
		if recoveryCanStart(status) {
			t.Fatalf("recoveryCanStart(%q) = true", status)
		}
	}
}

func TestManagerReconcileMarksDeadRunningPIDCrashed(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	m.processAlive = func(int) bool { return false }
	ws, _ := m.Lookup("foo")
	if err := (FileBundleStore{}).WriteWorkspaceEnv(ws, WorkspaceEnv{
		Slug:         "foo",
		CheckoutPath: checkout,
		ManagerURL:   m.runtime.ManagerURL,
		RestartToken: m.runtime.RestartToken,
	}); err != nil {
		t.Fatal(err)
	}
	if err := (FileBundleStore{}).WriteStatus(ws, RuntimeStatus{
		Status: StatusRunning,
		Ports:  map[BundleComponent]int{ComponentWeb: 9876},
		PIDs:   map[BundleComponent]int{ComponentWeb: 4321},
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := m.ReconcileWorkspace(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ObservedState != WorkspaceObservedCrashed ||
		snap.Workspace.Status != StatusCrashed || snap.Workspace.PID != 0 {
		t.Fatalf("snapshot=%#v", snap)
	}
}

func TestManagerReconcileExpiresStaleStartingTransition(t *testing.T) {
	m, checkout := newLifecycleTestManager(t)
	m.lifecycleStarter = &fakeLifecycleStarter{}
	started, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time {
		return time.Date(2026, 5, 15, 1, 30, 0, 0, time.UTC)
	}

	snap, err := m.ReconcileWorkspace(context.Background(), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ObservedState != WorkspaceObservedFailed || snap.TransitionID != "" ||
		!strings.Contains(snap.Error, started.TransitionID) {
		t.Fatalf("snapshot=%#v", snap)
	}
	state, err := FileBundleStore{}.ReadLifecycle(Workspace{CheckoutPath: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if state.ObservedState != WorkspaceObservedFailed || state.TransitionID != "" {
		t.Fatalf("state=%#v", state)
	}
}

func TestManagerExplicitStartCanRetryFailedDesiredRunning(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	starter := &fakeLifecycleStarter{}
	m.lifecycleStarter = starter
	ws, _ := m.Lookup("foo")
	if err := (FileBundleStore{}).WriteLifecycle(ws, WorkspaceLifecycleState{
		DesiredState:  WorkspaceDesiredRunning,
		ObservedState: WorkspaceObservedFailed,
		Error:         "previous failure",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ObservedState != WorkspaceObservedStarting || len(starter.starts) != 1 {
		t.Fatalf("snapshot=%#v starts=%#v", snap, starter.starts)
	}
}

func TestManagerReconcileNotifiesOnTerminalStateChange(t *testing.T) {
	m, _ := newLifecycleTestManager(t)
	m.lifecycleStarter = &fakeLifecycleStarter{}
	notifier := NewLifecycleNotifier()
	m.SetLifecycleNotifier(notifier)
	ch, unsubscribe := notifier.Subscribe()
	defer unsubscribe()
	if _, err := m.RequestLifecycle(
		context.Background(),
		WorkspaceLifecycleRequest{Slug: "foo", Kind: WorkspaceTransitionStart},
	); err != nil {
		t.Fatal(err)
	}
	<-ch
	m.now = func() time.Time {
		return time.Date(2026, 5, 15, 1, 30, 0, 0, time.UTC)
	}

	if _, err := m.ReconcileWorkspace(context.Background(), "foo"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reconcile notification")
	}
}

func TestManagerRejectsMainWorkspaceActions(t *testing.T) {
	parent := t.TempDir()
	mainCheckout := makeCheckout(t, parent, "cn-agents")
	m, err := NewManager(RuntimeConfig{
		ManagerURL: "https://main.cn-agents.test",
	}, DiscoveryConfig{
		ParentDir:        parent,
		Domain:           "cn-agents.test",
		StateDir:         filepath.Join(parent, "state"),
		MainCheckoutPath: mainCheckout,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), "main"); err == nil {
		t.Fatal("Start(main) error = nil")
	}
	if _, err := m.Stop(context.Background(), "main"); err == nil {
		t.Fatal("Stop(main) error = nil")
	}
}

func TestManagerStopWatcherDoesNotMarkStoppedChildCrashed(t *testing.T) {
	m, _ := newTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	if _, err := m.Start(context.Background(), "foo"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(context.Background(), "foo"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	ws, ok := m.Lookup("foo")
	if !ok || ws.Status != StatusStopped || ws.PID != 0 {
		t.Fatalf("workspace after stop/watch = %#v ok=%v", ws, ok)
	}
}

func TestManagerMarksCrashedOnUnexpectedExit(t *testing.T) {
	m, _ := newTestManager(t)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	if _, err := m.Start(context.Background(), "foo"); err != nil {
		t.Fatal(err)
	}
	runtime.web.finish(errors.New("boom"))
	waitFor(t, func() bool {
		ws, ok := m.Lookup("foo")
		return ok && ws.Status == StatusCrashed && ws.PID == 0 &&
			strings.Contains(ws.Error, "boom")
	})
}

func TestManagerMarkErrorRecordsWorkspaceError(t *testing.T) {
	m, _ := newTestManager(t)
	sink := &recordingWorkspaceErrorSink{}
	m.SetWorkspaceErrorRecorder(sink)
	m.bundleRuntime = &fakeBundleRuntime{startErr: errors.New("start failed")}

	if _, err := m.Start(context.Background(), "foo"); err == nil {
		t.Fatal("Start() error = nil")
	}

	events := sink.recorded()
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	event := events[0]
	if event.WorkspaceSlug != "foo" || event.Source != WorkspaceErrorSourceManager || event.Severity != WorkspaceErrorSeverityError {
		t.Fatalf("event=%#v", event)
	}
	if event.Message != "workspace manager reported failure" || !strings.Contains(event.Detail, "mark_error: start failed") {
		t.Fatalf("event=%#v", event)
	}
	if event.DedupeKey == "" {
		t.Fatal("empty dedupe key")
	}
}

func TestManagerWatchBundleCrashRecordsWorkspaceError(t *testing.T) {
	m, _ := newTestManager(t)
	sink := &recordingWorkspaceErrorSink{}
	m.SetWorkspaceErrorRecorder(sink)
	runtime := &fakeBundleRuntime{}
	m.bundleRuntime = runtime
	if _, err := m.Start(context.Background(), "foo"); err != nil {
		t.Fatal(err)
	}

	runtime.web.finish(errors.New("boom"))

	waitFor(t, func() bool {
		events := sink.recorded()
		return len(events) == 1 && events[0].WorkspaceSlug == "foo" &&
			events[0].Source == WorkspaceErrorSourceManager &&
			strings.Contains(events[0].Detail, "child_crashed: boom") &&
			events[0].DedupeKey != ""
	})
}

func TestManagerIgnoresCopiedRuntimeStateFromAnotherWorkspace(t *testing.T) {
	m, checkout := newTestManager(t)
	store := FileBundleStore{}
	ws := Workspace{Slug: "foo", CheckoutPath: checkout, Bundle: RuntimePaths(checkout)}
	if err := store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug: "old-workspace",
			CheckoutPath: filepath.Join(
				filepath.Dir(checkout),
				"cn-agents-old-workspace",
			),
			ManagerURL:   m.runtime.ManagerURL,
			RestartToken: m.runtime.RestartToken,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusRunning,
			Ports:  map[BundleComponent]int{ComponentWeb: 9876},
			PIDs:   map[BundleComponent]int{ComponentWeb: 4321},
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteLifecycle(ws, WorkspaceLifecycleState{
		DesiredState:  WorkspaceDesiredRunning,
		ObservedState: WorkspaceObservedRunning,
	}); err != nil {
		t.Fatal(err)
	}

	snaps, err := m.ListLifecycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got WorkspaceLifecycleSnapshot
	for _, snap := range snaps {
		if snap.Workspace.Slug == "foo" {
			got = snap
			break
		}
	}
	if got.Workspace.Slug == "" {
		t.Fatal("workspace foo not found")
	}
	if got.ObservedState != WorkspaceObservedStopped ||
		got.Workspace.Status != StatusStopped ||
		got.Workspace.PID != 0 ||
		got.Workspace.Port != 0 ||
		!strings.Contains(got.Error, "stale workspace env") {
		t.Fatalf("snapshot=%#v", got)
	}
}

func TestManagerRefreshPreservesTransitionWithStaleCopiedRuntimeState(t *testing.T) {
	m, checkout := newTestManager(t)
	store := FileBundleStore{}
	ws := Workspace{Slug: "foo", CheckoutPath: checkout, Bundle: RuntimePaths(checkout)}
	if err := store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug: "old-workspace",
			CheckoutPath: filepath.Join(
				filepath.Dir(checkout),
				"cn-agents-old-workspace",
			),
			ManagerURL:   m.runtime.ManagerURL,
			RestartToken: m.runtime.RestartToken,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusRunning,
			Ports:  map[BundleComponent]int{ComponentWeb: 9876},
			PIDs:   map[BundleComponent]int{ComponentWeb: 4321},
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteLifecycle(ws, WorkspaceLifecycleState{
		DesiredState:        WorkspaceDesiredRunning,
		ObservedState:       WorkspaceObservedStarting,
		TransitionKind:      WorkspaceTransitionStart,
		TransitionID:        "transition-1",
		TransitionStartedAt: m.lifecycleNow(),
		TransitionUpdatedAt: m.lifecycleNow(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok := m.Lookup("foo")
	if !ok {
		t.Fatal("workspace not found")
	}
	if got.Status != StatusStarting || got.Error == staleWorkspaceEnvError {
		t.Fatalf("workspace=%#v, want starting transition preserved", got)
	}
	if !m.transitionOwnsWorkspace("foo", "transition-1") {
		t.Fatalf("transition ownership lost after refresh")
	}
}

func TestManagerRefreshReconcilesFromMetadata(t *testing.T) {
	m, checkout := newTestManager(t)
	m.processAlive = func(pid int) bool { return pid == 4321 }
	store := FileBundleStore{}
	ws := Workspace{Slug: "foo", CheckoutPath: checkout, Bundle: RuntimePaths(checkout)}
	if err := store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug:         "foo",
			CheckoutPath: checkout,
			ManagerURL:   m.runtime.ManagerURL,
			RestartToken: m.runtime.RestartToken,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusRunning,
			Ports:  map[BundleComponent]int{ComponentWeb: 9876},
			PIDs:   map[BundleComponent]int{ComponentWeb: 4321},
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok := m.Lookup("foo")
	if !ok {
		t.Fatal("workspace not found")
	}
	if got.Status != StatusRunning || got.PID != 4321 || got.Port != 9876 {
		t.Fatalf("running reconcile=%#v", got)
	}

	m.processAlive = func(int) bool { return false }
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ = m.Lookup("foo")
	if got.Status != StatusCrashed || got.PID != 0 || got.Port != 9876 {
		t.Fatalf("crashed reconcile=%#v", got)
	}
}

func newLifecycleTestManager(t *testing.T) (*ManagerService, string) {
	t.Helper()
	m, checkout := newTestManager(t)
	ids := 0
	m.now = func() time.Time { return time.Date(2026, 5, 15, 1, 2, 3+ids, 0, time.UTC) }
	m.newTransitionID = func() string {
		ids++
		return fmt.Sprintf("transition-%d", ids)
	}
	return m, checkout
}

type recordingWorkspaceErrorSink struct {
	mu     sync.Mutex
	events []WorkspaceErrorRecordRequest
}

func (s *recordingWorkspaceErrorSink) Record(_ context.Context, req WorkspaceErrorRecordRequest) (WorkspaceErrorEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, req)
	return WorkspaceErrorEvent{ID: int64(len(s.events)), WorkspaceSlug: req.WorkspaceSlug, Source: string(req.Source), Severity: string(req.Severity), Message: req.Message, Detail: req.Detail, DedupeKey: req.DedupeKey, OccurrenceCount: 1}, nil
}

func (s *recordingWorkspaceErrorSink) recorded() []WorkspaceErrorRecordRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]WorkspaceErrorRecordRequest(nil), s.events...)
}

type fakeLifecycleStarter struct {
	starts []WorkspaceLifecycleWorkflowInput
	err    error
}

func (f *fakeLifecycleStarter) StartTransition(
	_ context.Context,
	input WorkspaceLifecycleWorkflowInput,
) error {
	f.starts = append(f.starts, input)
	return f.err
}

func newTestManager(t *testing.T) (*ManagerService, string) {
	t.Helper()
	parent := t.TempDir()
	checkout := makeCheckout(t, parent, "cn-agents-foo")
	state := filepath.Join(parent, "state")
	m, err := NewManager(RuntimeConfig{
		ManagerURL:   "https://main.cn-agents.test",
		RestartToken: "token",
	}, DiscoveryConfig{
		ParentDir: parent,
		Domain:    "cn-agents.test",
		StateDir:  state,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, checkout
}

type blockingStartRuntime struct {
	startCalls int
	started    chan struct{}
	release    chan struct{}
}

func (r *blockingStartRuntime) StartBundle(
	ctx context.Context,
	ws Workspace,
	rt RuntimeConfig,
) (Workspace, BundleHandles, error) {
	r.startCalls++
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
		return ws, nil, ctx.Err()
	}
	ws.Status = StatusRunning
	handles := BundleHandles{}
	handles[ComponentWeb] = fakeProcess(1001)
	return ws, handles, nil
}

func (r *blockingStartRuntime) StopBundle(
	context.Context,
	Workspace,
	BundleHandles,
) (Workspace, error) {
	return Workspace{}, nil
}

func (r *blockingStartRuntime) RestartComponents(
	context.Context,
	Workspace,
	BundleHandles,
	[]BundleComponent,
	RuntimeConfig,
	RestartComponentsOptions,
) (Workspace, BundleHandles, error) {
	return Workspace{}, nil, nil
}

type fakeBundleRuntime struct {
	startCalls int
	stopCalls  int
	startErr   error
	web        *ProcessHandle
}

func (f *fakeBundleRuntime) StartBundle(
	ctx context.Context,
	ws Workspace,
	rt RuntimeConfig,
) (Workspace, BundleHandles, error) {
	f.startCalls++
	if f.startErr != nil {
		return ws, nil, f.startErr
	}
	ws.Status = StatusRunning
	ws.Phase = ""
	ws.Ports = map[BundleComponent]int{ComponentWeb: 9000 + f.startCalls}
	ws.Port = ws.Ports[ComponentWeb]
	f.web = fakeProcess(1000 + f.startCalls)
	handles := BundleHandles{ComponentWeb: f.web}
	ws.PIDs = bundlePIDs(handles)
	ws.PID = ws.PIDs[ComponentWeb]
	ws.LogPath = ws.Bundle.WebLog
	store := FileBundleStore{}
	_ = store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug:         ws.Slug,
			CheckoutPath: ws.CheckoutPath,
			ManagerURL:   rt.ManagerURL,
			RestartToken: rt.RestartToken,
		},
	)
	_ = store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusRunning,
			Logs:   bundleLogs(ws.Bundle),
			Ports:  ws.Ports,
			PIDs:   ws.PIDs,
		},
	)
	return ws, handles, nil
}

func (f *fakeBundleRuntime) StopBundle(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
) (Workspace, error) {
	f.stopCalls++
	if handles[ComponentWeb] != nil {
		handles[ComponentWeb].finish(nil)
	}
	ws.Status = StatusStopped
	ws.PID = 0
	ws.PIDs = nil
	_ = FileBundleStore{}.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusStopped,
			Logs:   bundleLogs(ws.Bundle),
			Ports:  ws.Ports,
		},
	)
	return ws, nil
}

func (f *fakeBundleRuntime) RestartComponents(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
	components []BundleComponent,
	rt RuntimeConfig,
	opts RestartComponentsOptions,
) (Workspace, BundleHandles, error) {
	stopped, err := f.StopBundle(ctx, ws, handles)
	if err != nil {
		return stopped, handles, err
	}
	return f.StartBundle(ctx, stopped, rt)
}

func fakeProcess(pid int) *ProcessHandle {
	return &ProcessHandle{
		Command: &exec.Cmd{Process: &os.Process{Pid: pid}},
		done:    make(chan error, 1),
		exited:  make(chan struct{}),
	}
}

func envMap(env []string) map[string]string {
	out := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func assertEnv(t *testing.T, env map[string]string, key, want string) {
	t.Helper()
	if got := env[key]; got != want {
		t.Fatalf("%s=%q want %q", key, got, want)
	}
}

func assertEnvMissing(t *testing.T, env map[string]string, key string) {
	t.Helper()
	if value, ok := env[key]; ok {
		t.Fatalf("%s=%q, want missing", key, value)
	}
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met")
}
