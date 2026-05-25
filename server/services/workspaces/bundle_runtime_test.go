package workspaces

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceRuntimeStartBundleOrdersComponentsAndPersistsRunningAfterReadiness(
	t *testing.T,
) {
	ws := bundleTestWorkspace(t)
	starter := &recordingStarter{}
	prober := &recordingProber{}
	runtime := &WorkspaceRuntime{
		store:         FileBundleStore{},
		starter:       starter,
		stopper:       starter,
		prober:        prober,
		portAllocator: sequentialPorts(4300),
		now:           func() time.Time { return time.Unix(100, 0) },
	}

	started, handles, err := runtime.StartBundle(
		context.Background(),
		ws,
		RuntimeConfig{
			ManagerURL:   "https://main.test",
			RestartToken: "token",
			BaseEnv:      map[string]string{"GOOGLE_CREDENTIALS_FILE": "/google.json"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != StatusRunning || started.PID == 0 ||
		handles[ComponentWeb] == nil {
		t.Fatalf("started=%#v handles=%#v", started, handles)
	}
	wantOrder := []BundleComponent{ComponentTemporal, ComponentWeb, ComponentTSWorker}
	if !reflect.DeepEqual(starter.started, wantOrder) {
		t.Fatalf("start order=%v want %v", starter.started, wantOrder)
	}
	if !reflect.DeepEqual(
		prober.calls,
		[]string{"temporal", "web", "web-worker", "fresh-file"},
	) {
		t.Fatalf("prober calls=%v", prober.calls)
	}
	status, err := FileBundleStore{}.ReadStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusRunning || status.Phase != "" ||
		status.PIDs[ComponentWeb] == 0 ||
		status.Ports[ComponentTemporal] == 0 {
		t.Fatalf("status=%#v", status)
	}
}

func TestWorkspaceRuntimeStartBundleWritesRuntimeEnvSnapshot(t *testing.T) {
	ws := bundleTestWorkspace(t)
	starter := &recordingStarter{}
	runtime := &WorkspaceRuntime{
		store:         FileBundleStore{},
		starter:       starter,
		stopper:       starter,
		prober:        &recordingProber{},
		portAllocator: sequentialPorts(4300),
		now:           func() time.Time { return time.Unix(100, 0) },
	}

	started, _, err := runtime.StartBundle(
		context.Background(),
		ws,
		RuntimeConfig{ManagerURL: "https://main.test", RestartToken: "token"},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := FileBundleStore{}.ReadRuntimeEnvSnapshot(started)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WorkspaceSlug != ws.Slug || snapshot.CheckoutPath != ws.CheckoutPath {
		t.Fatalf("snapshot identity=%#v", snapshot)
	}
	if snapshot.Web.PID != started.PIDs[ComponentWeb] || snapshot.TSWorker.PID != started.PIDs[ComponentTSWorker] {
		t.Fatalf("snapshot pids=%#v started=%#v", snapshot, started.PIDs)
	}
	if snapshot.Web.InternalCallbackBaseURL != "http://127.0.0.1:4301" || snapshot.Web.TemporalAddress != "127.0.0.1:4302" {
		t.Fatalf("snapshot web proof=%#v", snapshot.Web)
	}
	if snapshot.TSWorker.TaskQueue != "agents-ts" || snapshot.TSWorker.ReadyMarker != RuntimePaths(ws.CheckoutPath).TSReadyMarker {
		t.Fatalf("snapshot ts proof=%#v", snapshot.TSWorker)
	}
}

func TestWorkspaceRuntimeStartBundleFailureStopsReverseAndPersistsFailedPhase(
	t *testing.T,
) {
	ws := bundleTestWorkspace(t)
	starter := &recordingStarter{}
	runtime := &WorkspaceRuntime{
		store:         FileBundleStore{},
		starter:       starter,
		stopper:       starter,
		prober:        &recordingProber{failWebWorker: errors.New("go worker not ready")},
		portAllocator: sequentialPorts(4400),
	}

	started, _, err := runtime.StartBundle(
		context.Background(),
		ws,
		RuntimeConfig{ManagerURL: "https://main.test"},
	)
	if err == nil || !strings.Contains(err.Error(), "go worker not ready") {
		t.Fatalf("err=%v", err)
	}
	if started.Status != StatusFailed || started.Phase != PhaseStartingWeb {
		t.Fatalf("started=%#v", started)
	}
	wantStopped := []BundleComponent{ComponentWeb, ComponentTemporal}
	if !reflect.DeepEqual(starter.stopped, wantStopped) {
		t.Fatalf("stop order=%v want %v", starter.stopped, wantStopped)
	}
	status, err := FileBundleStore{}.ReadStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusFailed || status.Phase != PhaseStartingWeb ||
		!strings.Contains(status.Error, "go worker not ready") {
		t.Fatalf("status=%#v", status)
	}
}

func TestTemporalArgsExactStartDevCommand(t *testing.T) {
	ws := Workspace{CheckoutPath: "/tmp/checkout"}
	got := TemporalArgs(ws, 7234, 8234)
	want := []string{
		"temporal",
		"server",
		"start-dev",
		"--db-filename",
		RuntimePaths(ws.CheckoutPath).TemporalDB,
		"--port",
		"7234",
		"--ui-port",
		"8234",
		"--ui-public-path",
		"/temporal",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TemporalArgs=%v want %v", got, want)
	}
}

func TestTSWorkerArgsRunFromPackagePath(t *testing.T) {
	got := TSWorkerArgs(Workspace{PackagePath: "/tmp/checkout/pkg/agents"})
	want := []string{"node", "dist/pkg/agents/temporal/workers/ts/worker.js"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TSWorkerArgs=%v want %v", got, want)
	}
}

func TestRuntimePathsIncludesRuntimeEnvSnapshot(t *testing.T) {
	paths := RuntimePaths("/tmp/checkout")
	want := filepath.Join("/tmp/checkout", ".vamos", "run", "runtime-env.json")
	if paths.RuntimeEnvSnapshot != want {
		t.Fatalf("RuntimeEnvSnapshot=%q want %q", paths.RuntimeEnvSnapshot, want)
	}
}

func TestBuildRuntimeEnvSnapshotRecordsSelectedChildProof(t *testing.T) {
	ws := Workspace{
		Slug:            "foo",
		CheckoutPath:    "/tmp/checkout",
		MetadataDirName: ".vamos",
		URL:             "https://foo.test/",
	}
	ports := map[BundleComponent]int{
		ComponentWeb:        4301,
		ComponentTemporal:   4302,
		ComponentTemporalUI: 4303,
	}
	pids := map[BundleComponent]int{ComponentWeb: 2002, ComponentTSWorker: 2003}
	writtenAt := time.Unix(123, 0)

	snapshot := BuildRuntimeEnvSnapshot(ws, RuntimeConfig{}, ports, pids, writtenAt)

	paths := RuntimePaths(ws.CheckoutPath, ws.MetadataDirName)
	if snapshot.Version != 1 || snapshot.WorkspaceSlug != ws.Slug || snapshot.CheckoutPath != ws.CheckoutPath || !snapshot.WrittenAt.Equal(writtenAt.UTC()) {
		t.Fatalf("snapshot identity=%#v", snapshot)
	}
	if snapshot.Web.PID != 2002 || snapshot.Web.ListenAddress != "127.0.0.1:4301" ||
		snapshot.Web.PublicBaseURL != "https://foo.test" ||
		snapshot.Web.InternalCallbackBaseURL != "http://127.0.0.1:4301" ||
		snapshot.Web.TemporalAddress != "127.0.0.1:4302" ||
		snapshot.Web.TemporalUIBaseURL != "http://127.0.0.1:4303" ||
		snapshot.Web.DatabasePath != paths.AgentsDB ||
		snapshot.Web.DefaultCWD != ws.CheckoutPath {
		t.Fatalf("web proof=%#v", snapshot.Web)
	}
	if snapshot.TSWorker.PID != 2003 || snapshot.TSWorker.TemporalAddress != "127.0.0.1:4302" ||
		snapshot.TSWorker.DefaultCWD != ws.CheckoutPath || snapshot.TSWorker.TaskQueue != "agents-ts" ||
		snapshot.TSWorker.ReadyMarker != paths.TSReadyMarker {
		t.Fatalf("ts proof=%#v", snapshot.TSWorker)
	}
	for _, forbidden := range []string{"token", "secret", "auth.json"} {
		if strings.Contains(snapshot.Web.PublicBaseURL, forbidden) || strings.Contains(snapshot.TSWorker.ReadyMarker, forbidden) {
			t.Fatalf("snapshot leaked forbidden value %q: %#v", forbidden, snapshot)
		}
	}
	if !containsString(snapshot.Web.RedactedKeys, "VAMOS_WORKSPACE_RESTART_TOKEN") ||
		!containsString(snapshot.Web.RedactedKeys, "VAMOS_INTERNAL_TOKEN") ||
		!containsString(snapshot.TSWorker.RedactedKeys, "PI_AUTH_PATH") {
		t.Fatalf("redacted keys web=%v ts=%v", snapshot.Web.RedactedKeys, snapshot.TSWorker.RedactedKeys)
	}
}

func TestTSWorkerEnvIncludesWorkspaceTemporalReadyMarkerAndPiAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ws := Workspace{Slug: "foo", CheckoutPath: "/tmp/checkout"}
	env := envMap(TSWorkerEnv(map[string]string{"PI_MODEL_ID": "gpt-5.5"}, ws, 7234))
	assertEnv(t, env, "TEMPORAL_ADDR", "127.0.0.1:7234")
	assertEnv(
		t,
		env,
		"VAMOS_TS_WORKER_READY_FILE",
		RuntimePaths(ws.CheckoutPath).TSReadyMarker,
	)
	assertEnv(t, env, "VAMOS_WORKSPACE_SLUG", "foo")
	assertEnv(t, env, "VAMOS_DEFAULT_CWD", "/tmp/checkout")
	assertEnv(t, env, "VAMOS_TS_WORKER_TASK_QUEUE", "agents-ts")
	assertEnv(t, env, "PI_AUTH_PATH", filepath.Join(home, ".pi", "agent", "auth.json"))
	assertEnv(t, env, "PI_MODEL_ID", "gpt-5.5")
}

func TestReadTSWorkerIdentityMarkerParsesStructuredJSON(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "ts-worker.ready")
	startedAt := time.Unix(123, 0).UTC()
	body := `{
		"version": 1,
		"pid": 2003,
		"started_at": "` + startedAt.Format(time.RFC3339) + `",
		"workspace_slug": "foo",
		"checkout_path": "/tmp/checkout",
		"temporal_address": "127.0.0.1:7234",
		"task_queue": "agents-ts",
		"ready_marker": "` + markerPath + `"
	}`
	if err := os.WriteFile(markerPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	marker, err := ReadTSWorkerIdentityMarker(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Version != 1 || marker.PID != 2003 || !marker.StartedAt.Equal(startedAt) ||
		marker.WorkspaceSlug != "foo" || marker.CheckoutPath != "/tmp/checkout" ||
		marker.TemporalAddress != "127.0.0.1:7234" || marker.TaskQueue != "agents-ts" ||
		marker.ReadyMarker != markerPath {
		t.Fatalf("marker=%#v", marker)
	}
}

func TestVerifyTSWorkerIdentityAcceptsExpectedMarker(t *testing.T) {
	ws := Workspace{Slug: "foo", CheckoutPath: "/tmp/checkout", MetadataDirName: ".vamos"}
	runtime := RuntimeStatus{
		Ports: map[BundleComponent]int{ComponentTemporal: 7234},
		PIDs:  map[BundleComponent]int{ComponentTSWorker: 2003},
	}
	marker := TSWorkerIdentityMarker{
		Version:         1,
		PID:             2003,
		WorkspaceSlug:   "foo",
		CheckoutPath:    "/tmp/checkout",
		TemporalAddress: "127.0.0.1:7234",
		TaskQueue:       "agents-ts",
		ReadyMarker:     RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).TSReadyMarker,
	}

	if err := VerifyTSWorkerIdentity(ws, runtime, marker); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyTSWorkerIdentityRejectsMismatch(t *testing.T) {
	ws := Workspace{Slug: "foo", CheckoutPath: "/tmp/checkout", MetadataDirName: ".vamos"}
	runtime := RuntimeStatus{
		Ports: map[BundleComponent]int{ComponentTemporal: 7234},
		PIDs:  map[BundleComponent]int{ComponentTSWorker: 2003},
	}
	base := TSWorkerIdentityMarker{
		Version:         1,
		PID:             2003,
		WorkspaceSlug:   "foo",
		CheckoutPath:    "/tmp/checkout",
		TemporalAddress: "127.0.0.1:7234",
		TaskQueue:       "agents-ts",
		ReadyMarker:     RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).TSReadyMarker,
	}
	cases := map[string]func(*TSWorkerIdentityMarker){
		"slug":     func(marker *TSWorkerIdentityMarker) { marker.WorkspaceSlug = "bar" },
		"checkout": func(marker *TSWorkerIdentityMarker) { marker.CheckoutPath = "/tmp/other" },
		"temporal": func(marker *TSWorkerIdentityMarker) { marker.TemporalAddress = "127.0.0.1:9999" },
		"queue":    func(marker *TSWorkerIdentityMarker) { marker.TaskQueue = "other" },
		"pid":      func(marker *TSWorkerIdentityMarker) { marker.PID = 9999 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			marker := base
			mutate(&marker)
			if err := VerifyTSWorkerIdentity(ws, runtime, marker); err == nil {
				t.Fatalf("VerifyTSWorkerIdentity(%s) succeeded", name)
			}
		})
	}
}

func TestFreshFileExistsWaitsPastStaleMarker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ts-worker.ready")
	if err := os.WriteFile(marker, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Unix(100, 0)
	if err := os.Chtimes(marker, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	notBefore := oldTime.Add(time.Second)
	done := make(chan error, 1)
	go func() {
		done <- LocalReadinessProber{}.FreshFileExists(context.Background(), marker, notBefore)
	}()

	select {
	case err := <-done:
		t.Fatalf("FreshFileExists returned before marker became fresh: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	freshTime := notBefore.Add(time.Second)
	if err := os.Chtimes(marker, freshTime, freshTime); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("FreshFileExists fresh err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FreshFileExists did not accept fresh marker")
	}
}

func TestChildEnvIncludesWorkspaceTemporalAndState(t *testing.T) {
	ws := Workspace{Slug: "foo", CheckoutPath: "/tmp/checkout", URL: "https://foo.test/"}
	ports := map[BundleComponent]int{
		ComponentWeb:        4100,
		ComponentTemporal:   7234,
		ComponentTemporalUI: 8234,
	}
	env := envMap(
		ChildEnv(
			map[string]string{},
			ws,
			ports,
			RuntimeConfig{
				ManagerURL:       "https://main.test",
				RestartToken:     "token",
				DevAuthVerifyKey: "verify",
				ThoughtsRepo:     "/tmp/host",
				ThoughtsRoot:     "/tmp/host/thoughts",
			},
		),
	)
	assertWorkspaceRuntimePaths(t, ws, ws.CheckoutPath)
	assertEnv(t, env, "CN_TEMPORAL", "true")
	assertEnv(t, env, "TEMPORAL_ADDRESS", "127.0.0.1:7234")
	assertEnv(t, env, "TEMPORAL_UI_BASE_URL", "http://127.0.0.1:8234")
	assertEnv(t, env, "VAMOS_DATABASE_PATH", RuntimePaths(ws.CheckoutPath).AgentsDB)
	assertEnv(t, env, "OPENCLAW_STATE_DIR", RuntimePaths(ws.CheckoutPath).OpenClawDir)
	assertEnv(t, env, "VAMOS_WORKSPACE_MODE", "child")
	assertEnv(t, env, "VAMOS_WORKSPACE_MANAGER_URL", "https://main.test")
	assertEnv(t, env, "VAMOS_WORKSPACE_RESTART_TOKEN", "token")
	assertEnv(t, env, "VAMOS_INTERNAL_CALLBACK_BASE_URL", "http://127.0.0.1:4100")
	assertEnv(t, env, "VAMOS_THOUGHTS_ROOT", "/tmp/host/thoughts")
}

func TestConfiguredCheckoutUsesFeatureWorkspaceRuntimeShape(t *testing.T) {
	parent := t.TempDir()
	checkout := makeCheckout(t, parent, "vamos-work")

	workspaces, err := Discover(DiscoveryConfig{
		ParentDir: parent,
		Domain:    "workspaces.example.test",
		ConfiguredCheckouts: map[string]ConfiguredCheckout{
			"work": {RootPath: checkout, DisplayName: "Working checkout"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("discovered %d workspaces, want 1", len(workspaces))
	}
	ws := workspaces[0]
	if ws.Slug != "work" || ws.URL != "https://work.workspaces.example.test/" {
		t.Fatalf("workspace = %#v", ws)
	}
	assertWorkspaceRuntimePaths(t, ws, checkout)

	ports := map[BundleComponent]int{
		ComponentWeb:        4100,
		ComponentTemporal:   7234,
		ComponentTemporalUI: 8234,
	}
	env := envMap(ChildEnv(nil, ws, ports, RuntimeConfig{ManagerURL: "https://main.test", RestartToken: "token"}))
	assertEnv(t, env, "VAMOS_WORKSPACE_MODE", "child")
	assertEnv(t, env, "VAMOS_DATABASE_PATH", filepath.Join(checkout, ".vamos", "state", "agents.db"))
	assertEnv(t, env, "TEMPORAL_ADDRESS", "127.0.0.1:7234")
	assertEnv(t, env, "OPENCLAW_STATE_DIR", filepath.Join(checkout, ".vamos", "state", "openclaw"))
	assertEnv(t, env, "VAMOS_WORKSPACE_MANAGER_URL", "https://main.test")
	assertEnv(t, env, "VAMOS_WORKSPACE_RESTART_TOKEN", "token")
}

func assertWorkspaceRuntimePaths(t *testing.T, ws Workspace, checkout string) {
	t.Helper()
	paths := RuntimePaths(checkout, ".vamos")
	if paths.AgentsDB != filepath.Join(checkout, ".vamos", "state", "agents.db") {
		t.Fatalf("AgentsDB = %q", paths.AgentsDB)
	}
	if paths.TemporalDB != filepath.Join(checkout, ".vamos", "state", "temporal.db") {
		t.Fatalf("TemporalDB = %q", paths.TemporalDB)
	}
	if paths.OpenClawDir != filepath.Join(checkout, ".vamos", "state", "openclaw") {
		t.Fatalf("OpenClawDir = %q", paths.OpenClawDir)
	}
	if ws.Bundle.AgentsDB != "" && ws.Bundle.AgentsDB != paths.AgentsDB {
		t.Fatalf("workspace bundle AgentsDB = %q, want %q", ws.Bundle.AgentsDB, paths.AgentsDB)
	}
	if ws.Bundle.TemporalDB != "" && ws.Bundle.TemporalDB != paths.TemporalDB {
		t.Fatalf("workspace bundle TemporalDB = %q, want %q", ws.Bundle.TemporalDB, paths.TemporalDB)
	}
}

func TestManagerRestartReturnsStopErrorWithoutStart(t *testing.T) {
	m, _ := newTestManager(t)
	runtime := &stopFailRuntime{err: errors.New("cannot stop")}
	m.bundleRuntime = runtime
	if _, err := m.Restart(
		context.Background(),
		"foo",
	); err == nil ||
		!strings.Contains(err.Error(), "cannot stop") {
		t.Fatalf("Restart err=%v", err)
	}
	if runtime.startCalls != 0 {
		t.Fatalf("startCalls=%d want 0", runtime.startCalls)
	}
}

func bundleTestWorkspace(t *testing.T) Workspace {
	t.Helper()
	checkout := t.TempDir()
	packagePath := filepath.Join(checkout, "pkg", "agents")
	if err := os.MkdirAll(packagePath, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := RuntimePaths(checkout)
	return Workspace{
		Slug:         "foo",
		CheckoutPath: checkout,
		PackagePath:  packagePath,
		Host:         "foo.test",
		URL:          "https://foo.test/",
		Bundle:       paths,
		LogPath:      paths.WebLog,
	}
}

func sequentialPorts(next int) func() (int, error) {
	return func() (int, error) {
		next++
		return next, nil
	}
}

func TestWorkspaceRuntimeRestartWebDoesNotRestartTemporal(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234}
	handles := BundleHandles{
		ComponentTemporal: handleFor(ComponentTemporal, 1001),
		ComponentWeb:      handleFor(ComponentWeb, 1002),
		ComponentTSWorker: handleFor(ComponentTSWorker, 1003),
	}
	starter := &recordingStarter{}
	prober := &recordingProber{}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  prober,
	}

	restarted, gotHandles, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{},
		RestartComponentsOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status != StatusRunning {
		t.Fatalf("status = %q, want running", restarted.Status)
	}
	if !reflect.DeepEqual(starter.stopped, []BundleComponent{ComponentWeb}) {
		t.Fatalf("stopped = %v, want web only", starter.stopped)
	}
	if !reflect.DeepEqual(starter.started, []BundleComponent{ComponentWeb}) {
		t.Fatalf("started = %v, want web only", starter.started)
	}
	if gotHandles[ComponentTemporal] != handles[ComponentTemporal] {
		t.Fatal("temporal handle was replaced")
	}
}

func TestWorkspaceRuntimeRestartWebUpdatesRuntimeEnvSnapshot(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234, ComponentTemporalUI: 8234}
	handles := BundleHandles{
		ComponentTemporal: handleFor(ComponentTemporal, 1001),
		ComponentWeb:      handleFor(ComponentWeb, 1002),
		ComponentTSWorker: handleFor(ComponentTSWorker, 1003),
	}
	starter := &recordingStarter{}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  &recordingProber{},
		now:     func() time.Time { return time.Unix(200, 0) },
	}

	restarted, _, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{},
		RestartComponentsOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := FileBundleStore{}.ReadRuntimeEnvSnapshot(restarted)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Web.PID != restarted.PIDs[ComponentWeb] || snapshot.Web.PID == 1002 {
		t.Fatalf("web pid snapshot=%d restarted=%#v", snapshot.Web.PID, restarted.PIDs)
	}
	if snapshot.TSWorker.PID != 1003 || snapshot.Web.TemporalAddress != "127.0.0.1:7234" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestWorkspaceRuntimeForceRestartWebIgnoresStopFailure(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusFailed
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234}
	handles := BundleHandles{
		ComponentTemporal: handleFor(ComponentTemporal, 1001),
		ComponentWeb:      handleFor(ComponentWeb, 1002),
		ComponentTSWorker: handleFor(ComponentTSWorker, 1003),
	}
	oldWeb := handles[ComponentWeb]
	starter := &recordingStarter{stopErr: errors.New("stop stuck")}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  &recordingProber{},
	}

	restarted, gotHandles, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{},
		RestartComponentsOptions{Force: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status != StatusRunning || restarted.PID == 0 ||
		restarted.PIDs[ComponentWeb] == 0 {
		t.Fatalf("restarted=%#v", restarted)
	}
	if !reflect.DeepEqual(starter.stopped, []BundleComponent{ComponentWeb}) {
		t.Fatalf("stopped = %v, want web", starter.stopped)
	}
	if !reflect.DeepEqual(starter.started, []BundleComponent{ComponentWeb}) {
		t.Fatalf("started = %v, want web", starter.started)
	}
	if gotHandles[ComponentWeb] == oldWeb {
		t.Fatal("web handle was not replaced")
	}
	if restarted.Ports[ComponentWeb] == 4300 {
		t.Fatalf("web port = %d, want fresh force-restart port", restarted.Ports[ComponentWeb])
	}
}

func TestWorkspaceRuntimeRestartTSWorkerUpdatesRuntimeEnvSnapshot(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234, ComponentTemporalUI: 8234}
	handles := BundleHandles{
		ComponentTemporal: handleFor(ComponentTemporal, 1001),
		ComponentWeb:      handleFor(ComponentWeb, 1002),
		ComponentTSWorker: handleFor(ComponentTSWorker, 1003),
	}
	starter := &recordingStarter{}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  &recordingProber{},
		now:     func() time.Time { return time.Unix(300, 0) },
	}

	restarted, _, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentTSWorker},
		RuntimeConfig{},
		RestartComponentsOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := FileBundleStore{}.ReadRuntimeEnvSnapshot(restarted)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TSWorker.PID != restarted.PIDs[ComponentTSWorker] || snapshot.TSWorker.PID == 1003 {
		t.Fatalf("ts pid snapshot=%d restarted=%#v", snapshot.TSWorker.PID, restarted.PIDs)
	}
	if snapshot.Web.PID != 1002 || snapshot.TSWorker.TemporalAddress != "127.0.0.1:7234" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestWorkspaceRuntimeRestartComponentsRewritesWorkspaceEnv(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234}
	stale := WorkspaceEnv{
		Slug:         "old-slug",
		CheckoutPath: ws.CheckoutPath,
		ManagerURL:   "https://old.example.test",
		RestartToken: "old-token",
	}
	if err := (FileBundleStore{}).WriteWorkspaceEnv(ws, stale); err != nil {
		t.Fatal(err)
	}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: &recordingStarter{},
		stopper: &recordingStarter{},
		prober:  &recordingProber{},
	}

	_, _, err := runtime.RestartComponents(
		context.Background(),
		ws,
		BundleHandles{ComponentWeb: handleFor(ComponentWeb, 1002)},
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{ManagerURL: "https://main.example.test", RestartToken: "new-token"},
		RestartComponentsOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (FileBundleStore{}).ReadWorkspaceEnv(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != ws.Slug || got.ManagerURL != "https://main.example.test" ||
		got.RestartToken != "new-token" {
		t.Fatalf("workspace env = %#v, want current slug/manager/token", got)
	}
}

func TestWorkspaceRuntimeRestartWebStopFailureDoesNotStartWithoutForce(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234}
	handles := BundleHandles{ComponentWeb: handleFor(ComponentWeb, 1002)}
	starter := &recordingStarter{stopErr: errors.New("stop stuck")}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  &recordingProber{},
	}

	_, _, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{},
		RestartComponentsOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "stop web: stop stuck") {
		t.Fatalf("err = %v, want stop failure", err)
	}
	if len(starter.started) != 0 {
		t.Fatalf("started = %v, want none", starter.started)
	}
}

func TestWorkspaceRuntimeForceRestartRequiresPersistedPorts(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusFailed
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: &recordingStarter{},
		stopper: &recordingStarter{},
		prober:  &recordingProber{},
	}

	_, _, err := runtime.RestartComponents(
		context.Background(),
		ws,
		nil,
		[]BundleComponent{ComponentWeb},
		RuntimeConfig{},
		RestartComponentsOptions{Force: true},
	)
	if err == nil || !strings.Contains(err.Error(), "missing runtime ports") {
		t.Fatalf("err = %v, want missing ports", err)
	}
}

func TestWorkspaceRuntimeRestartTSWorkerDoesNotStopWeb(t *testing.T) {
	ws := bundleTestWorkspace(t)
	ws.Status = StatusRunning
	ws.Ports = map[BundleComponent]int{ComponentWeb: 4300, ComponentTemporal: 7234}
	handles := BundleHandles{
		ComponentTemporal: handleFor(ComponentTemporal, 1001),
		ComponentWeb:      handleFor(ComponentWeb, 1002),
		ComponentTSWorker: handleFor(ComponentTSWorker, 1003),
	}
	starter := &recordingStarter{}
	runtime := &WorkspaceRuntime{
		store:   FileBundleStore{},
		starter: starter,
		stopper: starter,
		prober:  &recordingProber{},
	}

	_, gotHandles, err := runtime.RestartComponents(
		context.Background(),
		ws,
		handles,
		[]BundleComponent{ComponentTSWorker},
		RuntimeConfig{},
		RestartComponentsOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(starter.stopped, []BundleComponent{ComponentTSWorker}) {
		t.Fatalf("stopped = %v, want ts_worker only", starter.stopped)
	}
	if !reflect.DeepEqual(starter.started, []BundleComponent{ComponentTSWorker}) {
		t.Fatalf("started = %v, want ts_worker only", starter.started)
	}
	if gotHandles[ComponentWeb] != handles[ComponentWeb] {
		t.Fatal("web handle was replaced")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func handleFor(component BundleComponent, pid int) *ProcessHandle {
	return &ProcessHandle{
		Component: component,
		Command:   &exec.Cmd{Process: &os.Process{Pid: pid}},
		done:      make(chan error, 1),
		exited:    make(chan struct{}),
	}
}

type recordingStarter struct {
	started []BundleComponent
	stopped []BundleComponent
	stopErr error
}

func (r *recordingStarter) StartComponent(
	ctx context.Context,
	spec ComponentSpec,
) (*ProcessHandle, error) {
	r.started = append(r.started, spec.Component)
	pid := 2000 + len(r.started)
	return &ProcessHandle{
		Component: spec.Component,
		Command:   &exec.Cmd{Process: &os.Process{Pid: pid}},
		done:      make(chan error, 1),
		exited:    make(chan struct{}),
	}, nil
}

func (r *recordingStarter) StopComponent(
	ctx context.Context,
	handle *ProcessHandle,
) error {
	if handle == nil {
		return nil
	}
	r.stopped = append(r.stopped, handle.Component)
	if r.stopErr != nil {
		return r.stopErr
	}
	handle.finish(nil)
	return nil
}

type recordingProber struct {
	calls         []string
	failWebWorker error
}

func (p *recordingProber) TemporalReady(ctx context.Context, addr string) error {
	p.calls = append(p.calls, "temporal")
	return nil
}

func (p *recordingProber) WebReady(ctx context.Context, managerAddr, host string) error {
	p.calls = append(p.calls, "web")
	return nil
}

func (p *recordingProber) WebWorkerReady(
	ctx context.Context,
	managerAddr, host string,
) error {
	p.calls = append(p.calls, "web-worker")
	return p.failWebWorker
}

func (p *recordingProber) FreshFileExists(
	ctx context.Context,
	path string,
	notBefore time.Time,
) error {
	p.calls = append(p.calls, "fresh-file")
	return nil
}

type stopFailRuntime struct {
	err        error
	startCalls int
}

func (s *stopFailRuntime) StartBundle(
	context.Context,
	Workspace,
	RuntimeConfig,
) (Workspace, BundleHandles, error) {
	s.startCalls++
	return Workspace{}, nil, nil
}

func (s *stopFailRuntime) StopBundle(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
) (Workspace, error) {
	return ws, s.err
}

func (s *stopFailRuntime) RestartComponents(
	context.Context,
	Workspace,
	BundleHandles,
	[]BundleComponent,
	RuntimeConfig,
	RestartComponentsOptions,
) (Workspace, BundleHandles, error) {
	return Workspace{}, nil, nil
}
