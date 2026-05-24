package workspaces

import (
	"context"
	"net/http"
	"time"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusFailed   Status = "failed"
	StatusCrashed  Status = "crashed"
	StatusInvalid  Status = "invalid"
)

type WorkspaceDesiredState string

const (
	WorkspaceDesiredRunning WorkspaceDesiredState = "running"
	WorkspaceDesiredStopped WorkspaceDesiredState = "stopped"
)

type WorkspaceObservedState Status

const (
	WorkspaceObservedStopped WorkspaceObservedState = WorkspaceObservedState(
		StatusStopped,
	)
	WorkspaceObservedStarting WorkspaceObservedState = WorkspaceObservedState(
		StatusStarting,
	)
	WorkspaceObservedRunning WorkspaceObservedState = WorkspaceObservedState(
		StatusRunning,
	)
	WorkspaceObservedStopping WorkspaceObservedState = WorkspaceObservedState(
		StatusStopping,
	)
	WorkspaceObservedFailed WorkspaceObservedState = WorkspaceObservedState(
		StatusFailed,
	)
	WorkspaceObservedCrashed WorkspaceObservedState = WorkspaceObservedState(
		StatusCrashed,
	)
	WorkspaceObservedInvalid WorkspaceObservedState = WorkspaceObservedState(
		StatusInvalid,
	)
)

type WorkspaceTransitionKind string

const (
	WorkspaceTransitionStart   WorkspaceTransitionKind = "start"
	WorkspaceTransitionStop    WorkspaceTransitionKind = "stop"
	WorkspaceTransitionRestart WorkspaceTransitionKind = "restart"
)

type BundleComponent string

const (
	ComponentTemporal   BundleComponent = "temporal"
	ComponentTemporalUI BundleComponent = "temporal_ui"
	ComponentWeb        BundleComponent = "web"
	ComponentTSWorker   BundleComponent = "ts_worker"
)

type BundlePhase string

const (
	PhaseStartingTemporal BundlePhase = "starting_temporal"
	PhaseStartingWeb      BundlePhase = "starting_web"
	PhaseStartingTSWorker BundlePhase = "starting_ts_worker"
	PhaseRestartingWeb    BundlePhase = "restarting_web"
	PhaseRestartingTS     BundlePhase = "restarting_ts_worker"
	PhaseStopping         BundlePhase = "stopping"
)

type WorkspaceRuntimePaths struct {
	Root, RunDir, LogDir, StateDir             string
	WorkspaceEnv, DesiredJSON, StatusJSON      string
	LifecycleJSON, PortsJSON                   string
	WebPID, TemporalPID, TSWorkerPID           string
	WebLog, TemporalLog, TSWorkerLog, BuildLog string
	AgentsDB, TemporalDB, OpenClawDir          string
	TSReadyMarker                              string
}

type BuildStatus struct {
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastFailedAt  time.Time `json:"last_failed_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	LogPath       string    `json:"log_path,omitempty"`
}

type RuntimeStatus struct {
	Status Status                     `json:"status"`
	Phase  BundlePhase                `json:"phase,omitempty"`
	Error  string                     `json:"error,omitempty"`
	Logs   map[BundleComponent]string `json:"logs,omitempty"`
	Ports  map[BundleComponent]int    `json:"ports,omitempty"`
	PIDs   map[BundleComponent]int    `json:"pids,omitempty"`
	Build  BuildStatus                `json:"build,omitempty"`
}

type DesiredState struct {
	Desired Status `json:"desired"`
}

type WorkspaceLifecycleState struct {
	DesiredState        WorkspaceDesiredState   `json:"desired_state"`
	ObservedState       WorkspaceObservedState  `json:"observed_state"`
	TransitionKind      WorkspaceTransitionKind `json:"transition_kind,omitempty"`
	TransitionID        string                  `json:"transition_id,omitempty"`
	TransitionStartedAt time.Time               `json:"transition_started_at,omitempty"`
	TransitionUpdatedAt time.Time               `json:"transition_updated_at,omitempty"`
	Error               string                  `json:"error,omitempty"`
}

type WorkspaceLifecycleSnapshot struct {
	Workspace      Workspace
	DesiredState   WorkspaceDesiredState
	ObservedState  WorkspaceObservedState
	TransitionID   string
	TransitionKind WorkspaceTransitionKind
	Error          string
}

type WorkspaceLifecycleRequest struct {
	Slug         string
	DesiredState WorkspaceDesiredState
	Kind         WorkspaceTransitionKind
}

type WorkspaceLifecycleWorkflowInput struct {
	Slug         string                  `json:"slug"`
	TransitionID string                  `json:"transition_id"`
	Kind         WorkspaceTransitionKind `json:"kind"`
}

type WorkspaceTransitionResult struct {
	ObservedState WorkspaceObservedState
	Workspace     Workspace
	Error         string
}

type WorkspaceEnv struct {
	Slug         string `json:"slug"`
	CheckoutPath string `json:"checkout_path"`
	ManagerURL   string `json:"manager_url"`
	RestartToken string `json:"restart_token"`
	DatabasePath string `json:"database_path"`
}

type Workspace struct {
	Slug            string
	DisplayName     string
	CheckoutPath    string
	PackagePath     string
	MetadataDirName string
	Host            string
	URL             string
	Status          Status
	Phase           BundlePhase
	BuildStatus     BuildStatus
	Bundle          WorkspaceRuntimePaths
	Ports           map[BundleComponent]int
	PIDs            map[BundleComponent]int
	Port            int
	PID             int
	Branch          string
	Commit          string
	Error           string
	Warnings        []string
	LogPath         string
	StateDir        string
	IsMain          bool
	IsConfigured    bool
	DiscoveredAt    time.Time
	Stack           StackSummary
}

type RecoveryModel struct {
	Workspace     Workspace
	ManagerURL    string
	Status        RuntimeStatus
	LogTails      map[BundleComponent]string
	ReturnTo      string
	Authenticated bool
	UnsafeRequest bool
	Unknown       bool
}

type ConfiguredCheckout struct {
	RootPath    string
	DisplayName string
	IsMain      bool
}

type DiscoveryConfig struct {
	MainCheckoutPath    string
	ParentDir           string
	Domain              string
	StateDir            string
	MetadataDirName     string
	CheckoutPrefixes    []string
	MainCheckoutName    string
	ModuleMarker        string
	PackageSubdir       string
	ConfiguredCheckouts map[string]ConfiguredCheckout
}

type ImplWorkspaceDiscoveryConfig struct {
	MainCheckoutPath    string
	ParentDir           string
	Domain              string
	MetadataDirName     string
	CheckoutPrefixes    []string
	MainCheckoutName    string
	ModuleMarker        string
	PackageSubdir       string
	ConfiguredCheckouts map[string]ConfiguredCheckout
}

type WorkspaceWorkflowSummary struct {
	WorkflowType    string
	Stage           string
	Status          string
	Outcome         string
	WaitingHuman    bool
	NextStep        string
	PrimaryArtifact string
}

type WorkspaceWorkflowSummaryResolver interface {
	SummaryForPlanDir(ctx context.Context, planDir string) (WorkspaceWorkflowSummary, bool, error)
}

type RuntimeConfig struct {
	ListenAddress    string
	ManagerURL       string
	RestartToken     string
	DevAuthVerifyKey string
	BaseEnv          map[string]string
	ThoughtsRepo     string
	ThoughtsRoot     string
	MetadataDirName  string
}

type VerificationLayer string

const (
	VerificationLayerConfig           VerificationLayer = "config"
	VerificationLayerDNS              VerificationLayer = "dns"
	VerificationLayerDNSDirectCoreDNS VerificationLayer = "dns-direct-coredns"
	VerificationLayerDNSRemoteSystem  VerificationLayer = "dns-remote-system"
	VerificationLayerTLS              VerificationLayer = "tls"
	VerificationLayerTLSRemote        VerificationLayer = "tls-remote"
	VerificationLayerCaddy            VerificationLayer = "caddy"
	VerificationLayerCaddyPort        VerificationLayer = "caddy-port"
	VerificationLayerCaddyProxy       VerificationLayer = "caddy-proxy"
	VerificationLayerAuth             VerificationLayer = "auth"
	VerificationLayerLifecycle        VerificationLayer = "lifecycle"
	VerificationLayerMetadata         VerificationLayer = "metadata"
	VerificationLayerLogs             VerificationLayer = "logs"
	VerificationLayerProxy            VerificationLayer = "proxy"
	VerificationLayerHandoff          VerificationLayer = "handoff"
	VerificationLayerBrowser          VerificationLayer = "browser"
)

type VerifyWorkspaceError struct {
	Layer      VerificationLayer `json:"layer"`
	Message    string            `json:"message"`
	Suggestion string            `json:"suggestion,omitempty"`
}

type VerifyWorkspaceSnapshot struct {
	Label         string             `json:"label"`
	Workspace     Workspace          `json:"workspace"`
	Metadata      *WorkspaceMetadata `json:"metadata,omitempty"`
	RuntimeStatus *RuntimeStatus     `json:"runtime_status,omitempty"`
	DesiredState  *DesiredState      `json:"desired_state,omitempty"`
	PIDAlive      bool               `json:"pid_alive"`
	PortOpen      bool               `json:"port_open"`
}

type WorkspaceDiagnostics struct {
	Workspace          Workspace          `json:"workspace"`
	Metadata           *WorkspaceMetadata `json:"metadata,omitempty"`
	MetadataRaw        string             `json:"metadata_raw,omitempty"`
	MetadataPath       string             `json:"metadata_path"`
	RuntimeState       *RuntimeStatus     `json:"runtime_status,omitempty"`
	RuntimeStatusError string             `json:"runtime_status_error,omitempty"`
	DesiredState       *DesiredState      `json:"desired_state,omitempty"`
	DesiredStateError  string             `json:"desired_state_error,omitempty"`
	PIDAlive           bool               `json:"pid_alive"`
	PortOpen           bool               `json:"port_open"`
	LogPath            string             `json:"log_path"`
	LogTail            string             `json:"log_tail,omitempty"`
	ManagerURL         string             `json:"manager_url"`
	PublicURL          string             `json:"public_url"`
	LatestError        string             `json:"latest_error,omitempty"`
}

type VerifyWorkspaceRequest struct {
	Slug      string `json:"slug"`
	Start     bool   `json:"start"`
	Restart   bool   `json:"restart"`
	Stop      bool   `json:"stop"`
	TailLines int    `json:"tail_lines"`
	ReportDir string `json:"report_dir"`
}

type VerifyRunStatus string

const (
	VerifyRunPending VerifyRunStatus = "pending"
	VerifyRunRunning VerifyRunStatus = "running"
	VerifyRunPassed  VerifyRunStatus = "passed"
	VerifyRunFailed  VerifyRunStatus = "failed"
)

type VerifyPhaseStatus string

const (
	VerifyPhasePassed  VerifyPhaseStatus = "passed"
	VerifyPhaseFailed  VerifyPhaseStatus = "failed"
	VerifyPhaseSkipped VerifyPhaseStatus = "skipped"
)

type VerifyWorkspacePhase struct {
	Name       string                `json:"name"`
	Status     VerifyPhaseStatus     `json:"status"`
	Layer      VerificationLayer     `json:"layer"`
	StartedAt  time.Time             `json:"started_at"`
	DurationMS int64                 `json:"duration_ms"`
	Detail     string                `json:"detail,omitempty"`
	Error      *VerifyWorkspaceError `json:"error,omitempty"`
}

type VerifyWorkspaceRun struct {
	ID            string                    `json:"id"`
	Slug          string                    `json:"slug"`
	Status        VerifyRunStatus           `json:"status"`
	StartedAt     time.Time                 `json:"started_at"`
	CompletedAt   *time.Time                `json:"completed_at,omitempty"`
	Phases        []VerifyWorkspacePhase    `json:"phases"`
	Snapshots     []VerifyWorkspaceSnapshot `json:"snapshots"`
	Diagnostics   WorkspaceDiagnostics      `json:"diagnostics"`
	Runtime       RuntimeStatus             `json:"runtime"`
	TemporalOK    bool                      `json:"temporal_ok"`
	WebOK         bool                      `json:"web_ok"`
	TSWorkerOK    bool                      `json:"ts_worker_ok"`
	TemporalUIURL string                    `json:"temporal_ui_url,omitempty"`
	Errors        []string                  `json:"errors,omitempty"`
	Error         *VerifyWorkspaceError     `json:"error,omitempty"`
}

type VerifyRunStore interface {
	Create(ctx context.Context, req VerifyWorkspaceRequest) (VerifyWorkspaceRun, error)
	Update(ctx context.Context, run VerifyWorkspaceRun) error
	Get(ctx context.Context, id string) (VerifyWorkspaceRun, error)
	Subscribe(ctx context.Context, id string) (<-chan VerifyWorkspaceRun, error)
}

type LogTailer interface {
	Tail(path string, lines int) (string, error)
}

type LocalProber interface {
	PIDAlive(pid int) bool
	PortOpen(addr string) bool
	HTTPHost(ctx context.Context, addr, host, path string) (*http.Response, []byte, error)
}

type Registry interface {
	Refresh(ctx context.Context) error
	List() []Workspace
	Lookup(slug string) (Workspace, bool)
	LookupHost(host string) (Workspace, bool)
}

type Manager interface {
	Registry
	Start(ctx context.Context, slug string) (Workspace, error)
	Stop(ctx context.Context, slug string) (Workspace, error)
	Restart(ctx context.Context, slug string) (Workspace, error)
}

type LifecycleManager interface {
	Registry
	RequestLifecycle(
		ctx context.Context,
		req WorkspaceLifecycleRequest,
	) (WorkspaceLifecycleSnapshot, error)
	ListLifecycle(ctx context.Context) ([]WorkspaceLifecycleSnapshot, error)
	CompleteTransition(
		ctx context.Context,
		slug, transitionID string,
		result WorkspaceTransitionResult,
	) error
}

type WorkspaceLifecycleStarter interface {
	StartTransition(ctx context.Context, input WorkspaceLifecycleWorkflowInput) error
}
