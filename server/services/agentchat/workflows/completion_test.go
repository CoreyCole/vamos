package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestOnRunCompleteValidQRSPIResultAdvancesWorkflow(t *testing.T) {
	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{
		state:          state,
		assistantText:  validQuestionResultYAML(),
		artifactExists: true,
		run: db.AgentRun{
			ID:          "run-1",
			WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:    "thread-1",
			SessionID:   sql.NullString{String: "session-1", Valid: true},
			WorkflowNodeID: sql.NullString{
				String: string(qrspi.NodeQuestion),
				Valid:  true,
			},
		},
	}
	runner := &fakeRunner{}
	svc := &Service{Definitions: registry, Store: store, Runner: runner}

	err = svc.OnRunComplete(context.Background(), conversation.RunResult{
		RunID:       "run-1",
		ThreadID:    "thread-1",
		HeadEntryID: "assistant-1",
		SessionPath: "/tmp/session.jsonl",
	})
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if store.savedResult.SourceNodeID != qrspi.NodeQuestion ||
		store.savedResult.Status != "complete" {
		t.Fatalf("saved result = %#v", store.savedResult)
	}
	if store.savedState.CurrentNodeID != qrspi.NodeResearch ||
		store.savedState.Status != wruntime.WorkspaceStatusIdle {
		t.Fatalf(
			"saved state current/status = %q/%q",
			store.savedState.CurrentNodeID,
			store.savedState.Status,
		)
	}
	if !hasWorkflowNodeEvent(store.events, "workflow_node_ready", qrspi.NodeResearch) ||
		!hasWorkflowNodeEvent(store.events, "workflow_next_started", qrspi.NodeResearch) {
		t.Fatalf("events = %#v", store.events)
	}
	if len(runner.starts) != 1 || runner.starts[0].NodeID != qrspi.NodeResearch ||
		runner.starts[0].WorkspaceID != "workspace-1" {
		t.Fatalf("starts = %#v", runner.starts)
	}
}

func TestOnRunCompleteFallsBackToPersistedResultHeadEntry(t *testing.T) {
	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{
		state:          state,
		assistantText:  validQuestionResultYAML(),
		artifactExists: true,
		run: db.AgentRun{
			ID:          "run-1",
			WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:    "thread-1",
			ResultHeadEntryID: sql.NullString{
				String: "assistant-from-db",
				Valid:  true,
			},
			WorkflowNodeID: sql.NullString{
				String: string(qrspi.NodeQuestion),
				Valid:  true,
			},
		},
	}
	if err := (&Service{Definitions: registry, Store: store, Runner: &fakeRunner{}}).
		OnRunComplete(context.Background(), conversation.RunResult{RunID: "run-1"}); err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if store.finalHeadEntryID != "assistant-from-db" {
		t.Fatalf("FinalAssistantText headEntryID = %q", store.finalHeadEntryID)
	}
	if store.savedResult.SourceNodeID != qrspi.NodeQuestion {
		t.Fatalf("saved result = %#v", store.savedResult)
	}
}

func TestApplyQRSPIWorkspaceResultPersistsExecutionCwd(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	state := wruntime.State{CurrentNodeID: qrspi.NodeWorkspace}
	got, err := applyQRSPIWorkspaceResult(state, wruntime.WorkflowResult{
		SourceNodeID: qrspi.NodeWorkspace,
		Workspace:    workspaceDir,
	}, "/tmp/planning-checkout")
	if err != nil {
		t.Fatalf("applyQRSPIWorkspaceResult() error = %v", err)
	}
	if got.ExecutionCwd != workspaceDir {
		t.Fatalf("ExecutionCwd = %q, want %q", got.ExecutionCwd, workspaceDir)
	}
}

func TestApplyQRSPIWorkspaceResultRejectsPlanningCheckoutCwd(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	_, err := applyQRSPIWorkspaceResult(wruntime.State{}, wruntime.WorkflowResult{
		SourceNodeID: qrspi.NodeWorkspace,
		Workspace:    workspaceDir,
	}, workspaceDir)
	if err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf(
			"applyQRSPIWorkspaceResult() error = %v, want planning checkout rejection",
			err,
		)
	}
}

func TestOnRunCompleteIgnoresDuplicateWorkflowResult(t *testing.T) {
	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{run: db.AgentRun{
		ID:          "run-1",
		WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
		ThreadID:    "thread-1",
		WorkflowNodeID: sql.NullString{
			String: string(qrspi.NodeQuestion),
			Valid:  true,
		},
		WorkflowResultJson: sql.NullString{String: `{"status":"complete"}`, Valid: true},
	}}
	runner := &fakeRunner{}

	err = (&Service{Definitions: registry, Store: store, Runner: runner}).OnRunComplete(
		context.Background(),
		conversation.RunResult{RunID: "run-1", HeadEntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if store.loadStateCalls != 0 || len(runner.starts) != 0 {
		t.Fatalf(
			"duplicate was not ignored: loadStateCalls=%d starts=%#v",
			store.loadStateCalls,
			runner.starts,
		)
	}
}

func TestAdvanceHumanGateStartsExpectedNextNode(t *testing.T) {
	t.Parallel()

	state := wruntime.State{
		Type:          string(qrspi.AgentChatWorkflowType),
		CurrentNodeID: qrspi.NodeReviewOutline,
		Status:        wruntime.WorkspaceStatusWaitingHuman,
		Attempts:      map[wruntime.NodeID]int{},
		Nodes: map[wruntime.NodeID]wruntime.NodeState{
			qrspi.NodeHumanReviewOutline: {Status: wruntime.NodeStatusPending},
		},
		HumanGate: &wruntime.HumanGateState{
			From:   qrspi.NodeReviewOutline,
			To:     qrspi.NodeHumanReviewOutline,
			Reason: "outline approved by human",
		},
	}
	store := &fakeStore{state: state}
	runner := &fakeRunner{}

	runID, err := (&Service{Store: store, Runner: runner}).AdvanceHumanGate(
		t.Context(),
		"workspace-1",
		"person@example.com",
	)
	if err != nil {
		t.Fatalf("AdvanceHumanGate() error = %v", err)
	}
	if runID != "next-run" {
		t.Fatalf("runID = %q, want next-run", runID)
	}
	if store.savedState.Status != wruntime.WorkspaceStatusIdle ||
		store.savedState.CurrentNodeID != qrspi.NodeHumanReviewOutline ||
		store.savedState.HumanGate != nil {
		t.Fatalf("saved state = %#v", store.savedState)
	}
	if len(runner.starts) != 1 ||
		runner.starts[0].NodeID != qrspi.NodeHumanReviewOutline ||
		runner.starts[0].Attempt != 1 {
		t.Fatalf("starts = %#v", runner.starts)
	}
	if len(store.events) != 1 || store.events[0].Type != "workflow_human_gate_approved" {
		t.Fatalf("events = %#v", store.events)
	}
}

func TestAdvanceHumanGateRejectsWhenNotWaiting(t *testing.T) {
	t.Parallel()

	store := &fakeStore{state: wruntime.State{
		Type:          string(qrspi.AgentChatWorkflowType),
		CurrentNodeID: qrspi.NodeQuestion,
		Status:        wruntime.WorkspaceStatusIdle,
		Attempts:      map[wruntime.NodeID]int{},
		Nodes:         map[wruntime.NodeID]wruntime.NodeState{},
	}}
	runner := &fakeRunner{}

	_, err := (&Service{Store: store, Runner: runner}).AdvanceHumanGate(
		t.Context(),
		"workspace-1",
		"person@example.com",
	)
	if err == nil {
		t.Fatal("AdvanceHumanGate() error = nil, want error")
	}
	if len(runner.starts) != 0 || store.savedState.Type != "" {
		t.Fatalf(
			"unexpected side effects: starts=%#v saved=%#v",
			runner.starts,
			store.savedState,
		)
	}
}

func TestOnRunCompleteInvalidYAMLStartsCorrectionRun(t *testing.T) {
	t.Parallel()

	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{
		state:         state,
		assistantText: "not yaml",
		run: db.AgentRun{
			ID:          "run-1",
			WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:    "thread-1",
			WorkflowNodeID: sql.NullString{
				String: string(qrspi.NodeQuestion),
				Valid:  true,
			},
		},
	}
	runner := &fakeRunner{}

	err = (&Service{Definitions: registry, Store: store, Runner: runner}).OnRunComplete(
		t.Context(),
		conversation.RunResult{RunID: "run-1", HeadEntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if len(runner.starts) != 1 || runner.starts[0].NodeID != qrspi.NodeQuestion ||
		runner.starts[0].Attempt != 1 || runner.starts[0].Prompt == "" {
		t.Fatalf("starts = %#v", runner.starts)
	}
	if store.savedState.Status != wruntime.WorkspaceStatusRunning ||
		store.savedState.Attempts[qrspi.NodeQuestion] != 1 {
		t.Fatalf("saved state = %#v", store.savedState)
	}
	if len(store.events) != 1 || store.events[0].Type != "workflow_invalid_result_retry" {
		t.Fatalf("events = %#v", store.events)
	}
}

func TestOnRunCompleteInvalidYAMLUsesUpdatedRetryLimitPolicy(t *testing.T) {
	t.Parallel()

	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, mustQRSPIWorkflowPolicy(t, qrspi.Policy{
		AutoMode:                false,
		EnablePlanReviews:       true,
		InvalidResultRetryLimit: 1,
	}))
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	state.Policy = mustQRSPIWorkflowPolicy(t, qrspi.Policy{
		AutoMode:                false,
		EnablePlanReviews:       true,
		InvalidResultRetryLimit: 0,
	})
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{
		state:         state,
		assistantText: "not yaml",
		run: db.AgentRun{
			ID:          "run-1",
			WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:    "thread-1",
			WorkflowNodeID: sql.NullString{
				String: string(qrspi.NodeQuestion),
				Valid:  true,
			},
		},
	}
	runner := &fakeRunner{}

	err = (&Service{Definitions: registry, Store: store, Runner: runner}).OnRunComplete(
		t.Context(),
		conversation.RunResult{RunID: "run-1", HeadEntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if len(runner.starts) != 0 {
		t.Fatalf(
			"starts = %#v, want no retry after updated zero retry policy",
			runner.starts,
		)
	}
	if store.savedState.Status != wruntime.WorkspaceStatusError ||
		store.savedState.Nodes[qrspi.NodeQuestion].Status != wruntime.NodeStatusError {
		t.Fatalf("saved state = %#v, want workflow error", store.savedState)
	}
	if len(store.events) != 1 ||
		store.events[0].Type != "workflow_invalid_result_exhausted" {
		t.Fatalf("events = %#v, want exhausted", store.events)
	}
}

func TestOnRunCompleteInvalidYAMLExhaustionSetsWorkspaceError(t *testing.T) {
	t.Parallel()

	def, err := qrspi.Definition()
	if err != nil {
		t.Fatalf("qrspi.Definition() error = %v", err)
	}
	state, err := wruntime.InitialState(def, nil)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	state.Attempts[qrspi.NodeQuestion] = 1
	registry := wruntime.NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	store := &fakeStore{
		state:         state,
		assistantText: "not yaml",
		run: db.AgentRun{
			ID:              "run-1",
			WorkspaceID:     sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:        "thread-1",
			WorkflowAttempt: 1,
			WorkflowNodeID: sql.NullString{
				String: string(qrspi.NodeQuestion),
				Valid:  true,
			},
		},
	}
	runner := &fakeRunner{}

	err = (&Service{Definitions: registry, Store: store, Runner: runner}).OnRunComplete(
		t.Context(),
		conversation.RunResult{RunID: "run-1", HeadEntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if len(runner.starts) != 0 {
		t.Fatalf("starts = %#v", runner.starts)
	}
	if store.savedState.Status != wruntime.WorkspaceStatusError ||
		store.savedState.Nodes[qrspi.NodeQuestion].Status != wruntime.NodeStatusError {
		t.Fatalf("saved state = %#v", store.savedState)
	}
	if len(store.events) != 1 ||
		store.events[0].Type != "workflow_invalid_result_exhausted" {
		t.Fatalf("events = %#v", store.events)
	}
}

func TestOnRunCompleteIgnoresNonWorkflowRun(t *testing.T) {
	store := &fakeStore{
		run: db.AgentRun{
			ID:          "run-1",
			WorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
			ThreadID:    "thread-1",
		},
	}
	runner := &fakeRunner{}

	err := (&Service{Definitions: wruntime.NewRegistry(), Store: store, Runner: runner}).OnRunComplete(
		context.Background(),
		conversation.RunResult{RunID: "run-1", HeadEntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("OnRunComplete() error = %v", err)
	}
	if store.loadStateCalls != 0 || len(runner.starts) != 0 {
		t.Fatalf(
			"non-workflow run was not ignored: loadStateCalls=%d starts=%#v",
			store.loadStateCalls,
			runner.starts,
		)
	}
}

type fakeStore struct {
	run               db.AgentRun
	state             wruntime.State
	assistantText     string
	artifactExists    bool
	artifactExistence map[string]bool
	planningCwd       string

	loadStateCalls   int
	savedState       wruntime.State
	savedResult      wruntime.WorkflowResult
	events           []wruntime.Event
	finalHeadEntryID string
}

func (s *fakeStore) LoadWorkspaceState(context.Context, string) (wruntime.State, error) {
	s.loadStateCalls++
	return s.state, nil
}

func (s *fakeStore) SaveWorkspaceState(
	_ context.Context,
	_ string,
	state wruntime.State,
) error {
	s.savedState = state
	s.state = state
	return nil
}

func (s *fakeStore) LoadRun(context.Context, string) (db.AgentRun, error) {
	return s.run, nil
}

func (s *fakeStore) SaveRunResult(
	_ context.Context,
	_ string,
	result wruntime.WorkflowResult,
) error {
	s.savedResult = result
	if raw, err := json.Marshal(result); err == nil {
		s.run.WorkflowResultJson = sql.NullString{String: string(raw), Valid: true}
	}
	return nil
}

func (s *fakeStore) AppendWorkflowEvents(
	_ context.Context,
	_ string,
	_ db.AgentRun,
	events []wruntime.Event,
) error {
	s.events = append(s.events, events...)
	return nil
}

func (s *fakeStore) ArtifactExists(_ context.Context, _, relPath string) (bool, error) {
	if s.artifactExistence != nil {
		exists, ok := s.artifactExistence[relPath]
		if ok {
			return exists, nil
		}
	}
	return s.artifactExists, nil
}

func (s *fakeStore) FinalAssistantText(_ context.Context, _, headEntryID string) (string, error) {
	s.finalHeadEntryID = headEntryID
	return s.assistantText, nil
}

func (s *fakeStore) WorkspacePlanningCwd(context.Context, string) (string, error) {
	return s.planningCwd, nil
}

type fakeRunner struct {
	starts []StartNodeRunInput
}

func (r *fakeRunner) StartNodeRun(
	_ context.Context,
	input StartNodeRunInput,
) (string, error) {
	r.starts = append(r.starts, input)
	return "next-run", nil
}

func hasWorkflowNodeEvent(events []wruntime.Event, eventType string, nodeID wruntime.NodeID) bool {
	for _, event := range events {
		if event.Type == eventType && event.NodeID == nodeID {
			return true
		}
	}
	return false
}

func mustQRSPIWorkflowPolicy(t *testing.T, policy qrspi.Policy) []byte {
	t.Helper()
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("Marshal(policy) error = %v", err)
	}
	return encoded
}

func validQuestionResultYAML() string {
	return strings.Join([]string{
		"```yaml",
		"qrspi_result:",
		"  stage: \"question\"",
		"  status: \"complete\"",
		"  outcome: \"complete\"",
		"  policy:",
		"    auto_mode: false",
		"    enable_plan_reviews: true",
		"    invalid_result_retry_limit: 1",
		"  summary:",
		"    plan_goal: \"Build Agent Chat-native generic workflow runtime; QRSPI first.\"",
		"    stage_completed: \"Created research questions for runtime integration.\"",
		"    key_decisions: \"Proceed to research.\"",
		"  artifact: \"thoughts/creative-mode-agent/plans/example/questions/runtime.md\"",
		"  next:",
		"    steps:",
		"      - action: \"start_stage\"",
		"        param: \"q-research\"",
		"```",
	}, "\n")
}
