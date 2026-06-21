package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
)

func TestParsePiSessionJSONLRejectsMalformedAndMissingParent(t *testing.T) {
	service := newTestAgentChatService(t)
	path := filepath.Join(service.piSessionsDir, "bad.jsonl")
	writePiSessionFile(
		t,
		path,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"e1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"message","id":"e2","parentId":"missing","timestamp":"2026-04-30T12:00:02Z","message":{"role":"assistant","content":"no"}}`,
	)
	if _, err := ParsePiSessionJSONL(
		path,
	); err == nil ||
		!strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("ParsePiSessionJSONL() error = %v, want missing parent", err)
	}
}

func TestImportPiSessionAutoAdoptsSingleTouchedPlan(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_demo",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "session.jsonl")
	artifactPath := filepath.Join(planDir, "design.md")
	writePiSessionFile(t, sessionPath, piImportFixtureLines(artifactPath)...)

	result, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath, UserEmail: "viewer@example.com"},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if result.Status != "hydrated" || result.WorkspaceID == "" || result.ThreadID == "" ||
		result.ImportedHeadEntry != "tool-1" {
		t.Fatalf("result = %+v, want hydrated workspace/thread/head", result)
	}
	if result.Stats.BatchCount < 1 || result.Stats.EntriesRead != 3 ||
		result.Stats.EntriesImported != 3 || result.Stats.EntriesSkipped != 0 {
		t.Fatalf("result stats = %+v, want first import counts", result.Stats)
	}
	workspace, err := service.queries.GetWorkspace(
		context.Background(),
		result.WorkspaceID,
	)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if workspace.UserEmail != "user@example.com" || workspace.RootDocPath != planDir {
		t.Fatalf("workspace = %+v, want user/root", workspace)
	}

	reimportByOtherViewer, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{
			SessionPath: sessionPath,
			UserEmail:   "other-viewer@example.com",
		},
	)
	if err != nil {
		t.Fatalf("cross-viewer reimport ImportPiSession() error = %v", err)
	}
	if reimportByOtherViewer.WorkspaceID != result.WorkspaceID ||
		reimportByOtherViewer.ThreadID != result.ThreadID {
		t.Fatalf(
			"cross-viewer reimport = %+v, want workspace/thread %s/%s",
			reimportByOtherViewer,
			result.WorkspaceID,
			result.ThreadID,
		)
	}

	reimport, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("reimport ImportPiSession() error = %v", err)
	}
	if reimport.ThreadID != result.ThreadID ||
		reimport.ImportedHeadEntry != result.ImportedHeadEntry {
		t.Fatalf("reimport = %+v, want same thread/head as %+v", reimport, result)
	}
	if reimport.Stats.EntriesRead != 3 || reimport.Stats.EntriesImported != 0 ||
		reimport.Stats.EntriesSkipped != 3 {
		t.Fatalf("reimport stats = %+v, want duplicate skips", reimport.Stats)
	}
}

func TestImportPiSessionDivergenceCreatesSiblingThread(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_demo",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "session.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		piImportFixtureLines(filepath.Join(planDir, "design.md"))...)

	first, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          first.ThreadID,
			HeadEntryID: nullString("web-head"),
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}
	second, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("second ImportPiSession() error = %v", err)
	}
	if !second.Diverged || second.Status != "diverged" ||
		second.ThreadID == first.ThreadID {
		t.Fatalf("second = %+v, want diverged sibling", second)
	}
}

func TestImportPiSessionLargeSessionUsesMultipleBatches(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_large",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "large.jsonl")
	entryCount := defaultPiSessionImportBatchSize + 5
	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(filepath.Join(planDir, "design.md"), entryCount, -1)...,
	)

	result, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if result.Status != "hydrated" ||
		result.ImportedHeadEntry != piEntryID(entryCount-1) {
		t.Fatalf("result = %+v, want hydrated final head", result)
	}
	if result.Stats.BatchCount != 2 || result.Stats.EntriesRead != entryCount ||
		result.Stats.EntriesImported != entryCount || result.Stats.EntriesSkipped != 0 {
		t.Fatalf("stats = %+v, want two full import batches", result.Stats)
	}
}

func TestImportPiSessionPartialBatchFailureCanRetry(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_retry",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "retry.jsonl")
	entryCount := defaultPiSessionImportBatchSize + 5
	badIndex := defaultPiSessionImportBatchSize + 1
	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(
			filepath.Join(planDir, "design.md"),
			entryCount,
			badIndex,
		)...,
	)

	failed, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid timestamp") {
		t.Fatalf("ImportPiSession() error = %v, want invalid timestamp", err)
	}
	if failed.Status != "failed" || failed.Stats.FailedLine == 0 ||
		failed.Stats.EntriesImported != defaultPiSessionImportBatchSize {
		t.Fatalf("failed result = %+v, want partial failure stats", failed)
	}
	session, err := service.queries.GetAgentSession(
		t.Context(),
		failed.SessionID,
	)
	if err != nil {
		t.Fatalf("GetAgentSession() error = %v", err)
	}
	if session.ProjectionState != "failed" || session.LastImportedAt.Valid ||
		!session.LastError.Valid {
		t.Fatalf("session = %+v, want failed without last_imported_at", session)
	}
	if !strings.Contains(session.MetadataJson.String, `"failed_line"`) {
		t.Fatalf("metadata_json = %s, want failed_line", session.MetadataJson.String)
	}

	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(filepath.Join(planDir, "design.md"), entryCount, -1)...,
	)
	retry, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("retry ImportPiSession() error = %v", err)
	}
	if retry.Status != "hydrated" ||
		retry.Stats.EntriesSkipped != defaultPiSessionImportBatchSize ||
		retry.Stats.EntriesImported != entryCount-defaultPiSessionImportBatchSize {
		t.Fatalf("retry = %+v, want duplicate skip recovery", retry)
	}
}

func TestImportPiSessionRejectsSymlinkEscapeBeforeScan(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writePiSessionFile(
		t,
		outside,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`not-json`,
	)
	link := filepath.Join(service.piSessionsDir, "escape-import.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: link},
	); err == nil || !strings.Contains(err.Error(), "outside Pi sessions dir") {
		t.Fatalf("ImportPiSession() error = %v, want path rejection before scan", err)
	}
	count, err := service.queries.TestSupportCountAgentSessions(t.Context())
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Fatalf("agent_sessions count = %d, want no scan/import side effects", count)
	}
}

func TestInternalPiSessionImportRequiresToken(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	body, _ := json.Marshal(
		map[string]string{
			"session_path": filepath.Join(service.piSessionsDir, "missing.jsonl"),
		},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/agent-chat/import-session",
		bytes.NewReader(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.HandleInternalPiSessionImport(c); err == nil {
		t.Fatal("HandleInternalPiSessionImport() error = nil, want unauthorized")
	}
}

func TestImportPiSessionRejectsOwnerlessGlobalSessionReuse(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user@example.com", "plans", "2026-04-30_ownerless")
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "ownerless.jsonl")
	writePiSessionFile(t, sessionPath, piImportFixtureLines(filepath.Join(planDir, "design.md"))...)
	_, err := service.queries.UpsertAgentSessionIndex(t.Context(), db.UpsertAgentSessionIndexParams{
		ID:                "ownerless-global",
		IdentityKind:      "global_pi",
		ArtifactPath:      nullString(sessionPath),
		Agent:             "pi",
		FileSize:          1,
		LastIndexedOffset: 1,
		ProjectionState:   "unassigned",
	})
	if err != nil {
		t.Fatalf("UpsertAgentSessionIndex: %v", err)
	}
	_, err = service.ImportPiSession(t.Context(), SessionImportInput{SessionPath: sessionPath})
	if err == nil || !strings.Contains(err.Error(), "no owner") {
		t.Fatalf("ImportPiSession() error = %v, want ownerless reuse rejection", err)
	}
}

func TestValidatePiSessionPathRejectsSymlinkEscape(t *testing.T) {
	service := newTestAgentChatService(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writePiSessionFile(
		t,
		outside,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
	)
	link := filepath.Join(service.piSessionsDir, "escape.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := service.validatePiSessionPath(link); err == nil {
		t.Fatal("validatePiSessionPath() error = nil, want symlink escape rejection")
	}
}

func TestLegacyPiSessionScanRespectsMaxFiles(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("session-%02d.jsonl", i)), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(session): %v", err)
		}
	}
	var visited int
	if err := walkPiSessionFilesBounded(t.Context(), root, 2, 0, 0, func(string, os.FileInfo) error {
		visited++
		return nil
	}); err != nil {
		t.Fatalf("walkPiSessionFilesBounded() error = %v", err)
	}
	if visited != 2 {
		t.Fatalf("visited = %d, want 2", visited)
	}
}

func TestLegacyPiSessionScanRespectsDuration(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("session-%02d.jsonl", i)), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(session): %v", err)
		}
	}
	var visited int
	if err := walkPiSessionFilesBounded(t.Context(), root, 0, 0, time.Millisecond, func(string, os.FileInfo) error {
		visited++
		time.Sleep(2 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatalf("walkPiSessionFilesBounded() error = %v", err)
	}
	if visited == 0 || visited >= 10 {
		t.Fatalf("visited = %d, want bounded nonzero count", visited)
	}
}

func TestImportAdoptablePiSessionsImportsTerminalQRSPIWorkspace(t *testing.T) {
	service := newTestAgentChatService(t)
	service.workflowService.(*agentchatworkflows.Service).Runner = agentchatworkflows.NoopRunner{}
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-05-24_qrspi-terminal",
	)
	if err := ensureDir(filepath.Join(planDir, "questions")); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "questions", "research.md"), []byte("# Research\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(question): %v", err)
	}
	workspace, err := service.CreateWorkspace(t.Context(), WorkspaceCreateInput{
		UserEmail:    "user@example.com",
		Title:        "QRSPI Terminal",
		RootDocPath:  planDir,
		WorkflowType: WorkspaceWorkflowQRSPI,
		Source:       WorkspaceSourceImported,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "qrspi.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		piSessionWithQRSPIResult(
			planDir,
			strings.Join([]string{
				"qrspi_result:",
				"  stage: \"question\"",
				"  status: \"complete\"",
				"  outcome: \"complete\"",
				"  policy:",
				"    auto_mode: false",
				"    enable_plan_reviews: true",
				"    invalid_result_retry_limit: 1",
				"  summary:",
				"    plan_goal: \"Goal.\"",
				"    stage_completed: \"Questions done.\"",
				"    key_decisions: \"Research next.\"",
				"  artifact: \"questions/research.md\"",
				"  next:",
				"    steps:",
				"      - action: \"start_stage\"",
				"        param: \"q-research\"",
			}, "\n"),
		)...,
	)

	result, err := service.ImportAdoptablePiSessions(t.Context())
	if err != nil {
		t.Fatalf("ImportAdoptablePiSessions() error = %v", err)
	}
	if result.ImportedSessions != 1 || result.AdoptedQRSPIWorkspaces != 1 || !result.Changed {
		t.Fatalf("result = %+v, want imported/adopted changed", result)
	}
	stored, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !stored.SelectedThreadID.Valid || stored.SelectedThreadID.String == "" {
		t.Fatalf("SelectedThreadID = %v, want imported terminal thread", stored.SelectedThreadID)
	}
	var state wruntime.State
	if err := json.Unmarshal([]byte(stored.WorkflowStateJson.String), &state); err != nil {
		t.Fatalf("Unmarshal(workflow state): %v", err)
	}
	if state.CurrentNodeID != qrspi.NodeResearch || state.LastResult == nil ||
		state.LastResult.SourceNodeID != qrspi.NodeQuestion {
		t.Fatalf("state = %+v, want question adopted and research current", state)
	}
}

func TestImportAdoptablePiSessionsAdoptsLatestQRSPIResult(t *testing.T) {
	service := newTestAgentChatService(t)
	adapter := service.workflowService.(*agentchatworkflows.Service)
	adapter.Runner = agentchatworkflows.NoopRunner{}
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
	state.CurrentNodeID = qrspi.NodeReviewOutline
	state.PendingNextNodeID = qrspi.NodeReviewOutline
	state.Status = wruntime.WorkspaceStatusIdle
	state.Nodes[qrspi.NodeQuestion] = wruntime.NodeState{Status: wruntime.NodeStatusComplete}
	state.Nodes[qrspi.NodeResearch] = wruntime.NodeState{Status: wruntime.NodeStatusComplete}
	state.Nodes[qrspi.NodeDesign] = wruntime.NodeState{Status: wruntime.NodeStatusComplete}
	state.Nodes[qrspi.NodeOutline] = wruntime.NodeState{Status: wruntime.NodeStatusComplete}
	rawState, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal(state): %v", err)
	}
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-05-24_qrspi-auto",
	)
	if err := ensureDir(filepath.Join(planDir, "reviews", "outline-review")); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "reviews", "outline-review", "review.md"), []byte("# Review\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(review): %v", err)
	}
	workspace, err := service.CreateWorkspace(t.Context(), WorkspaceCreateInput{
		UserEmail:     "user@example.com",
		Title:         "QRSPI Auto",
		RootDocPath:   planDir,
		WorkflowType:  WorkspaceWorkflowQRSPI,
		WorkflowState: rawState,
		Source:        WorkspaceSourceImported,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "qrspi-auto.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		piSessionWithQRSPIResult(
			planDir,
			strings.Join([]string{
				"qrspi_result:",
				"  stage: \"review-outline\"",
				"  status: \"complete\"",
				"  outcome: \"ready-for-plan\"",
				"  policy:",
				"    auto_mode: true",
				"    enable_plan_reviews: true",
				"    invalid_result_retry_limit: 1",
				"  summary:",
				"    plan_goal: \"Goal.\"",
				"    stage_completed: \"Outline reviewed.\"",
				"    key_decisions: \"Plan next.\"",
				"  artifact: \"reviews/outline-review/review.md\"",
				"  next:",
				"    steps:",
				"      - action: \"start_stage\"",
				"        param: \"q-plan\"",
			}, "\n"),
		)...,
	)

	result, err := service.ImportAdoptablePiSessions(t.Context())
	if err != nil {
		t.Fatalf("ImportAdoptablePiSessions() error = %v", err)
	}
	if result.AdoptedQRSPIWorkspaces != 1 {
		t.Fatalf("result = %+v, want adopted", result)
	}
	stored, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	var got wruntime.State
	if err := json.Unmarshal([]byte(stored.WorkflowStateJson.String), &got); err != nil {
		t.Fatalf("Unmarshal(workflow state): %v", err)
	}
	if got.CurrentNodeID != qrspi.NodePlan || got.Nodes[qrspi.NodeReviewOutline].Status != wruntime.NodeStatusComplete {
		t.Fatalf("state = %+v, want outline review complete and plan current", got)
	}
}

func TestImportAdoptablePiSessionsDoesNotExportWebRunsToTerminal(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-05-24_web-only",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	_, err := service.CreateWorkspace(t.Context(), WorkspaceCreateInput{
		UserEmail:    "user@example.com",
		Title:        "Web only",
		RootDocPath:  planDir,
		WorkflowType: WorkspaceWorkflowQRSPI,
		Source:       WorkspaceSourceWeb,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	before, err := os.ReadDir(service.piSessionsDir)
	if err != nil {
		t.Fatalf("ReadDir(before): %v", err)
	}
	result, err := service.ImportAdoptablePiSessions(t.Context())
	if err != nil {
		t.Fatalf("ImportAdoptablePiSessions() error = %v", err)
	}
	after, err := os.ReadDir(service.piSessionsDir)
	if err != nil {
		t.Fatalf("ReadDir(after): %v", err)
	}
	if result.ImportedSessions != 0 || result.AdoptedQRSPIWorkspaces != 0 || result.Changed {
		t.Fatalf("result = %+v, want zero", result)
	}
	if len(after) != len(before) {
		t.Fatalf("session dir entries changed from %d to %d", len(before), len(after))
	}
}

func piSessionWithQRSPIResult(cwd, assistant string) []string {
	assistant = strings.ReplaceAll(assistant, "`", "\\`")
	payload, err := json.Marshal(assistant)
	if err != nil {
		panic(err)
	}
	cwd = filepath.ToSlash(cwd)
	return []string{
		`{"type":"session","id":"s-qrspi","timestamp":"2026-05-24T12:00:00Z","cwd":"` + cwd + `"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-05-24T12:00:01Z","message":{"role":"user","content":"continue qrspi"}}`,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-05-24T12:00:02Z","message":{"role":"assistant","content":` + string(payload) + `}}`,
	}
}

func piImportFixtureLines(artifactPath string) []string {
	return []string{
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"write design"}}`,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-30T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"write","arguments":{"path":"` + filepath.ToSlash(
			artifactPath,
		) + `","content":"# Demo"}}]}}`,
		`{"type":"message","id":"tool-1","parentId":"assistant-1","timestamp":"2026-04-30T12:00:03Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"write","content":"Wrote file","isError":false}}`,
	}
}

func largePiImportFixtureLines(
	artifactPath string,
	entryCount, badTimestampIndex int,
) []string {
	lines := []string{
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
	}
	base := time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC)
	for idx := range entryCount {
		id := piEntryID(idx)
		parentField := ""
		if idx > 0 {
			parentField = `,"parentId":"` + piEntryID(idx-1) + `"`
		}
		timestamp := base.Add(time.Duration(idx) * time.Second).Format(time.RFC3339)
		if idx == badTimestampIndex {
			timestamp = "not-a-timestamp"
		}
		switch idx {
		case 0:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"user","content":"write design"}}`,
			)
		case 1:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"write","arguments":{"path":"`+filepath.ToSlash(
					artifactPath,
				)+`","content":"# Demo"}}]}}`,
			)
		case 2:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"toolResult","toolCallId":"call-1","toolName":"write","content":"Wrote file","isError":false}}`,
			)
		default:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"assistant","content":"message `+id+`"}}`,
			)
		}
	}
	return lines
}

func piEntryID(idx int) string {
	return fmt.Sprintf("entry-%03d", idx)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
