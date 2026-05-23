//go:build !integration || unit
// +build !integration unit

package agentchat

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

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
		"Freeform chat",
		"/thoughts/chat/freeform/resume",
		"creative-mode-agent/plans/2026-04-30_test-plan/plan.md?",
		"context=chat",
		"chat_workspace=" + workspace.ID,
	} {
		if !strings.Contains(body+replacement.URL, want) {
			t.Fatalf("freeform embedded panel missing %q: body=%s replacement=%q", want, body, replacement.URL)
		}
	}
	for _, notWant := range []string{
		"/thoughts/chat/" + workspace.ID + "/send",
		"agent-chat-workspace-header",
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("freeform embedded panel contains %q: %s", notWant, body)
		}
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
