package semantic

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestSemanticApplyReviewPlanPositiveRouting(t *testing.T) {
	tests := []struct {
		name              string
		context           Context
		wantOutcome       wruntime.ResultOutcome
		wantNext          wruntime.NodeID
		wantAction        NextActionKind
		wantNormalization string
	}{
		{
			name:              "parent plan routes to workspace",
			context:           Context{PlanDir: "thoughts/example/plans/parent"},
			wantOutcome:       wruntime.OutcomeReadyForWorkspace,
			wantNext:          qrspi.NodeWorkspace,
			wantAction:        NextActionStartNext,
			wantNormalization: string(wruntime.OutcomeReadyForWorkspace),
		},
		{
			name:              "review dir routes to implement",
			context:           Context{PlanDir: "thoughts/example/plans/parent/reviews/2026-01-01_implementation-review"},
			wantOutcome:       wruntime.OutcomeReadyForImplement,
			wantNext:          qrspi.NodeImplement,
			wantAction:        NextActionStartNext,
			wantNormalization: string(wruntime.OutcomeReadyForImplement),
		},
		{
			name:              "implementation cwd routes to implement",
			context:           Context{PlanDir: "thoughts/example/plans/parent", ImplementationCwd: "/tmp/impl"},
			wantOutcome:       wruntime.OutcomeReadyForImplement,
			wantNext:          qrspi.NodeImplement,
			wantAction:        NextActionStartNext,
			wantNormalization: string(wruntime.OutcomeReadyForImplement),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := semanticTestState(t, qrspi.NodeReviewPlan)
			tt.context.State = state
			got, err := Apply(context.Background(), ApplyInput{
				RawOutput:    semanticResultYAML("review-plan", "complete", "complete", "thoughts/example/reviews/plan/review.md", ""),
				ParseContext: wruntime.ParseContext{ExpectedNodeID: qrspi.NodeReviewPlan},
				Context:      tt.context,
			})
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if got.WorkflowResult.Outcome != tt.wantOutcome || got.Decision.NextNodeID != tt.wantNext || got.NextAction.Kind != tt.wantAction {
				t.Fatalf("outcome/next/action = %q/%q/%q", got.WorkflowResult.Outcome, got.Decision.NextNodeID, got.NextAction.Kind)
			}
			if len(got.Normalizations) != 1 || got.Normalizations[0].Canonical != tt.wantNormalization {
				t.Fatalf("normalizations = %+v", got.Normalizations)
			}
		})
	}
}

func TestSemanticApplyStatusActions(t *testing.T) {
	tests := []struct {
		name       string
		node       wruntime.NodeID
		status     string
		outcome    string
		wantNext   wruntime.NodeID
		wantAction NextActionKind
	}{
		{name: "handoff", node: qrspi.NodeImplement, status: "handoff", wantNext: qrspi.NodeImplement, wantAction: NextActionContinuePending},
		{name: "blocked", node: qrspi.NodeDesign, status: "blocked", wantAction: NextActionBlocked},
		{name: "error", node: qrspi.NodeDesign, status: "error", wantAction: NextActionError},
		{name: "needs human", node: qrspi.NodeDesign, status: "needs_human", wantAction: NextActionWaitHuman},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := semanticTestState(t, tt.node)
			got, err := Apply(context.Background(), ApplyInput{
				RawOutput:    semanticResultYAML(string(tt.node), tt.status, tt.outcome, "thoughts/example/artifact.md", ""),
				ParseContext: wruntime.ParseContext{ExpectedNodeID: tt.node},
				Context:      Context{State: state},
			})
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if got.Decision.NextNodeID != tt.wantNext || got.NextAction.Kind != tt.wantAction {
				t.Fatalf("next/action = %q/%q", got.Decision.NextNodeID, got.NextAction.Kind)
			}
		})
	}
}

func TestSemanticApplyWorkspaceCwdEffect(t *testing.T) {
	state := semanticTestState(t, qrspi.NodeWorkspace)
	got, err := Apply(context.Background(), ApplyInput{
		RawOutput: semanticResultYAML("workspace", "complete", "complete", "thoughts/example/plan.md", strings.Join([]string{
			"  workspace: \"/tmp/top-workspace\"",
			"  workspace_metadata:",
			"    implementation_workspace: \"/tmp/metadata-workspace\"",
		}, "\n")+"\n"),
		ParseContext: wruntime.ParseContext{ExpectedNodeID: qrspi.NodeWorkspace},
		Context:      Context{State: state},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got.NextAction.Kind != NextActionStartNext || got.Decision.NextNodeID != qrspi.NodeImplement {
		t.Fatalf("next/action = %q/%q", got.Decision.NextNodeID, got.NextAction.Kind)
	}
	if len(got.Effects) == 0 || !hasEffect(got.Effects, EffectUpdateExecutionCwd, "/tmp/top-workspace") {
		t.Fatalf("effects = %+v", got.Effects)
	}
}

func semanticTestState(t *testing.T, node wruntime.NodeID) wruntime.State {
	t.Helper()
	def, err := qrspi.Definition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = node
	state.PendingNextNodeID = node
	return state
}

func semanticResultYAML(stage, status, outcome, artifact, extraTopLevel string) string {
	outcomeLine := ""
	if strings.TrimSpace(outcome) != "" {
		outcomeLine = "  outcome: \"" + outcome + "\"\n"
	}
	return "```yaml\n" +
		"qrspi_result:\n" +
		"  project: \"github.com/CoreyCole/vamos\"\n" +
		"  related_projects: []\n" +
		"  stage: \"" + stage + "\"\n" +
		"  status: \"" + status + "\"\n" +
		outcomeLine +
		extraTopLevel +
		"  policy:\n" +
		"    advance_mode: \"guided\"\n" +
		"    auto_mode: false\n" +
		"    enable_plan_reviews: true\n" +
		"    invalid_result_retry_limit: 1\n" +
		"  summary:\n" +
		"    plan_goal: \"test goal\"\n" +
		"    stage_completed: \"test complete\"\n" +
		"    key_decisions: \"test decisions\"\n" +
		"  artifact: \"" + artifact + "\"\n" +
		"  artifacts:\n" +
		"    - role: \"related\"\n" +
		"      path: \"" + artifact + "\"\n" +
		"  next:\n" +
		"    steps:\n" +
		"      - action: \"read_skill\"\n" +
		"        param: \".pi/skills/qrspi-planning/SKILL.md\"\n" +
		"      - action: \"start_stage\"\n" +
		"        param: \"next\"\n" +
		"```\n"
}

func hasEffect(effects []Effect, kind EffectKind, path string) bool {
	for _, effect := range effects {
		if effect.Kind == kind && effect.Path == path {
			return true
		}
	}
	return false
}
