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

func TestAgentChatQRSPIQuestionCompletionAutoStartsDesign(t *testing.T) {
	spec.Story(t, "agentchat qrspi question completion auto starts design").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.StartQRSPIQuestionToDesignWorkspace()).
		Expect(vamos.WaitForWorkflowRunResult("question")).
		Expect(vamos.WaitForWorkflowRunStarted("research")).
		Expect(vamos.ExpectDistinctWorkflowRuns("question", "research")).
		Expect(vamos.WaitForWorkflowRunResult("research")).
		Expect(vamos.ExpectWorkflowArtifactExists("qrspi_q_to_d_research_artifact_abs", "VAMOS_E2E_QRSPI_Q_TO_D_")).
		Expect(vamos.ExpectWorkflowRunResultContains("research", "research")).
		Expect(vamos.WaitForWorkflowRunStarted("design")).
		Expect(vamos.ExpectWorkflowCurrentNode("design")).
		Do(vamos.ReloadChat()).
		Expect(vamos.ExpectWorkflowCurrentNode("design")).
		Run()
}
