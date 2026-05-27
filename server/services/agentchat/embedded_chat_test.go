//go:build !integration || unit
// +build !integration unit

package agentchat

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

func TestBuildThoughtsChatDocURLIncludesChatQueryState(t *testing.T) {
	got := BuildThoughtsChatDocURL(EmbeddedChatURLState{
		DocPath:     "thoughts/user/plans/a/doc.md",
		WorkspaceID: "ws 1",
		ThreadID:    "thread/1",
		RunID:       "run+1",
	})
	for _, want := range []string{
		"/thoughts/user/plans/a/doc.md?",
		"context=chat",
		"chat_workspace=ws+1",
		"thread=thread%2F1",
		"run=run%2B1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BuildThoughtsChatDocURL() = %q, missing %q", got, want)
		}
	}
}

func TestParseEmbeddedChatURLReadsDocumentAndChatQuery(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/user/plans/a/doc.md?context=chat&chat_workspace=ws_1&thread=th_1&run=run_1",
		nil,
	)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	c.SetParamNames("*")
	c.SetParamValues("user/plans/a/doc.md")

	got := ParseEmbeddedChatURL(c)
	if got.DocPath != "user/plans/a/doc.md" || got.Context != "chat" ||
		got.WorkspaceID != "ws_1" || got.ThreadID != "th_1" || got.RunID != "run_1" {
		t.Fatalf("ParseEmbeddedChatURL() = %+v", got)
	}
}

func TestResolveEmbeddedChatSelectionExplicitURLWins(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: "other",
			ThreadID:    "other-thread",
			RunID:       "other-run",
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{
			Context:     "chat",
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			RunID:       "run_1",
		},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if !got.ExplicitURL || got.WorkspaceID != workspace.ID || got.ThreadID != thread.ID ||
		got.RunID != "run_1" {
		t.Fatalf("selection = %+v", got)
	}
}

func TestResolveEmbeddedChatSelectionWorkspaceDocUsesWorkspaceRowBeforeUserSelection(
	t *testing.T,
) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, workspaceThread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	freeformWorkspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	freeformThread := mustCreateAgentThread(
		t,
		service,
		"thread-2",
		"owner@example.com",
		freeformWorkspace.RootDocPath,
		"lineage-2",
	)
	if err := service.queries.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          freeformThread.ID,
			WorkspaceID: sql.NullString{String: freeformWorkspace.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace() error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceWorkflowState(
		ctx,
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: sql.NullString{},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: freeformWorkspace.ID,
			ThreadID:    freeformThread.ID,
			Scope:       EmbeddedChatSelectionScopeFreeform,
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{
			Context: ThoughtsChatContext,
			WorkspaceContext: markdown.DocumentWorkspaceContext{
				WorkspaceID: workspace.ID,
			},
		},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got.WorkspaceID != workspace.ID || got.ThreadID != workspaceThread.ID ||
		got.Scope != EmbeddedChatSelectionScopeWorkspace {
		t.Fatalf("selection = %+v", got)
	}
}

func TestResolveEmbeddedChatSelectionUsesPersistedWhenChatContextHasNoParams(
	t *testing.T,
) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			RunID:       "run_2",
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{Context: "chat"},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got.ExplicitURL || got.WorkspaceID != workspace.ID || got.ThreadID != thread.ID ||
		got.RunID != "run_2" {
		t.Fatalf("selection = %+v", got)
	}
}

func TestResolveEmbeddedChatSelectionDropsStaleCrossWorkspaceThread(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	_, thread := mustCreateWorkspaceThreadForHandlerTest(t, service, "owner@example.com")
	otherWorkspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: otherWorkspace.ID,
			ThreadID:    thread.ID,
			RunID:       "stale-run",
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{Context: ThoughtsChatContext},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got.WorkspaceID != otherWorkspace.ID || got.ThreadID != "" || got.RunID != "" {
		t.Fatalf("selection = %+v, want workspace-only fallback", got)
	}
}

func TestResolveEmbeddedChatSelectionRootIgnoresGlobalWorkspaceSelection(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	if err := service.queries.UpdateWorkspaceWorkflowState(
		ctx,
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: sql.NullString{},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{WorkspaceID: workspace.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{Context: ThoughtsChatContext},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got != (EmbeddedChatSelection{}) {
		t.Fatalf("selection = %+v, want empty", got)
	}
}

func TestPersistEmbeddedChatSelectionWritesScopedRows(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			Scope:       EmbeddedChatSelectionScopeWorkspace,
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection(workspace) error = %v", err)
	}
	workspaceRow, err := service.queries.GetUserChatSelection(
		ctx,
		db.GetUserChatSelectionParams{
			UserEmail: "owner@example.com",
			Scope:     string(EmbeddedChatSelectionScopeWorkspace),
			ScopeID:   workspace.ID,
		},
	)
	if err != nil {
		t.Fatalf("GetUserChatSelection(workspace) error = %v", err)
	}
	if workspaceRow.WorkspaceID != workspace.ID || workspaceRow.ThreadID.String != thread.ID {
		t.Fatalf("workspace row = %+v", workspaceRow)
	}

	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			Scope:       EmbeddedChatSelectionScopeFreeform,
		},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection(freeform) error = %v", err)
	}
	freeformRow, err := service.queries.GetUserChatSelection(
		ctx,
		db.GetUserChatSelectionParams{
			UserEmail: "owner@example.com",
			Scope:     string(EmbeddedChatSelectionScopeFreeform),
			ScopeID:   "",
		},
	)
	if err != nil {
		t.Fatalf("GetUserChatSelection(freeform) error = %v", err)
	}
	if freeformRow.WorkspaceID != workspace.ID || freeformRow.ScopeID != "" {
		t.Fatalf("freeform row = %+v", freeformRow)
	}
}

func TestResolveEmbeddedChatSelectionEmptyWithoutChatContext(t *testing.T) {
	service := newTestAgentChatService(t)
	got, err := service.ResolveEmbeddedChatSelection(
		context.Background(),
		"owner@example.com",
		EmbeddedChatURLState{},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got != (EmbeddedChatSelection{}) {
		t.Fatalf("selection = %+v, want empty", got)
	}
}

func TestResolveEmbeddedChatSelectionIgnoresPersistedOutsideChatContext(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"owner@example.com",
	)
	if err := service.PersistEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatSelection{WorkspaceID: workspace.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	got, err := service.ResolveEmbeddedChatSelection(
		ctx,
		"owner@example.com",
		EmbeddedChatURLState{Context: "comments"},
	)
	if err != nil {
		t.Fatalf("ResolveEmbeddedChatSelection() error = %v", err)
	}
	if got != (EmbeddedChatSelection{}) {
		t.Fatalf("selection = %+v, want empty", got)
	}
}

func TestRenderEmbeddedChatPanelUsesFreeformRendererForPersistedFreeformWorkspace(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	if err := service.PersistEmbeddedChatSelection(
		t.Context(),
		"user@example.com",
		EmbeddedChatSelection{WorkspaceID: workspace.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	component, replacement, err := service.RenderEmbeddedChatPanel(
		t.Context(),
		markdown.EmbeddedChatRenderRequest{
			UserEmail: "user@example.com",
			DocPath:   "creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
			Context:   ThoughtsChatContext,
		},
	)
	if err != nil {
		t.Fatalf("RenderEmbeddedChatPanel() error = %v", err)
	}
	body := renderTemplToString(t, component)
	for _, want := range []string{
		"Workspace-backed thread",
		"In this workspace",
		"/thoughts/chat/thread/" + thread.ID + "/resume",
		"/thoughts/chat/thread/" + thread.ID + "/stream",
		"creative-mode-agent/plans/2026-04-30_test-plan/plan.md?",
		"context=chat",
		"chat_workspace=" + workspace.ID,
	} {
		if !strings.Contains(body+replacement.URL, want) {
			t.Fatalf("freeform embedded panel missing %q: body=%s replacement=%q", want, body, replacement.URL)
		}
	}
	for _, notWant := range []string{
		"/agent-chat/stream",
		"/thoughts/chat/freeform/stream",
		"/thoughts/chat/" + workspace.ID + "/send",
		"agent-chat-workspace-header",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("freeform embedded panel contains %q: %s", notWant, body)
		}
	}
}

func TestRenderEmbeddedFreeformPanelShowsUpgradedThreadMetadata(t *testing.T) {
	service := newTestAgentChatService(t)
	freeformWorkspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	primaryWorkspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	if err := service.SetThreadPrimaryWorkspace(t.Context(), thread.ID, primaryWorkspace.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}
	if err := service.PersistEmbeddedChatSelection(
		t.Context(),
		"user@example.com",
		EmbeddedChatSelection{WorkspaceID: freeformWorkspace.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	component, _, err := service.RenderEmbeddedChatPanel(
		t.Context(),
		markdown.EmbeddedChatRenderRequest{
			UserEmail: "user@example.com",
			DocPath:   "creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
			Context:   ThoughtsChatContext,
		},
	)
	if err != nil {
		t.Fatalf("RenderEmbeddedChatPanel() error = %v", err)
	}
	body := renderTemplToString(t, component)
	for _, want := range []string{
		"Workspace-backed thread",
		"Primary workspace",
		"New thread",
		"In this workspace",
		"Freeform",
		"/agent-chat/thread/" + thread.ID + "/new?target_kind=primary",
		"workspace_id=" + primaryWorkspace.ID,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("upgraded freeform embedded panel missing %q: %s", want, body)
		}
	}
}

func TestBuildEmbeddedFreeformPanelArgsUsesWorkspaceResumeForUpgradedThread(t *testing.T) {
	service := newTestAgentChatService(t)
	freeformWorkspace, thread := mustCreateWorkspaceThreadForHandlerTest(t, service, "user@example.com")
	primaryWorkspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	if err := service.SetThreadPrimaryWorkspace(t.Context(), thread.ID, primaryWorkspace.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}
	if err := service.PersistEmbeddedChatSelection(
		t.Context(),
		"user@example.com",
		EmbeddedChatSelection{WorkspaceID: freeformWorkspace.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("PersistEmbeddedChatSelection() error = %v", err)
	}

	args, err := service.BuildEmbeddedFreeformPanelArgs(t.Context(), "user@example.com", thread.ID, "")
	if err != nil {
		t.Fatalf("BuildEmbeddedFreeformPanelArgs() error = %v", err)
	}
	want := "/thoughts/chat/thread/" + thread.ID + "/resume"
	if !strings.Contains(args.ComposerAction, want) {
		t.Fatalf("ComposerAction = %q, want %q", args.ComposerAction, want)
	}
}

func TestEmbeddedChatPanelRendersComposerBeforeThread(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	args, err := service.BuildEmbeddedChatPanelArgs(t.Context(), EmbeddedChatPatchInput{
		UserEmail:   "user@example.com",
		DocPath:     "creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
		WorkspaceID: workspace.ID,
		AttachDoc:   true,
	})
	if err != nil {
		t.Fatalf("BuildEmbeddedChatPanelArgs() error = %v", err)
	}
	body := renderTemplToString(t, EmbeddedChatRightRailContent(args))
	for _, want := range []string{
		"agent-chat-workspace-composer",
		"/thoughts/chat/" + workspace.ID + "/send",
		"Remove current doc",
		"agent-chat-composer-input",
		"creative-mode-agent/plans/2026-04-30_test-plan/plan.md",
		"doc=creative-mode-agent%2Fplans%2F2026-04-30_test-plan%2Fplan.md",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{
		"doc-right-chat-panel",
		"Open Chat with this document attached",
		"agent-chat-workspace-header",
		"agent-chat-workspace-shell",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("body contains %q: %s", notWant, body)
		}
	}

	panelBody := renderTemplToString(t, EmbeddedChatRightRailPanel(args))
	if !strings.Contains(panelBody, `id="doc-right-chat-panel"`) {
		t.Fatalf("panel body missing outer patch target: %s", panelBody)
	}
}

func renderTemplToString(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}
