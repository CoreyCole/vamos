package qrspicmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestManagerFlowQuestionToResearch(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := fixture.init(t)

	resultFile := filepath.Join(fixture.dir, "question-result.txt")
	writeFile(t, resultFile, testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))

	var decideOut strings.Builder
	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &decideOut); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	state := loadManagerState(t, stateFile)
	if state.Workflow.CurrentNodeID != qrspi.NodeResearch {
		t.Fatalf("current node = %q, want research", state.Workflow.CurrentNodeID)
	}
	if !strings.Contains(decideOut.String(), `"nextNode":"research"`) || !strings.Contains(decideOut.String(), `"startNext":true`) {
		t.Fatalf("decide output = %q", decideOut.String())
	}

	var promptOut strings.Builder
	if err := RunRenderPrompt(t.Context(), RenderPromptOptions{StateFile: stateFile, NodeID: "research", PlanDir: fixture.planDir}, deps{}, &promptOut); err != nil {
		t.Fatalf("RunRenderPrompt error = %v", err)
	}
	if !strings.Contains(promptOut.String(), ".pi/skills/q-research/SKILL.md") {
		t.Fatalf("prompt missing research skill:\n%s", promptOut.String())
	}
}

func TestManagerFlowWorkspaceSwitchesImplementationCwd(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	impl := filepath.Join(fixture.dir, "impl")
	if err := os.MkdirAll(impl, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(fixture.stateRoot, "workspace-state.json")
	state := ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: filepath.Join(fixture.projectRoot, fixture.planDir),
		ManagerRunID:     "workspace-run",
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeWorkspace, nil),
	}
	saveManagerState(t, stateFile, state)
	resultFile := filepath.Join(fixture.dir, "workspace-result.txt")
	writeFile(t, resultFile, testResultYAML("workspace", "complete", "complete", "thoughts/example/plan.md", "implementation_workspace: "+impl+"\n"))

	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	state = loadManagerState(t, stateFile)
	if state.Workflow.CurrentNodeID != qrspi.NodeImplement {
		t.Fatalf("current node = %q, want implement", state.Workflow.CurrentNodeID)
	}
	if state.ImplementationCwd != impl {
		t.Fatalf("implementation cwd = %q, want %q", state.ImplementationCwd, impl)
	}

	runner := &fakeChildRunner{writeResult: true}
	promptFile := filepath.Join(fixture.dir, "prompt.txt")
	writeFile(t, promptFile, "prompt")
	if err := RunChild(t.Context(), RunChildOptions{PlanDir: fixture.planDir, Stage: "implement", Cwd: impl, PromptFile: promptFile, StateFile: stateFile, Timeout: time.Second}, deps{Clock: fixture.clock, Runner: runner}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunChild error = %v", err)
	}
	if got := runner.started[0].Cwd; got != impl {
		t.Fatalf("child cwd = %q, want %q", got, impl)
	}
}

func TestRunInitCanStartAtAnyStage(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	impl := filepath.Join(fixture.dir, "impl")
	var out bytes.Buffer
	if err := RunInit(t.Context(), InitOptions{PlanDir: fixture.planDir, ProjectRoot: fixture.projectRoot, NodeID: string(qrspi.NodeReviewImplementation), ImplementationCwd: impl}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock}, &out); err != nil {
		t.Fatalf("RunInit error = %v", err)
	}
	state := loadManagerState(t, eventRefString(t, out.String(), "stateFile"))
	if state.Workflow.CurrentNodeID != qrspi.NodeReviewImplementation {
		t.Fatalf("current node = %q, want %q", state.Workflow.CurrentNodeID, qrspi.NodeReviewImplementation)
	}
	if state.ImplementationCwd != impl {
		t.Fatalf("implementation cwd = %q, want %q", state.ImplementationCwd, impl)
	}
	if !strings.Contains(out.String(), `"currentNode":"review-implementation"`) {
		t.Fatalf("init output = %q", out.String())
	}
}

func TestRunInitRejectsUnknownStage(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	err := RunInit(t.Context(), InitOptions{PlanDir: fixture.planDir, ProjectRoot: fixture.projectRoot, NodeID: "nope"}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `node "nope" is not in QRSPI definition`) {
		t.Fatalf("expected unknown node error, got %v", err)
	}
}

func TestManagerLockConflictStops(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	key := LockKey{RepoID: fixture.projectRoot, CanonicalPlanDir: filepath.Join(fixture.projectRoot, fixture.planDir)}
	store := FileStateStore{Root: fixture.stateRoot, Clock: fixture.clock}
	if _, err := store.AcquireLock(t.Context(), key, "other-manager", time.Hour); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := RunInit(t.Context(), InitOptions{PlanDir: fixture.planDir, ProjectRoot: fixture.projectRoot}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock}, &out)
	var conflict LockConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected LockConflictError, got %v (out %q)", err, out.String())
	}
	lock := readLock(t, LockPath(fixture.stateRoot, key))
	if lock.Owner != "other-manager" {
		t.Fatalf("lock owner = %q, want other-manager", lock.Owner)
	}
}

func TestDiscussPolicyDoesNotAutoStartCommandFlow(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	policyFile := filepath.Join(fixture.dir, "policy.json")
	writeFile(t, policyFile, `{"advanceMode":"discuss","enablePlanReviews":true,"invalidResultRetryLimit":1}`)
	var initOut bytes.Buffer
	if err := RunInit(t.Context(), InitOptions{PlanDir: fixture.planDir, ProjectRoot: fixture.projectRoot, PolicyFile: policyFile}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock}, &initOut); err != nil {
		t.Fatalf("RunInit error = %v", err)
	}
	stateFile := eventRefString(t, initOut.String(), "stateFile")
	resultFile := filepath.Join(fixture.dir, "question-result.txt")
	writeFile(t, resultFile, testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))

	var decideOut bytes.Buffer
	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &decideOut); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	if !strings.Contains(decideOut.String(), `"nextNode":"research"`) || !strings.Contains(decideOut.String(), `"startNext":false`) {
		t.Fatalf("decide output = %q", decideOut.String())
	}
}

func TestInvalidReviewPlanReadyForImplementStopsClearly(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "review-plan-state.json")
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil)}
	saveManagerState(t, stateFile, state)
	resultFile := filepath.Join(fixture.dir, "review-plan-result.txt")
	writeFile(t, resultFile, testResultYAML("review-plan", "complete", "ready-for-implement", "thoughts/example/reviews/plan/review.md", ""))

	err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "canonical QRSPI graph rejected result") || !strings.Contains(err.Error(), `outcome "ready-for-implement" is not valid`) {
		t.Fatalf("expected graph rejection, got %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeReviewPlan {
		t.Fatalf("current node mutated to %q", loaded.Workflow.CurrentNodeID)
	}
}

func TestEndToEndCommandSurface(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{writeResult: true}
	initOut, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, "init", "--plan-dir", fixture.planDir, "--project-root", fixture.projectRoot)
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	stateFile := eventRefString(t, initOut, "stateFile")
	if !strings.Contains(initOut, `"type":"initialized"`) {
		t.Fatalf("init output = %q", initOut)
	}

	renderOut, err := executeManagerCommand(deps{}, "render-prompt", "--state-file", stateFile, "--node", "question", "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("render command error = %v", err)
	}
	if !strings.Contains(renderOut, ".pi/skills/q-question/SKILL.md") {
		t.Fatalf("render output missing q-question:\n%s", renderOut)
	}
	promptFile := filepath.Join(fixture.dir, "prompt.txt")
	writeFile(t, promptFile, renderOut)

	runOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "run-child", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--stage", "question", "--cwd", fixture.projectRoot, "--prompt-file", promptFile, "--timeout", "1s")
	if err != nil {
		t.Fatalf("run-child command error = %v", err)
	}
	if !strings.Contains(runOut, `"type":"child_started"`) || !strings.Contains(runOut, `"type":"child_finished"`) {
		t.Fatalf("run-child output = %q", runOut)
	}
	for _, want := range []string{`"outputPath"`, `"sessionId"`, `"sessionDir"`, `"sessionPath"`, `"donePath"`, `"statusPath"`} {
		if !strings.Contains(runOut, want) {
			t.Fatalf("run-child output missing %s: %q", want, runOut)
		}
	}
	if strings.Contains(runOut, `"resultPath"`) {
		t.Fatalf("run-child output exposed default resultPath: %q", runOut)
	}
	state := loadManagerState(t, stateFile)
	if state.ActiveChild == nil {
		t.Fatalf("active child missing after run-child")
	}
	writeSessionTestFile(t, state.ActiveChild.SessionPath, sessionHeader(state.ActiveChild.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))+"\n")

	validateOut, err := executeManagerCommand(deps{}, "validate-result", "--state-file", stateFile, "--stage", "question", "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("validate command error = %v", err)
	}
	if !strings.Contains(validateOut, `"type":"validated"`) {
		t.Fatalf("validate output = %q", validateOut)
	}

	decideOut, err := executeManagerCommand(deps{}, "decide-next", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("decide command error = %v", err)
	}
	if !strings.Contains(decideOut, `"type":"decided"`) || !strings.Contains(decideOut, `"nextNode":"research"`) {
		t.Fatalf("decide output = %q", decideOut)
	}
}

type managerFlowFixture struct {
	dir         string
	projectRoot string
	stateRoot   string
	planDir     string
}

func newManagerFlowFixture(t *testing.T) managerFlowFixture {
	t.Helper()
	dir := t.TempDir()
	projectRoot := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectRoot, "thoughts", "example"), 0o755); err != nil {
		t.Fatal(err)
	}
	return managerFlowFixture{dir: dir, projectRoot: projectRoot, stateRoot: filepath.Join(dir, "state"), planDir: "thoughts/example"}
}

func (f managerFlowFixture) clock() time.Time               { return time.Unix(100, 123) }
func (f managerFlowFixture) stateRootFunc() (string, error) { return f.stateRoot, nil }

func (f managerFlowFixture) init(t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	if err := RunInit(t.Context(), InitOptions{PlanDir: f.planDir, ProjectRoot: f.projectRoot}, deps{StateRoot: f.stateRootFunc, Clock: f.clock}, &out); err != nil {
		t.Fatalf("RunInit error = %v", err)
	}
	return eventRefString(t, out.String(), "stateFile")
}

func executeManagerCommand(d deps, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := newCommand(d)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func eventRefString(t *testing.T, ndjson, key string) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(ndjson))
	for dec.More() {
		var event Event
		if err := dec.Decode(&event); err != nil {
			t.Fatalf("decode event from %q: %v", ndjson, err)
		}
		if value, ok := event.Ref[key].(string); ok {
			return value
		}
	}
	t.Fatalf("missing ref %q in %q", key, ndjson)
	return ""
}

func loadManagerState(t *testing.T, path string) ManagerState {
	t.Helper()
	state, err := (FileStateStore{}).Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func saveManagerState(t *testing.T, path string, state ManagerState) {
	t.Helper()
	if err := (FileStateStore{}).Save(path, state); err != nil {
		t.Fatal(err)
	}
}

func readLock(t *testing.T, path string) Lock {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	return lock
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
