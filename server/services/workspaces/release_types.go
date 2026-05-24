package workspaces

import (
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

type ReleaseQueueStatus string

const (
	ReleaseQueueStatusPending   ReleaseQueueStatus = "pending"
	ReleaseQueueStatusRunning   ReleaseQueueStatus = "running"
	ReleaseQueueStatusSucceeded ReleaseQueueStatus = "succeeded"
	ReleaseQueueStatusFailed    ReleaseQueueStatus = "failed"
	ReleaseQueueStatusCanceled  ReleaseQueueStatus = "canceled"
)

type ReleaseQueueItem struct {
	ID                   string
	DefinitionID         release.DefinitionID
	DefinitionVersion    string
	WorkflowID           runtime.WorkflowID
	WorkflowVersion      string
	FlowID               release.FlowID
	SourceSlug           string
	TargetLane           string
	ExpectedSourceCommit string
	ExpectedTargetCommit string
	Status               ReleaseQueueStatus
	CurrentNodeID        runtime.NodeID
	ActorEmail           string
	ErrorMessage         string
	PayloadJSON          string
	CreatedAt            time.Time
	StartedAt            *time.Time
	FinishedAt           *time.Time
	UpdatedAt            time.Time
}

type ReleaseQueueEvent struct {
	ID          int64
	ItemID      string
	Level       string
	NodeID      runtime.NodeID
	Message     string
	PayloadJSON string
	CreatedAt   time.Time
}

type ReleaseLaneView struct {
	ID        release.LaneID
	Label     string
	Workspace Workspace
	Actions   []ReleaseActionView
}

type ReleaseActionView struct {
	DefinitionID         release.DefinitionID
	DefinitionVersion    string
	FlowID               release.FlowID
	Label                string
	SourceSlug           string
	TargetLane           release.LaneID
	ExpectedSourceCommit string
	ExpectedTargetCommit string
	Disabled             bool
	DisabledReason       string
}

type ReleaseQueueView struct {
	Active  []ReleaseQueueItem
	Pending []ReleaseQueueItem
}

type ReleasePanelModel struct {
	Enabled        bool
	Lanes          []ReleaseLaneView
	FeatureActions []ReleaseActionView
	Queue          ReleaseQueueView
	History        []ReleaseQueueItem
}
