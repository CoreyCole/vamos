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
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
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

func TestDecideNextMarksOldChildPendingCleanupWhenStartNext(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := fixture.init(t)
	state := loadManagerState(t, stateFile)
	sessionPath := filepath.Join(fixture.dir, "old-session.jsonl")
	old := &ChildRunRef{ID: "old", Stage: "question", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "old-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	state.ActiveChild = old
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(old.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))+"\n")

	var decideOut strings.Builder
	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, PlanDir: fixture.planDir}, deps{}, &decideOut); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.PendingCleanupChild == nil || loaded.PendingCleanupChild.TmuxPaneID != "%old" {
		t.Fatalf("pending cleanup = %#v, want old child", loaded.PendingCleanupChild)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("active child = %#v, want preserved old child", loaded.ActiveChild)
	}
	if !strings.Contains(decideOut.String(), `"startNext":true`) {
		t.Fatalf("decide output = %q", decideOut.String())
	}
}

func TestDecideNextHumanGatePreservesOldChildWithoutPendingCleanup(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "design-state.json")
	sessionPath := filepath.Join(fixture.dir, "design-session.jsonl")
	old := &ChildRunRef{ID: "old", Stage: "design", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "design-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	saveManagerState(t, stateFile, ManagerState{ActiveChild: old, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)})
	writeSessionTestFile(t, sessionPath, sessionHeader(old.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("design", "needs_human", "", "thoughts/example/design.md", ""))+"\n")

	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, PlanDir: fixture.planDir}, deps{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.PendingCleanupChild != nil {
		t.Fatalf("pending cleanup = %#v, want nil", loaded.PendingCleanupChild)
	}
	if loaded.ActiveChild == nil || loaded.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("active child = %#v, want preserved old child", loaded.ActiveChild)
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

func TestReviewPlanReadyForImplementContinuesToImplementation(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "review-plan-state.json")
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil)}
	saveManagerState(t, stateFile, state)
	resultFile := filepath.Join(fixture.dir, "review-plan-result.txt")
	writeFile(t, resultFile, testResultYAML("review-plan", "complete", "ready-for-implement", "thoughts/example/reviews/plan/review.md", ""))

	var out bytes.Buffer
	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &out); err != nil {
		t.Fatalf("RunDecideNext error = %v", err)
	}
	if !strings.Contains(out.String(), `"nextNode":"implement"`) || !strings.Contains(out.String(), `"startNext":true`) {
		t.Fatalf("decide output = %q", out.String())
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.Workflow.CurrentNodeID != qrspi.NodeImplement {
		t.Fatalf("current node = %q, want implement", loaded.Workflow.CurrentNodeID)
	}
}

func TestReviewPlanGenericPositiveNormalizesAcrossManagerCommands(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "review-plan-normalize-state.json")
	state := ManagerState{CanonicalPlanDir: fixture.planDir, SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil)}
	saveManagerState(t, stateFile, state)
	resultFile := filepath.Join(fixture.dir, "review-plan-generic-result.txt")
	writeFile(t, resultFile, testResultYAML("review-plan", "complete", "complete", "thoughts/example/reviews/plan/review.md", ""))

	var out bytes.Buffer
	if err := RunDecideNext(t.Context(), DecideNextOptions{StateFile: stateFile, ResultFile: resultFile, PlanDir: fixture.planDir}, deps{}, &out); err != nil {
		t.Fatalf("RunDecideNext generic positive error = %v", err)
	}
	if !strings.Contains(out.String(), `"nextNode":"workspace"`) || !strings.Contains(out.String(), `"startNext":true`) {
		t.Fatalf("decide output = %q", out.String())
	}

	seedState := ManagerState{CanonicalPlanDir: fixture.planDir, SourceCwd: fixture.projectRoot, Workflow: testWorkflowState(t, qrspi.NodeReviewPlan, nil)}
	parsed, err := applyLatestResultSeed(&seedState, testResultYAML("review-plan", "complete", "complete", "thoughts/example/reviews/plan/review.md", ""))
	if err != nil {
		t.Fatalf("applyLatestResultSeed generic positive error = %v", err)
	}
	if parsed.Result.Outcome != "ready-for-workspace" || seedState.Workflow.CurrentNodeID != qrspi.NodeWorkspace {
		t.Fatalf("outcome/current = %q/%q, want ready-for-workspace/workspace", parsed.Result.Outcome, seedState.Workflow.CurrentNodeID)
	}
}

func TestWakeDrivenManagerLoopCleansOldPaneAfterNextLaunch(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{panes: []string{"%old", "%new"}}
	initOut, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, "init", "--plan-dir", fixture.planDir, "--project-root", fixture.projectRoot, "--manager-pane", "%parent")
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	stateFile := eventRefString(t, initOut, "stateFile")

	renderOut, err := executeManagerCommand(deps{}, "render-prompt", "--state-file", stateFile, "--node", "question", "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("render command error = %v", err)
	}
	promptFile := filepath.Join(fixture.dir, "prompt.txt")
	writeFile(t, promptFile, renderOut)

	runOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "run-child", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--stage", "question", "--cwd", fixture.projectRoot, "--prompt-file", promptFile, "--timeout", "0")
	if err != nil {
		t.Fatalf("run-child command error = %v", err)
	}
	if !strings.Contains(runOut, `"type":"child_started"`) || strings.Contains(runOut, `"type":"child_finished"`) {
		t.Fatalf("timeout 0 run output = %q", runOut)
	}
	state := loadManagerState(t, stateFile)
	if state.ManagerPaneID != "%parent" || state.ActiveChild == nil || state.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("state after child start = %+v", state)
	}
	writeFile(t, state.ActiveChild.StatusPath, `{"event":"agent_end","stage":"question","childId":"`+state.ActiveChild.ID+`","wakeTarget":"%parent"}`)
	writeFile(t, state.ActiveChild.DonePath, "")
	writeSessionTestFile(t, filepath.Join(state.ActiveChild.SessionDir, "session.jsonl"), sessionHeader(state.ActiveChild.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))+"\n")

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
	if !strings.Contains(decideOut, `"nextNode":"research"`) || !strings.Contains(decideOut, `"startNext":true`) {
		t.Fatalf("decide output = %q", decideOut)
	}
	state = loadManagerState(t, stateFile)
	if state.PendingCleanupChild == nil || state.PendingCleanupChild.TmuxPaneID != "%old" {
		t.Fatalf("pending cleanup after decide = %#v", state.PendingCleanupChild)
	}

	nextPromptFile := filepath.Join(fixture.dir, "research-prompt.txt")
	writeFile(t, nextPromptFile, "research prompt")
	tmux := &recordingTmux{}
	nextOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner, Tmux: tmux}, "run-child", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--stage", "research", "--cwd", fixture.projectRoot, "--prompt-file", nextPromptFile, "--timeout", "0")
	if err != nil {
		t.Fatalf("next run-child command error = %v", err)
	}
	if !strings.Contains(nextOut, `"type":"child_started"`) || !strings.Contains(nextOut, `"type":"child_cleaned"`) {
		t.Fatalf("next run output = %q", nextOut)
	}
	state = loadManagerState(t, stateFile)
	if state.ActiveChild == nil || state.ActiveChild.Stage != "research" || state.ActiveChild.TmuxPaneID != "%new" || state.PendingCleanupChild != nil {
		t.Fatalf("state after next launch = %+v", state)
	}
	if len(tmux.kills) != 1 || tmux.kills[0].ID != "%old" {
		t.Fatalf("kills = %#v, want %%old", tmux.kills)
	}
	if len(tmux.layouts) != 1 || tmux.layouts[0].pane.ID != "%new" || tmux.layouts[0].layout != "even-horizontal" {
		t.Fatalf("layouts = %#v, want even-horizontal on %%new", tmux.layouts)
	}
}

func TestContinueValidResultStartsNextChildAndCleansOldPane(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{panes: []string{"%old", "%new"}}
	initOut, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, "init", "--plan-dir", fixture.planDir, "--project-root", fixture.projectRoot, "--manager-pane", "%parent")
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	stateFile := eventRefString(t, initOut, "stateFile")

	promptFile := filepath.Join(fixture.dir, "prompt.txt")
	writeFile(t, promptFile, "question prompt")
	if _, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "run-child", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--stage", "question", "--cwd", fixture.projectRoot, "--prompt-file", promptFile, "--timeout", "0"); err != nil {
		t.Fatalf("run-child command error = %v", err)
	}
	state := loadManagerState(t, stateFile)
	writeFile(t, state.ActiveChild.StatusPath, `{"event":"agent_end","stage":"question","childId":"`+state.ActiveChild.ID+`","wakeTarget":"%parent"}`)
	writeFile(t, state.ActiveChild.DonePath, "")
	writeSessionTestFile(t, filepath.Join(state.ActiveChild.SessionDir, "session.jsonl"), sessionHeader(state.ActiveChild.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))+"\n")

	tmux := &recordingTmux{}
	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner, Tmux: tmux}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"validated: question complete", "next: research", "started child: research"} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue output missing %q: %q", want, continueOut)
		}
	}
	if strings.Contains(continueOut, "rawYaml") || strings.Contains(continueOut, "workflow") {
		t.Fatalf("continue output exposed raw decision dump: %q", continueOut)
	}
	state = loadManagerState(t, stateFile)
	if state.ActiveChild == nil || state.ActiveChild.Stage != "research" || state.ActiveChild.TmuxPaneID != "%new" || state.PendingCleanupChild != nil {
		t.Fatalf("state after continue = %+v", state)
	}
	if len(tmux.kills) != 1 || tmux.kills[0].ID != "%old" {
		t.Fatalf("kills = %#v, want %%old", tmux.kills)
	}
}

func TestContinueOutlineNeedsHumanPrintsSteeringGuidance(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "outline-state.json")
	sessionPath := filepath.Join(fixture.dir, "outline-session.jsonl")
	active := &ChildRunRef{ID: "outline-child", Stage: "outline", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "outline-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	state := ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		ManagerRunID:     "outline-run",
		SourceCwd:        fixture.projectRoot,
		ActiveChild:      active,
		Workflow:         testWorkflowState(t, qrspi.NodeOutline, nil),
	}
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(active.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("outline", "needs_human", "", "thoughts/example/design.md", ""))+"\n")

	runner := &fakeChildRunner{panes: []string{"%new"}}
	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"validated: outline needs_human", "artifact: thoughts/example/design.md", "stop: waiting human", "question: test goal", "feedback: vamos qrspi steer-child --state-file " + stateFile + " --feedback-file <file>"} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue output missing %q: %q", want, continueOut)
		}
	}
	for _, forbidden := range []string{"rawYaml", "workflow", "started child"} {
		if strings.Contains(continueOut, forbidden) {
			t.Fatalf("continue output contains %q: %q", forbidden, continueOut)
		}
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("active child = %#v, want preserved old child", loaded.ActiveChild)
	}
	if len(runner.started) != 0 {
		t.Fatalf("runner starts = %d, want none", len(runner.started))
	}
}

func TestContinueOutlineNeedsHumanNDJSONIncludesManagerRefs(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "outline-ndjson-state.json")
	sessionPath := filepath.Join(fixture.dir, "outline-ndjson-session.jsonl")
	active := &ChildRunRef{ID: "outline-child", Stage: "outline", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "outline-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, ManagerRunID: "outline-run", SourceCwd: fixture.projectRoot, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeOutline, nil)}
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(active.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("outline", "needs_human", "", "thoughts/example/design.md", ""))+"\n")

	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--output", "ndjson")
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{`"validated":true`, `"managerNeeded":true`, `"waitingHuman":true`, `"artifact":"thoughts/example/design.md"`, `"SuggestedFeedbackCommand":"vamos qrspi steer-child --state-file ` + stateFile + ` --feedback-file \u003cfile\u003e"`} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue NDJSON missing %q: %q", want, continueOut)
		}
	}
}

func TestContinueBlockedResultPrintsConciseStop(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "verify-state.json")
	sessionPath := filepath.Join(fixture.dir, "verify-session.jsonl")
	active := &ChildRunRef{ID: "verify-child", Stage: "verify", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "verify-session", SessionDir: fixture.dir, SessionPath: sessionPath}
	state := ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		ManagerRunID:     "verify-run",
		SourceCwd:        fixture.projectRoot,
		ActiveChild:      active,
		Workflow:         testWorkflowState(t, qrspi.NodeVerify, nil),
	}
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(active.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("verify", "blocked", "", "thoughts/example/verify.md", ""))+"\n")

	runner := &fakeChildRunner{panes: []string{"%new"}}
	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"validated: verify blocked", "artifact: thoughts/example/verify.md", "stop: result blocked"} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue output missing %q: %q", want, continueOut)
		}
	}
	for _, forbidden := range []string{"rawYaml", "workflow", "started child"} {
		if strings.Contains(continueOut, forbidden) {
			t.Fatalf("continue output contains %q: %q", forbidden, continueOut)
		}
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("active child = %#v, want preserved old child", loaded.ActiveChild)
	}
	if len(runner.started) != 0 {
		t.Fatalf("runner starts = %d, want none", len(runner.started))
	}
}

func TestContinueCommandInvalidResultRepromptsSameChild(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{panes: []string{"%old"}}
	initOut, err := executeManagerCommand(deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, "init", "--plan-dir", fixture.planDir, "--project-root", fixture.projectRoot, "--manager-pane", "%parent")
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	stateFile := eventRefString(t, initOut, "stateFile")
	promptFile := filepath.Join(fixture.dir, "prompt.txt")
	writeFile(t, promptFile, "design prompt")
	if _, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner}, "run-child", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--stage", "question", "--cwd", fixture.projectRoot, "--prompt-file", promptFile, "--timeout", "0"); err != nil {
		t.Fatalf("run-child command error = %v", err)
	}
	state := loadManagerState(t, stateFile)
	writeFile(t, state.ActiveChild.DonePath, "")
	writeSessionTestFile(t, filepath.Join(state.ActiveChild.SessionDir, "session.jsonl"), sessionHeader(state.ActiveChild.SessionID, fixture.projectRoot)+"\n"+assistantLine("plain invalid result")+"\n")

	tmux := &recordingTmux{}
	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner, Tmux: tmux}, "continue", "--state-file", stateFile)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	if !strings.Contains(continueOut, "retry: reprompted active child") {
		t.Fatalf("continue output = %q", continueOut)
	}
	if len(tmux.pastes) != 1 || tmux.pastes[0].pane.ID != "%old" {
		t.Fatalf("pastes = %#v, want one paste to %%old", tmux.pastes)
	}
	if len(runner.started) != 1 {
		t.Fatalf("runner starts = %d, want only initial child", len(runner.started))
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.ActiveChild == nil || loaded.ActiveChild.ID != state.ActiveChild.ID || loaded.ActiveChild.TmuxPaneID != "%old" {
		t.Fatalf("active child changed: %+v", loaded.ActiveChild)
	}
}

func TestContinueInvalidResultRetryExhaustionEmitsManagerNotice(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "retry-exhausted-state.json")
	sessionPath := filepath.Join(fixture.dir, "retry-exhausted-session.jsonl")
	active := &ChildRunRef{ID: "plan-child", Stage: "plan", Cwd: fixture.projectRoot, TmuxPaneID: "%old", SessionID: "plan-session", SessionDir: fixture.dir, SessionPath: sessionPath, ValidationRetryCount: 1, LastRepromptAttempt: 1}
	state := ManagerState{RepoID: fixture.projectRoot, CanonicalPlanDir: fixture.planDir, ManagerRunID: "retry-run", SourceCwd: fixture.projectRoot, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodePlan, nil)}
	saveManagerState(t, stateFile, state)
	writeSessionTestFile(t, sessionPath, sessionHeader(active.SessionID, fixture.projectRoot)+"\n"+assistantLine("HTTP/2 200 response headers, no fenced result")+"\n")

	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"retry: exhausted", "stop: invalid result after retry limit", "guidance: Inspect child output/artifacts", "next: vamos qrspi continue --state-file " + stateFile, "feedback: vamos qrspi steer-child --state-file " + stateFile + " --feedback-file <file>"} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue output missing %q: %q", want, continueOut)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(stateFile), "validation-recoveries.jsonl")); err != nil {
		t.Fatalf("validation recovery log missing: %v", err)
	}
}

func TestValidatedWakeHappyPathEndToEnd(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	runner := &fakeChildRunner{panes: []string{"%child", "%next"}}
	start, err := RunStartNext(t.Context(), StartNextOptions{PlanDir: fixture.planDir, ProjectRoot: fixture.projectRoot, ManagerPane: "%parent"}, deps{StateRoot: fixture.stateRootFunc, Clock: fixture.clock, Runner: runner}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunStartNext error = %v", err)
	}
	state := loadManagerState(t, start.StateFile)
	if state.ActiveChild == nil || state.ActiveChild.ValidationStatusPath == "" {
		t.Fatalf("active child = %+v, want validation status path", state.ActiveChild)
	}
	sessionPath := state.ActiveChild.SessionPath
	if sessionPath == "" {
		sessionPath = filepath.Join(state.ActiveChild.SessionDir, "session.jsonl")
	}
	writeSessionTestFile(t, sessionPath, sessionHeader(state.ActiveChild.SessionID, fixture.projectRoot)+"\n"+assistantLine(testResultYAML("question", "complete", "complete", "thoughts/example/questions/q.md", ""))+"\n")

	tmux := &recordingTmux{}
	status, err := RunChildComplete(t.Context(), ChildCompletionOptions{StateFile: start.StateFile, ChildID: state.ActiveChild.ID}, deps{Tmux: tmux}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunChildComplete error = %v", err)
	}
	if !status.Validated || status.Wake.Mode != "deliver" || len(tmux.pastes) != 1 || !strings.Contains(tmux.pastes[0].text, "q_manager_child_wake") {
		t.Fatalf("status=%+v pastes=%#v", status, tmux.pastes)
	}
	if _, err := os.Stat(state.ActiveChild.ValidationStatusPath); err != nil {
		t.Fatalf("validation status not written: %v", err)
	}

	continueOut, err := executeManagerCommand(deps{Clock: fixture.clock, Runner: runner, Tmux: &recordingTmux{}}, "continue", "--state-file", start.StateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"validated: question complete", "next: research", "started child: research"} {
		if !strings.Contains(continueOut, want) {
			t.Fatalf("continue output missing %q: %q", want, continueOut)
		}
	}
}

func TestProjectManagerActionCardFromSharedNextAction(t *testing.T) {
	state := ManagerState{CanonicalPlanDir: "thoughts/example", ActiveChild: &ChildRunRef{Stage: "plan", ValidationRetryCount: 0}}
	invalid := ProjectManagerActionCard(semantic.ProjectInvalidResultAction(qrspi.NodePlan, "missing qrspi_result", false), state, "/tmp/state.json")
	if invalid == nil || invalid.Kind != ActionInvalidChildYAML || invalid.SafeCommand == "" || invalid.RequiresHuman {
		t.Fatalf("invalid card = %#v", invalid)
	}
	human := ProjectManagerActionCard(semantic.NextAction{Kind: semantic.NextActionWaitHuman, Severity: "info", CurrentNodeID: qrspi.NodeOutline, Status: "needs_human", PrimaryArtifact: "thoughts/example/design.md", RecoveryReason: "child requested human input"}, state, "/tmp/state.json")
	if human == nil || human.Kind != ActionHumanGate || !human.RequiresHuman || !strings.Contains(human.SafeCommand, "steer-child") {
		t.Fatalf("human card = %#v", human)
	}
	if ProjectManagerActionCard(semantic.NextAction{Kind: semantic.NextActionStartNext}, state, "/tmp/state.json") != nil {
		t.Fatalf("start-next action should not require a manager card")
	}
}

func TestContinueActionCardsAreConciseAndDeterministic(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.stateRoot, "desync-state.json")
	state := ManagerState{
		RepoID:           fixture.projectRoot,
		CanonicalPlanDir: fixture.planDir,
		SourceCwd:        fixture.projectRoot,
		Workflow:         testWorkflowState(t, qrspi.NodeReviewPlan, nil),
	}
	saveManagerState(t, stateFile, state)
	out, err := executeManagerCommand(deps{Clock: fixture.clock}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"action: active_child_conflict", "recommended: start or inspect the graph-selected child", "safe command: vamos qrspi start-next --state-file " + stateFile} {
		if !strings.Contains(out, want) {
			t.Fatalf("action card output missing %q: %q", want, out)
		}
	}
	for _, forbidden := range []string{"rawYaml", "workflow:", "{\"type\":"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("action card output contains %q: %q", forbidden, out)
		}
	}
	loaded := loadManagerState(t, stateFile)
	if loaded.LastActionCard == nil || loaded.LastActionCard.Kind != ActionActiveChildConflict {
		t.Fatalf("last action card = %+v", loaded.LastActionCard)
	}
}

func TestContinueChildLaunchFailedTextActionCard(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.dir, "launch-failed-state.json")
	active := childHealthRef(fixture.dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\nFlags:\n  --session string\n  --name string\n")
	saveManagerState(t, stateFile, ManagerState{CanonicalPlanDir: fixture.planDir, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)})

	out, err := executeManagerCommand(deps{Clock: fixture.clock, Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	for _, want := range []string{"action: child_launch_failed", "summary: child exited before qrspi_result", "evidence: exitCode: 1", "evidence: output tail: Error: unknown option --session-id", "evidence: full output: " + active.OutputPath, "safe command: vamos qrspi repair-state --state-file " + stateFile + " --clear-failed-child --relaunch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("launch-failed output missing %q: %q", want, out)
		}
	}
	for _, forbidden := range []string{"Usage:", "pi [flags]", "--session string", "--name string"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("launch-failed output contains usage spam %q: %q", forbidden, out)
		}
	}
}

func TestQRSPIRuntimeErrorsDoNotPrintUsage(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.dir, "launch-failed-state.json")
	active := childHealthRef(fixture.dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\nFlags:\n  --session string\n")
	saveManagerState(t, stateFile, ManagerState{CanonicalPlanDir: fixture.planDir, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)})

	stdout, stderr, err := executeManagerCommandWithErr(deps{Clock: fixture.clock, Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir)
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	combined := stdout + stderr
	for _, forbidden := range []string{"Usage:", "Flags:", "--session string"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("runtime command output contains usage spam %q: stdout=%q stderr=%q", forbidden, stdout, stderr)
		}
	}
	if !strings.Contains(stdout, "action: child_launch_failed") {
		t.Fatalf("stdout missing action card: %q", stdout)
	}
}

func TestContinueChildLaunchFailedNDJSONActionCard(t *testing.T) {
	fixture := newManagerFlowFixture(t)
	stateFile := filepath.Join(fixture.dir, "launch-failed-state.json")
	active := childHealthRef(fixture.dir)
	writeFile(t, active.StatusPath, `{"exitCode":1}`)
	writeFile(t, active.DonePath, "")
	writeFile(t, active.OutputPath, "Error: unknown option --session-id\nUsage:\n  pi [flags]\n")
	saveManagerState(t, stateFile, ManagerState{CanonicalPlanDir: fixture.planDir, ActiveChild: active, Workflow: testWorkflowState(t, qrspi.NodeDesign, nil)})

	out, err := executeManagerCommand(deps{Clock: fixture.clock, Tmux: &recordingTmux{missingPanes: map[string]bool{"%9": true}}}, "continue", "--state-file", stateFile, "--plan-dir", fixture.planDir, "--output", "ndjson")
	if err != nil {
		t.Fatalf("continue command error = %v", err)
	}
	var event Event
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&event); err != nil {
		t.Fatalf("decode event from %q: %v", out, err)
	}
	if event.Type != "manager_action" || event.ActionCard == nil || event.ActionCard.Kind != ActionChildLaunchFailed {
		t.Fatalf("event = %#v", event)
	}
	if !containsLine(event.ActionCard.Evidence, "exitCode: 1") || !containsLine(event.ActionCard.Evidence, "full output: "+active.OutputPath) || !containsLine(event.ActionCard.Evidence, "output tail: Error: unknown option --session-id") {
		t.Fatalf("evidence = %#v", event.ActionCard.Evidence)
	}
}

func TestBuildPreflightFailedCard(t *testing.T) {
	card := BuildPreflightFailedCard(PiCompatibilityReport{OK: false, PiBinary: "pi", Problems: []PreflightProblem{{Summary: "Pi CLI missing required q-manager flag", Evidence: "--session-id"}}}, "/tmp/state.json")
	if card == nil || card.Kind != ActionPiCompatibilityFailed || !strings.Contains(strings.Join(card.Evidence, "\n"), "--session-id") || !strings.Contains(card.SafeCommand, "doctor --state-file /tmp/state.json") {
		t.Fatalf("card = %#v", card)
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
	t.Setenv("TMUX_PANE", "")
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
	out, _, err := executeManagerCommandWithErr(d, args...)
	return out, err
}

func executeManagerCommandWithErr(d deps, args ...string) (string, string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newCommand(d)
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), stderr.String(), err
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
