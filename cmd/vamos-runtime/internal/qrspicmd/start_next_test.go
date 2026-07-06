package qrspicmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestStartNextFirstLaunchInitializesStateAndStartsChild(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{panes: []string{"%144"}}
	var out bytes.Buffer
	result, err := RunStartNext(t.Context(), StartNextOptions{
		PlanDir:     fixture.planDir,
		ProjectRoot: fixture.projectRoot,
		ManagerPane: "%parent",
	}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, &out)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if result.StateFile == "" || result.PromptFile == "" || result.ActiveChild == nil {
		t.Fatalf("result = %+v, want state, prompt, and active child", result)
	}
	if result.CurrentNode != string(qrspi.NodeQuestion) ||
		result.ActiveChild.Stage != string(qrspi.NodeQuestion) {
		t.Fatalf("result node/child = %+v, want question", result)
	}
	if len(runner.started) != 1 || runner.started[0].PromptFile != result.PromptFile ||
		runner.started[0].Cwd != fixture.projectRoot {
		t.Fatalf("started = %+v, result prompt = %q", runner.started, result.PromptFile)
	}
	text := out.String()
	for _, want := range []string{"state:", "node: question", "prompt:", "started child: question (%144)", "vamos qrspi continue --state-file", "vamos qrspi steer-child --state-file"} {
		if !strings.Contains(text, want) {
			t.Fatalf("start-next output missing %q: %q", want, text)
		}
	}
	state := loadManagerState(t, result.StateFile)
	if state.ManagerPaneID != "%parent" || state.ActiveChild == nil ||
		state.ActiveChild.TmuxPaneID != "%144" {
		t.Fatalf("state = %+v", state)
	}
}

func TestStartNextCommandRegistration(t *testing.T) {
	out, err := executeManagerCommand(
		deps{StateRoot: func() (string, error) { return t.TempDir(), nil }},
		"start-next",
	)
	if err == nil || !strings.Contains(err.Error(), "plan-dir is required") {
		t.Fatalf("start-next err = %v, out = %q", err, out)
	}
}

func TestStartNextStateFileManagerPaneRebindsBeforeLaunch(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerPaneID:    "%old",
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
	})
	runner := &fakeChildRunner{panes: []string{"%child"}}
	result, err := RunStartNext(
		t.Context(),
		StartNextOptions{StateFile: stateFile, ManagerPane: "%new"},
		deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{}},
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if result.ActiveChild == nil || len(runner.started) != 1 ||
		runner.started[0].ParentPaneID != "%new" {
		t.Fatalf("result=%+v started=%+v", result, runner.started)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ManagerPaneID != "%new" || loaded.Delivery.ManagerPaneID != "%new" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestStartNextCurrentPaneAdoptsDeadStoredPane(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "state.json")
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ManagerPaneID:    "%dead",
		Delivery:         ManagerDeliveryState{ManagerPaneID: "%dead"},
		Workflow:         testWorkflowState(t, qrspi.NodeResearch, nil),
	})
	t.Setenv("TMUX_PANE", "%new")
	runner := &fakeChildRunner{panes: []string{"%child"}}
	result, err := RunStartNext(
		t.Context(),
		StartNextOptions{StateFile: stateFile},
		deps{
			Clock:  fixture.clock,
			Runner: runner,
			Tmux:   &recordingTmux{missingPanes: map[string]bool{"%dead": true}},
		},
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if result.ActiveChild == nil || len(runner.started) != 1 ||
		runner.started[0].ParentPaneID != "%new" {
		t.Fatalf("result=%+v started=%+v", result, runner.started)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ManagerPaneID != "%new" || loaded.Delivery.ManagerPaneID != "%new" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestStartNextActiveChildDoesNotLaunchDuplicate(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "active.json")
	active := &ChildRunRef{
		ID:         "child",
		Stage:      "plan",
		Cwd:        fixture.projectRoot,
		TmuxPaneID: "%old",
		SessionID:  "session",
	}
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		ActiveChild:      active,
		Workflow:         testWorkflowState(t, qrspi.NodePlan, nil),
	})
	runner := &fakeChildRunner{panes: []string{"%new"}}
	var out bytes.Buffer
	result, err := RunStartNext(
		t.Context(),
		StartNextOptions{StateFile: stateFile},
		deps{Runner: runner},
		&out,
	)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if result.ActiveChild == nil || result.ActiveChild.TmuxPaneID != "%old" ||
		result.StopReason != "active child already running" {
		t.Fatalf("result = %+v", result)
	}
	if len(runner.started) != 0 {
		t.Fatalf("started duplicate children: %+v", runner.started)
	}
	if !strings.Contains(out.String(), "do not launch duplicate child") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestStartNextLatestResultSeedLaunchesGraphSelectedNode(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "seed.json")
	saveManagerState(t, stateFile, ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeQuestion, nil),
	})
	seedFile := filepath.Join(fixture.dir, "seed.md")
	writeFile(
		t,
		seedFile,
		testResultYAML(
			"question",
			"complete",
			"complete",
			"thoughts/example/questions/q.md",
			"",
		),
	)
	runner := &fakeChildRunner{panes: []string{"%research"}}
	var out bytes.Buffer
	result, err := RunStartNext(
		t.Context(),
		StartNextOptions{StateFile: stateFile, LatestResultFile: seedFile},
		deps{Clock: fixture.clock, Runner: runner},
		&out,
	)
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	if result.CurrentNode != string(qrspi.NodeResearch) || result.ActiveChild == nil ||
		result.ActiveChild.Stage != string(qrspi.NodeResearch) {
		t.Fatalf("result = %+v, want research launch", result)
	}
	state := loadManagerState(t, stateFile)
	if state.Workflow.LastResult == nil ||
		state.Workflow.LastResult.SourceNodeID != qrspi.NodeQuestion ||
		state.Workflow.CurrentNodeID != qrspi.NodeResearch {
		t.Fatalf("state workflow = %+v", state.Workflow)
	}
	promptData, err := filepath.Abs(result.PromptFile)
	if err != nil || promptData == "" {
		t.Fatalf("prompt file path = %q, err = %v", result.PromptFile, err)
	}
	prompt := readText(t, result.PromptFile)
	if !strings.Contains(prompt, "Previous QRSPI result") ||
		!strings.Contains(prompt, "stage: question") {
		t.Fatalf("prompt missing previous result:\n%s", prompt)
	}
}

func TestStartNextInvalidSeedDoesNotMutateState(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "invalid-seed.json")
	initial := ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeQuestion, nil),
	}
	saveManagerState(t, stateFile, initial)
	seedFile := filepath.Join(fixture.dir, "invalid.md")
	writeFile(t, seedFile, "not a qrspi result")
	runner := &fakeChildRunner{panes: []string{"%new"}}
	_, err := RunStartNext(
		t.Context(),
		StartNextOptions{StateFile: stateFile, LatestResultFile: seedFile},
		deps{Runner: runner},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatalf("RunStartNext expected error")
	}
	if len(runner.started) != 0 {
		t.Fatalf("started child after invalid seed: %+v", runner.started)
	}
	state := loadManagerState(t, stateFile)
	if state.Workflow.CurrentNodeID != initial.Workflow.CurrentNodeID ||
		state.Workflow.LastResult != nil {
		t.Fatalf("state mutated after invalid seed: %+v", state.Workflow)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
