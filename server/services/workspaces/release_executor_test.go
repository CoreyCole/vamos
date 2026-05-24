package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

func TestDefaultReleaseStepExecutorPreflightBeforeMerge(t *testing.T) {
	ctx := context.Background()
	git := &recordingGit{heads: map[string]string{"/src": "source-head", "/stage": "stage-head"}, clean: map[string]bool{"/stage": true}}
	exec := DefaultReleaseStepExecutor{Git: git, Workspaces: resolver(map[string]Workspace{"feature": {Slug: "feature", CheckoutPath: "/src"}, "stage": {Slug: "stage", CheckoutPath: "/stage"}})}
	def, flow := executorDefFlow()
	item := ReleaseQueueItem{SourceSlug: "feature", ExpectedSourceCommit: "source-head", ExpectedTargetCommit: "stage-head"}

	if err := exec.ExecuteReleaseNode(ctx, item, def, flow, runtime.Node{Service: runtime.ServiceSpec{Type: "release.preflight"}}, nil); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if err := exec.ExecuteReleaseNode(ctx, item, def, flow, runtime.Node{Service: runtime.ServiceSpec{Type: "release.merge"}}, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	want := []string{"head:/src", "head:/stage", "clean:/stage", "fetch:/stage:/src:source-head", "head:/src", "fetch:/stage:/src:source-head", "merge:/stage:FETCH_HEAD"}
	if !reflect.DeepEqual(git.calls, want) {
		t.Fatalf("calls mismatch\nwant %#v\ngot  %#v", want, git.calls)
	}
}

func TestDefaultReleaseStepExecutorRejectsStaleExpectedCommit(t *testing.T) {
	git := &recordingGit{heads: map[string]string{"/src": "new-head", "/stage": "stage-head"}, clean: map[string]bool{"/stage": true}}
	exec := DefaultReleaseStepExecutor{Git: git, Workspaces: resolver(map[string]Workspace{"feature": {Slug: "feature", CheckoutPath: "/src"}, "stage": {Slug: "stage", CheckoutPath: "/stage"}})}
	def, flow := executorDefFlow()
	err := exec.ExecuteReleaseNode(context.Background(), ReleaseQueueItem{SourceSlug: "feature", ExpectedSourceCommit: "old-head", ExpectedTargetCommit: "stage-head"}, def, flow, runtime.Node{Service: runtime.ServiceSpec{Type: "release.preflight"}}, nil)
	if err == nil || err.Error() != "source commit changed since enqueue" {
		t.Fatalf("expected stale source error, got %v", err)
	}
}

func TestDefaultReleaseStepExecutorPushRequiresPolicy(t *testing.T) {
	git := &recordingGit{heads: map[string]string{"/src": "source-head", "/stage": "stage-head"}, clean: map[string]bool{"/stage": true}}
	exec := DefaultReleaseStepExecutor{Git: git, Workspaces: resolver(map[string]Workspace{"feature": {Slug: "feature", CheckoutPath: "/src"}, "stage": {Slug: "stage", CheckoutPath: "/stage"}})}
	def, flow := executorDefFlow()
	if err := exec.ExecuteReleaseNode(context.Background(), ReleaseQueueItem{SourceSlug: "feature"}, def, flow, runtime.Node{Service: runtime.ServiceSpec{Type: "release.push"}}, nil); err == nil {
		t.Fatal("expected push policy error")
	}
	flow.PushPolicy = release.PushAfterVerify
	if err := exec.ExecuteReleaseNode(context.Background(), ReleaseQueueItem{SourceSlug: "feature"}, def, flow, runtime.Node{Service: runtime.ServiceSpec{Type: "release.push"}}, nil); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got := git.calls[len(git.calls)-1]; got != "push:/stage:origin:HEAD" {
		t.Fatalf("last call = %q", got)
	}
}

func TestDefaultReleaseStepExecutorCommand(t *testing.T) {
	runner := &recordingCommandRunner{}
	exec := DefaultReleaseStepExecutor{Commands: runner}
	args, _ := json.Marshal(map[string]any{"dir": "/tmp/work", "argv": []string{"just", "build", "--no-restart"}})
	var lines []string
	err := exec.ExecuteReleaseNode(context.Background(), ReleaseQueueItem{}, release.Definition{}, release.FlowDefinition{}, runtime.Node{Service: runtime.ServiceSpec{Type: "release.command", Args: args}}, func(line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if runner.dir != "/tmp/work" || !reflect.DeepEqual(runner.argv, []string{"just", "build", "--no-restart"}) {
		t.Fatalf("runner got dir=%q argv=%v", runner.dir, runner.argv)
	}
	if len(lines) == 0 || lines[0] != "$ just build --no-restart" {
		t.Fatalf("missing command log: %#v", lines)
	}
}

func TestDefaultReleaseStepExecutorHostHook(t *testing.T) {
	host := &hostHookExecutor{}
	exec := DefaultReleaseStepExecutor{HostExecutors: map[string]ReleaseStepExecutor{"host.restart": host}}
	if err := exec.ExecuteReleaseNode(context.Background(), ReleaseQueueItem{}, release.Definition{}, release.FlowDefinition{}, runtime.Node{Service: runtime.ServiceSpec{Type: "host.restart"}}, nil); err != nil {
		t.Fatalf("host hook: %v", err)
	}
	if !host.called {
		t.Fatal("host hook was not called")
	}
}

func executorDefFlow() (release.Definition, release.FlowDefinition) {
	def := release.Definition{ID: "default", Version: "v1", Lanes: map[release.LaneID]release.LaneDefinition{"stage": {ID: "stage", CheckoutSlug: "stage"}}}
	flow := release.FlowDefinition{ID: "promote", TargetLane: "stage", PushPolicy: release.PushNever}
	return def, flow
}

func resolver(workspaces map[string]Workspace) WorkspaceResolverFunc {
	return func(_ context.Context, slug string) (Workspace, bool, error) {
		ws, ok := workspaces[slug]
		return ws, ok, nil
	}
}

type recordingGit struct {
	headErr error
	heads   map[string]string
	clean   map[string]bool
	calls   []string
}

func (g *recordingGit) Head(_ context.Context, checkout string) (string, error) {
	g.calls = append(g.calls, "head:"+checkout)
	return g.heads[checkout], g.headErr
}
func (g *recordingGit) IsClean(_ context.Context, checkout string) (bool, string, error) {
	g.calls = append(g.calls, "clean:"+checkout)
	return g.clean[checkout], "dirty", nil
}
func (g *recordingGit) IsAncestor(context.Context, string, string, string) (bool, error) {
	return true, nil
}
func (g *recordingGit) AheadBehind(context.Context, string, string, string) (int, int, error) {
	return 0, 1, nil
}
func (g *recordingGit) Fetch(_ context.Context, checkout, remote, ref string) error {
	g.calls = append(g.calls, "fetch:"+checkout+":"+remote+":"+ref)
	return nil
}
func (g *recordingGit) Merge(_ context.Context, checkout, ref string) error {
	g.calls = append(g.calls, "merge:"+checkout+":"+ref)
	return nil
}
func (g *recordingGit) FastForwardTo(_ context.Context, checkout, ref string) error {
	g.calls = append(g.calls, "ff:"+checkout+":"+ref)
	return nil
}
func (g *recordingGit) Push(_ context.Context, checkout, remote, ref string) error {
	g.calls = append(g.calls, "push:"+checkout+":"+remote+":"+ref)
	return nil
}

type recordingCommandRunner struct {
	dir  string
	argv []string
}

func (r *recordingCommandRunner) Run(_ context.Context, dir string, argv []string, _ func(string)) error {
	if len(argv) == 0 {
		return errors.New("empty")
	}
	r.dir = dir
	r.argv = append([]string(nil), argv...)
	return nil
}

type hostHookExecutor struct{ called bool }

func (h *hostHookExecutor) ExecuteReleaseNode(context.Context, ReleaseQueueItem, release.Definition, release.FlowDefinition, runtime.Node, func(string)) error {
	h.called = true
	return nil
}
