package workspaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

type ReleaseStepExecutor interface {
	ExecuteReleaseNode(
		ctx context.Context,
		item ReleaseQueueItem,
		def release.Definition,
		flow release.FlowDefinition,
		node runtime.Node,
		onLine func(string),
	) error
}

type ReleaseActivities struct {
	Store            ReleaseQueueStore
	ReleaseRegistry  *release.Registry
	WorkflowRegistry *runtime.Registry
	Executor         ReleaseStepExecutor
	Notifier         WorkspaceLifecycleNotifier
}

func (a *ReleaseActivities) ProcessNextReleaseQueueItem(ctx context.Context) (bool, error) {
	if a == nil || a.Store == nil {
		return false, fmt.Errorf("release queue store is not configured")
	}
	item, ok, err := a.Store.ClaimNextPendingReleaseQueueItem(ctx)
	if err != nil || !ok {
		return ok, err
	}
	if a.Notifier != nil {
		a.Notifier.Notify("release-queue")
	}
	if err := a.processClaimedItem(ctx, item); err != nil {
		return true, err
	}
	if a.Notifier != nil {
		a.Notifier.Notify("release-queue")
	}
	return true, nil
}

func (a *ReleaseActivities) processClaimedItem(ctx context.Context, item ReleaseQueueItem) error {
	def, flow, workflowDef, err := a.resolve(item)
	if err != nil {
		return a.failTerminal(ctx, item.ID, err)
	}
	nodes, err := orderedServiceNodes(workflowDef)
	if err != nil {
		return a.failTerminal(ctx, item.ID, err)
	}
	for _, node := range nodes {
		if err := a.Store.MarkReleaseQueueItemRunning(ctx, item.ID, node.ID); err != nil {
			return err
		}
		if err := a.Store.AppendReleaseQueueEvent(ctx, AppendReleaseQueueEventParams{ItemID: item.ID, Level: "info", NodeID: node.ID, Message: "started " + node.Service.Type}); err != nil {
			return err
		}
		if a.Notifier != nil {
			a.Notifier.Notify("release-queue")
		}
		if a.Executor == nil {
			return a.failTerminal(ctx, item.ID, fmt.Errorf("release executor is not configured"))
		}
		onLine := func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			_ = a.Store.AppendReleaseQueueEvent(ctx, AppendReleaseQueueEventParams{ItemID: item.ID, Level: "info", NodeID: node.ID, Message: line})
		}
		if err := a.Executor.ExecuteReleaseNode(ctx, item, def, flow, node, onLine); err != nil {
			_ = a.Store.AppendReleaseQueueEvent(ctx, AppendReleaseQueueEventParams{ItemID: item.ID, Level: "error", NodeID: node.ID, Message: err.Error()})
			return a.failTerminal(ctx, item.ID, err)
		}
		if err := a.Store.AppendReleaseQueueEvent(ctx, AppendReleaseQueueEventParams{ItemID: item.ID, Level: "info", NodeID: node.ID, Message: "completed " + node.Service.Type}); err != nil {
			return err
		}
	}
	return a.Store.MarkReleaseQueueItemTerminal(ctx, item.ID, ReleaseQueueStatusSucceeded, "")
}

func (a *ReleaseActivities) resolve(item ReleaseQueueItem) (release.Definition, release.FlowDefinition, runtime.Definition, error) {
	if a.ReleaseRegistry == nil {
		return release.Definition{}, release.FlowDefinition{}, runtime.Definition{}, fmt.Errorf("release registry is not configured")
	}
	if a.WorkflowRegistry == nil {
		return release.Definition{}, release.FlowDefinition{}, runtime.Definition{}, fmt.Errorf("workflow registry is not configured")
	}
	def, ok := a.ReleaseRegistry.Definition(item.DefinitionID, item.DefinitionVersion)
	if !ok {
		return release.Definition{}, release.FlowDefinition{}, runtime.Definition{}, fmt.Errorf("release definition %q version %q not found", item.DefinitionID, item.DefinitionVersion)
	}
	flow, ok := def.Flows[item.FlowID]
	if !ok {
		return release.Definition{}, release.FlowDefinition{}, runtime.Definition{}, fmt.Errorf("release flow %q not found", item.FlowID)
	}
	workflowID := item.WorkflowID
	if workflowID == "" {
		workflowID = flow.WorkflowID
	}
	workflowVersion := item.WorkflowVersion
	if workflowVersion == "" {
		workflowVersion = flow.WorkflowVersion
	}
	workflowDef, ok := a.WorkflowRegistry.GetVersion(workflowID, workflowVersion)
	if !ok {
		return release.Definition{}, release.FlowDefinition{}, runtime.Definition{}, fmt.Errorf("workflow %q version %q not found", workflowID, workflowVersion)
	}
	return def, flow, workflowDef, nil
}

func (a *ReleaseActivities) failTerminal(ctx context.Context, id string, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	if err := a.Store.MarkReleaseQueueItemTerminal(ctx, id, ReleaseQueueStatusFailed, msg); err != nil {
		return err
	}
	if a.Notifier != nil {
		a.Notifier.Notify("release-queue")
	}
	return nil
}

func orderedServiceNodes(def runtime.Definition) ([]runtime.Node, error) {
	if _, ok := def.Nodes[def.Start]; !ok {
		return nil, fmt.Errorf("workflow start node %q is not defined", def.Start)
	}
	visited := map[runtime.NodeID]bool{}
	var out []runtime.Node
	var walk func(runtime.NodeID) error
	walk = func(id runtime.NodeID) error {
		if visited[id] {
			return nil
		}
		visited[id] = true
		node := def.Nodes[id]
		if node.Kind == runtime.NodeKindService {
			out = append(out, node)
		}
		next := sortedOutgoingEdges(def.Edges, id)
		for _, edge := range next {
			if _, ok := def.Nodes[edge.To]; !ok {
				return fmt.Errorf("edge to %q is not defined", edge.To)
			}
			if err := walk(edge.To); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(def.Start); err != nil {
		return nil, err
	}
	return out, nil
}

func sortedOutgoingEdges(edges []runtime.Edge, from runtime.NodeID) []runtime.Edge {
	out := make([]runtime.Edge, 0)
	for _, edge := range edges {
		if edge.From == from {
			out = append(out, edge)
		}
	}
	// Preserve definition order; current release workflows are linear, and builder edge order
	// is the authored graph order.
	return out
}
