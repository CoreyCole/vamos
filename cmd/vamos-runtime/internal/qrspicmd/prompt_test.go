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

func TestResolveStageSkill(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	if got := ResolveStageSkill(def.Nodes[qrspi.NodeResearch]); got != ".pi/skills/q-research/SKILL.md" {
		t.Fatalf("ResolveStageSkill() = %q", got)
	}
}

func TestLoadManifestMissingReturnsEmpty(t *testing.T) {
	manifest, err := LoadManifest(t.TempDir())
	if err != nil {
		t.Fatalf("LoadManifest error = %v", err)
	}
	if manifest != "" {
		t.Fatalf("manifest = %q, want empty", manifest)
	}
}

func TestRenderStagePromptIncludesRequiredContext(t *testing.T) {
	previousRaw, err := json.Marshal(qrspi.Result{
		Project:  "github.com/CoreyCole/vamos",
		Stage:    string(qrspi.NodeReviewOutline),
		Status:   string(wruntime.StatusComplete),
		Outcome:  string(wruntime.OutcomeReadyForPlan),
		Summary:  qrspi.Summary{PlanGoal: "goal", StageCompleted: "outline reviewed", KeyDecisions: "plan next"},
		Artifact: "thoughts/example/reviews/outline/review.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderStagePrompt(PromptContext{
		Node:    def.Nodes[qrspi.NodePlan],
		PlanDir: "thoughts/example",
		State: ManagerState{
			SourceCwd:         "/repo",
			ImplementationCwd: "/repo-impl",
		},
		LastResult: &wruntime.WorkflowResultSnapshot{
			SourceNodeID:    qrspi.NodeReviewOutline,
			Status:          "complete",
			Outcome:         wruntime.OutcomeReadyForPlan,
			Summary:         "outline reviewed",
			PrimaryArtifact: "thoughts/example/reviews/outline/review.md",
			Raw:             previousRaw,
		},
	})
	if err != nil {
		t.Fatalf("RenderStagePrompt error = %v", err)
	}
	for _, want := range []string{
		".pi/skills/qrspi-planning/SKILL.md",
		".pi/skills/q-plan/SKILL.md",
		"thoughts/example/AGENTS.md",
		"thoughts/example/reviews/outline/review.md",
		"Current node: plan",
		"Previous QRSPI result",
		"qrspi_result:",
		"outline reviewed",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRenderStagePromptDoesNotDumpManagerManifest(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderStagePrompt(PromptContext{
		Node:     def.Nodes[qrspi.NodeQuestion],
		PlanDir:  "thoughts/example",
		State:    ManagerState{SourceCwd: "/repo"},
		Manifest: "# q-manager Manifest\n\nProject-specific rules.",
	})
	if err != nil {
		t.Fatalf("RenderStagePrompt error = %v", err)
	}
	if strings.Contains(prompt, "Project manifest excerpt") || strings.Contains(prompt, "Project-specific rules.") {
		t.Fatalf("prompt should not dump manager manifest:\n%s", prompt)
	}
}

func TestRunRenderPromptWritesPrompt(t *testing.T) {
	previousRaw, err := json.Marshal(qrspi.Result{
		Project:  "github.com/CoreyCole/vamos",
		Stage:    string(qrspi.NodePlan),
		Status:   string(wruntime.StatusComplete),
		Outcome:  string(wruntime.OutcomeComplete),
		Summary:  qrspi.Summary{PlanGoal: "goal", StageCompleted: "plan ready", KeyDecisions: "review next"},
		Artifact: "thoughts/example/plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	projectRoot := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectRoot, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "docs", "q-manager.md"), []byte("# q-manager Manifest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{
		SourceCwd:        projectRoot,
		CanonicalPlanDir: filepath.Join(projectRoot, "thoughts/example"),
		Workflow: wruntime.State{
			LastResult: &wruntime.WorkflowResultSnapshot{
				SourceNodeID:    qrspi.NodePlan,
				Status:          "complete",
				Outcome:         wruntime.OutcomeComplete,
				Summary:         "plan ready",
				PrimaryArtifact: "thoughts/example/plan.md",
				Raw:             previousRaw,
			},
		},
	}
	if err := (FileStateStore{}).Save(stateFile, state); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = RunRenderPrompt(t.Context(), RenderPromptOptions{StateFile: stateFile, NodeID: "review-plan", PlanDir: "thoughts/example"}, deps{}, &out)
	if err != nil {
		t.Fatalf("RunRenderPrompt error = %v", err)
	}
	for _, want := range []string{".pi/skills/q-review/SKILL.md", "Previous QRSPI result", "qrspi_result:", "thoughts/example/plan.md", "plan ready"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "# q-manager Manifest") {
		t.Fatalf("output should not dump manager manifest:\n%s", out.String())
	}
}
