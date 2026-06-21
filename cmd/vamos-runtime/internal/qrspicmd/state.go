package qrspicmd

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

const schemaVersion = 1

type ManagerState struct {
	SchemaVersion     int            `json:"schemaVersion"`
	RepoID            string         `json:"repoId"`
	CanonicalPlanDir  string         `json:"canonicalPlanDir"`
	ManagerRunID      string         `json:"managerRunId"`
	SourceCwd         string         `json:"sourceCwd"`
	ImplementationCwd string         `json:"implementationCwd,omitempty"`
	ManagerPaneID     string         `json:"managerPaneId,omitempty"`
	ActiveChild       *ChildRunRef   `json:"activeChild,omitempty"`
	Workflow          wruntime.State `json:"workflow"`
}

type ChildRunRef struct {
	ID          string `json:"id"`
	Stage       string `json:"stage"`
	Cwd         string `json:"cwd"`
	TmuxPaneID  string `json:"tmuxPaneId,omitempty"`
	OutputPath  string `json:"outputPath"`
	SessionID   string `json:"sessionId"`
	SessionDir  string `json:"sessionDir"`
	SessionPath string `json:"sessionPath,omitempty"`
	DonePath    string `json:"donePath"`
	StatusPath  string `json:"statusPath"`
	ResultPath  string `json:"resultPath,omitempty"`
}

type LockKey struct {
	RepoID           string `json:"repoId"`
	CanonicalPlanDir string `json:"canonicalPlanDir"`
}

type Lock struct {
	Key       LockKey `json:"key"`
	Owner     string  `json:"owner"`
	Path      string  `json:"path"`
	ExpiresAt int64   `json:"expiresAt"`
}
