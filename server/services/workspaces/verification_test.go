package workspaces

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVerifierStartRestartStopRunPasses(t *testing.T) {
	t.Parallel()

	manager := newVerificationFakeManager(t, "demo")
	prober := &verificationFakeProber{
		pidAlive:   true,
		portOpen:   true,
		statusCode: http.StatusOK,
	}
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		prober,
	)
	run := verifier.executeRun(
		context.Background(),
		VerifyWorkspaceRun{
			ID:        "run-1",
			Slug:      "demo",
			Status:    VerifyRunRunning,
			StartedAt: time.Now(),
		},
		VerifyWorkspaceRequest{Slug: "demo", Start: true, Restart: true, Stop: true},
	)

	if run.Status != VerifyRunPassed {
		t.Fatalf("status = %q, want passed; run = %#v", run.Status, run)
	}
	if !reflect.DeepEqual(
		manager.calls,
		[]string{"refresh", "start", "refresh", "restart", "stop", "refresh"},
	) {
		t.Fatalf("calls = %#v", manager.calls)
	}
	wantSnapshots := []string{
		"initial",
		"after-start",
		"before-restart",
		"after-restart",
		"final",
	}
	gotSnapshots := make([]string, 0, len(run.Snapshots))
	for _, snapshot := range run.Snapshots {
		gotSnapshots = append(gotSnapshots, snapshot.Label)
	}
	if !reflect.DeepEqual(gotSnapshots, wantSnapshots) {
		t.Fatalf("snapshots = %#v, want %#v", gotSnapshots, wantSnapshots)
	}
	wantPhases := []string{
		"refresh-discovery",
		"start",
		"metadata-log-pid-port",
		"local-host-dispatch",
		"restart",
		"stop",
	}
	gotPhases := make([]string, 0, len(run.Phases))
	for _, phase := range run.Phases {
		gotPhases = append(gotPhases, phase.Name)
	}
	if !reflect.DeepEqual(gotPhases, wantPhases) {
		t.Fatalf("phases = %#v, want %#v", gotPhases, wantPhases)
	}
}

func TestVerifierHostDispatchUsesManagerListenAddress(t *testing.T) {
	t.Parallel()

	manager := newVerificationFakeManager(t, "demo")
	prober := &verificationFakeProber{
		pidAlive:   true,
		portOpen:   true,
		statusCode: http.StatusUnauthorized,
	}
	verifier := NewVerifier(
		manager,
		"0.0.0.0:4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		prober,
	)
	if err := verifier.assertHostDispatch(context.Background(), "demo"); err != nil {
		t.Fatalf("assertHostDispatch: %v", err)
	}
	if prober.httpAddr != "127.0.0.1:4200" {
		t.Fatalf("http addr = %q, want manager listen addr", prober.httpAddr)
	}
	if prober.httpAddr == manager.workspace.LocalAddr() {
		t.Fatalf("host dispatch probed child local addr %q", prober.httpAddr)
	}
	if prober.httpHost != "demo.cn-agents.test" {
		t.Fatalf("host = %q", prober.httpHost)
	}
}

func TestVerifierVerifyBundleReportsComponentHealth(t *testing.T) {
	t.Parallel()

	manager := newVerificationFakeManager(t, "demo")
	paths := RuntimePaths(manager.workspace.CheckoutPath)
	manager.workspace.Bundle = paths
	manager.workspace.Ports = map[BundleComponent]int{
		ComponentWeb:        4101,
		ComponentTemporal:   7233,
		ComponentTemporalUI: 8233,
	}
	manager.workspace.PIDs = map[BundleComponent]int{
		ComponentWeb:      101,
		ComponentTemporal: 102,
		ComponentTSWorker: 103,
	}
	manager.workspace.Port = 4101
	manager.workspace.PID = 101
	if err := EnsureRuntimeDirs(paths); err != nil {
		t.Fatalf("EnsureRuntimeDirs: %v", err)
	}
	if err := (FileBundleStore{}).WriteStatus(manager.workspace, RuntimeStatus{
		Status: StatusRunning,
		Logs:   bundleLogs(paths),
		Ports:  manager.workspace.Ports,
		PIDs:   manager.workspace.PIDs,
		Build:  BuildStatus{LogPath: paths.BuildLog},
	}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	writeTestFile(t, paths.TSReadyMarker, "ready\n")
	prober := &verificationFakeProber{
		pidAlive:   true,
		portOpen:   true,
		statusCode: http.StatusOK,
	}
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		prober,
	)
	run := verifier.VerifyBundle(context.Background(), "demo")

	if run.Status != VerifyRunPassed {
		t.Fatalf("status = %q, errors = %#v", run.Status, run.Errors)
	}
	if !run.WebOK || !run.TemporalOK || !run.TSWorkerOK {
		t.Fatalf(
			"component health web/temporal/ts = %v/%v/%v",
			run.WebOK,
			run.TemporalOK,
			run.TSWorkerOK,
		)
	}
	if run.Runtime.Build.LogPath != paths.BuildLog {
		t.Fatalf(
			"build log path = %q, want %q",
			run.Runtime.Build.LogPath,
			paths.BuildLog,
		)
	}
	if prober.httpHost != "demo.cn-agents.test" {
		t.Fatalf("temporal UI host = %q", prober.httpHost)
	}
	if !reflect.DeepEqual(manager.calls, []string{"refresh"}) {
		t.Fatalf("manager calls = %#v, want refresh only", manager.calls)
	}
}

func TestVerifierVerifyBundleReportsFailureDetails(t *testing.T) {
	t.Parallel()

	manager := newVerificationFakeManager(t, "demo")
	paths := RuntimePaths(manager.workspace.CheckoutPath)
	manager.workspace.Bundle = paths
	manager.workspace.Ports = map[BundleComponent]int{ComponentWeb: 4101}
	manager.workspace.PIDs = map[BundleComponent]int{ComponentWeb: 101}
	if err := (FileBundleStore{}).WriteStatus(manager.workspace, RuntimeStatus{
		Status: StatusRunning,
		Ports:  manager.workspace.Ports,
		PIDs:   manager.workspace.PIDs,
	}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		&verificationFakeProber{
			pidAlive:   false,
			portOpen:   false,
			statusCode: http.StatusOK,
		},
	)
	run := verifier.VerifyBundle(context.Background(), "demo")

	if run.Status != VerifyRunFailed {
		t.Fatalf("status = %q, want failed", run.Status)
	}
	joined := strings.Join(run.Errors, "\n")
	for _, want := range []string{"web PID or port", "temporal PID or port", "ts worker PID"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("errors = %#v, want %q", run.Errors, want)
		}
	}
}

func TestVerifierClassifiesMetadataFailure(t *testing.T) {
	t.Parallel()

	manager := newVerificationFakeManager(t, "demo")
	if err := removeMetadataForTest(manager.workspace.CheckoutPath); err != nil {
		t.Fatalf("remove metadata: %v", err)
	}
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		&verificationFakeProber{
			pidAlive:   true,
			portOpen:   true,
			statusCode: http.StatusOK,
		},
	)
	run := verifier.executeRun(
		context.Background(),
		VerifyWorkspaceRun{
			ID:        "run-1",
			Slug:      "demo",
			Status:    VerifyRunRunning,
			StartedAt: time.Now(),
		},
		VerifyWorkspaceRequest{Slug: "demo"},
	)
	if run.Status != VerifyRunFailed {
		t.Fatalf("status = %q, want failed", run.Status)
	}
	if run.Error == nil || run.Error.Layer != VerificationLayerMetadata {
		t.Fatalf("error = %#v, want metadata layer", run.Error)
	}
}

func TestVerifierLifecycleStartPollsUntilTerminal(t *testing.T) {
	t.Parallel()

	manager := newVerificationLifecycleFakeManager(t, "demo")
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		&verificationFakeProber{
			pidAlive:   true,
			portOpen:   true,
			statusCode: http.StatusOK,
		},
	)
	run := verifier.executeRun(
		context.Background(),
		VerifyWorkspaceRun{
			ID:        "run-1",
			Slug:      "demo",
			Status:    VerifyRunRunning,
			StartedAt: time.Now(),
		},
		VerifyWorkspaceRequest{Slug: "demo", Start: true},
	)

	if run.Status != VerifyRunPassed {
		t.Fatalf("status = %q, error = %#v", run.Status, run.Error)
	}
	if !reflect.DeepEqual(
		manager.calls,
		[]string{"refresh", "request-start", "reconcile", "refresh", "refresh"},
	) {
		t.Fatalf("calls = %#v", manager.calls)
	}
}

func TestVerifierLifecycleFailureFailsPhase(t *testing.T) {
	t.Parallel()

	manager := newVerificationLifecycleFakeManager(t, "demo")
	manager.failState = WorkspaceObservedFailed
	manager.failError = "start failed"
	verifier := NewVerifier(
		manager,
		":4200",
		NewMemoryVerifyRunStore(),
		verificationFakeTailer{},
		&verificationFakeProber{
			pidAlive:   true,
			portOpen:   true,
			statusCode: http.StatusOK,
		},
	)
	run := verifier.executeRun(
		context.Background(),
		VerifyWorkspaceRun{
			ID:        "run-1",
			Slug:      "demo",
			Status:    VerifyRunRunning,
			StartedAt: time.Now(),
		},
		VerifyWorkspaceRequest{Slug: "demo", Start: true},
	)

	if run.Status != VerifyRunFailed {
		t.Fatalf("status = %q, want failed", run.Status)
	}
	if run.Error == nil || !strings.Contains(run.Error.Message, "start failed") {
		t.Fatalf("error = %#v", run.Error)
	}
}

func TestMemoryVerifyRunStoreSubscribe(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := NewMemoryVerifyRunStore()
	run, err := store.Create(ctx, VerifyWorkspaceRequest{Slug: "demo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updates, err := store.Subscribe(ctx, run.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	<-updates
	run.Status = VerifyRunPassed
	if err := store.Update(ctx, run); err != nil {
		t.Fatalf("Update: %v", err)
	}
	select {
	case got := <-updates:
		if got.Status != VerifyRunPassed {
			t.Fatalf("status = %q", got.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
	}
}

type verificationLifecycleFakeManager struct {
	*verificationFakeManager
	lastRequest WorkspaceLifecycleRequest
	failState   WorkspaceObservedState
	failError   string
}

func newVerificationLifecycleFakeManager(
	t *testing.T,
	slug string,
) *verificationLifecycleFakeManager {
	t.Helper()
	return &verificationLifecycleFakeManager{
		verificationFakeManager: newVerificationFakeManager(t, slug),
	}
}

func (m *verificationLifecycleFakeManager) RequestLifecycle(
	_ context.Context,
	req WorkspaceLifecycleRequest,
) (WorkspaceLifecycleSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "request-"+string(req.Kind))
	m.lastRequest = req
	observed := WorkspaceObservedStarting
	if req.Kind == WorkspaceTransitionStop {
		observed = WorkspaceObservedStopping
	}
	m.workspace.Status = statusFromObserved(observed)
	return WorkspaceLifecycleSnapshot{
		Workspace:      m.workspace,
		DesiredState:   req.DesiredState,
		ObservedState:  observed,
		TransitionID:   "transition-1",
		TransitionKind: req.Kind,
	}, nil
}

func (m *verificationLifecycleFakeManager) ReconcileWorkspace(
	_ context.Context,
	slug string,
) (WorkspaceLifecycleSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "reconcile")
	if slug != m.workspace.Slug {
		return WorkspaceLifecycleSnapshot{}, fmt.Errorf("unknown workspace %q", slug)
	}
	if m.failState != "" {
		m.workspace.Status = statusFromObserved(m.failState)
		m.workspace.Error = m.failError
		return WorkspaceLifecycleSnapshot{
			Workspace:     m.workspace,
			ObservedState: m.failState,
			Error:         m.failError,
		}, nil
	}
	observed := WorkspaceObservedRunning
	if m.lastRequest.Kind == WorkspaceTransitionStop {
		observed = WorkspaceObservedStopped
		m.workspace.PID = 0
	} else {
		m.workspace.PID = 101
	}
	m.workspace.Status = statusFromObserved(observed)
	return WorkspaceLifecycleSnapshot{
		Workspace:     m.workspace,
		DesiredState:  m.lastRequest.DesiredState,
		ObservedState: observed,
	}, nil
}

type verificationFakeManager struct {
	mu        sync.Mutex
	workspace Workspace
	calls     []string
}

func newVerificationFakeManager(t *testing.T, slug string) *verificationFakeManager {
	t.Helper()
	checkout := t.TempDir()
	stateDir := t.TempDir()
	logPath := filepath.Join(stateDir, "agents-server.log")
	writeTestFile(t, logPath, "log line\n")
	if err := WriteMetadata(
		WorkspaceMetadataPath(checkout),
		WorkspaceMetadata{
			Slug:         slug,
			CheckoutPath: checkout,
			ManagerURL:   "https://main.cn-agents.test",
			PID:          101,
			Port:         4101,
		},
	); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	return &verificationFakeManager{
		workspace: Workspace{
			Slug:         slug,
			CheckoutPath: checkout,
			Host:         slug + ".cn-agents.test",
			URL:          "https://" + slug + ".cn-agents.test/",
			Status:       StatusRunning,
			Port:         4101,
			PID:          101,
			StateDir:     stateDir,
			LogPath:      logPath,
		},
	}
}

func (m *verificationFakeManager) Refresh(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "refresh")
	return nil
}

func (m *verificationFakeManager) List() []Workspace { return []Workspace{m.workspace} }
func (m *verificationFakeManager) Lookup(slug string) (Workspace, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.workspace, slug == m.workspace.Slug
}

func (m *verificationFakeManager) LookupHost(host string) (Workspace, bool) {
	return m.workspace, host == m.workspace.Host
}

func (m *verificationFakeManager) Start(context.Context, string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "start")
	m.workspace.Status = StatusRunning
	m.workspace.PID = 101
	return m.workspace, nil
}

func (m *verificationFakeManager) Stop(context.Context, string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "stop")
	m.workspace.Status = StatusStopped
	return m.workspace, nil
}

func (m *verificationFakeManager) Restart(context.Context, string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "restart")
	m.workspace.Status = StatusRunning
	m.workspace.PID++
	return m.workspace, nil
}

type verificationFakeTailer struct{}

func (verificationFakeTailer) Tail(string, int) (string, error) { return "log line", nil }

type verificationFakeProber struct {
	pidAlive   bool
	portOpen   bool
	statusCode int
	httpAddr   string
	httpHost   string
}

func (p *verificationFakeProber) PIDAlive(int) bool    { return p.pidAlive }
func (p *verificationFakeProber) PortOpen(string) bool { return p.portOpen }

func (p *verificationFakeProber) HTTPHost(
	ctx context.Context,
	addr, host, path string,
) (*http.Response, []byte, error) {
	p.httpAddr = addr
	p.httpHost = host
	status := p.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, []byte(
			"ok",
		), nil
}

func removeMetadataForTest(checkout string) error {
	return os.Remove(WorkspaceMetadataPath(checkout))
}
