package qrspi

import (
	"errors"
	"os"
	"strings"
	"testing"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func validResultXML(stage string) string {
	return validResultXMLWithOutcome(stage, string(wruntime.OutcomeComplete))
}

func validResultXMLWithOutcome(stage, outcome string) string {
	return `<qrspi-result>
  <stage>` + stage + `</stage>
  <status>complete</status>
  <outcome>` + outcome + `</outcome>
  <policy>
    <autoMode>false</autoMode>
    <enablePlanReviews>true</enablePlanReviews>
    <invalidResultRetryLimit>1</invalidResultRetryLimit>
  </policy>
  <summary>
    <plan-goal>Build Agent Chat-native generic workflow runtime.</plan-goal>
    <stage-completed>Completed current workflow node.</stage-completed>
    <key-decisions>Continue safely.</key-decisions>
  </summary>
  <artifact>thoughts/example/primary.md</artifact>
  <artifacts>
    <artifact role="review">thoughts/example/review.md</artifact>
    <artifact>thoughts/example/related.md</artifact>
  </artifacts>
  <next>/q-next thoughts/example/primary.md</next>
</qrspi-result>`
}

func validResultXMLWithoutOutcome(stage string) string {
	return strings.Replace(
		validResultXML(stage),
		"\n  <outcome>complete</outcome>",
		"",
		1,
	)
}

func TestQRSPIXMLParserParse(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		expectedNode wruntime.NodeID
		wantErr      string
	}{
		{
			name:         "structured summary and artifacts",
			output:       validResultXML("design"),
			expectedNode: "design",
		},
		{
			name:         "fenced XML extraction",
			output:       "```xml\n" + validResultXML("review-plan") + "\n```",
			expectedNode: "review-plan",
		},
		{
			name: "plain text summary compatibility",
			output: `<qrspi-result>
  <stage>question</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <policy><autoMode>false</autoMode><enablePlanReviews>true</enablePlanReviews><invalidResultRetryLimit>1</invalidResultRetryLimit></policy>
  <summary>Plain text summary.</summary>
  <artifact>thoughts/example/questions.md</artifact>
  <next>/q-research thoughts/example/questions.md</next>
</qrspi-result>`,
			expectedNode: "question",
		},
		{
			name:    "missing XML",
			output:  "no result here",
			wantErr: "missing <qrspi-result>",
		},
		{
			name: "ambiguous review stage",
			output: strings.Replace(
				validResultXML("review-design"),
				"<stage>review-design</stage>",
				"<stage>review</stage>",
				1,
			),
			wantErr: "ambiguous qrspi review stage",
		},
		{
			name:         "mismatched expected node",
			output:       validResultXML("outline"),
			expectedNode: "design",
			wantErr:      "does not match expected workflow node",
		},
		{
			name:         "complete status missing outcome",
			output:       validResultXMLWithoutOutcome("question"),
			expectedNode: "question",
			wantErr:      "outcome is required",
		},
		{
			name: "missing summary",
			output: strings.Replace(
				validResultXML("design"),
				`<plan-goal>Build Agent Chat-native generic workflow runtime.</plan-goal>
    <stage-completed>Completed current workflow node.</stage-completed>
    <key-decisions>Continue safely.</key-decisions>`,
				"",
				1,
			),
			wantErr: "summary is required",
		},
	}

	parser := QRSPIXMLParser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(
				tt.output,
				wruntime.ParseContext{ExpectedNodeID: tt.expectedNode},
			)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(ResultXML)
			if parsed.Stage != string(tt.expectedNode) {
				t.Fatalf("stage = %q, want %q", parsed.Stage, tt.expectedNode)
			}
			if parsed.Summary.TextContent() == "" {
				t.Fatal("summary text is empty")
			}
		})
	}
}

func TestQRSPIXMLParserReviewStages(t *testing.T) {
	parser := QRSPIXMLParser{}
	tests := []struct {
		stage   wruntime.NodeID
		fixture string
	}{
		{stage: NodeReviewDesign, fixture: "testdata/review_design.xml"},
		{stage: NodeReviewOutline, fixture: "testdata/review_outline.xml"},
		{stage: NodeReviewPlan, fixture: "testdata/review_plan.xml"},
		{stage: NodeReviewImplementation, fixture: "testdata/review_implementation.xml"},
	}
	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			output, err := os.ReadFile(tt.fixture)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			got, err := parser.Parse(
				string(output),
				wruntime.ParseContext{ExpectedNodeID: tt.stage},
			)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(ResultXML)
			if parsed.Stage != string(tt.stage) {
				t.Fatalf("stage = %q, want %q", parsed.Stage, tt.stage)
			}
		})
	}
}

func TestQRSPIXMLParserAcceptsCanonicalStagesAndOutcomes(t *testing.T) {
	parser := QRSPIXMLParser{}
	tests := []struct {
		stage   wruntime.NodeID
		outcome wruntime.ResultOutcome
	}{
		{NodeQuestion, wruntime.OutcomeComplete},
		{NodeResearch, wruntime.OutcomeComplete},
		{NodeDesign, wruntime.OutcomeComplete},
		{NodeReviewDesign, wruntime.OutcomeReadyForOutline},
		{NodeResearchForReviewDesign, wruntime.OutcomeComplete},
		{NodeAddressReviewResearchDesign, wruntime.OutcomeComplete},
		{NodeOutline, wruntime.OutcomeComplete},
		{NodeReviewOutline, wruntime.OutcomeReadyForHumanReview},
		{NodeHumanReviewOutline, wruntime.OutcomeComplete},
		{NodeResearchForReviewOutline, wruntime.OutcomeComplete},
		{NodeAddressReviewResearchOutline, wruntime.OutcomeComplete},
		{NodePlan, wruntime.OutcomeComplete},
		{NodeReviewPlan, wruntime.OutcomeReadyForWorkspace},
		{NodeResearchForReviewPlan, wruntime.OutcomeComplete},
		{NodeAddressReviewResearchPlan, wruntime.OutcomeComplete},
		{NodeWorkspace, wruntime.OutcomeComplete},
		{NodeImplement, wruntime.OutcomeComplete},
		{NodeReviewImplementation, wruntime.OutcomeReadyForHumanReview},
		{NodeHumanReviewImplementation, wruntime.OutcomeComplete},
	}
	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got, err := parser.Parse(
				validResultXMLWithOutcome(string(tt.stage), string(tt.outcome)),
				wruntime.ParseContext{ExpectedNodeID: tt.stage},
			)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(ResultXML)
			if parsed.Outcome != string(tt.outcome) {
				t.Fatalf("outcome = %q, want %q", parsed.Outcome, tt.outcome)
			}
		})
	}
}

func TestQRSPIXMLParserRejectsAmbiguousReviewStage(t *testing.T) {
	_, err := (QRSPIXMLParser{}).Parse(
		validResultXMLWithOutcome("review", string(wruntime.OutcomeComplete)),
		wruntime.ParseContext{},
	)
	if err == nil || !strings.Contains(err.Error(), "ambiguous qrspi review stage") {
		t.Fatalf("Parse() error = %v, want ambiguous review stage", err)
	}
}

func TestQRSPIXMLParserRejectsCompleteWithoutOutcome(t *testing.T) {
	_, err := (QRSPIXMLParser{}).Parse(
		validResultXMLWithoutOutcome("question"),
		wruntime.ParseContext{ExpectedNodeID: NodeQuestion},
	)
	if err == nil || !strings.Contains(err.Error(), "outcome is required") {
		t.Fatalf("Parse() error = %v, want missing outcome", err)
	}
}

func TestQRSPIXMLParserParsesProject(t *testing.T) {
	output := strings.Replace(
		validResultXML("plan"),
		"<stage>plan</stage>",
		"<project>github.com/CoreyCole/vamos</project>\n  <stage>plan</stage>",
		1,
	)
	parsedAny, err := (QRSPIXMLParser{}).Parse(output, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := parsedAny.(ResultXML)
	if got := QRSPIResultProject(parsed); got != "github.com/CoreyCole/vamos" {
		t.Fatalf("project = %q", got)
	}
	result, err := (QRSPIResultConverter{}).ToWorkflowResult(
		parsed,
		wruntime.ParseContext{WorkflowType: string(AgentChatWorkflowType)},
	)
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if got := WorkflowResultProject(result); got != "github.com/CoreyCole/vamos" {
		t.Fatalf("WorkflowResultProject() = %q", got)
	}
}

func TestQRSPIXMLParserStructuredNextAndWorkspaceMetadata(t *testing.T) {
	output := `<qrspi-result>
  <stage>workspace</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <workspaceMetadata>
    <planWorkspace>/tmp/thoughts/agent/plans/example</planWorkspace>
    <implementationWorkspace>/tmp/vamos-example</implementationWorkspace>
    <trunkBranch>main</trunkBranch>
    <stackBottomBranch>cc/example_slice-1</stackBottomBranch>
    <parentBranch>main</parentBranch>
    <currentBranch>cc/example_slice-1</currentBranch>
  </workspaceMetadata>
  <policy><autoMode>false</autoMode><enablePlanReviews>true</enablePlanReviews><invalidResultRetryLimit>1</invalidResultRetryLimit></policy>
  <summary><plan-goal>Goal.</plan-goal><stage-completed>Workspace ready.</stage-completed><key-decisions>Start implement.</key-decisions></summary>
  <artifact>/tmp/thoughts/agent/plans/example/plan.md</artifact>
  <next>
    <step>Read q-implement.</step>
    <step>Start /q-implement immediately.</step>
  </next>
</qrspi-result>`
	parsedAny, err := (QRSPIXMLParser{}).Parse(output, wruntime.ParseContext{ExpectedNodeID: NodeWorkspace})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := parsedAny.(ResultXML)
	if parsed.WorkspaceMetadata.ImplementationWorkspace != "/tmp/vamos-example" || len(parsed.Next.Steps) != 2 {
		t.Fatalf("parsed metadata/next = %+v / %+v", parsed.WorkspaceMetadata, parsed.Next)
	}
	result, err := (QRSPIResultConverter{}).ToWorkflowResult(parsed, wruntime.ParseContext{WorkflowType: "qrspi"})
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if !strings.Contains(result.DisplayNext, "Read q-implement") || !strings.Contains(result.DisplayNext, "Start /q-implement") {
		t.Fatalf("DisplayNext = %q", result.DisplayNext)
	}
	if got := WorkflowResultImplementationWorkspace(result); got != "/tmp/vamos-example" {
		t.Fatalf("ImplementationWorkspace = %q", got)
	}
}

func TestQRSPIResultConverter(t *testing.T) {
	parser := QRSPIXMLParser{}
	parsedAny, err := parser.Parse(
		validResultXML("review-implementation"),
		wruntime.ParseContext{ExpectedNodeID: NodeReviewImplementation},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (QRSPIResultConverter{}).ToWorkflowResult(
		parsedAny,
		wruntime.ParseContext{
			WorkflowType:   "qrspi",
			ExpectedNodeID: NodeReviewImplementation,
			RunID:          "run-1",
			ThreadID:       "thread-1",
			SessionID:      "session-1",
			HeadEntryID:    "entry-1",
			SessionPath:    "/tmp/session.jsonl",
		},
	)
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if got.WorkflowType != "qrspi" || got.SourceNodeID != NodeReviewImplementation ||
		got.Status != "complete" {
		t.Fatalf("unexpected workflow result: %+v", got)
	}
	if got.PrimaryArtifact != "thoughts/example/primary.md" {
		t.Fatalf("primary artifact = %q", got.PrimaryArtifact)
	}
	if len(got.Artifacts) != 3 {
		t.Fatalf("artifacts len = %d, want 3", len(got.Artifacts))
	}
	if got.Artifacts[1].Role != "review" || got.Artifacts[2].Role != "related" {
		t.Fatalf("artifact roles = %+v", got.Artifacts)
	}
	if got.Evidence.RunID != "run-1" || got.Evidence.HeadEntryID != "entry-1" ||
		got.Evidence.SessionPath != "/tmp/session.jsonl" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
	if len(got.Policy) == 0 || len(got.Raw) == 0 {
		t.Fatalf("policy/raw not populated: policy=%s raw=%s", got.Policy, got.Raw)
	}
}

func TestCorrectionPromptMentionsCanonicalReviewStages(t *testing.T) {
	prompt := (QRSPIXMLParser{}).CorrectionPrompt(errors.New("bad"), 1)
	for _, stage := range []string{"review-design", "review-outline", "review-plan", "review-implementation"} {
		if !strings.Contains(prompt, stage) {
			t.Fatalf("CorrectionPrompt() = %q, missing %q", prompt, stage)
		}
	}
}

func TestDefinition(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if def.ID != AgentChatWorkflowType || def.Start != NodeQuestion {
		t.Fatalf("definition ID/start = %q/%q", def.ID, def.Start)
	}
	for _, node := range []wruntime.NodeID{NodeQuestion, NodeResearch, NodeDesign, NodeReviewDesign, NodeResearchForReviewDesign, NodeAddressReviewResearchDesign, NodeOutline, NodeReviewOutline, NodeHumanReviewOutline, NodeResearchForReviewOutline, NodeAddressReviewResearchOutline, NodePlan, NodeReviewPlan, NodeResearchForReviewPlan, NodeAddressReviewResearchPlan, NodeWorkspace, NodeImplement, NodeReviewImplementation, NodeHumanReviewImplementation, NodeDone} {
		if _, ok := def.Nodes[node]; !ok {
			t.Fatalf("missing node %q", node)
		}
	}
}
