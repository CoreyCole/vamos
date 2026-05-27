//go:build !integration || unit

package agentchat

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type fakeTemporalStarter struct {
	lastWorkflowID string
	lastInput      any
}

func (f *fakeTemporalStarter) StartWorkflow(
	_ context.Context,
	workflowID string,
	_, input any,
) (string, error) {
	f.lastWorkflowID = workflowID
	f.lastInput = input
	return "temporal-run-1", nil
}

func TestStartQRSPIWorkflowCreatesWorkspaceThreadAndFirstNodeRun(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	service.thoughtsRoot = t.TempDir()
	artifactRoot := filepath.Join(service.thoughtsRoot, "plans", "generic-workflow")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(artifactRoot): %v", err)
	}

	runID, err := service.StartWorkflow(t.Context(), StartWorkflowInput{
		UserEmail:    "user@example.com",
		Title:        "Generic workflow system design",
		RootDocPath:  artifactRoot,
		WorkflowType: WorkspaceWorkflowQRSPI,
	})
	if err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if strings.TrimSpace(runID) == "" {
		t.Fatal("StartWorkflow() returned empty run id")
	}

	run, err := service.queries.GetAgentRun(t.Context(), runID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if !run.WorkflowNodeID.Valid || run.WorkflowNodeID.String != "question" {
		t.Fatalf("WorkflowNodeID = %v, want question", run.WorkflowNodeID)
	}
	if run.WorkflowAttempt != 1 {
		t.Fatalf("WorkflowAttempt = %d, want 1", run.WorkflowAttempt)
	}
	if !strings.Contains(run.PromptText, "q-question/SKILL.md") ||
		!strings.Contains(run.PromptText, "<stage>question</stage>") {
		t.Fatalf("prompt = %q, want q-question skill and question stage", run.PromptText)
	}

	workspace, err := service.queries.GetWorkspace(
		t.Context(),
		run.WorkspaceID.String,
	)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if workspace.WorkflowType != string(WorkspaceWorkflowQRSPI) {
		t.Fatalf("WorkflowType = %q, want qrspi", workspace.WorkflowType)
	}
	if !workspace.SelectedThreadID.Valid ||
		workspace.SelectedThreadID.String != run.ThreadID {
		t.Fatalf(
			"SelectedThreadID = %v, want run thread %s",
			workspace.SelectedThreadID,
			run.ThreadID,
		)
	}
	var state map[string]any
	if err := json.Unmarshal(
		[]byte(workspace.WorkflowStateJson.String),
		&state,
	); err != nil {
		t.Fatalf("Unmarshal(workflow state): %v", err)
	}
	attempts, ok := state["attempts"].(map[string]any)
	if !ok || state["current_node_id"] != "question" || state["status"] != "running" ||
		attempts["question"] != float64(1) {
		t.Fatalf("state = %+v, want running question attempt 1", state)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf(
			"temporal input type = %T, want conversation.RunInput",
			fakeTemporal.lastInput,
		)
	}
	if input.RunID != run.ID || input.ThreadID != run.ThreadID ||
		input.WorkspaceID != workspace.ID || input.ChatSessionID == "" {
		t.Fatalf(
			"temporal input ids = %+v, want created workflow run ids and chat session",
			input,
		)
	}
	storedWorkspace, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !storedWorkspace.CurrentSessionID.Valid ||
		storedWorkspace.CurrentSessionID.String != input.ChatSessionID {
		t.Fatalf(
			"current chat session = %v, want %s",
			storedWorkspace.CurrentSessionID,
			input.ChatSessionID,
		)
	}
}

func TestStartNodeRunUsesEffectiveCwdForNewThread(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusIdle,
	)
	effectiveCwd := t.TempDir()

	runID, err := service.StartNodeRun(t.Context(), agentchatworkflows.StartNodeRunInput{
		WorkspaceID: workspace.ID,
		NodeID:      qrspi.NodeImplement,
		Prompt:      "implement next slice",
		Attempt:     1,
		Cwd:         effectiveCwd,
	})
	if err != nil {
		t.Fatalf("StartNodeRun() error = %v", err)
	}
	run, err := service.queries.GetAgentRun(t.Context(), runID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	thread, err := service.queries.GetAgentThread(t.Context(), run.ThreadID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if thread.Cwd != effectiveCwd {
		t.Fatalf("thread cwd = %q, want %q", thread.Cwd, effectiveCwd)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf(
			"temporal input type = %T, want conversation.RunInput",
			fakeTemporal.lastInput,
		)
	}
	if input.Cwd != effectiveCwd {
		t.Fatalf("temporal cwd = %q, want %q", input.Cwd, effectiveCwd)
	}
}

func TestStartWorkspaceThreadCreatesSessionRunAndBoundedRunInput(t *testing.T) {
	service := newTestAgentChatService(t)
	service.callbackBaseURL = "http://127.0.0.1:4301"
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	thread, run, session, err := service.StartWorkspaceThread(
		context.Background(),
		workspace.ID,
		"user@example.com",
		"hello workspace",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread() error = %v", err)
	}
	primary, ok, err := service.ResolvePrimaryWorkspaceForThread(context.Background(), "user@example.com", thread.ID)
	if err != nil || !ok || primary.ID != workspace.ID {
		t.Fatalf("primary workspace = (%v, %v, %v), want %s", primary.ID, ok, err, workspace.ID)
	}
	if !run.WorkspaceID.Valid || run.WorkspaceID.String != workspace.ID {
		t.Fatalf("run workspace = %v, want %s", run.WorkspaceID, workspace.ID)
	}
	if !run.SessionID.Valid || run.SessionID.String != session.ID {
		t.Fatalf("run session = %v, want %s", run.SessionID, session.ID)
	}
	if !session.WorkspaceID.Valid || session.WorkspaceID.String != workspace.ID {
		t.Fatalf("session workspace = %v, want %s", session.WorkspaceID, workspace.ID)
	}
	if thread.Cwd != service.projectRoot {
		t.Fatalf("thread cwd = %q, want project root %q", thread.Cwd, service.projectRoot)
	}
	if !session.Cwd.Valid || session.Cwd.String != service.projectRoot {
		t.Fatalf(
			"session cwd = %v, want project root %q",
			session.Cwd,
			service.projectRoot,
		)
	}
	if run.RootDocPath != workspace.RootDocPath {
		t.Fatalf(
			"run artifact root = %q, want workspace artifact root %q",
			run.RootDocPath,
			workspace.RootDocPath,
		)
	}
	storedWorkspace, err := service.queries.GetWorkspace(
		context.Background(),
		workspace.ID,
	)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !storedWorkspace.SelectedThreadID.Valid ||
		storedWorkspace.SelectedThreadID.String != thread.ID {
		t.Fatalf(
			"selected thread = %v, want %s",
			storedWorkspace.SelectedThreadID,
			thread.ID,
		)
	}
	if !storedWorkspace.CurrentSessionID.Valid ||
		storedWorkspace.CurrentSessionID.String == "" {
		t.Fatalf(
			"current chat session = %v, want durable chat session",
			storedWorkspace.CurrentSessionID,
		)
	}

	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf(
			"temporal input type = %T, want conversation.RunInput",
			fakeTemporal.lastInput,
		)
	}
	if input.WorkspaceID != workspace.ID || input.SessionID != session.ID ||
		input.ChatSessionID != storedWorkspace.CurrentSessionID.String ||
		input.RunID != run.ID ||
		input.ThreadID != thread.ID {
		t.Fatalf(
			"run input ids = %+v, want workspace/session/chat-session/run/thread ids",
			input,
		)
	}
	events, err := service.queries.ListChatSessionEventsAfter(
		t.Context(),
		db.ListChatSessionEventsAfterParams{
			SessionID: input.ChatSessionID,
			AfterSeq:  0,
			Limit:     10,
		},
	)
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter() error = %v", err)
	}
	if len(events) < 2 ||
		events[0].EventType != string(chatsession.EventMessageCreated) ||
		events[1].EventType != string(chatsession.EventRunStarted) {
		t.Fatalf("chat session events = %+v, want prompt then run started", events)
	}
	attachments, err := service.queries.ListAgentSurfaceAttachmentsBySession(
		t.Context(),
		input.ChatSessionID,
	)
	if err != nil {
		t.Fatalf("ListAgentSurfaceAttachmentsBySession() error = %v", err)
	}
	if len(attachments) != 1 || attachments[0].SurfaceKind != "temporal_worker" ||
		attachments[0].PermissionMode != string(chatsession.PermissionOwn) {
		t.Fatalf("surface attachments = %+v, want temporal owner", attachments)
	}
	if input.Cwd != service.projectRoot {
		t.Fatalf("input cwd = %q, want project root %q", input.Cwd, service.projectRoot)
	}
	if input.RootDocPath != workspace.RootDocPath {
		t.Fatalf(
			"input artifact root = %q, want workspace artifact root %q",
			input.RootDocPath,
			workspace.RootDocPath,
		)
	}
	if input.CallbackEndpoint != "http://127.0.0.1:4301/internal/agent-chat/events" {
		t.Fatalf(
			"callback endpoint = %q, want workspace-local endpoint",
			input.CallbackEndpoint,
		)
	}
	if input.SnapshotLoaderEndpoint != "http://127.0.0.1:4301/internal/agent-chat/snapshots" {
		t.Fatalf(
			"snapshot endpoint = %q, want workspace-local endpoint",
			input.SnapshotLoaderEndpoint,
		)
	}
	if strings.Contains(input.CallbackEndpoint, "localhost:4200") ||
		strings.Contains(input.SnapshotLoaderEndpoint, "localhost:4200") {
		t.Fatalf("run input endpoints include default localhost: %+v", input)
	}
	if input.SnapshotRef.LineageID != thread.LineageID {
		t.Fatalf(
			"snapshot ref lineage = %q, want %q",
			input.SnapshotRef.LineageID,
			thread.LineageID,
		)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal(run input) error = %v", err)
	}
	if bytes.Contains(encoded, []byte("entries")) ||
		bytes.Contains(encoded, []byte("payload_json")) {
		t.Fatalf("run input includes full snapshot payload: %s", encoded)
	}
}

func TestRunWorkspaceProbeUsesChildLocalCallbackSnapshotAndCwd(t *testing.T) {
	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	t.Setenv("VAMOS_INTERNAL_TOKEN", "secret")
	t.Setenv("TEMPORAL_ADDRESS", "127.0.0.1:7234")

	e := echo.New()
	e.GET("/internal/agent-chat/snapshots", handler.HandleInternalRunSnapshot)
	e.POST("/internal/agent-chat/events", handler.HandleInternalRunEvent)
	server := httptest.NewServer(e)
	t.Cleanup(server.Close)
	service.callbackBaseURL = server.URL

	result, err := service.RunWorkspaceProbe(t.Context(), workspaces.AgentChatProbeRequest{})
	if err != nil {
		t.Fatalf("RunWorkspaceProbe() error = %v; result = %+v", err, result)
	}
	if result.RunID == "" || result.WorkflowID == "" {
		t.Fatalf("probe ids = %+v, want run and workflow ids", result)
	}
	if result.CallbackEndpoint != server.URL+"/internal/agent-chat/events" {
		t.Fatalf("callback endpoint = %q, want child endpoint", result.CallbackEndpoint)
	}
	if result.SnapshotLoaderEndpoint != server.URL+"/internal/agent-chat/snapshots" {
		t.Fatalf("snapshot endpoint = %q, want child endpoint", result.SnapshotLoaderEndpoint)
	}
	if result.Cwd != service.projectRoot {
		t.Fatalf("cwd = %q, want project root %q", result.Cwd, service.projectRoot)
	}
	if !result.ReachedSnapshotLoader || !result.ReachedCallback {
		t.Fatalf("probe reached flags = %+v, want snapshot and callback", result)
	}
	if result.TemporalAddress != "127.0.0.1:7234" {
		t.Fatalf("temporal address = %q", result.TemporalAddress)
	}
	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf("temporal input type = %T, want conversation.RunInput", fakeTemporal.lastInput)
	}
	if input.CallbackEndpoint != result.CallbackEndpoint ||
		input.SnapshotLoaderEndpoint != result.SnapshotLoaderEndpoint ||
		input.Cwd != result.Cwd {
		t.Fatalf("temporal input = %+v, probe result = %+v", input, result)
	}
	storedRun, err := service.queries.GetAgentRun(t.Context(), result.RunID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if storedRun.Status != "failed" {
		t.Fatalf("probe run status = %q, want failed diagnostic run", storedRun.Status)
	}
}

func TestHandleInternalWorkspaceProbeRequiresInternalTrust(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/internal/agent-chat/probe", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.HandleInternalWorkspaceProbe(c); err == nil {
		t.Fatalf("HandleInternalWorkspaceProbe() error = nil, want unauthorized")
	}
}

func TestCoworkerCanStartResumeAndForkWorkspaceThread(t *testing.T) {
	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")

	thread, startedRun, _, err := service.StartWorkspaceThread(
		t.Context(),
		workspace.ID,
		"coworker@example.com",
		"shared hello",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread(coworker) error = %v", err)
	}
	if err := service.FinalizeRun(
		t.Context(),
		conversation.RunResult{
			WorkspaceID: workspace.ID,
			RunID:       startedRun.ID,
			ThreadID:    thread.ID,
		},
	); err != nil {
		t.Fatalf("FinalizeRun(started run) error = %v", err)
	}
	if _, _, _, err := service.ResumeWorkspaceThread(
		t.Context(),
		workspace.ID,
		"coworker@example.com",
		thread.ID,
		"shared resume",
	); err != nil {
		t.Fatalf("ResumeWorkspaceThread(coworker) error = %v", err)
	}
	entry := conversation.SnapshotEntry{
		LineageID:   thread.LineageID,
		EntryID:     "assistant-fork-source",
		EntryType:   "message",
		OriginOrder: 1,
		PayloadJSON: `{"type":"message","id":"assistant-fork-source","message":{"role":"assistant","content":"fork here"}}`,
	}
	if err := service.queries.CreateAgentEntry(t.Context(), db.CreateAgentEntryParams{
		LineageID:        thread.LineageID,
		EntryID:          entry.EntryID,
		ParentEntryID:    sql.NullString{},
		EntryType:        entry.EntryType,
		OriginOrder:      entry.OriginOrder,
		PayloadJson:      entry.PayloadJSON,
		OriginThreadID:   thread.ID,
		OriginRunID:      sql.NullString{},
		OriginSessionID:  sql.NullString{},
		SessionTimestamp: entry.Timestamp,
	}); err != nil {
		t.Fatalf("CreateAgentEntry() error = %v", err)
	}
	if _, _, _, err := service.ForkWorkspaceThread(
		t.Context(),
		workspace.ID,
		"coworker@example.com",
		thread.ID,
		entry.EntryID,
		"shared fork",
	); err != nil {
		t.Fatalf("ForkWorkspaceThread(coworker) error = %v", err)
	}
}

func TestViewingSideForkDoesNotUpdateWorkspaceCurrentSession(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")

	thread, _, _, err := service.StartWorkspaceThread(
		t.Context(),
		workspace.ID,
		"owner@example.com",
		"root prompt",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread() error = %v", err)
	}
	storedWorkspace, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if !storedWorkspace.CurrentSessionID.Valid {
		t.Fatalf("current session missing after start")
	}
	parentSessionID := storedWorkspace.CurrentSessionID.String
	entry := conversation.SnapshotEntry{
		LineageID:   thread.LineageID,
		EntryID:     "assistant-fork-source",
		EntryType:   "message",
		OriginOrder: 1,
		PayloadJSON: `{"type":"message","id":"assistant-fork-source","message":{"role":"assistant","content":"fork here"}}`,
	}
	if err := service.queries.CreateAgentEntry(t.Context(), db.CreateAgentEntryParams{
		LineageID:        thread.LineageID,
		EntryID:          entry.EntryID,
		ParentEntryID:    sql.NullString{},
		EntryType:        entry.EntryType,
		OriginOrder:      entry.OriginOrder,
		PayloadJson:      entry.PayloadJSON,
		OriginThreadID:   thread.ID,
		OriginRunID:      sql.NullString{},
		OriginSessionID:  sql.NullString{},
		SessionTimestamp: entry.Timestamp,
	}); err != nil {
		t.Fatalf("CreateAgentEntry() error = %v", err)
	}
	if _, _, _, err := service.ForkWorkspaceThread(
		t.Context(),
		workspace.ID,
		"owner@example.com",
		thread.ID,
		entry.EntryID,
		"side fork prompt",
	); err != nil {
		t.Fatalf("ForkWorkspaceThread() error = %v", err)
	}
	storedWorkspace, err = service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(after fork) error = %v", err)
	}
	if !storedWorkspace.CurrentSessionID.Valid ||
		storedWorkspace.CurrentSessionID.String != parentSessionID {
		t.Fatalf(
			"current session after side fork = %#v, want parent %s",
			storedWorkspace.CurrentSessionID,
			parentSessionID,
		)
	}
}

func TestAgentChatShellRendersWorkspaceSidebarDocAndChat(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	thread, _, _, err := service.StartWorkspaceThread(
		t.Context(),
		workspace.ID,
		"owner@example.com",
		"root prompt",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread() error = %v", err)
	}

	args, err := service.BuildWorkspacePageArgs(t.Context(), BuildWorkspacePageInput{
		UserEmail:   "owner@example.com",
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
	})
	if err != nil {
		t.Fatalf("BuildWorkspacePageArgs() error = %v", err)
	}
	var body bytes.Buffer
	if err := WorkspaceResource(*args).Render(t.Context(), &body); err != nil {
		t.Fatalf("WorkspaceResource.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`data-workbench-kind="workspace-topology"`,
		`id="agent-chat-workspace-topology-sidebar"`,
		`data-workbench-kind="doc"`,
		`id="agent-chat-doc-pane"`,
		`data-workbench-kind="chat"`,
		`id="agent-chat-chat-pane"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workspace shell html missing %q: %s", want, html)
		}
	}
}

func TestSelectingSidebarNodeDoesNotPromoteSession(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	thread, run, _, err := service.StartWorkspaceThread(
		t.Context(),
		workspace.ID,
		"owner@example.com",
		"root prompt",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread() error = %v", err)
	}
	before, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(before) error = %v", err)
	}
	if !before.CurrentSessionID.Valid {
		t.Fatal("current session missing before selection render")
	}

	if _, err := service.BuildWorkspacePageArgs(t.Context(), BuildWorkspacePageInput{
		UserEmail:   "owner@example.com",
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		RunID:       run.ID,
		DocRelPath:  "design.md",
		DocPath:     "design.md",
	}); err != nil {
		t.Fatalf("BuildWorkspacePageArgs(selection) error = %v", err)
	}
	after, err := service.queries.GetWorkspace(t.Context(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(after) error = %v", err)
	}
	if after.CurrentSessionID != before.CurrentSessionID ||
		after.CurrentBranchID != before.CurrentBranchID {
		t.Fatalf(
			"current session changed after sidebar selection render: before=%v/%v after=%v/%v",
			before.CurrentSessionID,
			before.CurrentBranchID,
			after.CurrentSessionID,
			after.CurrentBranchID,
		)
	}
}

func TestStartThreadAttachesPlanScopedFreeformRunBeforeTemporalStart(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	fakeTemporal := &fakeTemporalStarter{}
	service.temporal = fakeTemporal
	root := filepath.Join(t.TempDir(), "thoughts")
	planDir := filepath.Join(
		root,
		"creative-mode-agent",
		"plans",
		"2026-05-04_streaming-repro",
	)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(planDir) error = %v", err)
	}
	service.thoughtsRoot = root
	service.projectRoot = root

	thread, run, err := service.StartThread(
		t.Context(),
		"user@example.com",
		planDir,
		"hello freeform",
	)
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if thread == nil || run == nil {
		t.Fatalf("StartThread() thread/run = %v/%v, want values", thread, run)
	}
	primary, ok, err := service.ResolvePrimaryWorkspaceForThread(t.Context(), "user@example.com", thread.ID)
	if err != nil || !ok || strings.TrimSpace(primary.ID) == "" {
		t.Fatalf("primary workspace = (%v, %v, %v), want attached workspace", primary.ID, ok, err)
	}
	if !run.WorkspaceID.Valid || run.WorkspaceID.String != primary.ID {
		t.Fatalf(
			"run workspace = %v, want %s",
			run.WorkspaceID,
			primary.ID,
		)
	}

	input, ok := fakeTemporal.lastInput.(conversation.RunInput)
	if !ok {
		t.Fatalf(
			"temporal input type = %T, want conversation.RunInput",
			fakeTemporal.lastInput,
		)
	}
	if input.WorkspaceID != primary.ID {
		t.Fatalf(
			"input workspace = %q, want %q",
			input.WorkspaceID,
			primary.ID,
		)
	}

	storedRun, err := service.queries.GetAgentRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if !storedRun.WorkspaceID.Valid ||
		storedRun.WorkspaceID.String != primary.ID {
		t.Fatalf(
			"stored run workspace = %v, want %s",
			storedRun.WorkspaceID,
			primary.ID,
		)
	}
	events, err := service.queries.ListWorkspaceEvents(
		t.Context(),
		db.ListWorkspaceEventsParams{
			WorkspaceID: primary.ID,
			Limit:       10,
		},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	foundRunStarted := false
	for _, event := range events {
		if event.EventType == "run_started" && event.RunID.Valid &&
			event.RunID.String == run.ID {
			foundRunStarted = true
		}
	}
	if !foundRunStarted {
		t.Fatalf("run_started event for run %s not found in %+v", run.ID, events)
	}
}

func TestStartThreadWithoutCwdCreatesFreeformWorkspaceWithAgentsArtifact(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	root := filepath.Join(t.TempDir(), "thoughts")
	defaultCwd := filepath.Join(root, "user@example.com", "plans")
	if err := os.MkdirAll(defaultCwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(defaultCwd) error = %v", err)
	}
	service.thoughtsRoot = root
	service.projectRoot = root
	service.defaultCwd = defaultCwd

	thread, run, err := service.StartThread(
		t.Context(),
		"user@example.com",
		defaultCwd,
		"start clean workspace",
	)
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	if thread == nil || run == nil {
		t.Fatalf("StartThread() thread/run = %v/%v, want values", thread, run)
	}
	primary, ok, err := service.ResolvePrimaryWorkspaceForThread(t.Context(), "user@example.com", thread.ID)
	if err != nil || !ok || strings.TrimSpace(primary.ID) == "" {
		t.Fatalf("primary workspace = (%v, %v, %v), want workspace", primary.ID, ok, err)
	}
	if !strings.Contains(thread.Cwd, filepath.Join("user@example.com", "freeform")) {
		t.Fatalf("thread cwd = %q, want user freeform workspace", thread.Cwd)
	}
	if _, err := os.Stat(filepath.Join(thread.Cwd, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md stat error = %v", err)
	}

	pane, err := service.BuildArtifactPane(t.Context(), thread.ID, run.ID, "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if len(pane.Tree) == 0 {
		t.Fatalf("artifact tree is empty, want AGENTS.md")
	}
	if !pane.Selected.Exists || pane.Selected.RelativePath != "AGENTS.md" {
		t.Fatalf("selected artifact = %#v, want AGENTS.md", pane.Selected)
	}
}

func TestStartWorkspaceThreadSeedsPendingUserPromptLiveTranscript(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	thread, run, _, err := service.StartWorkspaceThread(
		t.Context(),
		workspace.ID,
		"user@example.com",
		"show me immediately",
	)
	if err != nil {
		t.Fatalf("StartWorkspaceThread() error = %v", err)
	}
	if thread == nil || run == nil {
		t.Fatalf("StartWorkspaceThread() thread/run = %v/%v, want values", thread, run)
	}

	args, err := service.BuildWorkspacePageArgs(
		t.Context(),
		BuildWorkspacePageInput{
			UserEmail:   "user@example.com",
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			RunID:       run.ID,
		},
	)
	if err != nil {
		t.Fatalf("BuildWorkspacePageArgs() error = %v", err)
	}
	if len(args.Projection.Transcript.Live.Items) != 1 {
		t.Fatalf(
			"live transcript item count = %d, want 1: %+v",
			len(args.Projection.Transcript.Live.Items),
			args.Projection.Transcript.Live.Items,
		)
	}
	item := args.Projection.Transcript.Live.Items[0]
	if item.Role != "user" || !strings.Contains(item.Content, "show me immediately") {
		t.Fatalf("live transcript item = %+v, want visible user prompt", item)
	}
}

func TestBuildWorkspacePageArgsAllowsSharedWorkspaceAccess(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"creator@example.com",
	)

	args, err := service.BuildWorkspacePageArgs(
		t.Context(),
		BuildWorkspacePageInput{
			UserEmail:   "viewer@example.com",
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
		},
	)
	if err != nil {
		t.Fatalf("BuildWorkspacePageArgs() error = %v", err)
	}
	if args.WorkspaceID != workspace.ID || args.Projection.SelectedThread == nil ||
		args.Projection.SelectedThread.ID != thread.ID {
		t.Fatalf(
			"BuildWorkspacePageArgs() = workspace %q thread %+v, want workspace %q thread %q",
			args.WorkspaceID,
			args.Projection.SelectedThread,
			workspace.ID,
			thread.ID,
		)
	}
}

func TestBuildWorkspacePageArgsAllowsSharedBareWorkspaceAccess(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "creator@example.com")

	args, err := service.BuildWorkspacePageArgs(
		t.Context(),
		BuildWorkspacePageInput{
			UserEmail:   "viewer@example.com",
			WorkspaceID: workspace.ID,
		},
	)
	if err != nil {
		t.Fatalf("BuildWorkspacePageArgs() error = %v", err)
	}
	if args.WorkspaceID != workspace.ID {
		t.Fatalf("WorkspaceID = %q, want %q", args.WorkspaceID, workspace.ID)
	}
}

func TestWorkspaceResumeAndForkReturnNoRedirectSSE(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-1",
		"",
		"message",
		0,
		`{"type":"message","id":"assistant-1","timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "assistant-1", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}

	handler := NewHandler(service, nil)
	for _, tc := range []struct {
		name   string
		path   string
		form   url.Values
		handle func(echo.Context) error
	}{
		{
			name:   "resume",
			path:   "/agent-chat/" + workspace.ID + "/thread/" + thread.ID + "/resume",
			form:   url.Values{"prompt": {"continue"}},
			handle: handler.ResumeWorkspaceThread,
		},
		{
			name:   "fork",
			path:   "/agent-chat/" + workspace.ID + "/thread/" + thread.ID + "/fork",
			form:   url.Values{"source_entry_id": {"assistant-1"}, "prompt": {"fork this"}},
			handle: handler.ForkWorkspaceThread,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				tc.path,
				strings.NewReader(tc.form.Encode()),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set("user_email", "user@example.com")
			c.SetParamNames("workspace_id", "thread_id")
			c.SetParamValues(workspace.ID, thread.ID)

			if err := tc.handle(c); err != nil {
				t.Fatalf("handler error = %v", err)
			}
			body := rec.Body.String()
			if strings.Contains(strings.ToLower(body), "redirect") ||
				strings.Contains(body, "/agent-chat/") {
				t.Fatalf("workspace write returned redirect/full navigation: %s", body)
			}
			if !strings.Contains(body, "agentChatLastWriteOK") {
				t.Fatalf("workspace write missing lightweight success signal: %s", body)
			}
		})
	}
}

func TestDuplicateCheckpointDoesNotDuplicateWorkspaceEvents(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	session, err := service.queries.CreateAgentSession(
		context.Background(),
		db.CreateAgentSessionParams{
			ID:                  "session-1",
			WorkspaceID:         nullString(workspace.ID),
			ThreadID:            nullString(thread.ID),
			Source:              string(AgentSessionSourceWeb),
			Status:              "pending",
			Cwd:                 nullString(workspace.RootDocPath),
			SessionPath:         sql.NullString{},
			SessionID:           sql.NullString{},
			ParentSessionID:     sql.NullString{},
			InferredWorkspaceID: sql.NullString{},
			InferredPlanDir:     sql.NullString{},
			ImportedHeadEntryID: sql.NullString{},
			LastError:           sql.NullString{},
			MetadataJson:        sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentSession() error = %v", err)
	}
	run, err := service.queries.CreateAgentRun(
		context.Background(),
		db.CreateAgentRunParams{
			ID:                 "run-1",
			WorkspaceID:        nullString(workspace.ID),
			ThreadID:           thread.ID,
			SessionID:          nullString(session.ID),
			Trigger:            "send",
			Status:             "running",
			PromptText:         "hello",
			RestoreHeadEntryID: sql.NullString{},
			ResultHeadEntryID:  sql.NullString{},
			WorkflowID:         "workflow-run-1",
			TemporalRunID:      sql.NullString{},
			RootDocPath:        workspace.RootDocPath,
			ErrorMessage:       sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	cp := conversation.Checkpoint{
		WorkspaceID: workspace.ID,
		SessionID:   session.ID,
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		TurnIndex:   1,
		Header: conversation.SnapshotHeader{
			SessionID: session.ID,
			Cwd:       workspace.RootDocPath,
		},
		NewEntries: []conversation.SnapshotEntry{
			{
				LineageID:   thread.LineageID,
				EntryID:     "assistant-1",
				EntryType:   "message",
				OriginOrder: 0,
				PayloadJSON: `{"type":"message","id":"assistant-1","timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":"done"}}`,
			},
		},
		EventKey: "run-1:checkpoint:1",
	}
	if err := service.ApplyCheckpoint(context.Background(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint(first) error = %v", err)
	}
	if err := service.ApplyCheckpoint(context.Background(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint(duplicate) error = %v", err)
	}
	events, err := service.queries.ListWorkspaceEvents(
		context.Background(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	count := 0
	for _, event := range events {
		if event.EventKey.Valid && event.EventKey.String == "run-1:checkpoint:1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("checkpoint event count = %d, want 1; events = %+v", count, events)
	}
}

func TestApplyCheckpointWritesAssistantMessageSessionEvent(t *testing.T) {
	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-session-checkpoint",
	)
	chatSession := mustCreateLifecycleChatSession(t, service, workspace, run)
	cp := conversation.Checkpoint{
		WorkspaceID:   workspace.ID,
		SessionID:     runSessionID(run),
		ChatSessionID: chatSession.ID,
		RunID:         run.ID,
		ThreadID:      thread.ID,
		HeadEntryID:   "assistant-session-1",
		TurnIndex:     1,
		NewEntries: []conversation.SnapshotEntry{{
			LineageID:   thread.LineageID,
			EntryID:     "assistant-session-1",
			EntryType:   "message",
			OriginOrder: 1,
			PayloadJSON: `{"message":{"role":"assistant","content":"durable hello"}}`,
		}},
	}
	if err := service.ApplyCheckpoint(t.Context(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint() error = %v", err)
	}
	events, err := service.queries.ListChatSessionEventsAfter(
		t.Context(),
		db.ListChatSessionEventsAfterParams{
			SessionID: chatSession.ID,
			AfterSeq:  0,
			Limit:     10,
		},
	)
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter() error = %v", err)
	}
	if len(events) != 2 ||
		events[1].EventType != string(chatsession.EventMessageCreated) ||
		!strings.Contains(events[1].PayloadJson, "durable hello") {
		t.Fatalf("chat session events = %+v, want assistant message", events)
	}
}

func TestFinalizeRunWritesSessionRunCompletedEvent(t *testing.T) {
	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-session-complete",
	)
	chatSession := mustCreateLifecycleChatSession(t, service, workspace, run)
	result := conversation.RunResult{
		WorkspaceID:   workspace.ID,
		ChatSessionID: chatSession.ID,
		RunID:         run.ID,
		ThreadID:      thread.ID,
		HeadEntryID:   "assistant-1",
	}
	if err := service.FinalizeRun(t.Context(), result); err != nil {
		t.Fatalf("FinalizeRun() error = %v", err)
	}
	events, err := service.queries.ListChatSessionEventsAfter(
		t.Context(),
		db.ListChatSessionEventsAfterParams{
			SessionID: chatSession.ID,
			AfterSeq:  0,
			Limit:     10,
		},
	)
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter() error = %v", err)
	}
	if len(events) != 2 || events[1].EventType != string(chatsession.EventRunCompleted) {
		t.Fatalf("chat session events = %+v, want run completed", events)
	}
}

func TestFailRunWritesSessionRunFailedEvent(t *testing.T) {
	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-session-failed",
	)
	chatSession := mustCreateLifecycleChatSession(t, service, workspace, run)
	failure := conversation.RunFailure{
		WorkspaceID:   workspace.ID,
		ChatSessionID: chatSession.ID,
		RunID:         run.ID,
		ThreadID:      thread.ID,
		ErrorMessage:  "boom",
	}
	if err := service.FailRun(t.Context(), failure); err != nil {
		t.Fatalf("FailRun() error = %v", err)
	}
	events, err := service.queries.ListChatSessionEventsAfter(
		t.Context(),
		db.ListChatSessionEventsAfterParams{
			SessionID: chatSession.ID,
			AfterSeq:  0,
			Limit:     10,
		},
	)
	if err != nil {
		t.Fatalf("ListChatSessionEventsAfter() error = %v", err)
	}
	if len(events) != 2 || events[1].EventType != string(chatsession.EventRunFailed) ||
		!strings.Contains(events[1].PayloadJson, "boom") {
		t.Fatalf("chat session events = %+v, want run failed", events)
	}
}

func TestDuplicateRunCompleteDoesNotDuplicateWorkspaceEvents(t *testing.T) {
	t.Parallel()

	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-complete-1",
	)
	result := conversation.RunResult{
		WorkspaceID: workspace.ID,
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		EventKey:    run.ID + ":run_complete",
	}

	if err := service.FinalizeRun(t.Context(), result); err != nil {
		t.Fatalf("FinalizeRun(first) error = %v", err)
	}
	if err := service.FinalizeRun(t.Context(), result); err != nil {
		t.Fatalf("FinalizeRun(duplicate) error = %v", err)
	}

	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		result.EventKey,
	); count != 1 {
		t.Fatalf("run_complete event count = %d, want 1", count)
	}
}

func TestDuplicateRunFailedDoesNotDuplicateWorkspaceEvents(t *testing.T) {
	t.Parallel()

	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-failed-1",
	)
	failure := conversation.RunFailure{
		WorkspaceID:  workspace.ID,
		RunID:        run.ID,
		ThreadID:     thread.ID,
		ErrorMessage: "boom",
		EventKey:     run.ID + ":run_failed",
	}

	if err := service.FailRun(t.Context(), failure); err != nil {
		t.Fatalf("FailRun(first) error = %v", err)
	}
	if err := service.FailRun(t.Context(), failure); err != nil {
		t.Fatalf("FailRun(duplicate) error = %v", err)
	}

	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		failure.EventKey,
	); count != 1 {
		t.Fatalf("run_failed event count = %d, want 1", count)
	}
}

func TestFinalizeRunRollsBackWhenWorkspaceEventAppendFails(t *testing.T) {
	t.Parallel()

	service, _, thread, run := mustCreateWorkspaceLifecycleRun(t, "run-rollback-1")
	service.appendWorkspaceEventForTest = func(context.Context, *db.Queries, AppendWorkspaceEventInput) (db.WorkspaceEvent, error) {
		return db.WorkspaceEvent{}, errors.New("append failed")
	}

	err := service.FinalizeRun(t.Context(), conversation.RunResult{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		EventKey:    run.ID + ":run_complete",
	})
	if err == nil {
		t.Fatal("FinalizeRun() error = nil, want append failure")
	}

	updated, err := service.queries.GetAgentRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if updated.Status != "running" {
		t.Fatalf("run status = %q, want running", updated.Status)
	}
	if updated.ResultHeadEntryID.Valid {
		t.Fatalf("result head = %v, want NULL", updated.ResultHeadEntryID)
	}
	if updated.CompletedAt.Valid {
		t.Fatalf("completed at = %v, want NULL", updated.CompletedAt)
	}
}

func TestFailConversationRunAfterActivityErrorMarksRunningRunFailed(t *testing.T) {
	t.Parallel()

	service, workspace, _, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-activity-failed-1",
	)
	input := conversation.ActivityFailureInput{
		WorkspaceID:  workspace.ID,
		RunID:        run.ID,
		ThreadID:     run.ThreadID,
		RootDocPath:  run.RootDocPath,
		ErrorMessage: "callback delivery failed",
		EventKey:     run.ID + ":run_failed",
	}

	if err := service.FailConversationRunAfterActivityError(
		t.Context(),
		input,
	); err != nil {
		t.Fatalf("FailConversationRunAfterActivityError() error = %v", err)
	}

	updated, err := service.queries.GetAgentRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if updated.Status != "failed" {
		t.Fatalf("run status = %q, want failed", updated.Status)
	}
	if !updated.ErrorMessage.Valid ||
		updated.ErrorMessage.String != "callback delivery failed" {
		t.Fatalf(
			"error message = %v, want callback delivery failed",
			updated.ErrorMessage,
		)
	}
	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		input.EventKey,
	); count != 1 {
		t.Fatalf("run_failed event count = %d, want 1", count)
	}
}

func TestFailConversationRunAfterActivityErrorIsDuplicateSafe(t *testing.T) {
	t.Parallel()

	service, workspace, _, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-activity-failed-duplicate-1",
	)
	input := conversation.ActivityFailureInput{
		WorkspaceID:  workspace.ID,
		RunID:        run.ID,
		ThreadID:     run.ThreadID,
		RootDocPath:  run.RootDocPath,
		ErrorMessage: "callback delivery failed",
		EventKey:     run.ID + ":run_failed",
	}

	if err := service.FailConversationRunAfterActivityError(
		t.Context(),
		input,
	); err != nil {
		t.Fatalf("FailConversationRunAfterActivityError(first) error = %v", err)
	}
	if err := service.FailConversationRunAfterActivityError(
		t.Context(),
		input,
	); err != nil {
		t.Fatalf("FailConversationRunAfterActivityError(duplicate) error = %v", err)
	}
	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		input.EventKey,
	); count != 1 {
		t.Fatalf("run_failed event count = %d, want 1", count)
	}
}

func TestFailConversationRunAfterActivityErrorDoesNotOverwriteCompleteRun(t *testing.T) {
	t.Parallel()

	service, workspace, thread, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-activity-complete-1",
	)
	completeKey := run.ID + ":run_complete"
	if err := service.FinalizeRun(t.Context(), conversation.RunResult{
		WorkspaceID: workspace.ID,
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventKey:    completeKey,
	}); err != nil {
		t.Fatalf("FinalizeRun() error = %v", err)
	}

	if err := service.FailConversationRunAfterActivityError(
		t.Context(),
		conversation.ActivityFailureInput{
			WorkspaceID:  workspace.ID,
			RunID:        run.ID,
			ThreadID:     run.ThreadID,
			RootDocPath:  run.RootDocPath,
			ErrorMessage: "late activity failure",
			EventKey:     run.ID + ":run_failed",
		},
	); err != nil {
		t.Fatalf("FailConversationRunAfterActivityError() error = %v", err)
	}

	updated, err := service.queries.GetAgentRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if updated.Status != "complete" {
		t.Fatalf("run status = %q, want complete", updated.Status)
	}
	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		run.ID+":run_failed",
	); count != 0 {
		t.Fatalf("run_failed event count = %d, want 0", count)
	}
}

func TestFailConversationRunAfterActivityErrorDoesNotOverwriteFailedRun(t *testing.T) {
	t.Parallel()

	service, workspace, _, run := mustCreateWorkspaceLifecycleRun(
		t,
		"run-activity-already-failed-1",
	)
	original := conversation.RunFailure{
		WorkspaceID:  workspace.ID,
		RunID:        run.ID,
		ThreadID:     run.ThreadID,
		ErrorMessage: "prompt failed",
		EventKey:     run.ID + ":run_failed",
	}
	if err := service.FailRun(t.Context(), original); err != nil {
		t.Fatalf("FailRun() error = %v", err)
	}
	if err := service.FailConversationRunAfterActivityError(
		t.Context(),
		conversation.ActivityFailureInput{
			WorkspaceID:  workspace.ID,
			RunID:        run.ID,
			ThreadID:     run.ThreadID,
			RootDocPath:  run.RootDocPath,
			ErrorMessage: "late callback failure",
			EventKey:     run.ID + ":run_failed",
		},
	); err != nil {
		t.Fatalf("FailConversationRunAfterActivityError() error = %v", err)
	}

	updated, err := service.queries.GetAgentRun(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if updated.Status != "failed" {
		t.Fatalf("run status = %q, want failed", updated.Status)
	}
	if !updated.ErrorMessage.Valid || updated.ErrorMessage.String != "prompt failed" {
		t.Fatalf("error message = %v, want original prompt failed", updated.ErrorMessage)
	}
	if count := countWorkspaceEventsByKey(
		t,
		service,
		workspace.ID,
		run.ID+":run_failed",
	); count != 1 {
		t.Fatalf("run_failed event count = %d, want 1", count)
	}
}

func mustCreateWorkspaceLifecycleRun(
	t *testing.T,
	runID string,
) (*Service, db.Workspace, db.AgentThread, db.AgentRun) {
	t.Helper()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	session, err := service.queries.CreateAgentSession(
		t.Context(),
		db.CreateAgentSessionParams{
			ID:                  "session-" + runID,
			WorkspaceID:         nullString(workspace.ID),
			ThreadID:            nullString(thread.ID),
			Source:              string(AgentSessionSourceWeb),
			Status:              "pending",
			Cwd:                 nullString(workspace.RootDocPath),
			SessionPath:         sql.NullString{},
			SessionID:           sql.NullString{},
			ParentSessionID:     sql.NullString{},
			InferredWorkspaceID: sql.NullString{},
			InferredPlanDir:     sql.NullString{},
			ImportedHeadEntryID: sql.NullString{},
			LastError:           sql.NullString{},
			MetadataJson:        sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentSession() error = %v", err)
	}
	run, err := service.queries.CreateAgentRun(
		t.Context(),
		db.CreateAgentRunParams{
			ID:                 runID,
			WorkspaceID:        nullString(workspace.ID),
			ThreadID:           thread.ID,
			SessionID:          nullString(session.ID),
			Trigger:            "send",
			Status:             "running",
			PromptText:         "hello",
			RestoreHeadEntryID: sql.NullString{},
			ResultHeadEntryID:  sql.NullString{},
			WorkflowID:         "workflow-" + runID,
			TemporalRunID:      sql.NullString{},
			RootDocPath:        workspace.RootDocPath,
			ErrorMessage:       sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	return service, workspace, thread, run
}

func mustCreateLifecycleChatSession(
	t *testing.T,
	service *Service,
	workspace db.Workspace,
	run db.AgentRun,
) db.ChatSession {
	t.Helper()
	chatSession, err := chatsession.CreateSessionWithQueries(
		t.Context(),
		service.queries,
		chatsession.CreateSessionInput{
			WorkspaceID:     workspace.ID,
			ActorEmail:      workspace.UserEmail,
			WorkflowID:      run.WorkflowID,
			WorkflowAttempt: int(run.WorkflowAttempt),
		},
	)
	if err != nil {
		t.Fatalf("CreateSessionWithQueries() error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceCurrentSession(
		t.Context(),
		db.UpdateWorkspaceCurrentSessionParams{
			ID:               workspace.ID,
			CurrentSessionID: sql.NullString{String: chatSession.ID, Valid: true},
			CurrentBranchID:  sql.NullString{String: chatSession.BranchID, Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceCurrentSession() error = %v", err)
	}
	if _, err := chatsession.AppendEventWithQueries(
		t.Context(),
		service.queries,
		chatsession.AppendEventInput{
			SessionID:   chatSession.ID,
			EventType:   chatsession.EventRunStarted,
			RunID:       run.ID,
			PayloadJSON: json.RawMessage(`{"id":"` + run.ID + `"}`),
		},
	); err != nil {
		t.Fatalf("AppendEventWithQueries() error = %v", err)
	}
	return chatSession
}

func countWorkspaceEventsByKey(
	t *testing.T,
	service *Service,
	workspaceID string,
	eventKey string,
) int {
	t.Helper()
	events, err := service.queries.ListWorkspaceEvents(
		t.Context(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspaceID, Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	count := 0
	for _, event := range events {
		if event.EventKey.Valid && event.EventKey.String == eventKey {
			count++
		}
	}
	return count
}

func TestHandleInternalRunEventRejectsWrongWorkspace(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	run, err := service.queries.CreateAgentRun(
		context.Background(),
		db.CreateAgentRunParams{
			ID:                 "run-1",
			WorkspaceID:        nullString(workspace.ID),
			ThreadID:           thread.ID,
			SessionID:          sql.NullString{},
			Trigger:            "send",
			Status:             "running",
			PromptText:         "hello",
			RestoreHeadEntryID: sql.NullString{},
			ResultHeadEntryID:  sql.NullString{},
			WorkflowID:         "workflow-run-1",
			TemporalRunID:      sql.NullString{},
			RootDocPath:        workspace.RootDocPath,
			ErrorMessage:       sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	env := conversation.EventEnvelope{
		WorkspaceID: "wrong-workspace",
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{}`,
	}
	body, _ := json.Marshal(env)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/agent-chat/events",
		bytes.NewReader(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Vamos-Internal-Token", "secret")
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err = NewHandler(
		service,
		nil,
		HandlerOptions{InternalToken: "secret"},
	).HandleInternalRunEvent(c)
	if err == nil {
		t.Fatal("HandleInternalRunEvent() error = nil, want bad request")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", httpErr.Code, http.StatusBadRequest)
	}
}
