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
	spec.Story(t, "durable session chat qrspi plan workspace chat updates verification artifact through pi and temporal").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.OpenPlanWorkspace(planWorkspacePath)).
		Do(vamos.OpenWorkspaceChat("current")).
		Do(vamos.RememberFileHash(planDocsReviewPath)).
		Do(vamos.SendPiDocsReviewPrompt("VAMOS_E2E_PLAN_DOCS_REVIEW_OK", planDocsReviewPath)).
		Expect(vamos.WaitForChatMarker("VAMOS_E2E_PLAN_DOCS_REVIEW_OK")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK")).
		Expect(vamos.ExpectFileHashChanged(planDocsReviewPath)).
		Expect(vamos.ExpectPiReviewFileSections(planDocsReviewPath)).
		Expect(vamos.ExpectOnlyFileChanged(planDocsReviewPath)).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK")).
		Do(vamos.ReopenCurrentChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_PLAN_DOCS_REVIEW_OK")).
		Run()
}

func TestDurableSessionChat_FreeformChatFixtureReplaysDurableTranscript(t *testing.T) {
	spec.Story(t, "durable session chat freeform chat fixture replays durable transcript").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture)).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Do(vamos.ReopenCurrentChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Run()
}

func TestDurableSessionChat_FreeformChatStartedFromThoughtsRootSurvivesRefreshAndResume(t *testing.T) {
	spec.Story(t, "durable session chat freeform chat started from thoughts root survives refresh and resume").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.OpenThoughtsRootChat("current")).
		Do(vamos.SendFreeformChatPrompt("VAMOS_E2E_FREEFORM_REFRESH_FIRST")).
		Do(vamos.WaitForLatestFreeformChatRunCompletion("current")).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REFRESH_FIRST")).
		Do(vamos.SendFreeformChatPrompt("VAMOS_E2E_FREEFORM_REFRESH_SECOND")).
		Do(vamos.WaitForLatestFreeformChatRunCompletion("current")).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REFRESH_SECOND")).
		Run()
}

func TestDurableSessionChat_FreeformChatAdoptsQrspiProjectMetadata(t *testing.T) {
	spec.Story(t, "durable session chat freeform chat adopts qrspi project metadata").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedProjectPlanWorkspaces("example.com/alpha/app", "example.com/beta/app")).
		Do(vamos.OpenThoughtsRootChat("current")).
		Do(vamos.SeedLatestFreeformChatQRSPIProjectResult("example.com/alpha/app")).
		Expect(vamos.ExpectThreadMetadataProject("example.com/alpha/app")).
		Do(vamos.ReloadChat()).
		Expect(vamos.ExpectThreadMetadataProject("example.com/alpha/app")).
		Run()
}

func TestDurableSessionChat_AnchorDocumentNavigationPreservesEmbeddedChat(t *testing.T) {
	spec.Story(t, "durable session chat anchor document navigation preserves embedded chat").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.FollowFirstSidebarDocumentLink()).
		Expect(spec.ExpectStep(spec.URLContains("context=chat"))).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Run()
}

func TestDurableSessionChat_WorkspaceDocumentWithoutChatParamsRestoresWorkspaceChat(t *testing.T) {
	spec.Story(t, "durable session chat workspace document without chat params restores workspace chat").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedLatestWorkspaceChats("VAMOS_E2E_WORKSPACE_DOC_RESTORE", "VAMOS_E2E_WORKSPACE_UNUSED")).
		Do(vamos.OpenWorkspaceDocumentWithoutChatParams("A")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_DOC_RESTORE")).
		Run()
}

func TestDurableSessionChat_RootThoughtsRestoresLatestFreeformChat(t *testing.T) {
	spec.Story(t, "durable session chat root thoughts restores latest freeform chat").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.OpenThoughtsRootChatContext("current")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestDurableSessionChat_AgentChatReloadScrollsTranscriptToBottom(t *testing.T) {
	spec.Story(t, "durable session chat agent chat reload scrolls transcript to bottom").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.OpenFreeformChatFixture(fixtures.DurableFreeformFixture)).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_FREEFORM_REPLAY_OK")).
		Expect(spec.ExpectStep(spec.Visible(vamos.TranscriptBottom()))).
		Run()
}

func TestDurableSessionChat_WorkspaceSwitchingRestoresEachWorkspaceLatestChat(t *testing.T) {
	spec.Story(t, "durable session chat workspace switching restores each workspace latest chat").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedLatestWorkspaceChats("VAMOS_E2E_WORKSPACE_A_LATEST", "VAMOS_E2E_WORKSPACE_B_LATEST")).
		Do(vamos.OpenSeededWorkspaceChat("A")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_A_LATEST")).
		Do(vamos.OpenSeededWorkspaceChat("B")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_B_LATEST")).
		Do(vamos.OpenSeededWorkspaceChat("A")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_A_LATEST")).
		Do(vamos.OpenSeededWorkspaceChat("B")).
		Expect(vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_B_LATEST")).
		Run()
}
