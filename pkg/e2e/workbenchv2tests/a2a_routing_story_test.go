package tests

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestAgentMemoryA2aRoutingContractStory(t *testing.T) {
	spec.Story(t, "agent-memory a2a routing contract").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(vamos.SeedA2ARoutingContract()).
		Do(vamos.OpenA2ARoutingPairwiseRoom()).
		Expect(vamos.ExpectA2APairwiseContainsRoutedBody()).
		Do(vamos.OpenA2ARoutingBotHome(vamos.A2ARoutingFromSlug)).
		Expect(vamos.ExpectA2AFromHomeChipNotGroupMail()).
		Do(vamos.OpenA2ARoutingBotHome(vamos.A2ARoutingToSlug)).
		Expect(vamos.ExpectA2AToHomeHasNoRoutedMail()).
		Do(vamos.OpenA2ARoutingPlanThread()).
		Expect(vamos.ExpectA2APlanInbound()).
		Expect(vamos.Console.Clean()).
		Run()
}
