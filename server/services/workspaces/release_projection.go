package workspaces

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/release"
)

type ReleaseProjector struct {
	Registry *release.Registry
	Store    ReleaseQueueStore
	Git      GitInspector
	Now      func() time.Time
}

type ReleasePreflightCheck struct {
	ID      release.CheckID
	OK      bool
	Message string
}

type ReleasePreflightResult struct {
	OK                   bool
	DisabledReason       string
	SourceCommit         string
	TargetCommit         string
	SourceClean          bool
	TargetClean          bool
	SourceAheadOfTarget  bool
	TargetAncestorOfHead bool
	Checks               []ReleasePreflightCheck
}

func (p *ReleaseProjector) BuildPanel(ctx context.Context, views []ImplWorkspaceView) (ReleasePanelModel, error) {
	if p == nil || p.Registry == nil {
		return ReleasePanelModel{Enabled: false}, nil
	}
	workspaces := flattenImplWorkspaceViews(views)
	defs := p.Registry.Definitions()
	if len(defs) == 0 {
		return ReleasePanelModel{Enabled: false}, nil
	}

	active, history, err := p.releaseQueue(ctx)
	if err != nil {
		return ReleasePanelModel{}, err
	}
	activeByStatus := splitReleaseQueue(active)

	panel := ReleasePanelModel{Enabled: true, Queue: activeByStatus, History: history}
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		lanes := matchReleaseLanes(def, workspaces)
		for _, lane := range sortedLaneDefinitions(def) {
			ws := lanes[lane.ID]
			actions := p.evaluateActions(ctx, def, lanes, ws, active)
			panel.Lanes = append(panel.Lanes, ReleaseLaneView{ID: lane.ID, Label: lane.Label, Workspace: ws, Actions: actions})
		}
		for _, ws := range workspaces {
			if workspaceIsReleaseLane(lanes, ws) {
				continue
			}
			actions := p.evaluateActions(ctx, def, lanes, ws, active)
			for _, action := range actions {
				if action.SourceSlug != "" {
					panel.FeatureActions = append(panel.FeatureActions, action)
				}
			}
		}
	}
	return panel, nil
}

func (p *ReleaseProjector) releaseQueue(ctx context.Context) ([]ReleaseQueueItem, []ReleaseQueueItem, error) {
	if p.Store == nil {
		return nil, nil, nil
	}
	active, err := p.Store.ListActiveReleaseQueueItems(ctx)
	if err != nil {
		return nil, nil, err
	}
	history, err := p.Store.ListRecentReleaseQueueItems(ctx, 10)
	if err != nil {
		return nil, nil, err
	}
	return active, history, nil
}

func (p *ReleaseProjector) evaluateActions(ctx context.Context, def release.Definition, lanes map[release.LaneID]Workspace, ws Workspace, active []ReleaseQueueItem) []ReleaseActionView {
	actions := EvaluateReleaseActions(p.Registry, lanes, ws)
	for i := range actions {
		if hasActiveReleaseQueueItem(active) {
			actions[i].Disabled = true
			actions[i].DisabledReason = "release queue already has active work"
			continue
		}
		if p.Git == nil || actions[i].Disabled {
			continue
		}
		flow, ok := def.Flows[actions[i].FlowID]
		if !ok {
			continue
		}
		result := InspectReleasePreconditions(ctx, p.Git, def, flow, ws, lanes[flow.TargetLane])
		if !result.OK {
			actions[i].Disabled = true
			actions[i].DisabledReason = result.DisabledReason
		}
		if result.SourceCommit != "" {
			actions[i].ExpectedSourceCommit = result.SourceCommit
		}
		if result.TargetCommit != "" {
			actions[i].ExpectedTargetCommit = result.TargetCommit
		}
	}
	return actions
}

func EvaluateReleaseActions(reg *release.Registry, lanes map[release.LaneID]Workspace, ws Workspace) []ReleaseActionView {
	if reg == nil || strings.TrimSpace(ws.Slug) == "" {
		return nil
	}
	var actions []ReleaseActionView
	for _, def := range reg.Definitions() {
		if !def.Enabled {
			continue
		}
		for _, flow := range sortedFlowDefinitions(def) {
			if !flowAppliesToWorkspace(flow, lanes, ws) {
				continue
			}
			target := lanes[flow.TargetLane]
			action := ReleaseActionView{DefinitionID: def.ID, DefinitionVersion: def.Version, FlowID: flow.ID, Label: firstNonEmpty(flow.Label, string(flow.ID)), SourceSlug: ws.Slug, TargetLane: flow.TargetLane, ExpectedSourceCommit: ws.Commit, ExpectedTargetCommit: target.Commit}
			if strings.TrimSpace(target.Slug) == "" {
				action.Disabled = true
				action.DisabledReason = fmt.Sprintf("target lane %q checkout is missing", flow.TargetLane)
			} else if strings.TrimSpace(ws.Commit) == "" {
				action.Disabled = true
				action.DisabledReason = "source commit is unknown"
			} else if strings.TrimSpace(target.Commit) == "" {
				action.Disabled = true
				action.DisabledReason = "target commit is unknown"
			}
			actions = append(actions, action)
		}
	}
	return actions
}

func InspectReleasePreconditions(ctx context.Context, inspector GitInspector, _ release.Definition, flow release.FlowDefinition, source Workspace, target Workspace) ReleasePreflightResult {
	result := ReleasePreflightResult{OK: true}
	fail := func(id release.CheckID, msg string) {
		result.OK = false
		if result.DisabledReason == "" {
			result.DisabledReason = msg
		}
		result.Checks = append(result.Checks, ReleasePreflightCheck{ID: id, OK: false, Message: msg})
	}
	pass := func(id release.CheckID, msg string) {
		result.Checks = append(result.Checks, ReleasePreflightCheck{ID: id, OK: true, Message: msg})
	}

	if strings.TrimSpace(source.CheckoutPath) == "" {
		fail("source_exists", "source checkout is missing")
		return result
	}
	pass("source_exists", "source checkout exists")
	if strings.TrimSpace(target.CheckoutPath) == "" {
		fail("target_exists", fmt.Sprintf("target lane %q checkout is missing", flow.TargetLane))
		return result
	}
	pass("target_exists", "target checkout exists")
	if inspector == nil {
		return result
	}

	sourceHead, err := inspector.Head(ctx, source.CheckoutPath)
	if err != nil {
		fail("source_head", "source head unavailable: "+err.Error())
	} else {
		result.SourceCommit = sourceHead
		pass("source_head", "source head available")
	}
	targetHead, err := inspector.Head(ctx, target.CheckoutPath)
	if err != nil {
		fail("target_head", "target head unavailable: "+err.Error())
	} else {
		result.TargetCommit = targetHead
		pass("target_head", "target head available")
	}
	clean, detail, err := inspector.IsClean(ctx, target.CheckoutPath)
	if err != nil {
		fail("target_clean", "target cleanliness unavailable: "+err.Error())
	} else if !clean {
		fail("target_clean", "target checkout is dirty: "+strings.TrimSpace(detail))
	} else {
		result.TargetClean = true
		pass("target_clean", "target checkout is clean")
	}
	if strings.TrimSpace(source.Commit) != "" && result.SourceCommit != "" && source.Commit != result.SourceCommit {
		fail("expected_source_commit", "source commit changed since projection")
	} else {
		pass("expected_source_commit", "source commit matches projection")
	}
	if strings.TrimSpace(target.Commit) != "" && result.TargetCommit != "" && target.Commit != result.TargetCommit {
		fail("expected_target_commit", "target commit changed since projection")
	} else {
		pass("expected_target_commit", "target commit matches projection")
	}
	if result.SourceCommit != "" && result.TargetCommit != "" {
		ahead, behind, err := inspector.AheadBehind(ctx, target.CheckoutPath, result.TargetCommit, result.SourceCommit)
		if err != nil {
			fail("ahead_behind", "ahead/behind unavailable: "+err.Error())
		} else if behind == 0 {
			fail("source_ahead", "source is not ahead of target")
		} else {
			result.SourceAheadOfTarget = true
			pass("source_ahead", fmt.Sprintf("source ahead of target by %d commit(s)", behind))
			_ = ahead
		}
	}
	return result
}

func flattenImplWorkspaceViews(views []ImplWorkspaceView) []Workspace {
	out := make([]Workspace, 0, len(views))
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			out = append(out, view.Runtime.Workspace)
			walk(view.Children)
		}
	}
	walk(views)
	return out
}

func matchReleaseLanes(def release.Definition, workspaces []Workspace) map[release.LaneID]Workspace {
	lanes := make(map[release.LaneID]Workspace, len(def.Lanes))
	for _, lane := range def.Lanes {
		for _, ws := range workspaces {
			if workspaceMatchesLane(ws, lane) {
				lanes[lane.ID] = ws
				break
			}
		}
	}
	return lanes
}

func workspaceMatchesLane(ws Workspace, lane release.LaneDefinition) bool {
	return strings.TrimSpace(ws.Slug) == strings.TrimSpace(lane.CheckoutSlug)
}

func workspaceIsReleaseLane(lanes map[release.LaneID]Workspace, ws Workspace) bool {
	for _, laneWS := range lanes {
		if laneWS.Slug != "" && laneWS.Slug == ws.Slug {
			return true
		}
	}
	return false
}

func flowAppliesToWorkspace(flow release.FlowDefinition, lanes map[release.LaneID]Workspace, ws Workspace) bool {
	switch flow.Source.Kind {
	case release.SourceLane:
		laneWS := lanes[flow.Source.Lane]
		return laneWS.Slug != "" && laneWS.Slug == ws.Slug
	case release.SourceFeature, "":
		for _, laneWS := range lanes {
			if laneWS.Slug != "" && laneWS.Slug == ws.Slug {
				return false
			}
		}
		return !ws.IsMain
	default:
		return false
	}
}

func splitReleaseQueue(items []ReleaseQueueItem) ReleaseQueueView {
	view := ReleaseQueueView{}
	for _, item := range items {
		switch item.Status {
		case ReleaseQueueStatusPending:
			view.Pending = append(view.Pending, item)
		default:
			view.Active = append(view.Active, item)
		}
	}
	return view
}

func hasActiveReleaseQueueItem(items []ReleaseQueueItem) bool {
	for _, item := range items {
		if item.Status == ReleaseQueueStatusPending || item.Status == ReleaseQueueStatusRunning {
			return true
		}
	}
	return false
}

func sortedLaneDefinitions(def release.Definition) []release.LaneDefinition {
	lanes := make([]release.LaneDefinition, 0, len(def.Lanes))
	for _, lane := range def.Lanes {
		lanes = append(lanes, lane)
	}
	sort.Slice(lanes, func(i, j int) bool { return lanes[i].ID < lanes[j].ID })
	return lanes
}

func sortedFlowDefinitions(def release.Definition) []release.FlowDefinition {
	flows := make([]release.FlowDefinition, 0, len(def.Flows))
	for _, flow := range def.Flows {
		flows = append(flows, flow)
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	return flows
}
