package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
)

func TestApplyQRSPIProjectionsAdvancesWorkflowWithoutTranscriptEntries(t *testing.T) {
	service := newTestAgentChatService(t)
	service.workflowService.(*agentchatworkflows.Service).Runner = agentchatworkflows.NoopRunner{}
	workspace, planDirRel := mustCreateProjectionQRSPIWorkspace(t, service)

	_, err := service.queries.UpsertQRSPIProjectionPending(t.Context(), db.UpsertQRSPIProjectionPendingParams{
		ID:                  "projection-valid",
		SourceEventID:       "event-valid",
		SessionID:           nullableString("pi-session-1"),
		SessionArtifactPath: nullableString(filepath.Join(t.TempDir(), "session.jsonl")),
		PlanDir:             planDirRel,
		WorkflowNodeID:      nullableString(string(qrspi.NodeQuestion)),
		Stage:               nullableString(string(qrspi.NodeQuestion)),
		Status:              nullableString(string(wruntime.StatusComplete)),
		Outcome:             nullableString(string(wruntime.OutcomeComplete)),
		Artifact:            nullableString(planDirRel + "/questions/question.md"),
		ResultJson:          validProjectionQRSPIResultJSON(t, qrspi.NodeQuestion, wruntime.OutcomeComplete, planDirRel+"/questions/question.md"),
		EventTime:           time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertQRSPIProjectionPending() error = %v", err)
	}

	result, err := service.ApplyQRSPIProjections(t.Context(), QRSPIProjectionApplyInput{MaxResults: 10})
	if err != nil {
		t.Fatalf("ApplyQRSPIProjections() error = %v", err)
	}
	if result.Applied != 1 || result.Skipped != 0 || result.Failed != 0 || !result.Changed {
		t.Fatalf("ApplyQRSPIProjections() = %+v, want one applied", result)
	}
	stored, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	var state wruntime.State
	if err := json.Unmarshal([]byte(stored.WorkflowStateJson.String), &state); err != nil {
		t.Fatalf("Unmarshal(workflow state) error = %v", err)
	}
	if state.CurrentNodeID != qrspi.NodeResearch || state.LastResult == nil || state.LastResult.SourceNodeID != qrspi.NodeQuestion {
		t.Fatalf("state = %+v, want question result adopted and research current", state)
	}
	entries, err := service.queries.TestSupportCountAgentEntries(t.Context())
	if err != nil {
		t.Fatalf("TestSupportCountAgentEntries() error = %v", err)
	}
	if entries != 0 {
		t.Fatalf("agent_entries = %d, want no transcript rows", entries)
	}
	threadID := metadataProjectionThreadID("event-valid")
	if _, err := service.queries.GetAgentThread(t.Context(), threadID); err != nil {
		t.Fatalf("GetAgentThread(metadata provenance) error = %v", err)
	}
}

func TestApplyQRSPIProjectionsMarksInvalidResultFailedAndContinues(t *testing.T) {
	service := newTestAgentChatService(t)
	service.workflowService.(*agentchatworkflows.Service).Runner = agentchatworkflows.NoopRunner{}
	workspace, planDirRel := mustCreateProjectionQRSPIWorkspace(t, service)

	base := time.Now().UTC()
	if _, err := service.queries.UpsertQRSPIProjectionPending(t.Context(), db.UpsertQRSPIProjectionPendingParams{
		ID:             "projection-invalid",
		SourceEventID:  "event-invalid",
		PlanDir:        planDirRel,
		WorkflowNodeID: nullableString(string(qrspi.NodeQuestion)),
		ResultJson:     `{}`,
		EventTime:      base,
	}); err != nil {
		t.Fatalf("Upsert invalid projection: %v", err)
	}
	if _, err := service.queries.UpsertQRSPIProjectionPending(t.Context(), db.UpsertQRSPIProjectionPendingParams{
		ID:             "projection-valid",
		SourceEventID:  "event-valid",
		PlanDir:        planDirRel,
		WorkflowNodeID: nullableString(string(qrspi.NodeQuestion)),
		ResultJson:     validProjectionQRSPIResultJSON(t, qrspi.NodeQuestion, wruntime.OutcomeComplete, planDirRel+"/questions/question.md"),
		EventTime:      base.Add(time.Second),
	}); err != nil {
		t.Fatalf("Upsert valid projection: %v", err)
	}

	result, err := service.ApplyQRSPIProjections(t.Context(), QRSPIProjectionApplyInput{MaxResults: 10})
	if err != nil {
		t.Fatalf("ApplyQRSPIProjections() error = %v", err)
	}
	if result.Applied != 1 || result.Failed != 1 || result.Skipped != 0 {
		t.Fatalf("ApplyQRSPIProjections() = %+v, want one failed and one applied", result)
	}
	pending, err := service.queries.ListPendingQRSPIProjections(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListPendingQRSPIProjections() error = %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending projections = %d, want none", len(pending))
	}
	stored, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	var state wruntime.State
	if err := json.Unmarshal([]byte(stored.WorkflowStateJson.String), &state); err != nil {
		t.Fatalf("Unmarshal(workflow state) error = %v", err)
	}
	if state.CurrentNodeID != qrspi.NodeResearch {
		t.Fatalf("CurrentNodeID = %s, want research", state.CurrentNodeID)
	}
}

func TestApplyQRSPIProjectionsLeavesBusyRowPending(t *testing.T) {
	service := newTestAgentChatService(t)
	service.workflowService.(*agentchatworkflows.Service).Runner = agentchatworkflows.NoopRunner{}
	_, planDirRel := mustCreateProjectionQRSPIWorkspace(t, service)
	if _, err := service.queries.UpsertQRSPIProjectionPending(t.Context(), db.UpsertQRSPIProjectionPendingParams{
		ID:             "projection-busy",
		SourceEventID:  "event-busy",
		PlanDir:        planDirRel,
		WorkflowNodeID: nullableString(string(qrspi.NodeQuestion)),
		ResultJson:     validProjectionQRSPIResultJSON(t, qrspi.NodeQuestion, wruntime.OutcomeComplete, planDirRel+"/questions/question.md"),
		EventTime:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Upsert projection: %v", err)
	}
	service.qrspiProjectionBeforeApplyForTest = func() error { return sql.ErrTxDone }
	result, err := service.ApplyQRSPIProjections(t.Context(), QRSPIProjectionApplyInput{MaxResults: 10})
	if err != nil {
		t.Fatalf("ApplyQRSPIProjections(non-busy) error = %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("ApplyQRSPIProjections(non-busy) = %+v, want failed", result)
	}

	service = newTestAgentChatService(t)
	service.workflowService.(*agentchatworkflows.Service).Runner = agentchatworkflows.NoopRunner{}
	_, planDirRel = mustCreateProjectionQRSPIWorkspace(t, service)
	if _, err := service.queries.UpsertQRSPIProjectionPending(t.Context(), db.UpsertQRSPIProjectionPendingParams{
		ID:             "projection-busy",
		SourceEventID:  "event-busy",
		PlanDir:        planDirRel,
		WorkflowNodeID: nullableString(string(qrspi.NodeQuestion)),
		ResultJson:     validProjectionQRSPIResultJSON(t, qrspi.NodeQuestion, wruntime.OutcomeComplete, planDirRel+"/questions/question.md"),
		EventTime:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Upsert projection: %v", err)
	}
	service.qrspiProjectionBeforeApplyForTest = func() error { return errorsNew("database is locked") }
	result, err = service.ApplyQRSPIProjections(t.Context(), QRSPIProjectionApplyInput{MaxResults: 10})
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("ApplyQRSPIProjections(busy) error = %v, want database locked", err)
	}
	if result.Applied != 0 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("ApplyQRSPIProjections(busy) = %+v, want no row state changes", result)
	}
	pending, err := service.queries.ListPendingQRSPIProjections(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPendingQRSPIProjections() error = %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "projection-busy" {
		t.Fatalf("pending projections = %+v, want busy row pending", pending)
	}
}

func mustCreateProjectionQRSPIWorkspace(t *testing.T, service *Service) (db.Workspace, string) {
	t.Helper()
	adapter := service.workflowService.(*agentchatworkflows.Service)
	def, ok := adapter.Definitions.Get(qrspi.AgentChatWorkflowType)
	if !ok {
		t.Fatal("qrspi definition not registered")
	}
	policy, err := json.Marshal(qrspi.Policy{AutoMode: true, EnablePlanReviews: true, InvalidResultRetryLimit: 1})
	if err != nil {
		t.Fatalf("Marshal(policy): %v", err)
	}
	state, err := wruntime.InitialState(def, policy)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	rawState, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal(state): %v", err)
	}
	planDirRel := "thoughts/agent/plans/2026-06-02_plan"
	planDirAbs := filepath.Join(service.projectRoot, filepath.FromSlash(planDirRel))
	if err := ensureDir(filepath.Join(planDirAbs, "questions")); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	workspace, err := service.CreateWorkspace(t.Context(), WorkspaceCreateInput{
		UserEmail:     "user@example.com",
		Title:         "QRSPI Projection",
		RootDocPath:   planDirAbs,
		Cwd:           planDirAbs,
		WorkflowType:  WorkspaceWorkflowQRSPI,
		WorkflowState: rawState,
		Source:        WorkspaceSourceImported,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return workspace, planDirRel
}

func validProjectionQRSPIResultJSON(t *testing.T, node wruntime.NodeID, outcome wruntime.ResultOutcome, artifact string) string {
	t.Helper()
	payload := map[string]any{
		"qrspi_result": map[string]any{
			"project": "github.com/CoreyCole/vamos",
			"stage":   string(node),
			"status":  string(wruntime.StatusComplete),
			"outcome": string(outcome),
			"policy": map[string]any{
				"auto_mode":                  true,
				"enable_plan_reviews":        true,
				"invalid_result_retry_limit": 1,
			},
			"summary": map[string]any{
				"plan_goal":       "Goal.",
				"stage_completed": "Node complete.",
				"key_decisions":   "Continue.",
			},
			"artifact": artifact,
			"next": map[string]any{
				"steps": []map[string]any{{"action": "start_stage", "param": "q-research"}},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(result): %v", err)
	}
	return string(raw)
}

func errorsNew(message string) error { return &projectionTestError{message: message} }

type projectionTestError struct{ message string }

func (e *projectionTestError) Error() string { return e.message }
