package appletruntime

import "context"

// RuntimeConfig describes a local, generated applet process.
// It is intentionally applet-scoped and separate from workspace lifecycle state.
type RuntimeConfig struct {
	AppID        string
	FilesRoot    string
	SourceDir    string
	BuildCommand []string
	StartCommand []string
	HealthPath   string
	Env          map[string]string
}

// ProcessState is the active applet process visible to the shell/proxy.
type ProcessState struct {
	AppID     string
	SourceDir string
	Port      int
	PID       int
	BaseURL   string
	Healthy   bool
	LogPath   string
}

// Manager starts, stops, health-checks, and locates applet processes.
type Manager interface {
	Start(ctx context.Context, cfg RuntimeConfig) (ProcessState, error)
	Stop(ctx context.Context, appID string) error
	Health(ctx context.Context, appID string) (ProcessState, error)
	ProxyTarget(appID string) (string, bool)
}
