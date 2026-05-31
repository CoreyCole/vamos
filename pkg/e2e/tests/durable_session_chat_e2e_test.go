package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

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
			spec.ConsoleClean(),
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
		Then(
			vamos.TranscriptContains("VAMOS_E2E_WORKSPACE_DOC_RESTORE"),
			spec.ConsoleClean(),
		).
		Run(t)
}
