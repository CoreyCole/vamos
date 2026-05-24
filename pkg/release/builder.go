package release

import (
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type Builder struct {
	def Definition
	err error
}

type LaneOption func(*LaneDefinition)
type FlowOption func(*FlowDefinition)

func NewDefinition(id DefinitionID) *Builder {
	return &Builder{def: Definition{ID: id, Version: "v1", Enabled: true, Lanes: map[LaneID]LaneDefinition{}, Flows: map[FlowID]FlowDefinition{}}}
}

func (b *Builder) Version(version string) *Builder { b.def.Version = version; return b }
func (b *Builder) Name(name string) *Builder       { b.def.Name = name; return b }
func (b *Builder) Enabled(enabled bool) *Builder   { b.def.Enabled = enabled; return b }

func (b *Builder) Lane(id LaneID, opts ...LaneOption) *Builder {
	if b.err != nil {
		return b
	}
	if id == "" {
		b.err = fmt.Errorf("lane id is required")
		return b
	}
	if _, exists := b.def.Lanes[id]; exists {
		b.err = fmt.Errorf("duplicate lane %q", id)
		return b
	}
	lane := LaneDefinition{ID: id, CheckoutSlug: string(id), Label: string(id)}
	for _, opt := range opts {
		opt(&lane)
	}
	b.def.Lanes[id] = lane
	return b
}

func (b *Builder) Flow(id FlowID, workflowID runtime.WorkflowID, opts ...FlowOption) *Builder {
	if b.err != nil {
		return b
	}
	if id == "" {
		b.err = fmt.Errorf("flow id is required")
		return b
	}
	if _, exists := b.def.Flows[id]; exists {
		b.err = fmt.Errorf("duplicate flow %q", id)
		return b
	}
	flow := FlowDefinition{ID: id, Label: string(id), WorkflowID: workflowID, WorkflowVersion: "v1", PushPolicy: PushNever}
	for _, opt := range opts {
		opt(&flow)
	}
	b.def.Flows[id] = flow
	return b
}

func (b *Builder) Build(workflows WorkflowRegistry) (Definition, error) {
	if b.err != nil {
		return Definition{}, b.err
	}
	if err := ValidateDefinition(b.def, workflows); err != nil {
		return Definition{}, err
	}
	return b.def, nil
}

func (b *Builder) MustBuild(workflows WorkflowRegistry) Definition {
	def, err := b.Build(workflows)
	if err != nil {
		panic(err)
	}
	return def
}

func CheckoutSlug(slug string) LaneOption { return func(l *LaneDefinition) { l.CheckoutSlug = slug } }
func Label(label string) LaneOption       { return func(l *LaneDefinition) { l.Label = label } }
func Protected() LaneOption               { return func(l *LaneDefinition) { l.Protected = true } }

func WorkflowVersion(version string) FlowOption {
	return func(f *FlowDefinition) { f.WorkflowVersion = version }
}
func FlowLabel(label string) FlowOption { return func(f *FlowDefinition) { f.Label = label } }
func FromFeature() FlowOption {
	return func(f *FlowDefinition) { f.Source = SourceSelector{Kind: SourceFeature} }
}
func FromLane(id LaneID) FlowOption {
	return func(f *FlowDefinition) { f.Source = SourceSelector{Kind: SourceLane, Lane: id} }
}
func ToLane(id LaneID) FlowOption { return func(f *FlowDefinition) { f.TargetLane = id } }
func NoPush() FlowOption          { return func(f *FlowDefinition) { f.PushPolicy = PushNever } }
func PushAfterVerifyPolicy() FlowOption {
	return func(f *FlowDefinition) { f.PushPolicy = PushAfterVerify }
}
func Preconditions(checks ...CheckSpec) FlowOption {
	return func(f *FlowDefinition) { f.Preconditions = append([]CheckSpec(nil), checks...) }
}

func ValidateDefinition(def Definition, workflows WorkflowRegistry) error {
	if strings.TrimSpace(string(def.ID)) == "" {
		return fmt.Errorf("definition id is required")
	}
	if strings.TrimSpace(def.Version) == "" {
		return fmt.Errorf("definition version is required")
	}
	for id, lane := range def.Lanes {
		if id != lane.ID {
			return fmt.Errorf("lane key %q does not match id %q", id, lane.ID)
		}
		if strings.TrimSpace(lane.CheckoutSlug) == "" {
			return fmt.Errorf("lane %q checkout slug is required", id)
		}
	}
	for id, flow := range def.Flows {
		if id != flow.ID {
			return fmt.Errorf("flow key %q does not match id %q", id, flow.ID)
		}
		if flow.WorkflowID == "" {
			return fmt.Errorf("flow %q workflow id is required", id)
		}
		if flow.WorkflowVersion == "" {
			return fmt.Errorf("flow %q workflow version is required", id)
		}
		if _, ok := def.Lanes[flow.TargetLane]; !ok {
			return fmt.Errorf("flow %q target lane %q is not defined", id, flow.TargetLane)
		}
		if flow.Source.Kind == SourceLane {
			lane, ok := def.Lanes[flow.Source.Lane]
			if !ok {
				return fmt.Errorf("flow %q source lane %q is not defined", id, flow.Source.Lane)
			}
			if lane.Protected {
				return fmt.Errorf("flow %q uses protected source lane %q", id, flow.Source.Lane)
			}
		}
		wf, ok := workflows.GetVersion(flow.WorkflowID, flow.WorkflowVersion)
		if !ok {
			return fmt.Errorf("flow %q workflow %q version %q is not registered", id, flow.WorkflowID, flow.WorkflowVersion)
		}
		if flow.PushPolicy == PushAfterVerify && !hasReachablePushService(wf) {
			return fmt.Errorf("flow %q push policy requires reachable release.push service node", id)
		}
	}
	return nil
}

func hasReachablePushService(def runtime.Definition) bool {
	seen := map[runtime.NodeID]bool{}
	var visit func(runtime.NodeID) bool
	visit = func(id runtime.NodeID) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		node, ok := def.Nodes[id]
		if !ok {
			return false
		}
		if node.Kind == runtime.NodeKindService && node.Service.Type == "release.push" {
			return true
		}
		for _, edge := range def.Edges {
			if edge.From == id && visit(edge.To) {
				return true
			}
		}
		return false
	}
	return visit(def.Start)
}
