package vamos

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
)

func TestStoreFixtureRoundTripsThroughRuntimeMemory(t *testing.T) {
	ctx := &duiruntime.Context{Memory: map[string]string{}}
	storeFixture(ctx, fixtures.State{
		Name: "freeform-chat.durable",
		Data: map[string]any{"thread_id": "thread-1", "workspace_id": "workspace-1"},
	})
	state, ok := fixtureFromMemory(ctx).(fixtures.State)
	if !ok {
		t.Fatalf("fixtureFromMemory() type %T, want fixtures.State", fixtureFromMemory(ctx))
	}
	if state.Name != "freeform-chat.durable" || state.Data["thread_id"] != "thread-1" || state.Data["workspace_id"] != "workspace-1" {
		t.Fatalf("fixtureFromMemory()=%#v", state)
	}
}

func TestTypedHelpersSatisfyDatastarUIInterfaces(t *testing.T) {
	var _ spec.Actor = Robot
	var _ spec.Fixture = WorkspaceFixture("thoughts-workbench.basic")
	var _ spec.Page = Thoughts.Page()
	var _ spec.Expectation = Thoughts.Ready()
	var _ spec.Expectation = Console.Clean()
}
