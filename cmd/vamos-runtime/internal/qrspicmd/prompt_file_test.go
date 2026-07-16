package qrspicmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func TestStartStateInitializesWithSameStateKeyAsInit(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	state, stateFile, err := resolveOrInitStartState(t.Context(), StartNextOptions{
		PlanDir:     fixture.planDir,
		ProjectRoot: fixture.projectRoot,
		ManagerPane: "%parent",
	}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock})
	if err != nil {
		t.Fatalf("resolveOrInitStartState error = %v", err)
	}
	wantKey := LockKey{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: filepath.Join(fixture.projectRoot, fixture.planDir),
	}
	if stateFile != StatePath(fixture.stateRoot, wantKey, state.ManagerRunID) {
		t.Fatalf("stateFile = %q, want state path for key", stateFile)
	}
	if state.ManagerPaneID != "%parent" ||
		state.Workflow.CurrentNodeID != qrspi.NodeQuestion {
		t.Fatalf("state = %+v", state)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.CanonicalPlanDir != state.CanonicalPlanDir {
		t.Fatalf(
			"loaded canonical plan dir = %q, want %q",
			loaded.CanonicalPlanDir,
			state.CanonicalPlanDir,
		)
	}
}

func TestStartStateFastPresetStartsAtOutline(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	state, _, err := resolveOrInitStartState(t.Context(), StartNextOptions{
		PlanDir:      fixture.planDir,
		ProjectRoot:  fixture.projectRoot,
		PolicyPreset: "fast",
	}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock})
	if err != nil {
		t.Fatalf("resolveOrInitStartState error = %v", err)
	}
	if state.Workflow.CurrentNodeID != qrspi.NodeOutline {
		t.Fatalf("current node = %q, want outline", state.Workflow.CurrentNodeID)
	}
	policy := qrspi.ParsePolicy(state.Workflow.Policy)
	if policy.EffectiveAdvanceMode() != qrspi.AdvanceModeAutopilot ||
		policy.EnablePlanReviews {
		t.Fatalf("policy = %#v, want fast", policy)
	}
}

func TestStartStateLoadsExplicitStateFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	want := ManagerState{
		SourceCwd:        dir,
		CanonicalPlanDir: filepath.Join(dir, "thoughts/example"),
		Workflow:         testWorkflowState(t, qrspi.NodePlan, nil),
	}
	saveManagerState(t, stateFile, want)
	got, gotFile, err := resolveOrInitStartState(
		t.Context(),
		StartNextOptions{StateFile: stateFile},
		deps{},
	)
	if err != nil {
		t.Fatalf("resolveOrInitStartState error = %v", err)
	}
	if gotFile != stateFile || got.Workflow.CurrentNodeID != qrspi.NodePlan {
		t.Fatalf("state = %+v, file = %q", got, gotFile)
	}
}

func TestStartStateSelectLaunchNode(t *testing.T) {
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)}
	node, err := selectLaunchNode(state, StartNextOptions{})
	if err != nil {
		t.Fatalf("selectLaunchNode error = %v", err)
	}
	if node.ID != qrspi.NodeDesign {
		t.Fatalf("node = %q, want design", node.ID)
	}
	node, err = selectLaunchNode(state, StartNextOptions{NodeID: string(qrspi.NodePlan)})
	if err != nil {
		t.Fatalf("selectLaunchNode override error = %v", err)
	}
	if node.ID != qrspi.NodePlan {
		t.Fatalf("node = %q, want plan", node.ID)
	}
	if _, err := selectLaunchNode(
		state,
		StartNextOptions{NodeID: "missing"},
	); err == nil ||
		!strings.Contains(err.Error(), `node "missing" is not in QRSPI definition`) {
		t.Fatalf("expected unknown node error, got %v", err)
	}
}

func TestDefaultChildCwd(t *testing.T) {
	state := ManagerState{SourceCwd: "/repo", ImplementationCwd: "/impl"}
	cases := []struct {
		name     string
		node     string
		override string
		want     string
	}{
		{
			name:     "override",
			node:     string(qrspi.NodeImplement),
			override: "/override",
			want:     "/override",
		},
		{name: "implementation", node: string(qrspi.NodeImplement), want: "/impl"},
		{
			name: "review implementation",
			node: string(qrspi.NodeReviewImplementation),
			want: "/impl",
		},
		{name: "verify", node: string(qrspi.NodeVerify), want: "/impl"},
		{name: "planning", node: string(qrspi.NodePlan), want: "/repo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := defaultChildCwd(state, wruntime.NodeID(tc.node), tc.override)
			if err != nil {
				t.Fatalf("defaultChildCwd error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("cwd = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPromptFileKeepsGraphNodeFilenameForResumeLaunch(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state", "run.json")
	state := ManagerState{
		SourceCwd:        dir,
		CanonicalPlanDir: filepath.Join(dir, "thoughts/example"),
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
	}
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteStagePromptFile(
		t.Context(),
		state,
		def.Nodes[qrspi.NodeResearch],
		PromptFileOptions{
			StateFile: stateFile,
			Timestamp: time.Unix(100, 123),
			Launch: &ChildLaunchIntent{
				Kind:            ChildLaunchResumeHandoff,
				NodeID:          qrspi.NodeResearch,
				SkillPath:       ".pi/skills/q-resume/SKILL.md",
				PrimaryArtifact: "/repo/thoughts/example/handoffs/research.md",
			},
		},
	)
	if err != nil {
		t.Fatalf("WriteStagePromptFile error = %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), "research-") {
		t.Fatalf("prompt filename = %q, want research graph node prefix", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ".pi/skills/q-resume/SKILL.md") {
		t.Fatalf("prompt missing q-resume skill:\n%s", data)
	}
}

func TestPromptFileWritesAtomicallyUnderStateDir(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state", "run.json")
	state := ManagerState{
		SourceCwd:        dir,
		CanonicalPlanDir: filepath.Join(dir, "thoughts/example"),
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
	}
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteStagePromptFile(
		t.Context(),
		state,
		def.Nodes[qrspi.NodeResearch],
		PromptFileOptions{StateFile: stateFile, Timestamp: time.Unix(100, 123)},
	)
	if err != nil {
		t.Fatalf("WriteStagePromptFile error = %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(filepath.Dir(stateFile), "prompts")) {
		t.Fatalf("prompt path = %q, want under state prompts dir", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ".pi/skills/q-research/SKILL.md") {
		t.Fatalf("prompt missing research skill:\n%s", string(data))
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("temporary prompt file was left behind: %s", entry.Name())
		}
	}
}
