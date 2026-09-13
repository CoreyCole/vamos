package tests

import (
	"os"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

// Agent-memory VA fixture Stories (density / group_bubble / pairwise).
//
// Query contract (dual form):
//   - density:      ?density_fixture=1  OR ?fixture=density
//   - group_bubble: ?group_bubble_fixture=1 OR ?fixture=group
//   - pairwise:     ?pairwise_fixture=1 OR ?fixture=pairwise
//
// Density is live on tip 49da083+. Group/pairwise require the BE query gates
// that call RenderSharedThreadChatWith{GroupBubble,Pairwise}Fixture (same tip
// as these Stories, or a later FE tip that wires the same keys).
//
// Run against managed/local server (WorkbenchV2 fixture seeds wb2_alpha):
//
//	just e2e --config datastarui-e2e-workbench-v2.yml \
//	  --story agent-memory-chat-density-fixture
//
// Or against the feature tip host (auth required; optional thread override):
//
//	VAMOS_E2E_MACHINE_PROFILE=todo52-host \
//	VAMOS_E2E_AGENT_MEMORY_THREAD_ID=65f8ec8e-02c0-43bd-8cd4-fe3638178221 \
//	  just e2e --base-url https://2026-09-08-10-10-54-agent-memory-observable-context.workspaces.creative-mode.ai \
//	  --no-restart --story agent-memory-chat-density-fixture

func TestAgentMemoryChatDensityFixtureStory(t *testing.T) {
	spec.Story(t, "agent-memory chat density fixture").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(vamos.OpenAgentMemoryDensityFixture()).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(vamos.ExpectChatDensityFixtureSeeded()).
		Expect(spec.ExpectStep(spec.Visible(vamos.AgentMemory.ChromaHighlight()))).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestAgentMemoryGroupBubbleBotDMChipStory(t *testing.T) {
	skipUnlessAgentMemoryFixtureGates(t, "group_bubble")
	spec.Story(t, "agent-memory group bubble BotDMChip").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(vamos.OpenAgentMemoryGroupBubbleFixture()).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(vamos.ExpectGroupBubbleFixtureSeeded()).
		Expect(vamos.ExpectBotDMChipPairwiseLinks()).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestAgentMemoryPairwiseFixtureViewOnlyStory(t *testing.T) {
	skipUnlessAgentMemoryFixtureGates(t, "pairwise")
	spec.Story(t, "agent-memory pairwise fixture view-only").
		App(vamos.App()).
		Viewport(duiruntime.ViewportDesktopFull).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.WorkbenchV2Fixture)).
		Do(vamos.OpenAgentMemoryPairwiseFixture()).
		Expect(vamos.WorkbenchV2.Ready()).
		Expect(vamos.ExpectPairwiseFixtureViewOnly()).
		Expect(vamos.Console.Clean()).
		Run()
}

// skipUnlessAgentMemoryFixtureGates lets CI list/compile Stories while a tip
// host is still on density-only 49da083. Local managed runs (default) always
// execute — this checkout wires group_bubble_fixture / pairwise_fixture.
func skipUnlessAgentMemoryFixtureGates(t *testing.T, kind string) {
	t.Helper()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("VAMOS_E2E_REQUIRE_AGENT_MEMORY_FIXTURES")), "1") {
		return
	}
	base := strings.TrimSpace(os.Getenv("VAMOS_E2E_BASE_URL"))
	if base == "" {
		// DatastarUI may only expose base via --base-url into runtime config;
		// when unset we assume managed/local checkout under test.
		return
	}
	if strings.Contains(base, "agent-memory-observable-context") &&
		strings.TrimSpace(os.Getenv("VAMOS_E2E_AGENT_MEMORY_ALLOW_PENDING_GATES")) == "1" {
		t.Skipf("pending FE/BE tip for %s fixture gates on tip host; unset VAMOS_E2E_AGENT_MEMORY_ALLOW_PENDING_GATES after tip", kind)
	}
}
