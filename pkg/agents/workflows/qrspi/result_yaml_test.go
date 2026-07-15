package qrspi

import (
	"encoding/json"
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
	for _, action := range []string{"read_skill", "read_artifact", "start_stage", "request_human_approval", "request_human_decision", "request_human_decisions"} {
		if !strings.Contains(err.Error(), action) {
			t.Fatalf("Parse() error = %v, missing allowed action %q", err, action)
		}
	}
	missingParam := strings.Replace(validResultYAML("plan"), `param: "q-next"`, `param: ""`, 1)
	_, err = (QRSPIResultParser{}).Parse(missingParam, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "param") {
		t.Fatalf("Parse() error = %v, want param validation", err)
	}
}

func TestQRSPIResultParserAcceptsRequestHumanDecisionActions(t *testing.T) {
	tests := map[string]string{
		"request_human_decision":  "Request human decision: q-next",
		"request_human_decisions": "Request human decisions: q-next",
	}
	for action, wantDisplay := range tests {
		yamlText := strings.Replace(validResultYAML("address-review-research-outline"), `action: "start_stage"`, `action: "`+action+`"`, 1)
		parsedAny, err := (QRSPIResultParser{}).Parse(yamlText, wruntime.ParseContext{ExpectedNodeID: NodeAddressReviewResearchOutline})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", action, err)
		}
		parsed := parsedAny.(Result)
		if got := parsed.Next.Steps[1].DisplayText(); got != wantDisplay {
			t.Fatalf("DisplayText() = %q, want %q", got, wantDisplay)
		}
	}
}

func TestQRSPIAddressReviewResearchAllowsNeedsHuman(t *testing.T) {
	definition, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []wruntime.NodeID{NodeAddressReviewResearchOutline, NodeAddressReviewResearchPlan} {
		state, err := wruntime.InitialState(definition, nil)
		if err != nil {
			t.Fatal(err)
		}
		state.CurrentNodeID = node
		result := wruntime.WorkflowResult{SourceNodeID: node, Status: wruntime.StatusNeedsHuman, Summary: "Human decision required.", PrimaryArtifact: "thoughts/example/review.md"}
		decision, err := wruntime.DecideTransition(definition, state, result)
		if err != nil {
			t.Fatalf("DecideTransition(%q) error = %v", node, err)
		}
		if !decision.WaitingHuman || decision.StartNext {
			t.Fatalf("DecideTransition(%q) = %+v, want human wait", node, decision)
		}
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

func TestQRSPIResultParserFindsLaterFencedResult(t *testing.T) {
	output := strings.Join([]string{
		"```yaml",
		"not_qrspi: true",
		"```",
		"Some prose between blocks.",
		validResultYAML("plan"),
	}, "\n")

	got, err := (QRSPIResultParser{}).Parse(output, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.(Result).Stage != string(NodePlan) {
		t.Fatalf("stage = %q, want %q", got.(Result).Stage, NodePlan)
	}
}

func TestQRSPIResultParserInvalidFencedResultReturnsConcreteError(t *testing.T) {
	output := strings.Join([]string{
		"```yaml",
		"qrspi_result:",
		"  stage: [not: valid",
		"```",
	}, "\n")

	_, err := (QRSPIResultParser{}).Parse(output, wruntime.ParseContext{ExpectedNodeID: NodePlan})
	if err == nil || !strings.Contains(err.Error(), "parse qrspi result YAML") {
		t.Fatalf("Parse() error = %v, want concrete YAML parse error", err)
	}
}

func TestQRSPIResultParserProjectsAdvanceMode(t *testing.T) {
	yamlText := strings.Replace(validResultYAML("question"), `advance_mode: "guided"`, `advance_mode: "discuss"`, 1)
	parsedAny, err := (QRSPIResultParser{}).Parse(yamlText, wruntime.ParseContext{ExpectedNodeID: NodeQuestion})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	parsed := parsedAny.(Result)
	if parsed.Policy.AdvanceMode != AdvanceModeDiscuss {
		t.Fatalf("parsed advance_mode = %q, want %q", parsed.Policy.AdvanceMode, AdvanceModeDiscuss)
	}
	result, err := (QRSPIResultConverter{}).ToWorkflowResult(parsed, wruntime.ParseContext{WorkflowType: string(AgentChatWorkflowType)})
	if err != nil {
		t.Fatalf("ToWorkflowResult() error = %v", err)
	}
	var policy Policy
	if err := json.Unmarshal(result.Policy, &policy); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if policy.AdvanceMode != AdvanceModeDiscuss {
		t.Fatalf("projected advanceMode = %q, want %q", policy.AdvanceMode, AdvanceModeDiscuss)
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
		{NodeVerify, wruntime.OutcomeComplete},
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

func TestQRSPIResultParserNormalizesDeterministicPositiveReviewOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		stage   wruntime.NodeID
		outcome string
		want    wruntime.ResultOutcome
	}{
		{name: "review-outline complete", stage: NodeReviewOutline, outcome: "complete", want: wruntime.OutcomeReadyForPlan},
		{name: "review-implementation done", stage: NodeReviewImplementation, outcome: "done", want: wruntime.OutcomeReadyForHumanReview},
		{name: "review-implementation approved", stage: NodeReviewImplementation, outcome: "approved", want: wruntime.OutcomeReadyForHumanReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (QRSPIResultParser{}).Parse(validResultYAMLWithOutcome(string(tt.stage), tt.outcome), wruntime.ParseContext{ExpectedNodeID: tt.stage})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			parsed := got.(Result)
			if parsed.Outcome != string(tt.want) {
				t.Fatalf("outcome = %q, want %q", parsed.Outcome, tt.want)
			}
			if len(parsed.Normalizations) != 1 || parsed.Normalizations[0].Original != tt.outcome {
				t.Fatalf("normalizations = %+v", parsed.Normalizations)
			}
		})
	}
}

func TestQRSPIResultParserDoesNotNormalizeNegativeOrAmbiguousOutcomes(t *testing.T) {
	_, err := (QRSPIResultParser{}).Parse(validResultYAMLWithOutcome(string(NodeReviewOutline), string(wruntime.OutcomeNeedsReviewResearch)), wruntime.ParseContext{ExpectedNodeID: NodeReviewOutline})
	if err != nil {
		t.Fatalf("negative review outcome should remain valid: %v", err)
	}
	parsedAny, err := (QRSPIResultParser{}).Parse(validResultYAMLWithOutcome(string(NodeReviewPlan), "complete"), wruntime.ParseContext{ExpectedNodeID: NodeReviewPlan})
	if err != nil {
		t.Fatalf("review-plan parser should leave ambiguous positive outcome unchanged before graph validation: %v", err)
	}
	if parsedAny.(Result).Outcome != "complete" || len(parsedAny.(Result).Normalizations) != 0 {
		t.Fatalf("review-plan parsed = %+v", parsedAny.(Result))
	}
}

func TestQRSPIResultParserRejectsAmbiguousReviewStage(t *testing.T) {
	for _, stage := range []string{"review", "review-design"} {
		t.Run(stage, func(t *testing.T) {
			_, err := (QRSPIResultParser{}).Parse(validResultYAMLWithOutcome(stage, string(wruntime.OutcomeComplete)), wruntime.ParseContext{})
			if err == nil || !strings.Contains(err.Error(), "ambiguous qrspi review stage") {
				t.Fatalf("Parse() error = %v, want ambiguous review stage", err)
			}
		})
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
	prompt := (QRSPIResultParser{}).CorrectionPrompt(errors.New("session /tmp/x has no assistant text containing qrspi_result"), 1)
	for _, want := range []string{
		"Validation error:",
		"has no assistant text containing qrspi_result",
		"```yaml",
		"qrspi_result:",
		"workspace_metadata:",
		"policy:",
		"summary:",
		"next:",
		"Valid next.steps actions:",
		"request_human_decisions",
		"review-outline",
		"review-plan",
		"review-implementation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("CorrectionPrompt() = %q, missing %q", prompt, want)
		}
	}
	if _, err := (QRSPIResultParser{}).Parse(ExampleQRSPIResultYAML(), wruntime.ParseContext{ExpectedNodeID: NodePlan}); err != nil {
		t.Fatalf("ExampleQRSPIResultYAML() does not parse: %v", err)
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
	for _, node := range []wruntime.NodeID{NodeQuestion, NodeResearch, NodeDesign, NodeOutline, NodeReviewOutline, NodeHumanReviewOutline, NodeResearchForReviewOutline, NodeAddressReviewResearchOutline, NodePlan, NodeReviewPlan, NodeResearchForReviewPlan, NodeAddressReviewResearchPlan, NodeWorkspace, NodeImplement, NodeReviewImplementation, NodeVerify, NodeHumanReviewImplementation, NodeDone} {
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
