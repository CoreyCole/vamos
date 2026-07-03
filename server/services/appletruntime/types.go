package appletruntime

import (
	"context"
	"time"
)

// RuntimeConfig describes a local, generated applet process.
// It is intentionally applet-scoped and separate from workspace lifecycle state.
type RuntimeConfig struct {
	AppID        string
	FilesRoot    string
	SourceDir    string
	BuildCommand []string
	StartCommand []string
	HealthPath   string
	IdleTimeout  time.Duration
	Env          map[string]string
}

type ProcessStatus string

const (
	ProcessStatusStopped   ProcessStatus = "stopped"
	ProcessStatusStarting  ProcessStatus = "starting"
	ProcessStatusHealthy   ProcessStatus = "healthy"
	ProcessStatusUnhealthy ProcessStatus = "unhealthy"
)

// AppletProcessState is the active applet process visible to the shell/proxy.
type AppletProcessState struct {
	AppID             string
	SourceDir         string
	Status            ProcessStatus
	PID               int
	Port              int
	BaseURL           string
	LogPath           string
	LastSeenAt        time.Time
	ActiveConnections int
	IdleTimeout       time.Duration

	// Healthy is kept during the transition from the older boolean process model.
	Healthy bool
}

// ProcessState preserves the previous public name while applet callers migrate.
type ProcessState = AppletProcessState

// Manager starts, stops, health-checks, and locates applet processes.
type Manager interface {
	EnsureStarted(ctx context.Context, cfg RuntimeConfig) (AppletProcessState, error)
	Start(ctx context.Context, cfg RuntimeConfig) (ProcessState, error)
	Stop(ctx context.Context, appID string) error
	Health(ctx context.Context, appID string) (AppletProcessState, error)
	ProxyTarget(appID string) (string, bool)
	Touch(appID string, activeDelta int)
	SweepInactive(ctx context.Context, now time.Time) ([]AppletProcessState, error)
}
