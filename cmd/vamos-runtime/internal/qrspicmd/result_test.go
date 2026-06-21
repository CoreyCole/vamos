package qrspicmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestParseValidateDecideDesignToOutline(t *testing.T) {
	state := testWorkflowState(t, qrspi.NodeDesign, nil)
	parsed, err := ParseValidateDecide(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", ""), state, wruntime.ParseContext{})
	if err != nil {
		t.Fatalf("ParseValidateDecide error = %v", err)
	}
	if parsed.Result.SourceNodeID != qrspi.NodeDesign {
		t.Fatalf("source node = %q", parsed.Result.SourceNodeID)
	}
	if parsed.Decision.NextNodeID != qrspi.NodeOutline || !parsed.Decision.StartNext {
		t.Fatalf("decision next/start = %q/%v, want outline/true", parsed.Decision.NextNodeID, parsed.Decision.StartNext)
	}
	if !strings.Contains(parsed.RawYAML, "qrspi_result:") {
		t.Fatalf("raw YAML missing qrspi_result: %q", parsed.RawYAML)
	}
}

func TestParseValidateDecideReviewOutlineReadyForPlan(t *testing.T) {
	state := testWorkflowState(t, qrspi.NodeReviewOutline, nil)
	parsed, err := ParseValidateDecide(testResultYAML("review-outline", "complete", "ready-for-plan", "thoughts/example/reviews/outline/review.md", ""), state, wruntime.ParseContext{})
	if err != nil {
		t.Fatalf("ParseValidateDecide error = %v", err)
	}
	if parsed.Decision.NextNodeID != qrspi.NodePlan || !parsed.Decision.StartNext {
		t.Fatalf("decision next/start = %q/%v, want plan/true", parsed.Decision.NextNodeID, parsed.Decision.StartNext)
	}
}

func TestParseValidateDecidePlanReadyForReviewPlan(t *testing.T) {
	state := testWorkflowState(t, qrspi.NodePlan, nil)
	parsed, err := ParseValidateDecide(testResultYAML("plan", "complete", "complete", "thoughts/example/plan.md", ""), state, wruntime.ParseContext{})
	if err != nil {
		t.Fatalf("ParseValidateDecide error = %v", err)
	}
	if parsed.Decision.NextNodeID != qrspi.NodeReviewPlan || !parsed.Decision.StartNext {
		t.Fatalf("decision next/start = %q/%v, want review-plan/true", parsed.Decision.NextNodeID, parsed.Decision.StartNext)
	}
}

func TestUpdateImplementationCwdFromWorkspaceResult(t *testing.T) {
	state := testWorkflowState(t, qrspi.NodeWorkspace, nil)
	parsed, err := ParseValidateDecide(testResultYAML("workspace", "complete", "complete", "thoughts/example/plan.md", "implementation_workspace: /tmp/vamos-example\n"), state, wruntime.ParseContext{})
	if err != nil {
		t.Fatalf("ParseValidateDecide error = %v", err)
	}
	manager := UpdateImplementationCwd(ManagerState{}, parsed.Result)
	if manager.ImplementationCwd != "/tmp/vamos-example" {
		t.Fatalf("ImplementationCwd = %q", manager.ImplementationCwd)
	}
}

func TestReviewPlanReadyForImplementRejectedByCanonicalGraph(t *testing.T) {
	state := testWorkflowState(t, qrspi.NodeReviewPlan, nil)
	_, err := ParseValidateDecide(testResultYAML("review-plan", "complete", "ready-for-implement", "thoughts/example/reviews/plan/review.md", ""), state, wruntime.ParseContext{})
	if err == nil {
		t.Fatal("expected ready-for-implement rejection")
	}
	if !strings.Contains(err.Error(), "canonical QRSPI graph rejected result") || !strings.Contains(err.Error(), "outcome \"ready-for-implement\" is not valid for node \"review-plan\"") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscussPolicyDoesNotStartNext(t *testing.T) {
	policy := json.RawMessage(`{"advanceMode":"discuss","enablePlanReviews":true,"invalidResultRetryLimit":1}`)
	state := testWorkflowState(t, qrspi.NodeQuestion, policy)
	parsed, err := ParseValidateDecide(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""), state, wruntime.ParseContext{})
	if err != nil {
		t.Fatalf("ParseValidateDecide error = %v", err)
	}
	if parsed.Decision.NextNodeID != qrspi.NodeResearch {
		t.Fatalf("next node = %q, want research", parsed.Decision.NextNodeID)
	}
	if parsed.Decision.StartNext {
		t.Fatal("StartNext = true, want false for discuss policy")
	}
}

func TestRunValidateResultWritesValidatedEvent(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	resultFile := filepath.Join(dir, "result.txt")
	store := FileStateStore{}
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)}
	if err := store.Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultFile, []byte(testResultYAML("design", "complete", "complete", "thoughts/example/design.md", "")), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err := RunValidateResult(t.Context(), ValidateResultOptions{Stage: "design", StateFile: stateFile, ResultFile: resultFile, PlanDir: "thoughts/example"}, deps{}, &out)
	if err != nil {
		t.Fatalf("RunValidateResult error = %v", err)
	}
	if !strings.Contains(out.String(), `"type":"validated"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunDecideNextPersistsDecisionAndImplementationCwd(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	resultFile := filepath.Join(dir, "result.txt")
	store := FileStateStore{}
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodeWorkspace, nil), ActiveChild: &ChildRunRef{ID: "child"}}
	if err := store.Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultFile, []byte(testResultYAML("workspace", "complete", "complete", "thoughts/example/plan.md", "implementation_workspace: /tmp/vamos-example\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: "thoughts/example"}, deps{}, &out)
	if err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	if !strings.Contains(out.String(), `"type":"decided"`) || !strings.Contains(out.String(), `/tmp/vamos-example`) {
		t.Fatalf("output = %q", out.String())
	}
	loaded, err := store.Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workflow.CurrentNodeID != qrspi.NodeImplement {
		t.Fatalf("CurrentNodeID = %q, want implement", loaded.Workflow.CurrentNodeID)
	}
	if loaded.ImplementationCwd != "/tmp/vamos-example" {
		t.Fatalf("ImplementationCwd = %q", loaded.ImplementationCwd)
	}
	if loaded.ActiveChild != nil {
		t.Fatalf("ActiveChild = %#v, want nil", loaded.ActiveChild)
	}
}

func testWorkflowState(t *testing.T, node wruntime.NodeID, policy json.RawMessage) wruntime.State {
	t.Helper()
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	state, err := wruntime.InitialState(def, policy)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentNodeID = node
	state.PendingNextNodeID = node
	return state
}

func testResultYAML(stage, status, outcome, artifact, workspaceMetadata string) string {
	metadata := ""
	if strings.TrimSpace(workspaceMetadata) != "" {
		metadata = "  workspace_metadata:\n" + indent(workspaceMetadata, "    ")
	}
	return "```yaml\n" +
		"qrspi_result:\n" +
		"  project: \"github.com/CoreyCole/vamos\"\n" +
		"  related_projects: []\n" +
		"  stage: \"" + stage + "\"\n" +
		"  status: \"" + status + "\"\n" +
		"  outcome: \"" + outcome + "\"\n" +
		metadata +
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

func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i := range lines {
		if strings.TrimSpace(lines[i]) != "" {
			lines[i] = prefix + lines[i]
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
