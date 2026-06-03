package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestAgentChatQRSPIContinuationStory(t *testing.T) {
	spec.Story(t, "agentchat qrspi continuation").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.SeedQRSPIContinuationWorkspace()).
		Do(vamos.OpenQRSPIContinuationWorkspaceChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_QRSPI_CONTINUATION_PROMPT")).
		Expect(vamos.WorkflowCardShowsNextSteps()).
		Expect(vamos.WorkflowCardShowsAgentProgress()).
		Expect(vamos.WorkflowCardHasJumpCurrent()).
		Expect(vamos.WorkflowCardHasJumpNextEnd()).
		Do(vamos.SetFirstTranscriptMessageHash()).
		Do(vamos.ReloadChat()).
		Expect(vamos.TranscriptContains("VAMOS_E2E_QRSPI_CONTINUATION_PROMPT")).
		Expect(vamos.ExpectHashAnchorPreserved()).
		Expect(vamos.MobileTranscriptHasNoHorizontalOverflow()).
		Expect(vamos.ToolWriteEditRendered()).
		Run()
}

func TestAgentChatQRSPIQuestionCompletionAutoStartsResearch(t *testing.T) {
	spec.Story(t, "agentchat qrspi question completion auto starts research").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.StartQRSPIQuestionToResearchWorkspace()).
		Expect(vamos.WaitForWorkflowRunResult("question")).
		Expect(vamos.WaitForWorkflowRunStarted("research")).
		Expect(vamos.ExpectWorkflowCurrentNode("research")).
		Expect(vamos.ExpectDistinctWorkflowRuns("question", "research")).
		Do(vamos.ReloadChat()).
		Expect(vamos.ExpectWorkflowCurrentNode("research")).
		Expect(vamos.ExpectResearchRunVisible()).
		Run()
}
