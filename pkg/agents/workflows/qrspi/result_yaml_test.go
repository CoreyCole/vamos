package qrspi

import (
	"errors"
	"os"
	"strings"
	"testing"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func validResultYAML(stage string) string {
	return validResultYAMLWithOutcome(stage, string(wruntime.OutcomeComplete))
}

func validResultYAMLWithOutcome(stage, outcome string) string {
	return "```yaml\n" + validResultYAMLBody(stage, outcome) + "\n```"
}

func validResultYAMLBody(stage, outcome string) string {
	return `qrspi_result:
  project: "github.com/CoreyCole/vamos"
  related_projects:
    - "dotfiles"
  stage: "` + stage + `"
  status: "complete"
  outcome: "` + outcome + `"
  workspace: "/tmp/thoughts/example"
  workspace_metadata:
    plan_workspace: ""
    implementation_workspace: ""
    trunk_branch: "main"
    stack_bottom_branch: ""
    parent_branch: ""
    current_branch: "main"
  policy:
    advance_mode: "guided"
    auto_mode: false
    enable_plan_reviews: true
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "Build Agent Chat-native generic workflow runtime."
    stage_completed: "Completed current workflow node."
    key_decisions: "Continue safely."
  artifact: "thoughts/example/primary.md"
  artifacts:
    - role: "review"
      path: "thoughts/example/review.md"
    - path: "thoughts/example/related.md"
  next:
    steps:
      - action: "read_skill"
        param: "~/.agents/skills/qrspi-planning/SKILL.md"
      - action: "start_stage"
        param: "q-next"
`
}

func validResultYAMLWithoutOutcome(stage string) string {
	return strings.Replace(validResultYAML(stage), "  outcome: \"complete\"\n", "", 1)
}

func TestQRSPIResultParserParseYAML(t *testing.T) {
	parser := QRSPIResultParser{}
	got, err := parser.Parse(validResultYAML("design"), wruntime.ParseContext{ExpectedNodeID: "design"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := got.(Result)
	if parsed.Stage != "design" || parsed.Summary.TextContent() == "" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if len(parsed.RelatedProjects) != 1 || parsed.RelatedProjects[0] != "dotfiles" {
		t.Fatalf("related_projects = %+v", parsed.RelatedProjects)
	}
}

func TestQRSPIResultParserRejectsXML(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse("<qrspi-result><stage>plan</stage></qrspi-result>", wruntime.ParseContext{})
	if err == nil || !strings.Contains(err.Error(), "missing fenced YAML qrspi_result") {
		t.Fatalf("Parse() error = %v, want missing YAML", err)
	}
}

func TestQRSPIResultParserRejectsMissingWrapper(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse("```yaml\nstage: plan\nstatus: complete\n```", wruntime.ParseContext{})
	if err == nil || !strings.Contains(err.Error(), "missing fenced YAML qrspi_result") {
		t.Fatalf("Parse() error = %v, want missing wrapper", err)
	}
}

func TestQRSPIResultParserRejectsUnknownField(t *testing.T) {
	yamlText := strings.Replace(validResultYAML("plan"), "  stage:", "  unexpected_field: true\n  stage:", 1)
	_, err := (QRSPIResultParser{}).Parse(yamlText, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "field unexpected_field not found") {
		t.Fatalf("Parse() error = %v, want unknown field", err)
	}
}

func TestQRSPIResultParserStructuredNextActions(t *testing.T) {
	badAction := strings.Replace(validResultYAML("plan"), `action: "start_stage"`, `action: "start_magic"`, 1)
	_, err := (QRSPIResultParser{}).Parse(badAction, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("Parse() error = %v, want action validation", err)
	}
	missingParam := strings.Replace(validResultYAML("plan"), `param: "q-next"`, `param: ""`, 1)
	_, err = (QRSPIResultParser{}).Parse(missingParam, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "param") {
		t.Fatalf("Parse() error = %v, want param validation", err)
	}
}

func TestQRSPIResultParserWholeOutputYAMLOnlyWhenUnambiguous(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse(validResultYAMLBody("plan", string(wruntime.OutcomeComplete)), wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err != nil {
		t.Fatalf("Parse() whole YAML error = %v", err)
	}
	_, err = (QRSPIResultParser{}).Parse("other: true\n"+validResultYAMLBody("plan", string(wruntime.OutcomeComplete)), wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "missing fenced YAML qrspi_result") {
		t.Fatalf("Parse() error = %v, want ambiguous whole-output rejection", err)
	}
}

func TestQRSPIResultParserReviewStages(t *testing.T) {
	parser := QRSPIResultParser{}
	tests := []struct {
		stage   wruntime.NodeID
		fixture string
	}{
		{stage: NodeReviewOutline, fixture: "testdata/review_outline.yaml"},
		{stage: NodeReviewPlan, fixture: "testdata/review_plan.yaml"},
		{stage: NodeReviewImplementation, fixture: "testdata/review_implementation.yaml"},
	}
	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			output, err := os.ReadFile(tt.fixture)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			got, err := parser.Parse(string(output), wruntime.ParseContext{ExpectedNodeID: tt.stage})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(Result)
			if parsed.Stage != string(tt.stage) {
				t.Fatalf("stage = %q, want %q", parsed.Stage, tt.stage)
			}
		})
	}
}

func TestQRSPIResultParserAcceptsCanonicalStagesAndOutcomes(t *testing.T) {
	parser := QRSPIResultParser{}
	tests := []struct {
		stage   wruntime.NodeID
		outcome wruntime.ResultOutcome
	}{
		{NodeQuestion, wruntime.OutcomeComplete},
		{NodeResearch, wruntime.OutcomeComplete},
		{NodeDesign, wruntime.OutcomeComplete},
		{NodeOutline, wruntime.OutcomeComplete},
		{NodeReviewOutline, wruntime.OutcomeReadyForPlan},
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
			got, err := parser.Parse(validResultYAMLWithOutcome(string(tt.stage), string(tt.outcome)), wruntime.ParseContext{ExpectedNodeID: tt.stage})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(Result)
			if parsed.Outcome != string(tt.outcome) {
				t.Fatalf("outcome = %q, want %q", parsed.Outcome, tt.outcome)
			}
		})
	}
}

func TestQRSPIResultParserRejectsAmbiguousReviewStage(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse(validResultYAMLWithOutcome("review", string(wruntime.OutcomeComplete)), wruntime.ParseContext{})
	if err == nil || !strings.Contains(err.Error(), "ambiguous qrspi review stage") {
		t.Fatalf("Parse() error = %v, want ambiguous review stage", err)
	}
}

func TestQRSPIResultParserRejectsCompleteWithoutOutcome(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse(validResultYAMLWithoutOutcome("question"), wruntime.ParseContext{ExpectedNodeID: NodeQuestion})
	if err == nil || !strings.Contains(err.Error(), "outcome is required") {
		t.Fatalf("Parse() error = %v, want missing outcome", err)
	}
}

func TestQRSPIResultParserParsesProjectAndRelatedProjects(t *testing.T) {
	parsedAny, err := (QRSPIResultParser{}).Parse(validResultYAML("plan"), wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := parsedAny.(Result)
	if got := QRSPIResultProject(parsed); got != "github.com/CoreyCole/vamos" {
		t.Fatalf("project = %q", got)
	}
	result, err := (QRSPIResultConverter{}).ToWorkflowResult(parsed, wruntime.ParseContext{WorkflowType: string(AgentChatWorkflowType)})
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if got := WorkflowResultProject(result); got != "github.com/CoreyCole/vamos" {
		t.Fatalf("WorkflowResultProject() = %q", got)
	}
}

func TestQRSPIResultParserStructuredNextAndWorkspaceMetadata(t *testing.T) {
	yamlText := strings.Replace(validResultYAML("workspace"), `implementation_workspace: ""`, `implementation_workspace: "/tmp/vamos-example"`, 1)
	yamlText = strings.Replace(yamlText, `- action: "start_stage"`, `- action: "read_artifact"`, 1)
	yamlText = strings.Replace(yamlText, `param: "q-next"`, `param: "thoughts/example/plan.md"`, 1)
	parsedAny, err := (QRSPIResultParser{}).Parse(yamlText, wruntime.ParseContext{ExpectedNodeID: NodeWorkspace})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := parsedAny.(Result)
	if parsed.WorkspaceMetadata.ImplementationWorkspace != "/tmp/vamos-example" || len(parsed.Next.Steps) != 2 {
		t.Fatalf("parsed metadata/next = %+v / %+v", parsed.WorkspaceMetadata, parsed.Next)
	}
	result, err := (QRSPIResultConverter{}).ToWorkflowResult(parsed, wruntime.ParseContext{WorkflowType: "qrspi"})
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if !strings.Contains(result.DisplayNext, "Read skill:") || !strings.Contains(result.DisplayNext, "Read artifact:") {
		t.Fatalf("DisplayNext = %q", result.DisplayNext)
	}
	if got := WorkflowResultImplementationWorkspace(result); got != "/tmp/vamos-example" {
		t.Fatalf("ImplementationWorkspace = %q", got)
	}
}

func TestQRSPIResultConverter(t *testing.T) {
	parsedAny, err := (QRSPIResultParser{}).Parse(validResultYAML("review-implementation"), wruntime.ParseContext{ExpectedNodeID: NodeReviewImplementation})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (QRSPIResultConverter{}).ToWorkflowResult(parsedAny, wruntime.ParseContext{WorkflowType: "qrspi", ExpectedNodeID: NodeReviewImplementation, RunID: "run-1", ThreadID: "thread-1", SessionID: "session-1", HeadEntryID: "entry-1", SessionPath: "/tmp/session.jsonl"})
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	if got.WorkflowType != "qrspi" || got.SourceNodeID != NodeReviewImplementation || got.Status != "complete" {
		t.Fatalf("unexpected workflow result: %+v", got)
	}
	if got.PrimaryArtifact != "thoughts/example/primary.md" || len(got.Artifacts) != 3 {
		t.Fatalf("artifact projection = primary %q artifacts %+v", got.PrimaryArtifact, got.Artifacts)
	}
	if got.Artifacts[1].Role != "review" || got.Artifacts[2].Role != "related" {
		t.Fatalf("artifact roles = %+v", got.Artifacts)
	}
	if got.Evidence.RunID != "run-1" || got.Evidence.HeadEntryID != "entry-1" || got.Evidence.SessionPath != "/tmp/session.jsonl" {
		t.Fatalf("evidence = %+v", got.Evidence)
	}
	if len(got.Policy) == 0 || len(got.Raw) == 0 {
		t.Fatalf("policy/raw not populated: policy=%s raw=%s", got.Policy, got.Raw)
	}
}

func TestCorrectionPromptMentionsYAMLAndCanonicalReviewStages(t *testing.T) {
	prompt := (QRSPIResultParser{}).CorrectionPrompt(errors.New("bad"), 1)
	if !strings.Contains(prompt, "fenced YAML") || !strings.Contains(prompt, "qrspi_result") {
		t.Fatalf("CorrectionPrompt() = %q, want YAML contract", prompt)
	}
	for _, stage := range []string{"review-outline", "review-plan", "review-implementation"} {
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
	for _, node := range []wruntime.NodeID{NodeQuestion, NodeResearch, NodeDesign, NodeOutline, NodeReviewOutline, NodeHumanReviewOutline, NodeResearchForReviewOutline, NodeAddressReviewResearchOutline, NodePlan, NodeReviewPlan, NodeResearchForReviewPlan, NodeAddressReviewResearchPlan, NodeWorkspace, NodeImplement, NodeReviewImplementation, NodeHumanReviewImplementation, NodeDone} {
		if _, ok := def.Nodes[node]; !ok {
			t.Fatalf("missing node %q", node)
		}
	}
	for id, node := range def.Nodes {
		if node.Kind == wruntime.NodeKindAgent && strings.Contains(node.Prompt.SkillPath, "~/.agents/skills/q-") {
			t.Fatalf("node %s uses global q-skill path %q", id, node.Prompt.SkillPath)
		}
	}
}
