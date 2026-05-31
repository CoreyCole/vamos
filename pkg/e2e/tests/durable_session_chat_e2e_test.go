package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

const planDocsReviewPath = "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/context/implement/e2e-pi-plan-docs-review.md"
const planWorkspacePath = "thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go"

func TestDurableSessionChat_QrspiPlanWorkspaceChatUpdatesVerificationArtifactThroughPiAndTemporal(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("qrspi plan workspace chat updates verification artifact through pi and temporal").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.OpenPlanWorkspace(planWorkspacePath),
			vamos.OpenWorkspaceChat("current"),
			vamos.RememberFileHash(planDocsReviewPath),
		).
		When(
			vamos.SendPiDocsReviewPrompt("VAMOS_E2E_PLAN_DOCS_REVIEW_OK", planDocsReviewPath),
			vamos.WaitForChatMarker("VAMOS_E2E_PLAN_DOCS_REVIEW_OK"),
		).
		Then(
			vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK"),
			vamos.ExpectFileHashChanged(planDocsReviewPath),
			vamos.ExpectPiReviewFileSections(planDocsReviewPath),
			vamos.ExpectOnlyFileChanged(planDocsReviewPath),
			vamos.ReloadChat(),
			vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK"),
			vamos.ReopenCurrentChat(),
			vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK"),
		).
		Run(t)
}

func TestDurableSessionChat_FreeformChatFixtureReplaysDurableTranscript(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("freeform chat fixture replays durable transcript").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture(fixtures.DurableFreeformFixture).Build(),
			vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture),
		).
		Then(
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
			vamos.ReloadChat(),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
			vamos.ReopenCurrentChat(),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
		).
		Run(t)
}

func TestDurableSessionChat_FreeformChatStartedFromThoughtsRootSurvivesRefreshAndResume(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("freeform chat started from thoughts root survives refresh and resume").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.OpenThoughtsRootChat("current"),
		).
		When(
			vamos.SendFreeformChatPrompt("VAMOS_E2E_FREEFORM_REFRESH_FIRST"),
			vamos.WaitForLatestFreeformChatRunCompletion("current"),
		).
		Then(
			vamos.ReloadChat(),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REFRESH_FIRST"),
			vamos.SendFreeformChatPrompt("VAMOS_E2E_FREEFORM_REFRESH_SECOND"),
			vamos.WaitForLatestFreeformChatRunCompletion("current"),
			vamos.ReloadChat(),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REFRESH_SECOND"),
		).
		Run(t)
}

func TestDurableSessionChat_FreeformChatAdoptsQrspiProjectMetadata(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("freeform chat adopts qrspi project metadata").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.SeedProjectPlanWorkspaces("example.com/alpha/app", "example.com/beta/app"),
			vamos.OpenThoughtsRootChat("current"),
			vamos.SeedLatestFreeformChatQRSPIProjectResult("example.com/alpha/app"),
		).
		Then(
			vamos.ExpectThreadMetadataProject("example.com/alpha/app"),
			vamos.ReloadChat(),
			vamos.ExpectThreadMetadataProject("example.com/alpha/app"),
		).
		Run(t)
}

func TestDurableSessionChat_AnchorDocumentNavigationPreservesEmbeddedChat(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("anchor document navigation preserves embedded chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture(fixtures.DurableFreeformFixture).Build(),
			vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture),
		).
		When(vamos.FollowFirstSidebarDocumentLink()).
		Then(
			spec.URLContains("context=chat"),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
		).
		Run(t)
}

func TestDurableSessionChat_WorkspaceDocumentWithoutChatParamsRestoresWorkspaceChat(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("workspace document without chat params restores workspace chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.SeedLatestWorkspaceChats("VAMOS_E2E_WORKSPACE_DOC_RESTORE", "VAMOS_E2E_WORKSPACE_UNUSED"),
			vamos.OpenWorkspaceDocumentWithoutChatParams("A"),
		).
		Then(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_DOC_RESTORE")).
		Run(t)
}

func TestDurableSessionChat_RootThoughtsRestoresLatestFreeformChat(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("root thoughts restores latest freeform chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture(fixtures.DurableFreeformFixture).Build(),
			vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture),
			vamos.OpenThoughtsRootChatContext("current"),
		).
		Then(
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
			vamos.ExpectConsoleClean(),
		).
		Run(t)
}

func TestDurableSessionChat_AgentChatReloadScrollsTranscriptToBottom(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("agent chat reload scrolls transcript to bottom").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.WorkspaceFixture(fixtures.DurableFreeformFixture).Build(),
			vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture),
		).
		Then(
			vamos.ReloadChat(),
			vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK"),
			spec.Visible(vamos.TranscriptBottom()),
		).
		Run(t)
}

func TestDurableSessionChat_WorkspaceSwitchingRestoresEachWorkspaceLatestChat(t *testing.T) {
	spec.Feature("Durable session chat").
		Scenario("workspace switching restores each workspace latest chat").
		Given(
			vamos.AuthenticatedAs("playwright@localhost"),
			vamos.SeedLatestWorkspaceChats("VAMOS_E2E_WORKSPACE_A_LATEST", "VAMOS_E2E_WORKSPACE_B_LATEST"),
		).
		Then(
			vamos.OpenSeededWorkspaceChat("A"),
			vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_A_LATEST"),
			vamos.OpenSeededWorkspaceChat("B"),
			vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_B_LATEST"),
			vamos.OpenSeededWorkspaceChat("A"),
			vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_A_LATEST"),
			vamos.OpenSeededWorkspaceChat("B"),
			vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_B_LATEST"),
		).
		Run(t)
}
