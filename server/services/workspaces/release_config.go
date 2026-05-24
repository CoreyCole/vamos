package workspaces

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

const (
	DefaultReleaseDefinitionID      release.DefinitionID = "default"
	DefaultPromoteToStageWorkflowID runtime.WorkflowID   = "release.promote_to_stage"
	DefaultReleaseToMainWorkflowID  runtime.WorkflowID   = "release.stage_to_main"
	DefaultPromoteToStageFlowID     release.FlowID       = "promote-to-stage"
	DefaultReleaseToMainFlowID      release.FlowID       = "release-to-main"
	DefaultStageLaneID              release.LaneID       = "stage"
	DefaultMainLaneID               release.LaneID       = "main"
)

type ReleaseLaneRole string

const (
	ReleaseLaneRoleStage ReleaseLaneRole = "stage"
	ReleaseLaneRoleMain  ReleaseLaneRole = "main"
)

type ReleaseLaneWorkspace struct {
	LaneID    release.LaneID
	Role      ReleaseLaneRole
	Slug      string
	Label     string
	Protected bool
}

func ReleaseLaneWorkspaces(reg *release.Registry) []ReleaseLaneWorkspace {
	if reg == nil {
		return nil
	}
	var out []ReleaseLaneWorkspace
	for _, def := range reg.Definitions() {
		for _, lane := range def.Lanes {
			role := ReleaseLaneRole(lane.ID)
			out = append(out, ReleaseLaneWorkspace{
				LaneID: lane.ID,
				Role:   role,
				Slug:   strings.TrimSpace(lane.CheckoutSlug),
				Label:  firstNonEmpty(lane.Label, string(lane.ID)),
				Protected: lane.Protected ||
					lane.ID == DefaultMainLaneID ||
					lane.ID == DefaultStageLaneID,
			})
		}
	}
	return out
}

func ProtectedReleaseSlugs(reg *release.Registry) map[string]ReleaseLaneWorkspace {
	lanes := ReleaseLaneWorkspaces(reg)
	out := make(map[string]ReleaseLaneWorkspace, len(lanes))
	for _, lane := range lanes {
		if lane.Slug != "" && lane.Protected {
			out[lane.Slug] = lane
		}
	}
	return out
}

// BuildDefaultReleaseRegistry builds the reusable two-lane release model used by
// hosts that configure generic stage/main checkouts. Hosts with different lanes
// can construct their own release.Registry and pass it to WithReleaseQueue.
func BuildDefaultReleaseRegistry(stageSlug, mainSlug string) (*runtime.Registry, *release.Registry, error) {
	stageSlug = strings.TrimSpace(stageSlug)
	mainSlug = strings.TrimSpace(mainSlug)
	if stageSlug == "" || mainSlug == "" {
		return nil, nil, fmt.Errorf("stage and main checkout slugs are required")
	}

	workflows := runtime.NewRegistry()
	promote, err := runtime.New[struct{}](DefaultPromoteToStageWorkflowID).
		Version("v1").
		Name("Promote feature workspace to stage").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight", Timeout: 5 * time.Minute}).
		Service("merge", runtime.ServiceSpec{Type: "release.merge", Timeout: 10 * time.Minute}).
		Done("done").
		Edge("preflight", "merge").
		Edge("merge", "done").
		Build()
	if err != nil {
		return nil, nil, err
	}
	if err := workflows.Register(promote); err != nil {
		return nil, nil, err
	}

	stageToMain, err := runtime.New[struct{}](DefaultReleaseToMainWorkflowID).
		Version("v1").
		Name("Release stage to main").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight", Timeout: 5 * time.Minute}).
		Service("merge", runtime.ServiceSpec{Type: "release.merge", Timeout: 10 * time.Minute}).
		Service("push", runtime.ServiceSpec{Type: "release.push", Timeout: 5 * time.Minute}).
		Done("done").
		Edge("preflight", "merge").
		Edge("merge", "push").
		Edge("push", "done").
		Build()
	if err != nil {
		return nil, nil, err
	}
	if err := workflows.Register(stageToMain); err != nil {
		return nil, nil, err
	}

	def, err := release.NewDefinition(DefaultReleaseDefinitionID).
		Version("v1").
		Name("Default stage/main release lane").
		Lane(DefaultStageLaneID, release.CheckoutSlug(stageSlug), release.Label("Stage")).
		Lane(DefaultMainLaneID, release.CheckoutSlug(mainSlug), release.Label("Main"), release.Protected()).
		Flow(DefaultPromoteToStageFlowID, DefaultPromoteToStageWorkflowID,
			release.FlowLabel("Promote to stage"),
			release.FromFeature(),
			release.ToLane(DefaultStageLaneID),
			release.NoPush(),
		).
		Flow(DefaultReleaseToMainFlowID, DefaultReleaseToMainWorkflowID,
			release.FlowLabel("Release to main"),
			release.FromLane(DefaultStageLaneID),
			release.ToLane(DefaultMainLaneID),
			release.PushAfterVerifyPolicy(),
		).
		Build(workflows)
	if err != nil {
		return nil, nil, err
	}
	releases := release.NewRegistry(workflows)
	if err := releases.Register(def); err != nil {
		return nil, nil, err
	}
	return workflows, releases, nil
}

func VerifyReleaseDefinitions(reg *release.Registry, checkouts map[string]ConfiguredCheckout) error {
	if reg == nil {
		return nil
	}
	for _, def := range reg.Definitions() {
		for _, lane := range def.Lanes {
			slug := strings.TrimSpace(lane.CheckoutSlug)
			if slug == "" {
				return fmt.Errorf("release lane %q has empty checkout slug", lane.ID)
			}
			checkout, ok := checkouts[slug]
			if !ok || strings.TrimSpace(checkout.RootPath) == "" {
				return fmt.Errorf("release lane %q checkout slug %q is not configured", lane.ID, slug)
			}
		}
		for _, flow := range def.Flows {
			if _, ok := def.Lanes[flow.TargetLane]; !ok {
				return fmt.Errorf("release flow %q target lane %q is not configured", flow.ID, flow.TargetLane)
			}
			if flow.PushPolicy == release.PushAfterVerify && flow.TargetLane == "" {
				return fmt.Errorf("release flow %q push policy requires a target lane", flow.ID)
			}
		}
	}
	return nil
}

func DiscoveryWorkspaceResolver(cfg DiscoveryConfig) WorkspaceResolver {
	return WorkspaceResolverFunc(func(_ context.Context, slug string) (Workspace, bool, error) {
		items, err := Discover(cfg)
		if err != nil {
			return Workspace{}, false, err
		}
		for _, ws := range items {
			if ws.Slug == slug {
				return ws, true, nil
			}
		}
		return Workspace{}, false, nil
	})
}
