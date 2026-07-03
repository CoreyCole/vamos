package qrspicmd

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

const schemaVersion = 1

type ManagerState struct {
	SchemaVersion       int                  `json:"schemaVersion"`
	RepoID              string               `json:"repoId"`
	CanonicalPlanDir    string               `json:"canonicalPlanDir"`
	ManagerRunID        string               `json:"managerRunId"`
	SourceCwd           string               `json:"sourceCwd"`
	ImplementationCwd   string               `json:"implementationCwd,omitempty"`
	PiModel             string               `json:"piModel,omitempty"`
	ManagerPaneID       string               `json:"managerPaneId,omitempty"`
	ManagerSessionPath  string               `json:"managerSessionPath,omitempty"`
	LastManagerUsage    *ManagerUsageSample  `json:"lastManagerUsage,omitempty"`
	Delivery            ManagerDeliveryState `json:"delivery,omitempty"`
	LastActionCard      *ManagerActionCard   `json:"lastActionCard,omitempty"`
	ActiveChild         *ChildRunRef         `json:"activeChild,omitempty"`
	PendingCleanupChild *ChildRunRef         `json:"pendingCleanupChild,omitempty"`
	Workflow            wruntime.State       `json:"workflow"`
}

type ManagerDeliveryState struct {
	Status         string      `json:"status,omitempty"`
	ManagerPaneID  string      `json:"managerPaneId,omitempty"`
	QueuedWake     *QueuedWake `json:"queuedWake,omitempty"`
	LastDeliveryID string      `json:"lastDeliveryId,omitempty"`
}

type QueuedWake struct {
	DeliveryID      string `json:"deliveryId"`
	ChildID         string `json:"childId"`
	ChildGeneration int    `json:"childGeneration"`
	Payload         string `json:"payload"`
	QueuedAt        string `json:"queuedAt"`
	DeliveredAt     string `json:"deliveredAt,omitempty"`
}

type ChildRunRef struct {
	ID                   string `json:"id"`
	Stage                string `json:"stage"`
	Cwd                  string `json:"cwd"`
	TmuxPaneID           string `json:"tmuxPaneId,omitempty"`
	OutputPath           string `json:"outputPath"`
	SessionID            string `json:"sessionId"`
	SessionDir           string `json:"sessionDir"`
	SessionPath          string `json:"sessionPath,omitempty"`
	DonePath             string `json:"donePath"`
	StatusPath           string `json:"statusPath"`
	ResultPath           string `json:"resultPath,omitempty"`
	ValidationStatusPath string `json:"validationStatusPath,omitempty"`
	LastDeliveryID       string `json:"lastDeliveryId,omitempty"`
	LifecycleStatus      string `json:"lifecycleStatus,omitempty"`
	Generation           int    `json:"generation,omitempty"`
	ValidationRetryCount int    `json:"validationRetryCount,omitempty"`
	LastRepromptAttempt  int    `json:"lastRepromptAttempt,omitempty"`
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
