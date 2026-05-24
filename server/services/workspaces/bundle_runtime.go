package workspaces

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type WorkspaceRuntime struct {
	store         BundleStore
	starter       ComponentStarter
	stopper       ComponentStopper
	prober        ReadinessProber
	portAllocator func() (int, error)
	now           func() time.Time
}

type ReadinessProber interface {
	TemporalReady(ctx context.Context, addr string) error
	WebReady(ctx context.Context, managerAddr, host string) error
	WebWorkerReady(ctx context.Context, managerAddr, host string) error
	FreshFileExists(ctx context.Context, path string, notBefore time.Time) error
}

func NewWorkspaceRuntime() *WorkspaceRuntime {
	runner := LocalComponentRunner{}
	return &WorkspaceRuntime{
		store:         FileBundleStore{},
		starter:       runner,
		stopper:       runner,
		prober:        LocalReadinessProber{},
		portAllocator: findFreePort,
		now:           time.Now,
	}
}

func (r *WorkspaceRuntime) StartBundle(
	ctx context.Context,
	ws Workspace,
	rt RuntimeConfig,
) (Workspace, BundleHandles, error) {
	r = r.withDefaults()
	paths := r.store.Paths(ws)
	ws.Bundle = paths
	ws.LogPath = paths.WebLog
	if err := EnsureRuntimeDirs(paths); err != nil {
		return ws, nil, err
	}
	ports, err := r.allocatePorts()
	if err != nil {
		return ws, nil, err
	}
	ws.Ports = ports
	ws.Port = ports[ComponentWeb]
	logs := bundleLogs(paths)
	status := RuntimeStatus{
		Status: StatusStarting,
		Phase:  PhaseStartingTemporal,
		Logs:   logs,
		Ports:  ports,
	}
	if err := r.store.WriteDesired(ws, DesiredState{Desired: StatusRunning}); err != nil {
		return ws, nil, err
	}
	if err := r.store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug:         ws.Slug,
			CheckoutPath: ws.CheckoutPath,
			ManagerURL:   rt.ManagerURL,
			RestartToken: rt.RestartToken,
		},
	); err != nil {
		return ws, nil, err
	}
	if err := r.store.WriteStatus(ws, status); err != nil {
		return ws, nil, err
	}

	handles := BundleHandles{}
	fail := func(phase BundlePhase, cause error) (Workspace, BundleHandles, error) {
		stopErr := r.stopStarted(ctx, handles)
		if stopErr != nil {
			cause = fmt.Errorf("%w; stop partial bundle: %v", cause, stopErr)
		}
		ws.Status = StatusFailed
		ws.Phase = phase
		ws.Error = cause.Error()
		ws.PIDs = bundlePIDs(handles)
		ws.PID = 0
		_ = r.store.WriteStatus(
			ws,
			RuntimeStatus{
				Status: StatusFailed,
				Phase:  phase,
				Error:  cause.Error(),
				Logs:   logs,
				Ports:  ports,
				PIDs:   ws.PIDs,
				Build:  ws.BuildStatus,
			},
		)
		return ws, handles, cause
	}

	temporal, err := r.starter.StartComponent(
		ctx,
		ComponentSpec{
			Component: ComponentTemporal,
			Args: TemporalArgs(
				ws,
				ports[ComponentTemporal],
				ports[ComponentTemporalUI],
			),
			Dir:     ws.PackagePath,
			Env:     os.Environ(),
			LogPath: paths.TemporalLog,
			PIDPath: paths.TemporalPID,
		},
	)
	if err != nil {
		return fail(PhaseStartingTemporal, err)
	}
	handles[ComponentTemporal] = temporal
	if err := r.prober.TemporalReady(
		ctx,
		"127.0.0.1:"+strconv.Itoa(ports[ComponentTemporal]),
	); err != nil {
		return fail(PhaseStartingTemporal, err)
	}

	status.Phase = PhaseStartingWeb
	_ = r.store.WriteStatus(ws, status)
	web, err := r.starter.StartComponent(
		ctx,
		ComponentSpec{
			Component: ComponentWeb,
			Args:      []string{filepath.Join(ws.PackagePath, "agents-server")},
			Dir:       ws.PackagePath,
			Env:       ChildEnv(rt.BaseEnv, ws, ports, rt),
			LogPath:   paths.WebLog,
			PIDPath:   paths.WebPID,
		},
	)
	if err != nil {
		return fail(PhaseStartingWeb, err)
	}
	handles[ComponentWeb] = web
	webAddr := "127.0.0.1:" + strconv.Itoa(ports[ComponentWeb])
	if err := r.prober.WebReady(ctx, webAddr, ws.Host); err != nil {
		return fail(PhaseStartingWeb, err)
	}
	if err := r.prober.WebWorkerReady(ctx, webAddr, ws.Host); err != nil {
		return fail(PhaseStartingWeb, err)
	}

	status.Phase = PhaseStartingTSWorker
	_ = os.Remove(paths.TSReadyMarker)
	_ = r.store.WriteStatus(ws, status)
	tsStart := r.now()
	ts, err := r.starter.StartComponent(
		ctx,
		ComponentSpec{
			Component: ComponentTSWorker,
			Args:      TSWorkerArgs(ws),
			Dir:       ws.PackagePath,
			Env:       TSWorkerEnv(rt.BaseEnv, ws, ports[ComponentTemporal]),
			LogPath:   paths.TSWorkerLog,
			PIDPath:   paths.TSWorkerPID,
		},
	)
	if err != nil {
		return fail(PhaseStartingTSWorker, err)
	}
	handles[ComponentTSWorker] = ts
	if err := r.prober.FreshFileExists(ctx, paths.TSReadyMarker, tsStart); err != nil {
		return fail(PhaseStartingTSWorker, err)
	}

	ws.Status = StatusRunning
	ws.Phase = ""
	ws.Error = ""
	ws.PIDs = bundlePIDs(handles)
	ws.PID = ws.PIDs[ComponentWeb]
	if err := r.store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusRunning,
			Logs:   logs,
			Ports:  ports,
			PIDs:   ws.PIDs,
			Build:  ws.BuildStatus,
		},
	); err != nil {
		return fail(PhaseStartingTSWorker, err)
	}
	return ws, handles, nil
}

func (r *WorkspaceRuntime) StopBundle(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
) (Workspace, error) {
	r = r.withDefaults()
	paths := r.store.Paths(ws)
	logs := bundleLogs(paths)
	_ = r.store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusStopping,
			Phase:  PhaseStopping,
			Logs:   logs,
			Ports:  ws.Ports,
			PIDs:   ws.PIDs,
			Build:  ws.BuildStatus,
		},
	)
	var stopErr error
	for _, component := range []BundleComponent{ComponentTSWorker, ComponentWeb, ComponentTemporal} {
		if err := r.stopper.StopComponent(
			ctx,
			handles[component],
		); err != nil &&
			stopErr == nil {
			stopErr = fmt.Errorf("stop %s: %w", component, err)
		}
	}
	_ = os.Remove(paths.WebPID)
	_ = os.Remove(paths.TemporalPID)
	_ = os.Remove(paths.TSWorkerPID)
	_ = os.Remove(paths.TSReadyMarker)
	if stopErr != nil {
		ws.Status = StatusFailed
		ws.Phase = PhaseStopping
		ws.Error = stopErr.Error()
		_ = r.store.WriteStatus(
			ws,
			RuntimeStatus{
				Status: StatusFailed,
				Phase:  PhaseStopping,
				Error:  stopErr.Error(),
				Logs:   logs,
				Ports:  ws.Ports,
				PIDs:   ws.PIDs,
				Build:  ws.BuildStatus,
			},
		)
		return ws, stopErr
	}
	ws.Status = StatusStopped
	ws.Phase = ""
	ws.Error = ""
	ws.PID = 0
	ws.PIDs = nil
	_ = r.store.WriteDesired(ws, DesiredState{Desired: StatusStopped})
	_ = r.store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: StatusStopped,
			Logs:   logs,
			Ports:  ws.Ports,
			Build:  ws.BuildStatus,
		},
	)
	return ws, nil
}

func (r *WorkspaceRuntime) RestartComponents(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
	components []BundleComponent,
	rt RuntimeConfig,
	opts RestartComponentsOptions,
) (Workspace, BundleHandles, error) {
	r = r.withDefaults()
	if ws.Status != StatusRunning {
		if !opts.Force ||
			(ws.Status != StatusFailed && ws.Status != StatusCrashed && ws.Status != StatusStopped) {
			return ws, handles, fmt.Errorf(
				"workspace %q is %s; retry from recovery UI before component restart",
				ws.Slug,
				ws.Status,
			)
		}
	}
	if len(components) == 0 {
		return ws, handles, nil
	}
	paths := r.store.Paths(ws)
	ws.Bundle = paths
	if err := EnsureRuntimeDirs(paths); err != nil {
		return ws, handles, err
	}
	if ws.Ports == nil {
		ws.Ports = map[BundleComponent]int{}
	}
	if opts.Force && (ws.Ports[ComponentWeb] == 0 || ws.Ports[ComponentTemporal] == 0) {
		return ws, handles, fmt.Errorf(
			"workspace %q is missing runtime ports; use workspace start/retry before component restart",
			ws.Slug,
		)
	}
	logs := bundleLogs(paths)
	if err := r.store.WriteWorkspaceEnv(
		ws,
		WorkspaceEnv{
			Slug:         ws.Slug,
			CheckoutPath: ws.CheckoutPath,
			ManagerURL:   rt.ManagerURL,
			RestartToken: rt.RestartToken,
		},
	); err != nil {
		return ws, handles, err
	}
	if handles == nil {
		handles = BundleHandles{}
	}
	for _, component := range normalizedRestartComponents(components) {
		switch component {
		case ComponentWeb:
			restarted, err := r.restartWeb(ctx, ws, handles, rt, logs, opts)
			if err != nil {
				return r.failComponentRestart(ws, handles, logs, PhaseRestartingWeb, err)
			}
			ws = restarted
		case ComponentTSWorker:
			restarted, err := r.restartTSWorker(ctx, ws, handles, rt, logs, opts)
			if err != nil {
				return r.failComponentRestart(ws, handles, logs, PhaseRestartingTS, err)
			}
			ws = restarted
		}
	}
	ws.Status = StatusRunning
	ws.Phase = ""
	ws.Error = ""
	ws.PIDs = bundlePIDs(handles)
	ws.PID = ws.PIDs[ComponentWeb]
	if err := r.store.WriteStatus(ws, RuntimeStatus{
		Status: StatusRunning,
		Logs:   logs,
		Ports:  ws.Ports,
		PIDs:   ws.PIDs,
		Build:  ws.BuildStatus,
	}); err != nil {
		return r.failComponentRestart(ws, handles, logs, PhaseRestartingWeb, err)
	}
	return ws, handles, nil
}

func (r *WorkspaceRuntime) restartWeb(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
	rt RuntimeConfig,
	logs map[BundleComponent]string,
	opts RestartComponentsOptions,
) (Workspace, error) {
	paths := r.store.Paths(ws)
	ws.Status = StatusStarting
	ws.Phase = PhaseRestartingWeb
	_ = r.store.WriteStatus(ws, RuntimeStatus{
		Status: StatusStarting,
		Phase:  PhaseRestartingWeb,
		Logs:   logs,
		Ports:  ws.Ports,
		PIDs:   bundlePIDs(handles),
		Build:  ws.BuildStatus,
	})
	if err := r.stopper.StopComponent(ctx, handles[ComponentWeb]); err != nil {
		if !opts.Force {
			return ws, fmt.Errorf("stop web: %w", err)
		}
	}
	delete(handles, ComponentWeb)
	_ = os.Remove(paths.WebPID)
	if opts.Force {
		port, err := r.portAllocator()
		if err != nil {
			return ws, err
		}
		ws.Ports[ComponentWeb] = port
		ws.Port = port
	}
	web, err := r.starter.StartComponent(ctx, ComponentSpec{
		Component: ComponentWeb,
		Args:      []string{filepath.Join(ws.PackagePath, "agents-server")},
		Dir:       ws.PackagePath,
		Env:       ChildEnv(rt.BaseEnv, ws, ws.Ports, rt),
		LogPath:   paths.WebLog,
		PIDPath:   paths.WebPID,
	})
	if err != nil {
		return ws, err
	}
	handles[ComponentWeb] = web
	webAddr := "127.0.0.1:" + strconv.Itoa(ws.Ports[ComponentWeb])
	if err := r.prober.WebReady(ctx, webAddr, ws.Host); err != nil {
		return ws, err
	}
	if err := r.prober.WebWorkerReady(ctx, webAddr, ws.Host); err != nil {
		return ws, err
	}
	ws.PIDs = bundlePIDs(handles)
	ws.PID = ws.PIDs[ComponentWeb]
	return ws, nil
}

func (r *WorkspaceRuntime) restartTSWorker(
	ctx context.Context,
	ws Workspace,
	handles BundleHandles,
	rt RuntimeConfig,
	logs map[BundleComponent]string,
	opts RestartComponentsOptions,
) (Workspace, error) {
	paths := r.store.Paths(ws)
	ws.Status = StatusStarting
	ws.Phase = PhaseRestartingTS
	_ = r.store.WriteStatus(ws, RuntimeStatus{
		Status: StatusStarting,
		Phase:  PhaseRestartingTS,
		Logs:   logs,
		Ports:  ws.Ports,
		PIDs:   bundlePIDs(handles),
		Build:  ws.BuildStatus,
	})
	if err := r.stopper.StopComponent(ctx, handles[ComponentTSWorker]); err != nil {
		if !opts.Force {
			return ws, fmt.Errorf("stop ts_worker: %w", err)
		}
	}
	delete(handles, ComponentTSWorker)
	_ = os.Remove(paths.TSWorkerPID)
	_ = os.Remove(paths.TSReadyMarker)
	tsStart := r.now()
	ts, err := r.starter.StartComponent(ctx, ComponentSpec{
		Component: ComponentTSWorker,
		Args:      TSWorkerArgs(ws),
		Dir:       ws.PackagePath,
		Env:       TSWorkerEnv(rt.BaseEnv, ws, ws.Ports[ComponentTemporal]),
		LogPath:   paths.TSWorkerLog,
		PIDPath:   paths.TSWorkerPID,
	})
	if err != nil {
		return ws, err
	}
	handles[ComponentTSWorker] = ts
	if err := r.prober.FreshFileExists(ctx, paths.TSReadyMarker, tsStart); err != nil {
		return ws, err
	}
	ws.PIDs = bundlePIDs(handles)
	return ws, nil
}

func (r *WorkspaceRuntime) failComponentRestart(
	ws Workspace,
	handles BundleHandles,
	logs map[BundleComponent]string,
	phase BundlePhase,
	cause error,
) (Workspace, BundleHandles, error) {
	ws.Status = StatusFailed
	ws.Phase = phase
	ws.Error = cause.Error()
	ws.PIDs = bundlePIDs(handles)
	ws.PID = ws.PIDs[ComponentWeb]
	_ = r.store.WriteStatus(ws, RuntimeStatus{
		Status: StatusFailed,
		Phase:  phase,
		Error:  cause.Error(),
		Logs:   logs,
		Ports:  ws.Ports,
		PIDs:   ws.PIDs,
		Build:  ws.BuildStatus,
	})
	return ws, handles, cause
}

func normalizedRestartComponents(components []BundleComponent) []BundleComponent {
	seen := map[BundleComponent]bool{}
	out := []BundleComponent{}
	for _, want := range []BundleComponent{ComponentWeb, ComponentTSWorker} {
		for _, component := range components {
			if component == want && !seen[want] {
				seen[want] = true
				out = append(out, want)
			}
		}
	}
	return out
}

func (r *WorkspaceRuntime) withDefaults() *WorkspaceRuntime {
	copy := *r
	if copy.store == nil {
		copy.store = FileBundleStore{}
	}
	if copy.starter == nil || copy.stopper == nil {
		runner := LocalComponentRunner{}
		if copy.starter == nil {
			copy.starter = runner
		}
		if copy.stopper == nil {
			copy.stopper = runner
		}
	}
	if copy.prober == nil {
		copy.prober = LocalReadinessProber{}
	}
	if copy.portAllocator == nil {
		copy.portAllocator = findFreePort
	}
	if copy.now == nil {
		copy.now = time.Now
	}
	return &copy
}

func (r *WorkspaceRuntime) allocatePorts() (map[BundleComponent]int, error) {
	ports := map[BundleComponent]int{}
	for _, component := range []BundleComponent{ComponentWeb, ComponentTemporal, ComponentTemporalUI} {
		port, err := r.portAllocator()
		if err != nil {
			return nil, err
		}
		ports[component] = port
	}
	return ports, nil
}

func (r *WorkspaceRuntime) stopStarted(ctx context.Context, handles BundleHandles) error {
	var stopErr error
	for _, component := range []BundleComponent{ComponentTSWorker, ComponentWeb, ComponentTemporal} {
		if err := r.stopper.StopComponent(
			ctx,
			handles[component],
		); err != nil &&
			stopErr == nil {
			stopErr = err
		}
	}
	return stopErr
}

func bundleLogs(paths WorkspaceRuntimePaths) map[BundleComponent]string {
	return map[BundleComponent]string{
		ComponentTemporal: paths.TemporalLog,
		ComponentWeb:      paths.WebLog,
		ComponentTSWorker: paths.TSWorkerLog,
	}
}

func TemporalArgs(ws Workspace, temporalPort, uiPort int) []string {
	return []string{
		"temporal",
		"server",
		"start-dev",
		"--db-filename",
		RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).TemporalDB,
		"--port",
		strconv.Itoa(temporalPort),
		"--ui-port",
		strconv.Itoa(uiPort),
		"--ui-public-path",
		"/temporal",
	}
}

func TSWorkerArgs(ws Workspace) []string {
	return []string{
		"node",
		"dist/pkg/agents/temporal/workers/ts/worker.js",
	}
}

func TSWorkerEnv(parent map[string]string, ws Workspace, temporalPort int) []string {
	env := os.Environ()
	for key, value := range parent {
		env = appendEnv(env, key, value)
	}
	env = appendEnv(env, "TEMPORAL_ADDR", "127.0.0.1:"+strconv.Itoa(temporalPort))
	env = appendEnv(
		env,
		"VAMOS_TS_WORKER_READY_FILE",
		RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).TSReadyMarker,
	)
	if home := os.Getenv("HOME"); home != "" {
		env = appendEnv(
			env,
			"PI_AUTH_PATH",
			filepath.Join(home, ".pi", "agent", "auth.json"),
		)
	}
	return env
}

type LocalReadinessProber struct{}

func (LocalReadinessProber) TemporalReady(ctx context.Context, addr string) error {
	return waitForTCP(ctx, addr)
}

func (LocalReadinessProber) WebReady(ctx context.Context, addr, host string) error {
	return waitForHTTP(ctx, addr, host, "/")
}

func (LocalReadinessProber) WebWorkerReady(ctx context.Context, addr, host string) error {
	return waitForHTTP(ctx, addr, host, "/")
}

func (LocalReadinessProber) FreshFileExists(
	ctx context.Context,
	path string,
	notBefore time.Time,
) error {
	return waitUntil(ctx, func() error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.ModTime().Before(notBefore) {
			return fmt.Errorf("%s is stale", path)
		}
		return nil
	})
}

func waitForTCP(ctx context.Context, addr string) error {
	return waitUntil(ctx, func() error {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	})
}

func waitForHTTP(ctx context.Context, addr, host, path string) error {
	return waitUntil(ctx, func() error {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"http://"+addr+path,
			nil,
		)
		if err != nil {
			return err
		}
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil
	})
}

func waitUntil(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for {
		if err := fn(); err == nil {
			return nil
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
