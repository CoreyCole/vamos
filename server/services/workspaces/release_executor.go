package workspaces

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

type WorkspaceResolver interface {
	ResolveWorkspace(ctx context.Context, slug string) (Workspace, bool, error)
}

type WorkspaceResolverFunc func(ctx context.Context, slug string) (Workspace, bool, error)

func (f WorkspaceResolverFunc) ResolveWorkspace(ctx context.Context, slug string) (Workspace, bool, error) {
	return f(ctx, slug)
}

type DefaultReleaseStepExecutor struct {
	Git           GitMutator
	Commands      CommandRunner
	Workspaces    WorkspaceResolver
	HostExecutors map[string]ReleaseStepExecutor
}

type releaseCommandSpec struct {
	Checkout string   `json:"checkout,omitempty"`
	Dir      string   `json:"dir,omitempty"`
	Argv     []string `json:"argv,omitempty"`
	Args     []string `json:"args,omitempty"`
}

func (e DefaultReleaseStepExecutor) ExecuteReleaseNode(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition, node runtime.Node, onLine func(string)) error {
	if node.Service.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, node.Service.Timeout)
		defer cancel()
	}
	switch strings.TrimSpace(node.Service.Type) {
	case "release.preflight":
		return e.preflight(ctx, item, def, flow, onLine)
	case "release.merge":
		return e.merge(ctx, item, def, flow, onLine)
	case "release.command":
		return e.command(ctx, node, onLine)
	case "release.push":
		return e.push(ctx, item, def, flow, onLine)
	default:
		if e.HostExecutors != nil {
			if host, ok := e.HostExecutors[node.Service.Type]; ok && host != nil {
				return host.ExecuteReleaseNode(ctx, item, def, flow, node, onLine)
			}
		}
		return fmt.Errorf("release service %q has no executor", node.Service.Type)
	}
}

func (e DefaultReleaseStepExecutor) preflight(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition, onLine func(string)) error {
	source, target, err := e.resolveSourceTarget(ctx, item, def, flow)
	if err != nil {
		return err
	}
	if e.Git == nil {
		return fmt.Errorf("release git mutator is not configured")
	}
	sourceHead, err := e.Git.Head(ctx, source.CheckoutPath)
	if err != nil {
		return fmt.Errorf("source head unavailable: %w", err)
	}
	targetHead, err := e.Git.Head(ctx, target.CheckoutPath)
	if err != nil {
		return fmt.Errorf("target head unavailable: %w", err)
	}
	if item.ExpectedSourceCommit != "" && item.ExpectedSourceCommit != sourceHead {
		return fmt.Errorf("source commit changed since enqueue")
	}
	if item.ExpectedTargetCommit != "" && item.ExpectedTargetCommit != targetHead {
		return fmt.Errorf("target commit changed since enqueue")
	}
	clean, detail, err := e.Git.IsClean(ctx, target.CheckoutPath)
	if err != nil {
		return fmt.Errorf("target cleanliness unavailable: %w", err)
	}
	if !clean {
		return fmt.Errorf("target checkout is dirty: %s", strings.TrimSpace(detail))
	}
	if err := e.Git.Fetch(ctx, target.CheckoutPath, source.CheckoutPath, sourceHead); err != nil {
		return fmt.Errorf("fetch source into target: %w", err)
	}
	line(onLine, "preflight passed")
	return nil
}

func (e DefaultReleaseStepExecutor) merge(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition, onLine func(string)) error {
	source, target, err := e.resolveSourceTarget(ctx, item, def, flow)
	if err != nil {
		return err
	}
	if e.Git == nil {
		return fmt.Errorf("release git mutator is not configured")
	}
	sourceHead, err := e.Git.Head(ctx, source.CheckoutPath)
	if err != nil {
		return fmt.Errorf("source head unavailable: %w", err)
	}
	if err := e.Git.Fetch(ctx, target.CheckoutPath, source.CheckoutPath, sourceHead); err != nil {
		return fmt.Errorf("fetch source into target: %w", err)
	}
	if err := e.Git.Merge(ctx, target.CheckoutPath, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("merge source into target: %w", err)
	}
	line(onLine, "merged "+source.Slug+" into "+string(flow.TargetLane))
	return nil
}

func (e DefaultReleaseStepExecutor) command(ctx context.Context, node runtime.Node, onLine func(string)) error {
	if e.Commands == nil {
		return fmt.Errorf("release command runner is not configured")
	}
	var spec releaseCommandSpec
	if len(node.Service.Args) > 0 {
		if err := json.Unmarshal(node.Service.Args, &spec); err != nil {
			return fmt.Errorf("decode release command args: %w", err)
		}
	}
	argv := spec.Argv
	if len(argv) == 0 {
		argv = spec.Args
	}
	dir := strings.TrimSpace(spec.Dir)
	if dir == "" {
		dir = strings.TrimSpace(spec.Checkout)
	}
	if dir == "" {
		dir = "."
	}
	line(onLine, "$ "+strings.Join(argv, " "))
	return e.Commands.Run(ctx, dir, argv, onLine)
}

func (e DefaultReleaseStepExecutor) push(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition, onLine func(string)) error {
	if flow.PushPolicy != release.PushAfterVerify {
		return fmt.Errorf("release flow %q does not allow push", flow.ID)
	}
	_, target, err := e.resolveSourceTarget(ctx, item, def, flow)
	if err != nil {
		return err
	}
	if e.Git == nil {
		return fmt.Errorf("release git mutator is not configured")
	}
	remote := "origin"
	ref := "HEAD"
	if err := e.Git.Push(ctx, target.CheckoutPath, remote, ref); err != nil {
		return fmt.Errorf("push target lane %q: %w", flow.TargetLane, err)
	}
	line(onLine, "pushed "+string(flow.TargetLane))
	return nil
}

func (e DefaultReleaseStepExecutor) resolveSourceTarget(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition) (Workspace, Workspace, error) {
	if e.Workspaces == nil {
		return Workspace{}, Workspace{}, fmt.Errorf("release workspace resolver is not configured")
	}
	sourceSlug := item.SourceSlug
	if sourceSlug == "" && flow.Source.Kind == release.SourceLane {
		sourceSlug = def.Lanes[flow.Source.Lane].CheckoutSlug
	}
	targetLane, ok := def.Lanes[flow.TargetLane]
	if !ok || strings.TrimSpace(targetLane.CheckoutSlug) == "" {
		return Workspace{}, Workspace{}, fmt.Errorf("target lane %q checkout is not configured", flow.TargetLane)
	}
	source, ok, err := e.Workspaces.ResolveWorkspace(ctx, sourceSlug)
	if err != nil {
		return Workspace{}, Workspace{}, err
	}
	if !ok || strings.TrimSpace(source.CheckoutPath) == "" {
		return Workspace{}, Workspace{}, fmt.Errorf("source workspace %q is missing", sourceSlug)
	}
	target, ok, err := e.Workspaces.ResolveWorkspace(ctx, targetLane.CheckoutSlug)
	if err != nil {
		return Workspace{}, Workspace{}, err
	}
	if !ok || strings.TrimSpace(target.CheckoutPath) == "" {
		return Workspace{}, Workspace{}, fmt.Errorf("target workspace %q is missing", targetLane.CheckoutSlug)
	}
	return source, target, nil
}

func line(onLine func(string), msg string) {
	if onLine != nil {
		onLine(msg)
	}
}

var _ ReleaseStepExecutor = DefaultReleaseStepExecutor{}
var _ GitMutator = ShellGitInspector{}
var _ CommandRunner = ExecCommandRunner{}
