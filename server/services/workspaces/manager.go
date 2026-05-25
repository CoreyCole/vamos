package workspaces

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type bundleRuntime interface {
	StartBundle(
		context.Context,
		Workspace,
		RuntimeConfig,
	) (Workspace, BundleHandles, error)
	StopBundle(context.Context, Workspace, BundleHandles) (Workspace, error)
	RestartComponents(
		context.Context,
		Workspace,
		BundleHandles,
		[]BundleComponent,
		RuntimeConfig,
		RestartComponentsOptions,
	) (Workspace, BundleHandles, error)
}

type processAliveFunc func(int) bool
type processMatchesWorkspaceFunc func(Workspace, int) bool

type ManagerService struct {
	mu        sync.Mutex
	runtime   RuntimeConfig
	discovery DiscoveryConfig

	workspaces map[string]Workspace
	children   map[string]BundleHandles

	bundleRuntime           bundleRuntime
	store                   BundleStore
	processAlive            processAliveFunc
	processMatchesWorkspace processMatchesWorkspaceFunc

	lifecycleStarter       WorkspaceLifecycleStarter
	lifecycleNotifier      WorkspaceLifecycleNotifier
	workspaceErrorRecorder WorkspaceErrorSink
	now                    func() time.Time
	newTransitionID        func() string
}

func NewManager(cfg RuntimeConfig, discovery DiscoveryConfig) (*ManagerService, error) {
	m := &ManagerService{
		runtime:                 cfg,
		discovery:               discovery,
		workspaces:              map[string]Workspace{},
		children:                map[string]BundleHandles{},
		bundleRuntime:           NewWorkspaceRuntime(),
		store:                   FileBundleStore{},
		processAlive:            processAlive,
		processMatchesWorkspace: processMatchesWorkspace,
		now:                     func() time.Time { return time.Now().UTC() },
		newTransitionID:         func() string { return uuid.NewString() },
	}
	if err := m.Refresh(context.Background()); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *ManagerService) Refresh(ctx context.Context) error {
	discovered, err := Discover(m.discovery)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	next := map[string]Workspace{}
	for _, ws := range discovered {
		if current, ok := m.workspaces[ws.Slug]; ok {
			ws.Status = current.Status
			ws.Phase = current.Phase
			ws.Port = current.Port
			ws.PID = current.PID
			ws.Ports = current.Ports
			ws.PIDs = current.PIDs
			ws.LogPath = current.LogPath
			ws.Error = current.Error
			ws.BuildStatus = current.BuildStatus
		}
		if ws.LogPath == "" {
			ws.LogPath = ws.Bundle.WebLog
		}
		if ws.IsMain {
			ws.Status = StatusRunning
			ws.PID = os.Getpid()
			next[ws.Slug] = ws
			continue
		}
		if handles := m.children[ws.Slug]; len(handles) > 0 &&
			handles[ComponentWeb] != nil &&
			handles[ComponentWeb].pid() != 0 {
			ws.Status = StatusRunning
			ws.PID = handles[ComponentWeb].pid()
			ws.PIDs = bundlePIDs(handles)
		} else {
			ws = m.reconcileRuntimeLocked(ws)
		}
		next[ws.Slug] = ws
	}
	m.workspaces = next
	return nil
}

const staleWorkspaceEnvError = "stale workspace env ignored; start workspace to recreate runtime state"

func (m *ManagerService) reconcileRuntimeLocked(ws Workspace) Workspace {
	if m.store == nil {
		m.store = FileBundleStore{}
	}
	workspaceEnv, envErr := m.store.ReadWorkspaceEnv(ws)
	status, statusErr := m.store.ReadStatus(ws)
	if envErr != nil && statusErr != nil {
		if ws.Status == "" || ws.Status == StatusStarting || ws.Status == StatusStopping {
			ws.Status = StatusStopped
		}
		return ws
	}
	if envErr != nil {
		ws.Status = StatusInvalid
		ws.Error = fmt.Sprintf("read workspace env: %v", envErr)
		return ws
	}
	if workspaceEnv.Slug != ws.Slug || workspaceEnv.CheckoutPath != ws.CheckoutPath ||
		workspaceEnv.ManagerURL != m.runtime.ManagerURL {
		if state, err := m.store.ReadLifecycle(ws); err == nil &&
			(state.ObservedState == WorkspaceObservedStarting ||
				state.ObservedState == WorkspaceObservedStopping) {
			ws.Status = statusFromObserved(state.ObservedState)
			ws.Phase = phaseForTransition(state.TransitionKind)
			ws.Error = state.Error
			return ws
		}
		ws.Status = StatusStopped
		ws.Phase = ""
		ws.Port = 0
		ws.PID = 0
		ws.Ports = nil
		ws.PIDs = nil
		ws.Error = staleWorkspaceEnvError
		return ws
	}
	if statusErr != nil {
		ws.Status = StatusInvalid
		ws.Error = fmt.Sprintf("read runtime status: %v", statusErr)
		return ws
	}
	ws.Status = status.Status
	ws.Phase = status.Phase
	ws.Error = status.Error
	ws.Ports = status.Ports
	ws.PIDs = status.PIDs
	ws.BuildStatus = status.Build
	ws.Port = status.Ports[ComponentWeb]
	ws.PID = status.PIDs[ComponentWeb]
	if ws.PID != 0 && ws.Status == StatusRunning {
		if !m.processAlive(ws.PID) {
			ws.Status = StatusCrashed
			ws.PID = 0
			ws.Error = "workspace process is not alive"
		} else if m.processMatchesWorkspace != nil &&
			!m.processMatchesWorkspace(ws, ws.PID) {
			ws.Status = StatusStopped
			ws.Phase = ""
			ws.Port = 0
			ws.PID = 0
			ws.Ports = nil
			ws.PIDs = nil
			ws.Error = "stale workspace process ignored; start workspace to recreate runtime state"
		}
	}
	return ws
}

func (m *ManagerService) List() []Workspace {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		out = append(out, ws)
	}
	sortWorkspaces(out)
	return out
}

func (m *ManagerService) Lookup(slug string) (Workspace, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws, ok := m.workspaces[slug]
	return ws, ok
}

func (m *ManagerService) LookupHost(host string) (Workspace, bool) {
	slug, ok := SlugFromHost(host, m.discovery.Domain)
	if !ok {
		return Workspace{}, false
	}
	return m.Lookup(slug)
}

func (m *ManagerService) SetLifecycleStarter(starter WorkspaceLifecycleStarter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleStarter = starter
}

func (m *ManagerService) SetLifecycleNotifier(notifier WorkspaceLifecycleNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleNotifier = notifier
}

func (m *ManagerService) SetWorkspaceErrorRecorder(recorder WorkspaceErrorSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaceErrorRecorder = recorder
}

func (m *ManagerService) workspaceErrorSinkLocked() WorkspaceErrorSink {
	return m.workspaceErrorRecorder
}

func (m *ManagerService) notifyLifecycleChanged(slug string) {
	m.mu.Lock()
	notifier := m.lifecycleNotifier
	m.mu.Unlock()
	if notifier != nil {
		notifier.Notify(slug)
	}
}

func desiredFromStatus(status Status) WorkspaceDesiredState {
	switch status {
	case StatusRunning, StatusStarting:
		return WorkspaceDesiredRunning
	default:
		return WorkspaceDesiredStopped
	}
}

func observedFromStatus(status Status) WorkspaceObservedState {
	if status == "" {
		return WorkspaceObservedStopped
	}
	return WorkspaceObservedState(status)
}

func statusFromObserved(observed WorkspaceObservedState) Status {
	if observed == "" {
		return StatusStopped
	}
	return Status(observed)
}

func snapshotFromState(
	ws Workspace,
	state WorkspaceLifecycleState,
) WorkspaceLifecycleSnapshot {
	if state.DesiredState == "" {
		state.DesiredState = desiredFromStatus(ws.Status)
	}
	if state.ObservedState == "" {
		state.ObservedState = observedFromStatus(ws.Status)
	}
	ws.Status = statusFromObserved(state.ObservedState)
	ws.Error = state.Error
	return WorkspaceLifecycleSnapshot{
		Workspace:      ws,
		DesiredState:   state.DesiredState,
		ObservedState:  state.ObservedState,
		TransitionID:   state.TransitionID,
		TransitionKind: state.TransitionKind,
		Error:          state.Error,
	}
}

func (m *ManagerService) lifecycleStateLocked(ws Workspace) WorkspaceLifecycleState {
	if m.store == nil {
		m.store = FileBundleStore{}
	}
	if ws.Error == staleWorkspaceEnvError {
		return WorkspaceLifecycleState{
			DesiredState:  desiredFromStatus(ws.Status),
			ObservedState: observedFromStatus(ws.Status),
			Error:         ws.Error,
		}
	}
	state, err := m.store.ReadLifecycle(ws)
	if err != nil {
		return WorkspaceLifecycleState{
			DesiredState:  desiredFromStatus(ws.Status),
			ObservedState: observedFromStatus(ws.Status),
			Error:         ws.Error,
		}
	}
	if state.DesiredState == "" {
		state.DesiredState = desiredFromStatus(ws.Status)
	}
	if state.ObservedState == "" {
		state.ObservedState = observedFromStatus(ws.Status)
	}
	return state
}

const workspaceLifecycleTransitionTimeout = 10 * time.Minute

func lifecycleTransitionExpired(state WorkspaceLifecycleState, now time.Time) bool {
	if state.TransitionID == "" {
		return false
	}
	if state.ObservedState != WorkspaceObservedStarting &&
		state.ObservedState != WorkspaceObservedStopping {
		return false
	}
	if state.TransitionUpdatedAt.IsZero() {
		return false
	}
	return now.Sub(state.TransitionUpdatedAt) > workspaceLifecycleTransitionTimeout
}

func (m *ManagerService) ListLifecycle(
	ctx context.Context,
) ([]WorkspaceLifecycleSnapshot, error) {
	if err := m.Refresh(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	out := make([]WorkspaceLifecycleSnapshot, 0, len(m.workspaces))
	changedSlugs := make([]string, 0)
	for _, slug := range sortedWorkspaceSlugs(m.workspaces) {
		snap, changed, err := m.reconcileWorkspaceLocked(slug)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		if changed {
			changedSlugs = append(changedSlugs, slug)
		}
		out = append(out, snap)
	}
	m.mu.Unlock()
	for _, slug := range changedSlugs {
		m.notifyLifecycleChanged(slug)
	}
	return out, nil
}

func (m *ManagerService) ReconcileWorkspace(
	ctx context.Context,
	slug string,
) (WorkspaceLifecycleSnapshot, error) {
	if err := m.Refresh(ctx); err != nil {
		return WorkspaceLifecycleSnapshot{}, err
	}
	m.mu.Lock()
	snap, changed, err := m.reconcileWorkspaceLocked(slug)
	m.mu.Unlock()
	if changed {
		m.notifyLifecycleChanged(slug)
	}
	return snap, err
}

func (m *ManagerService) reconcileWorkspaceLocked(
	slug string,
) (WorkspaceLifecycleSnapshot, bool, error) {
	ws, ok := m.workspaces[slug]
	if !ok {
		return WorkspaceLifecycleSnapshot{}, false, fmt.Errorf(
			"unknown workspace %q",
			slug,
		)
	}
	state := m.lifecycleStateLocked(ws)
	changed := false
	if ws.Status == StatusRunning && ws.PID != 0 && !m.processAlive(ws.PID) {
		ws.Status = StatusCrashed
		ws.PID = 0
		ws.Error = "workspace process is not alive"
		state.ObservedState = WorkspaceObservedCrashed
		state.Error = ws.Error
		state.TransitionID = ""
		state.TransitionKind = ""
		state.TransitionUpdatedAt = m.lifecycleNow()
		changed = true
	}
	if ws.Status == StatusCrashed && state.ObservedState == WorkspaceObservedCrashed &&
		state.Error != "" && state.TransitionUpdatedAt.IsZero() {
		state.TransitionUpdatedAt = m.lifecycleNow()
		changed = true
	}
	if lifecycleTransitionExpired(state, m.lifecycleNow()) {
		state.ObservedState = WorkspaceObservedFailed
		state.Error = fmt.Sprintf(
			"workspace %s transition %s expired",
			state.TransitionKind,
			state.TransitionID,
		)
		state.TransitionID = ""
		state.TransitionKind = ""
		state.TransitionUpdatedAt = m.lifecycleNow()
		ws.Status = StatusFailed
		ws.Error = state.Error
		changed = true
	}
	if changed {
		if err := m.store.WriteLifecycle(ws, state); err != nil {
			return WorkspaceLifecycleSnapshot{}, false, err
		}
		if err := m.writeRuntimeStatus(ws); err != nil {
			return WorkspaceLifecycleSnapshot{}, false, err
		}
		m.workspaces[slug] = ws
	}
	return snapshotFromState(ws, state), changed, nil
}

func (m *ManagerService) Start(ctx context.Context, slug string) (Workspace, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return Workspace{}, fmt.Errorf("unknown workspace %q", slug)
	}
	if ws.Status == StatusInvalid {
		m.mu.Unlock()
		return ws, fmt.Errorf("workspace %q is invalid: %s", slug, ws.Error)
	}
	if ws.IsMain {
		m.mu.Unlock()
		return ws, fmt.Errorf("workspace %q is managed by the control server", slug)
	}
	if ws.Status == StatusRunning || ws.Status == StatusStarting ||
		ws.Status == StatusStopping {
		m.mu.Unlock()
		return ws, nil
	}
	m.mu.Unlock()
	return m.startWorkspaceProcess(ctx, ws)
}

func (m *ManagerService) startOwnedLifecycleTransition(
	ctx context.Context,
	input WorkspaceLifecycleWorkflowInput,
) (Workspace, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[input.Slug]
	if !ok {
		m.mu.Unlock()
		return Workspace{}, fmt.Errorf("unknown workspace %q", input.Slug)
	}
	if !m.transitionOwnsWorkspaceLocked(ws, input.TransitionID) {
		m.mu.Unlock()
		return ws, nil
	}
	if adopted, ok := m.adoptRunningRuntimeLocked(ws); ok {
		m.mu.Unlock()
		return adopted, nil
	}
	m.mu.Unlock()
	return m.startWorkspaceProcess(ctx, ws)
}

func (m *ManagerService) startWorkspaceProcess(
	ctx context.Context,
	ws Workspace,
) (Workspace, error) {
	slug := ws.Slug
	m.mu.Lock()
	current, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return Workspace{}, fmt.Errorf("unknown workspace %q", slug)
	}
	current.Status = StatusStarting
	current.Phase = PhaseStartingTemporal
	current.LogPath = current.Bundle.WebLog
	current.Error = ""
	m.workspaces[slug] = current
	log.Printf(
		"workspace_start_requested slug=%q checkout=%q package=%q log=%q",
		current.Slug,
		current.CheckoutPath,
		current.PackagePath,
		current.LogPath,
	)
	m.mu.Unlock()

	started, handles, err := m.bundleRuntime.StartBundle(ctx, current, m.runtime)
	if err != nil {
		return m.markError(slug, StatusFailed, err)
	}

	m.mu.Lock()
	m.children[slug] = handles
	m.workspaces[slug] = started
	log.Printf(
		"workspace_started slug=%q pid=%d port=%d log=%q",
		started.Slug,
		started.PID,
		started.Port,
		started.LogPath,
	)
	m.mu.Unlock()
	go m.watchBundle(slug, handles)
	return started, nil
}

func (m *ManagerService) CleanupWorkspace(
	ctx context.Context,
	input WorkspaceCleanupWorkflowInput,
) error {
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		return fmt.Errorf("workspace slug is required")
	}
	if input.Disposition == WorkspaceCleanupDispositionUnmerged && !input.Confirmed {
		return fmt.Errorf("unmerged workspace cleanup requires confirmation")
	}

	m.mu.Lock()
	ws, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	if ws.IsMain || slug == mainWorkspaceSlug {
		m.mu.Unlock()
		return fmt.Errorf("workspace %q is managed by the control server", slug)
	}
	checkoutPath := ws.CheckoutPath
	if err := m.validateCleanupPathLocked(ws); err != nil {
		m.mu.Unlock()
		return err
	}
	shouldStop := ws.Status != "" && ws.Status != StatusStopped
	m.mu.Unlock()

	if shouldStop {
		if _, err := m.Stop(ctx, slug); err != nil {
			return err
		}
	}
	if checkoutPath != "" {
		if err := os.RemoveAll(checkoutPath); err != nil {
			return fmt.Errorf("remove workspace checkout: %w", err)
		}
	}

	m.mu.Lock()
	delete(m.children, slug)
	delete(m.workspaces, slug)
	m.mu.Unlock()
	m.notifyLifecycleChanged(slug)
	return nil
}

func (m *ManagerService) validateCleanupPathLocked(ws Workspace) error {
	checkoutPath, err := filepath.Abs(strings.TrimSpace(ws.CheckoutPath))
	if err != nil || checkoutPath == "" {
		return fmt.Errorf("workspace %q has invalid checkout path", ws.Slug)
	}
	checkoutPath = filepath.Clean(checkoutPath)
	if checkoutPath == string(filepath.Separator) {
		return fmt.Errorf("workspace %q checkout path is unsafe", ws.Slug)
	}
	mainPath := strings.TrimSpace(m.discovery.MainCheckoutPath)
	if mainPath != "" {
		if absMain, err := filepath.Abs(mainPath); err == nil && checkoutPath == filepath.Clean(absMain) {
			return fmt.Errorf("workspace %q checkout path is the main checkout", ws.Slug)
		}
	}
	for slug, checkout := range m.discovery.ConfiguredCheckouts {
		root := strings.TrimSpace(checkout.RootPath)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err == nil && checkoutPath == filepath.Clean(absRoot) {
			return fmt.Errorf("workspace %q checkout path is configured checkout %q", ws.Slug, slug)
		}
	}
	parent := strings.TrimSpace(m.discovery.ParentDir)
	if parent == "" && mainPath != "" {
		parent = filepath.Dir(mainPath)
	}
	if parent == "" {
		return nil
	}
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return fmt.Errorf("workspace parent path is invalid: %w", err)
	}
	absParent = filepath.Clean(absParent)
	rel, err := filepath.Rel(absParent, checkoutPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("workspace %q checkout path is outside workspace parent", ws.Slug)
	}
	return nil
}

func (m *ManagerService) Stop(ctx context.Context, slug string) (Workspace, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return Workspace{}, fmt.Errorf("unknown workspace %q", slug)
	}
	if ws.IsMain {
		m.mu.Unlock()
		return ws, fmt.Errorf("workspace %q is managed by the control server", slug)
	}
	handles := m.children[slug]
	ws.Status = StatusStopping
	ws.Phase = PhaseStopping
	m.workspaces[slug] = ws
	log.Printf(
		"workspace_stop_requested slug=%q pid=%d port=%d log=%q",
		ws.Slug,
		ws.PID,
		ws.Port,
		ws.LogPath,
	)
	m.mu.Unlock()

	stopped, err := m.bundleRuntime.StopBundle(ctx, ws, handles)
	if err != nil {
		return m.markError(slug, StatusFailed, err)
	}

	m.mu.Lock()
	delete(m.children, slug)
	m.workspaces[slug] = stopped
	log.Printf(
		"workspace_stopped slug=%q port=%d log=%q",
		stopped.Slug,
		stopped.Port,
		stopped.LogPath,
	)
	m.mu.Unlock()
	return stopped, nil
}

func (m *ManagerService) Restart(ctx context.Context, slug string) (Workspace, error) {
	if _, err := m.Stop(ctx, slug); err != nil {
		return m.markError(slug, StatusFailed, err)
	}
	return m.Start(ctx, slug)
}

func (m *ManagerService) RestartComponents(
	ctx context.Context,
	slug string,
	components []BundleComponent,
	opts RestartComponentsOptions,
) (Workspace, error) {
	if opts.Force {
		_ = m.Refresh(ctx)
	}
	m.mu.Lock()
	ws, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return Workspace{}, fmt.Errorf("unknown workspace %q", slug)
	}
	if ws.IsMain {
		m.mu.Unlock()
		return ws, fmt.Errorf("workspace %q is managed by the control server", slug)
	}
	if len(components) == 0 {
		m.mu.Unlock()
		return ws, nil
	}
	if opts.Force {
		ws = m.reconcileRuntimeLocked(ws)
		m.workspaces[slug] = ws
		if ws.Status == StatusFailed || ws.Status == StatusCrashed ||
			ws.Status == StatusStopped {
			m.mu.Unlock()
			return m.Restart(ctx, slug)
		}
	}
	handles := m.children[slug]
	m.mu.Unlock()

	restarted, nextHandles, err := m.bundleRuntime.RestartComponents(
		ctx,
		ws,
		handles,
		components,
		m.runtime,
		opts,
	)
	if err != nil {
		return m.markError(slug, StatusFailed, err)
	}

	m.mu.Lock()
	m.children[slug] = nextHandles
	m.workspaces[slug] = restarted
	m.mu.Unlock()
	go m.watchBundle(slug, nextHandles)
	return restarted, nil
}

func (m *ManagerService) watchBundle(slug string, handles BundleHandles) {
	web := handles[ComponentWeb]
	if web == nil {
		return
	}
	err := web.wait(context.Background())
	m.mu.Lock()
	current, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return
	}
	if m.children[slug][ComponentWeb] != web {
		m.mu.Unlock()
		return
	}
	delete(m.children, slug)
	if current.Status == StatusStopping || current.Status == StatusStopped {
		current.Status = StatusStopped
		log.Printf("workspace_child_exited slug=%q status=%q", slug, current.Status)
	} else {
		current.Status = StatusCrashed
		if err != nil {
			current.Error = err.Error()
		}
		log.Printf("workspace_child_crashed slug=%q error=%q", slug, current.Error)
	}
	current.PID = 0
	m.workspaces[slug] = current
	_ = m.writeRuntimeStatus(current)
	sink := m.workspaceErrorSinkLocked()
	crashed := current.Status == StatusCrashed
	m.mu.Unlock()
	if crashed {
		if recErr := recordWorkspaceManagerError(context.Background(), sink, current, "child_crashed"); recErr != nil {
			log.Printf("workspace_manager_error_record_failed slug=%q error=%v", slug, recErr)
		}
	}
	m.notifyLifecycleChanged(slug)
}

func (m *ManagerService) markError(
	slug string,
	status Status,
	err error,
) (Workspace, error) {
	m.mu.Lock()
	ws := m.workspaces[slug]
	ws.Status = status
	ws.Error = err.Error()
	ws.PID = 0
	m.workspaces[slug] = ws
	_ = m.writeRuntimeStatus(ws)
	sink := m.workspaceErrorSinkLocked()
	m.mu.Unlock()
	log.Printf("workspace_error slug=%q status=%q error=%q", slug, status, ws.Error)
	if recErr := recordWorkspaceManagerError(context.Background(), sink, ws, "mark_error"); recErr != nil {
		log.Printf("workspace_manager_error_record_failed slug=%q error=%v", slug, recErr)
	}
	return ws, err
}

func (m *ManagerService) writeRuntimeStatus(ws Workspace) error {
	if m.store == nil {
		m.store = FileBundleStore{}
	}
	return m.store.WriteStatus(
		ws,
		RuntimeStatus{
			Status: ws.Status,
			Phase:  ws.Phase,
			Error:  ws.Error,
			Logs:   bundleLogs(ws.Bundle),
			Ports:  ws.Ports,
			PIDs:   ws.PIDs,
			Build:  ws.BuildStatus,
		},
	)
}

func (m *ManagerService) RequestLifecycle(
	ctx context.Context,
	req WorkspaceLifecycleRequest,
) (WorkspaceLifecycleSnapshot, error) {
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		return WorkspaceLifecycleSnapshot{}, fmt.Errorf("workspace slug is required")
	}
	if req.DesiredState == "" {
		req.DesiredState = desiredStateForTransition(req.Kind)
	}
	if err := validateLifecycleRequest(req); err != nil {
		return WorkspaceLifecycleSnapshot{}, err
	}
	if m.lifecycleStarter == nil {
		return WorkspaceLifecycleSnapshot{}, fmt.Errorf(
			"workspace lifecycle starter is not configured",
		)
	}

	var input WorkspaceLifecycleWorkflowInput
	m.mu.Lock()
	ws, ok := m.workspaces[req.Slug]
	if !ok {
		m.mu.Unlock()
		return WorkspaceLifecycleSnapshot{}, fmt.Errorf("unknown workspace %q", req.Slug)
	}
	if ws.Status == StatusInvalid {
		snap := snapshotFromState(ws, m.lifecycleStateLocked(ws))
		m.mu.Unlock()
		return snap, fmt.Errorf("workspace %q is invalid: %s", req.Slug, ws.Error)
	}
	if ws.IsMain {
		snap := snapshotFromState(ws, m.lifecycleStateLocked(ws))
		m.mu.Unlock()
		return snap, fmt.Errorf("workspace %q is managed by the control server", req.Slug)
	}

	state := m.lifecycleStateLocked(ws)
	if state.ObservedState == WorkspaceObservedStarting ||
		state.ObservedState == WorkspaceObservedStopping {
		if state.DesiredState == req.DesiredState {
			snap := snapshotFromState(ws, state)
			m.mu.Unlock()
			return snap, nil
		}
		state.DesiredState = req.DesiredState
		state.TransitionUpdatedAt = m.lifecycleNow()
		if err := m.store.WriteLifecycle(ws, state); err != nil {
			m.mu.Unlock()
			return WorkspaceLifecycleSnapshot{}, err
		}
		snap := snapshotFromState(ws, state)
		m.workspaces[req.Slug] = snap.Workspace
		m.mu.Unlock()
		m.notifyLifecycleChanged(req.Slug)
		return snap, nil
	}

	if lifecycleAlreadyTerminalForRequest(state, req) {
		snap := snapshotFromState(ws, state)
		m.mu.Unlock()
		return snap, nil
	}

	input, snap, err := m.createTransitionLocked(ws, req)
	if err != nil {
		m.mu.Unlock()
		return WorkspaceLifecycleSnapshot{}, err
	}
	m.mu.Unlock()
	m.notifyLifecycleChanged(req.Slug)

	if err := m.lifecycleStarter.StartTransition(ctx, input); err != nil {
		_ = m.CompleteTransition(
			context.Background(),
			req.Slug,
			input.TransitionID,
			WorkspaceTransitionResult{
				ObservedState: WorkspaceObservedFailed,
				Error:         err.Error(),
			},
		)
		return snap, err
	}
	return snap, nil
}

func (m *ManagerService) CompleteTransition(
	ctx context.Context,
	slug, transitionID string,
	result WorkspaceTransitionResult,
) error {
	var followup *WorkspaceLifecycleWorkflowInput

	m.mu.Lock()
	ws, ok := m.workspaces[slug]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown workspace %q", slug)
	}
	state := m.lifecycleStateLocked(ws)
	if state.TransitionID != transitionID {
		m.mu.Unlock()
		return nil
	}
	if result.Workspace.Slug != "" {
		ws = result.Workspace
	}
	state.ObservedState = result.ObservedState
	state.Error = result.Error
	state.TransitionUpdatedAt = m.lifecycleNow()
	if result.ObservedState != WorkspaceObservedStarting &&
		result.ObservedState != WorkspaceObservedStopping {
		state.TransitionID = ""
		state.TransitionKind = ""
	}
	if err := m.store.WriteLifecycle(ws, state); err != nil {
		m.mu.Unlock()
		return err
	}
	ws.Status = statusFromObserved(state.ObservedState)
	ws.Error = state.Error
	m.workspaces[slug] = ws
	if err := m.writeRuntimeStatus(ws); err != nil {
		m.mu.Unlock()
		return err
	}
	if state.TransitionID == "" &&
		desiredDisagreesWithObserved(state.DesiredState, state.ObservedState) {
		nextReq := lifecycleFollowupRequest(slug, state.DesiredState)
		input, _, err := m.createTransitionLocked(ws, nextReq)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		followup = &input
	}
	m.mu.Unlock()
	m.notifyLifecycleChanged(slug)

	if followup != nil {
		if err := m.lifecycleStarter.StartTransition(ctx, *followup); err != nil {
			_ = m.CompleteTransition(
				context.Background(),
				slug,
				followup.TransitionID,
				WorkspaceTransitionResult{
					ObservedState: WorkspaceObservedFailed,
					Error:         err.Error(),
				},
			)
			return err
		}
	}
	return nil
}

func (m *ManagerService) transitionOwnsWorkspace(slug, transitionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws, ok := m.workspaces[slug]
	if !ok {
		return false
	}
	return m.transitionOwnsWorkspaceLocked(ws, transitionID)
}

func (m *ManagerService) transitionOwnsWorkspaceLocked(
	ws Workspace,
	transitionID string,
) bool {
	state := m.lifecycleStateLocked(ws)
	return state.TransitionID == transitionID
}

func (m *ManagerService) adoptRunningRuntimeLocked(ws Workspace) (Workspace, bool) {
	if ws.Status == StatusRunning && ws.PID != 0 && m.processAlive(ws.PID) {
		return ws, true
	}
	status, err := m.store.ReadStatus(ws)
	if err != nil || status.Status != StatusRunning {
		return ws, false
	}
	pid := status.PIDs[ComponentWeb]
	if pid == 0 || !m.processAlive(pid) {
		return ws, false
	}
	ws.Status = StatusRunning
	ws.Phase = status.Phase
	ws.Error = status.Error
	ws.Ports = status.Ports
	ws.PIDs = status.PIDs
	ws.BuildStatus = status.Build
	ws.Port = status.Ports[ComponentWeb]
	ws.PID = pid
	m.workspaces[ws.Slug] = ws
	return ws, true
}

func (m *ManagerService) createTransitionLocked(
	ws Workspace,
	req WorkspaceLifecycleRequest,
) (WorkspaceLifecycleWorkflowInput, WorkspaceLifecycleSnapshot, error) {
	transitionID := m.nextTransitionID()
	observed := WorkspaceObservedStarting
	if req.Kind == WorkspaceTransitionStop {
		observed = WorkspaceObservedStopping
	}
	now := m.lifecycleNow()
	state := WorkspaceLifecycleState{
		DesiredState:        req.DesiredState,
		ObservedState:       observed,
		TransitionKind:      req.Kind,
		TransitionID:        transitionID,
		TransitionStartedAt: now,
		TransitionUpdatedAt: now,
	}
	if err := m.store.WriteLifecycle(ws, state); err != nil {
		return WorkspaceLifecycleWorkflowInput{}, WorkspaceLifecycleSnapshot{}, err
	}
	ws.Status = statusFromObserved(observed)
	ws.Phase = phaseForTransition(req.Kind)
	ws.Error = ""
	if err := m.writeRuntimeStatus(ws); err != nil {
		return WorkspaceLifecycleWorkflowInput{}, WorkspaceLifecycleSnapshot{}, err
	}
	m.workspaces[req.Slug] = ws
	input := WorkspaceLifecycleWorkflowInput{
		Slug:         req.Slug,
		TransitionID: transitionID,
		Kind:         req.Kind,
	}
	return input, snapshotFromState(ws, state), nil
}

func (m *ManagerService) lifecycleNow() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now().UTC()
}

func (m *ManagerService) nextTransitionID() string {
	if m.newTransitionID != nil {
		return m.newTransitionID()
	}
	return uuid.NewString()
}

func desiredStateForTransition(kind WorkspaceTransitionKind) WorkspaceDesiredState {
	switch kind {
	case WorkspaceTransitionStop:
		return WorkspaceDesiredStopped
	case WorkspaceTransitionStart, WorkspaceTransitionRestart:
		return WorkspaceDesiredRunning
	default:
		return ""
	}
}

func validateLifecycleRequest(req WorkspaceLifecycleRequest) error {
	switch req.Kind {
	case WorkspaceTransitionStart, WorkspaceTransitionStop, WorkspaceTransitionRestart:
	default:
		return fmt.Errorf("unsupported workspace transition %q", req.Kind)
	}
	switch req.DesiredState {
	case WorkspaceDesiredRunning, WorkspaceDesiredStopped:
		return nil
	default:
		return fmt.Errorf("unsupported workspace desired state %q", req.DesiredState)
	}
}

func phaseForTransition(kind WorkspaceTransitionKind) BundlePhase {
	switch kind {
	case WorkspaceTransitionStop:
		return PhaseStopping
	case WorkspaceTransitionRestart:
		return PhaseRestartingWeb
	default:
		return PhaseStartingTemporal
	}
}

func lifecycleAlreadyTerminalForRequest(
	state WorkspaceLifecycleState,
	req WorkspaceLifecycleRequest,
) bool {
	if req.Kind == WorkspaceTransitionRestart {
		return false
	}
	return (req.DesiredState == WorkspaceDesiredRunning && state.ObservedState == WorkspaceObservedRunning) ||
		(req.DesiredState == WorkspaceDesiredStopped && state.ObservedState == WorkspaceObservedStopped)
}

func desiredDisagreesWithObserved(
	desired WorkspaceDesiredState,
	observed WorkspaceObservedState,
) bool {
	switch observed {
	case WorkspaceObservedRunning:
		return desired == WorkspaceDesiredStopped
	case WorkspaceObservedStopped:
		return desired == WorkspaceDesiredRunning
	default:
		return false
	}
}

func lifecycleFollowupRequest(
	slug string,
	desired WorkspaceDesiredState,
) WorkspaceLifecycleRequest {
	kind := WorkspaceTransitionStart
	if desired == WorkspaceDesiredStopped {
		kind = WorkspaceTransitionStop
	}
	return WorkspaceLifecycleRequest{Slug: slug, DesiredState: desired, Kind: kind}
}

func sortedWorkspaceSlugs(workspaces map[string]Workspace) []string {
	items := make([]Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		items = append(items, ws)
	}
	sortWorkspaces(items)
	slugs := make([]string, 0, len(items))
	for _, ws := range items {
		slugs = append(slugs, ws.Slug)
	}
	return slugs
}
