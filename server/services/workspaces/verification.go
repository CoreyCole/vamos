package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func (v *Verifier) StartRun(
	ctx context.Context,
	req VerifyWorkspaceRequest,
) (VerifyWorkspaceRun, error) {
	if v.Runs == nil {
		return VerifyWorkspaceRun{}, errors.New("verification store is not configured")
	}
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		return VerifyWorkspaceRun{}, errors.New("slug is required")
	}
	run, err := v.Runs.Create(ctx, req)
	if err != nil {
		return VerifyWorkspaceRun{}, err
	}
	run.Status = VerifyRunRunning
	if err := v.Runs.Update(ctx, run); err != nil {
		return VerifyWorkspaceRun{}, err
	}
	go func() {
		done := v.executeRun(context.Background(), run, req)
		_ = v.Runs.Update(context.Background(), done)
	}()
	return run, nil
}

func (v *Verifier) GetRun(ctx context.Context, id string) (VerifyWorkspaceRun, error) {
	if v.Runs == nil {
		return VerifyWorkspaceRun{}, errors.New("verification store is not configured")
	}
	return v.Runs.Get(ctx, id)
}

func (v *Verifier) VerifyBundle(ctx context.Context, slug string) VerifyWorkspaceRun {
	slug = strings.TrimSpace(slug)
	run := VerifyWorkspaceRun{
		Slug:      slug,
		Status:    VerifyRunPassed,
		StartedAt: time.Now().UTC(),
	}
	if slug == "" {
		return finishBundleVerification(run, "slug is required")
	}
	diagnostics, err := v.Diagnostics(ctx, slug, 200)
	if err != nil {
		return finishBundleVerification(run, err.Error())
	}
	run.Diagnostics = diagnostics
	run.Runtime = diagnostics.RuntimeStatus()
	run.TemporalUIURL = diagnostics.PublicURL
	if diagnostics.Workspace.Ports[ComponentTemporalUI] != 0 {
		run.TemporalUIURL = strings.TrimRight(diagnostics.PublicURL, "/") + "/temporal"
	}
	if diagnostics.Workspace.Status != StatusRunning &&
		run.Runtime.Status != StatusRunning {
		run.Errors = append(
			run.Errors,
			fmt.Sprintf(
				"workspace status is %q, want %q",
				diagnostics.Workspace.Status,
				StatusRunning,
			),
		)
	}
	if diagnostics.RuntimeStatusError != "" {
		run.Errors = append(run.Errors, "runtime status: "+diagnostics.RuntimeStatusError)
	}
	if diagnostics.Metadata == nil {
		run.Errors = append(run.Errors, "workspace env is missing or invalid")
	}
	if v.Prober == nil {
		run.Errors = append(run.Errors, "local prober is not configured")
	} else {
		run.WebOK = v.verifyPIDAndPort(diagnostics, ComponentWeb)
		run.TemporalOK = v.verifyPIDAndPort(diagnostics, ComponentTemporal)
		run.TSWorkerOK = v.verifyPID(diagnostics, ComponentTSWorker) &&
			freshReadyMarker(diagnostics.Workspace.Bundle.TSReadyMarker)
		if !run.WebOK {
			run.Errors = append(run.Errors, "web PID or port is not healthy")
		}
		if !run.TemporalOK {
			run.Errors = append(run.Errors, "temporal PID or port is not healthy")
		}
		if !run.TSWorkerOK {
			run.Errors = append(
				run.Errors,
				"ts worker PID or ready marker is not healthy",
			)
		}
		if err := v.verifyTemporalUI(ctx, diagnostics.Workspace); err != nil {
			run.Errors = append(run.Errors, "temporal UI: "+err.Error())
		}
	}
	completedAt := time.Now().UTC()
	run.CompletedAt = &completedAt
	if len(run.Errors) > 0 {
		run.Status = VerifyRunFailed
		run.Error = &VerifyWorkspaceError{
			Layer:   VerificationLayerMetadata,
			Message: run.Errors[0],
		}
	}
	return run
}

func (v *Verifier) RunEvents(
	ctx context.Context,
	id string,
) (<-chan VerifyWorkspaceRun, error) {
	if v.Runs == nil {
		return nil, errors.New("verification store is not configured")
	}
	return v.Runs.Subscribe(ctx, id)
}

type lifecycleVerificationManager interface {
	RequestLifecycle(
		context.Context,
		WorkspaceLifecycleRequest,
	) (WorkspaceLifecycleSnapshot, error)
	ReconcileWorkspace(context.Context, string) (WorkspaceLifecycleSnapshot, error)
}

func (v *Verifier) executeRun(
	ctx context.Context,
	run VerifyWorkspaceRun,
	req VerifyWorkspaceRequest,
) VerifyWorkspaceRun {
	run.Snapshots = append(run.Snapshots, v.snapshot(ctx, "initial", req.Slug))
	v.phase(
		ctx,
		&run,
		"refresh-discovery",
		VerificationLayerConfig,
		func(ctx context.Context) error {
			return v.Manager.Refresh(ctx)
		},
	)
	if req.Start {
		v.phase(
			ctx,
			&run,
			"start",
			VerificationLayerLifecycle,
			func(ctx context.Context) error {
				if manager, ok := v.Manager.(lifecycleVerificationManager); ok {
					_, err := manager.RequestLifecycle(ctx, WorkspaceLifecycleRequest{
						Slug:         req.Slug,
						Kind:         WorkspaceTransitionStart,
						DesiredState: WorkspaceDesiredRunning,
					})
					if err != nil {
						return err
					}
					return v.waitForLifecycleTerminal(
						ctx,
						req.Slug,
						WorkspaceObservedRunning,
					)
				}
				_, err := v.Manager.Start(ctx, req.Slug)
				return err
			},
		)
	}
	run.Snapshots = append(run.Snapshots, v.snapshot(ctx, "after-start", req.Slug))
	v.phase(
		ctx,
		&run,
		"metadata-log-pid-port",
		VerificationLayerMetadata,
		func(ctx context.Context) error {
			return v.assertWorkspaceHealthy(ctx, req.Slug, req.TailLines)
		},
	)
	v.phase(
		ctx,
		&run,
		"local-host-dispatch",
		VerificationLayerProxy,
		func(ctx context.Context) error {
			return v.assertHostDispatch(ctx, req.Slug)
		},
	)
	if req.Restart {
		before := v.snapshot(ctx, "before-restart", req.Slug)
		run.Snapshots = append(run.Snapshots, before)
		v.phase(
			ctx,
			&run,
			"restart",
			VerificationLayerLifecycle,
			func(ctx context.Context) error {
				if manager, ok := v.Manager.(lifecycleVerificationManager); ok {
					_, err := manager.RequestLifecycle(ctx, WorkspaceLifecycleRequest{
						Slug:         req.Slug,
						Kind:         WorkspaceTransitionRestart,
						DesiredState: WorkspaceDesiredRunning,
					})
					if err != nil {
						return err
					}
					return v.waitForLifecycleTerminal(
						ctx,
						req.Slug,
						WorkspaceObservedRunning,
					)
				}
				_, err := v.Manager.Restart(ctx, req.Slug)
				return err
			},
		)
		after := v.snapshot(ctx, "after-restart", req.Slug)
		run.Snapshots = append(run.Snapshots, after)
		if before.Workspace.PID != 0 && before.Workspace.PID == after.Workspace.PID {
			v.addFailedPhase(
				&run,
				"restart-pid-change",
				VerificationLayerLifecycle,
				fmt.Errorf("workspace PID did not change after restart"),
			)
		}
	}
	if req.Stop {
		v.phase(
			ctx,
			&run,
			"stop",
			VerificationLayerLifecycle,
			func(ctx context.Context) error {
				if manager, ok := v.Manager.(lifecycleVerificationManager); ok {
					_, err := manager.RequestLifecycle(ctx, WorkspaceLifecycleRequest{
						Slug:         req.Slug,
						Kind:         WorkspaceTransitionStop,
						DesiredState: WorkspaceDesiredStopped,
					})
					if err != nil {
						return err
					}
					return v.waitForLifecycleTerminal(
						ctx,
						req.Slug,
						WorkspaceObservedStopped,
					)
				}
				_, err := v.Manager.Stop(ctx, req.Slug)
				return err
			},
		)
	}
	run.Snapshots = append(run.Snapshots, v.snapshot(ctx, "final", req.Slug))
	if diagnostics, err := v.Diagnostics(ctx, req.Slug, req.TailLines); err == nil {
		run.Diagnostics = diagnostics
	}
	completedAt := time.Now().UTC()
	run.CompletedAt = &completedAt
	run.Status = VerifyRunPassed
	for _, phase := range run.Phases {
		if phase.Status == VerifyPhaseFailed {
			run.Status = VerifyRunFailed
			if run.Error == nil {
				run.Error = phase.Error
			}
			break
		}
	}
	return run
}

func (v *Verifier) waitForLifecycleTerminal(
	ctx context.Context,
	slug string,
	want WorkspaceObservedState,
) error {
	manager, ok := v.Manager.(lifecycleVerificationManager)
	if !ok {
		return errors.New("workspace lifecycle manager is not configured")
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		snap, err := manager.ReconcileWorkspace(deadline, slug)
		if err != nil {
			return err
		}
		if snap.ObservedState == want {
			return nil
		}
		switch snap.ObservedState {
		case WorkspaceObservedFailed, WorkspaceObservedCrashed, WorkspaceObservedInvalid:
			return fmt.Errorf("workspace reached %s: %s", snap.ObservedState, snap.Error)
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

func (v *Verifier) phase(
	ctx context.Context,
	run *VerifyWorkspaceRun,
	name string,
	layer VerificationLayer,
	fn func(context.Context) error,
) {
	startedAt := time.Now().UTC()
	err := fn(ctx)
	phase := VerifyWorkspacePhase{
		Name:       name,
		Status:     VerifyPhasePassed,
		Layer:      layer,
		StartedAt:  startedAt,
		DurationMS: time.Since(startedAt).Milliseconds(),
	}
	if err != nil {
		phase.Status = VerifyPhaseFailed
		phase.Error = &VerifyWorkspaceError{Layer: layer, Message: err.Error()}
	}
	run.Phases = append(run.Phases, phase)
}

func (v *Verifier) addFailedPhase(
	run *VerifyWorkspaceRun,
	name string,
	layer VerificationLayer,
	err error,
) {
	startedAt := time.Now().UTC()
	run.Phases = append(run.Phases, VerifyWorkspacePhase{
		Name:      name,
		Status:    VerifyPhaseFailed,
		Layer:     layer,
		StartedAt: startedAt,
		Error:     &VerifyWorkspaceError{Layer: layer, Message: err.Error()},
	})
}

func (v *Verifier) snapshot(
	ctx context.Context,
	label, slug string,
) VerifyWorkspaceSnapshot {
	if v.Manager == nil {
		return VerifyWorkspaceSnapshot{Label: label}
	}
	ws, ok := v.Manager.Lookup(slug)
	if !ok {
		return VerifyWorkspaceSnapshot{Label: label}
	}
	snapshot := VerifyWorkspaceSnapshot{Label: label, Workspace: ws}
	store := FileBundleStore{}
	if metadata, err := ReadMetadata(ws.Bundle.WorkspaceEnv); err == nil {
		snapshot.Metadata = &metadata
	}
	if runtimeStatus, err := store.ReadStatus(ws); err == nil {
		snapshot.RuntimeStatus = &runtimeStatus
	}
	if desired, err := store.ReadDesired(ws); err == nil {
		snapshot.DesiredState = &desired
	}
	if v.Prober != nil {
		snapshot.PIDAlive = v.Prober.PIDAlive(ws.PID)
		snapshot.PortOpen = v.Prober.PortOpen(ws.LocalAddr())
	}
	return snapshot
}

func (v *Verifier) assertWorkspaceHealthy(
	ctx context.Context,
	slug string,
	tailLines int,
) error {
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for {
		lastErr = v.checkWorkspaceHealthy(ctx, slug, tailLines)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (v *Verifier) checkWorkspaceHealthy(
	ctx context.Context,
	slug string,
	tailLines int,
) error {
	diagnostics, err := v.Diagnostics(ctx, slug, tailLines)
	if err != nil {
		return err
	}
	ws := diagnostics.Workspace
	if ws.Status != StatusRunning {
		return fmt.Errorf(
			"workspace %q status is %q, want %q",
			slug,
			ws.Status,
			StatusRunning,
		)
	}
	if diagnostics.Metadata == nil {
		return fmt.Errorf(
			"workspace %q metadata is missing or invalid at %s",
			slug,
			diagnostics.MetadataPath,
		)
	}
	if diagnostics.Metadata.Slug != slug {
		return fmt.Errorf(
			"workspace %q metadata slug is %q",
			slug,
			diagnostics.Metadata.Slug,
		)
	}
	if strings.TrimSpace(diagnostics.LogPath) == "" {
		return fmt.Errorf("workspace %q log path is empty", slug)
	}
	if !diagnostics.PIDAlive {
		return fmt.Errorf("workspace %q PID %d is not alive", slug, ws.PID)
	}
	if !diagnostics.PortOpen {
		return fmt.Errorf(
			"workspace %q local address %q is not open",
			slug,
			ws.LocalAddr(),
		)
	}
	return nil
}

func finishBundleVerification(run VerifyWorkspaceRun, message string) VerifyWorkspaceRun {
	completedAt := time.Now().UTC()
	run.CompletedAt = &completedAt
	run.Status = VerifyRunFailed
	run.Errors = append(run.Errors, message)
	run.Error = &VerifyWorkspaceError{Layer: VerificationLayerConfig, Message: message}
	return run
}

func (v *Verifier) verifyPIDAndPort(
	diagnostics WorkspaceDiagnostics,
	component BundleComponent,
) bool {
	return v.verifyPID(diagnostics, component) && v.verifyPort(diagnostics, component)
}

func (v *Verifier) verifyPID(
	diagnostics WorkspaceDiagnostics,
	component BundleComponent,
) bool {
	pid := diagnostics.RuntimeStatus().PIDs[component]
	if pid == 0 && component == ComponentWeb {
		pid = diagnostics.Workspace.PID
	}
	return pid != 0 && v.Prober.PIDAlive(pid)
}

func (v *Verifier) verifyPort(
	diagnostics WorkspaceDiagnostics,
	component BundleComponent,
) bool {
	port := diagnostics.RuntimeStatus().Ports[component]
	if port == 0 && component == ComponentWeb {
		port = diagnostics.Workspace.Port
	}
	if port == 0 {
		return false
	}
	return v.Prober.PortOpen("127.0.0.1:" + strconv.Itoa(port))
}

func ReadTSWorkerIdentityMarker(path string) (TSWorkerIdentityMarker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TSWorkerIdentityMarker{}, err
	}
	var marker TSWorkerIdentityMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return TSWorkerIdentityMarker{}, err
	}
	return marker, nil
}

func VerifyTSWorkerIdentity(
	ws Workspace,
	runtime RuntimeStatus,
	marker TSWorkerIdentityMarker,
) error {
	if marker.Version != 1 {
		return fmt.Errorf("ts worker identity marker version = %d, want 1", marker.Version)
	}
	if marker.WorkspaceSlug != ws.Slug {
		return fmt.Errorf("ts worker slug = %q, want %q", marker.WorkspaceSlug, ws.Slug)
	}
	if !samePath(marker.CheckoutPath, ws.CheckoutPath) {
		return fmt.Errorf("ts worker checkout = %q, want %q", marker.CheckoutPath, ws.CheckoutPath)
	}
	if temporalPort := runtime.Ports[ComponentTemporal]; temporalPort != 0 {
		wantTemporal := "127.0.0.1:" + strconv.Itoa(temporalPort)
		if marker.TemporalAddress != wantTemporal {
			return fmt.Errorf("ts worker temporal address = %q, want %q", marker.TemporalAddress, wantTemporal)
		}
	}
	if marker.TaskQueue != "agents-ts" {
		return fmt.Errorf("ts worker task queue = %q, want agents-ts", marker.TaskQueue)
	}
	if pid := runtime.PIDs[ComponentTSWorker]; pid != 0 && marker.PID != pid {
		return fmt.Errorf("ts worker pid = %d, want %d", marker.PID, pid)
	}
	if wantMarker := RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).TSReadyMarker; marker.ReadyMarker != wantMarker {
		return fmt.Errorf("ts worker marker path = %q, want %q", marker.ReadyMarker, wantMarker)
	}
	return nil
}

func freshReadyMarker(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && time.Since(info.ModTime()) < 24*time.Hour
}

func (v *Verifier) verifyTemporalUI(ctx context.Context, ws Workspace) error {
	if ws.Host == "" || v.ManagerListenAddr == "" {
		return nil
	}
	resp, body, err := v.Prober.HTTPHost(ctx, v.ManagerListenAddr, ws.Host, "/temporal")
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("no response")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 ||
		resp.StatusCode == http.StatusUnauthorized {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (v *Verifier) assertHostDispatch(ctx context.Context, slug string) error {
	if v.Manager == nil {
		return errors.New("workspace manager is not configured")
	}
	if v.Prober == nil {
		return errors.New("local prober is not configured")
	}
	ws, ok := v.Manager.Lookup(slug)
	if !ok {
		return fmt.Errorf("unknown workspace %q", slug)
	}
	resp, body, err := v.Prober.HTTPHost(ctx, v.ManagerListenAddr, ws.Host, "/")
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("host dispatch returned no response")
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 ||
		resp.StatusCode == http.StatusUnauthorized {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound &&
		strings.Contains(string(body), "Workspace unavailable") {
		return fmt.Errorf(
			"workspace host %q rendered unavailable page through manager",
			ws.Host,
		)
	}
	return fmt.Errorf("workspace host %q returned HTTP %d", ws.Host, resp.StatusCode)
}
