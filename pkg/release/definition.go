package release

import "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

type DefinitionID string
type LaneID string
type FlowID string
type CheckID string

type PushPolicy string

type SourceKind string

const (
	PushNever       PushPolicy = "never"
	PushAfterVerify PushPolicy = "after_verify"

	SourceFeature SourceKind = "feature"
	SourceLane    SourceKind = "lane"
)

type Definition struct {
	ID      DefinitionID
	Version string
	Name    string
	Enabled bool
	Lanes   map[LaneID]LaneDefinition
	Flows   map[FlowID]FlowDefinition
}

type LaneDefinition struct {
	ID           LaneID
	CheckoutSlug string
	Label        string
	Protected    bool
}

type FlowDefinition struct {
	ID              FlowID
	Label           string
	WorkflowID      runtime.WorkflowID
	WorkflowVersion string
	Source          SourceSelector
	TargetLane      LaneID
	PushPolicy      PushPolicy
	Preconditions   []CheckSpec
}

type SourceSelector struct {
	Kind SourceKind
	Lane LaneID
}

type CheckSpec struct {
	ID   CheckID
	Args map[string]string
}
