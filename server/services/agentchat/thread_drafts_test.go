package agentchat

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func newThreadDraftService(t *testing.T) (*Service, *db.Queries) {
	t.Helper()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "drafts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return &Service{db: database.DB(), queries: database.Queries}, database.Queries
}

func createDraftThread(t *testing.T, queries *db.Queries, id string) {
	t.Helper()
	_, err := queries.CreateAgentThread(
		t.Context(),
		db.CreateAgentThreadParams{
			ID:        id,
			UserEmail: "owner@example.com",
			Title:     "Draft",
			Cwd:       "thoughts/plan",
			LineageID: "lineage_" + id,
			ProjectID: "project_1",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func saveDraftRequest(
	t *testing.T,
	handler *Handler,
	threadID, user, body string,
) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/thread/"+threadID+"/draft",
		bytes.NewBufferString(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues(threadID)
	ctx.Set("user_email", user)
	return rec, handler.SaveThreadDraft(ctx)
}

func TestThreadDraftsAreIsolatedByUserAndThread(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	createDraftThread(t, queries, "thread_2")
	for _, save := range []struct{ user, thread, content string }{{"one@example.com", "thread_1", "one-thread-one"}, {"two@example.com", "thread_1", "two-thread-one"}, {"one@example.com", "thread_2", "one-thread-two"}} {
		if err := service.SaveThreadDraft(
			t.Context(),
			save.user,
			save.thread,
			save.content,
			nextDraftOperationOrder(),
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct{ user, thread, content string }{{"one@example.com", "thread_1", "one-thread-one"}, {"two@example.com", "thread_1", "two-thread-one"}, {"one@example.com", "thread_2", "one-thread-two"}} {
		got, err := service.GetThreadDraft(t.Context(), want.user, want.thread)
		if err != nil || got != want.content {
			t.Fatalf("draft for %s/%s = %q, %v", want.user, want.thread, got, err)
		}
	}
}

func TestThreadDraftSaveClearAndNewerSave(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	missing, err := service.GetThreadDraft(t.Context(), "reader@example.com", "thread_1")
	if err != nil || missing != "" {
		t.Fatalf("missing draft = %q, %v", missing, err)
	}
	if err := service.SaveThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
		"first",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
		"second",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	got, err := service.GetThreadDraft(t.Context(), "reader@example.com", "thread_1")
	if err != nil || got != "second" {
		t.Fatalf("overwritten draft = %q, %v", got, err)
	}
	if err := service.SaveThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
		"",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	got, err = service.GetThreadDraft(t.Context(), "reader@example.com", "thread_1")
	if err != nil || got != "" {
		t.Fatalf("empty draft = %q, %v", got, err)
	}
	if err := service.ClearThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
	); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
		"after clear",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	got, err = service.GetThreadDraft(t.Context(), "reader@example.com", "thread_1")
	if err != nil || got != "after clear" {
		t.Fatalf("newer draft = %q, %v", got, err)
	}
}

func TestOlderDraftSaveCannotOverwriteAcceptedClear(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	older := nextDraftOperationOrder()
	if err := service.ClearThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
	); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveThreadDraft(
		t.Context(),
		"reader@example.com",
		"thread_1",
		"sent prompt",
		older,
	); err != nil {
		t.Fatal(err)
	}
	draft, err := service.GetThreadDraft(t.Context(), "reader@example.com", "thread_1")
	if err != nil || draft != "" {
		t.Fatalf("tombstone draft = %q, %v", draft, err)
	}
}

func TestSaveThreadDraftHandlerDecodesSignalsAndIsolatesUsers(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	handler := NewHandler(service, nil)
	content := "quotes \" and\nUnicode 🌰"
	for _, user := range []string{"one@example.com", "two@example.com"} {
		body := `{"chatDraft":` + strconv.Quote(content+user) + `}`
		rec, err := saveDraftRequest(t, handler, "thread_1", user, body)
		if err != nil || rec.Code != http.StatusNoContent || rec.Body.String() != "" {
			t.Fatalf("save %s = %d %q %v", user, rec.Code, rec.Body.String(), err)
		}
	}
	for _, user := range []string{"one@example.com", "two@example.com"} {
		got, err := service.GetThreadDraft(t.Context(), user, "thread_1")
		if err != nil || got != content+user {
			t.Fatalf("draft %s = %q, %v", user, got, err)
		}
	}
	_ = queries
}

func TestSaveThreadDraftHandlerRejectsInvalidRequestsWithoutWrites(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	handler := NewHandler(service, nil)
	for _, test := range []struct {
		name, thread, user, body string
		status                   int
	}{{"missing user", "thread_1", "", `{"chatDraft":"x"}`, 401}, {"unknown thread", "missing", "reader@example.com", `{"chatDraft":"x"}`, 404}, {"malformed", "thread_1", "reader@example.com", `{`, 400}} {
		t.Run(test.name, func(t *testing.T) {
			rec, err := saveDraftRequest(t, handler, test.thread, test.user, test.body)
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != test.status {
				t.Fatalf("error = %#v", err)
			}
			if rec.Body.String() != "" {
				t.Fatalf("unexpected SSE response: %q", rec.Body.String())
			}
			got, getErr := service.GetThreadDraft(
				t.Context(),
				"reader@example.com",
				"thread_1",
			)
			if getErr != nil || got != "" {
				t.Fatalf("draft written = %q, %v", got, getErr)
			}
		})
	}
	_ = queries
}

func renderDraftComponent(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestComposerDraftSignalAndSaveContract(t *testing.T) {
	draft := "quotes \" and\nUnicode 🌰"
	html := renderDraftComponent(
		t,
		AgentChatComposer(
			AgentChatComposerArgs{
				Action:          "@post('/send')",
				ThreadID:        "thread_1",
				HasThread:       true,
				InitialDraft:    draft,
				DraftSaveAction: "@post('/agent-chat/thread/thread_1/draft', {filterSignals: {include: /^chatDraft$/}})",
			},
		),
	)
	if strings.Count(html, "chatDraft") < 3 ||
		!strings.Contains(html, `data-bind="chatDraft"`) ||
		!strings.Contains(html, `data-on:input__debounce.500ms`) ||
		!strings.Contains(html, `include: /^chatDraft$/`) {
		t.Fatalf("composer does not bind one filtered debounced draft signal: %s", html)
	}
	if !strings.Contains(html, `data-on:submit__prevent=`) ||
		strings.Contains(html, `data-on:submit=`) ||
		strings.Contains(html, `input.value = &#39;&#39;`) {
		t.Fatalf(
			"composer does not prevent native submit without eagerly clearing the draft: %s",
			html,
		)
	}
	if !strings.Contains(html, "chatDraft: &#34;") ||
		!strings.Contains(html, `\&#34; and\nUnicode 🌰&#34;`) {
		t.Fatalf("initial draft is not safely encoded: %s", html)
	}
	if strings.Contains(html, "localStorage") ||
		strings.Contains(html, "sessionStorage") ||
		strings.Contains(html, "revision") ||
		strings.Contains(html, "acknowledg") {
		t.Fatalf("composer has client draft state: %s", html)
	}
	action := composerSubmitAction("@post('/send')")
	if !strings.Contains(action, "prompt.value = input.value") ||
		strings.Contains(action, "input.value = ''") ||
		strings.Contains(action, "dispatchEvent") ||
		strings.Contains(action, "localStorage") ||
		strings.Contains(action, "preventDefault") ||
		strings.Contains(action, "await") {
		t.Fatalf("submit action mutates or waits: %s", action)
	}
}

func TestAgentChatComposerStartsAsSingleLine(t *testing.T) {
	html := renderDraftComponent(t, AgentChatComposer(AgentChatComposerArgs{
		Action:    "@post('/send')",
		ThreadID:  "thread_1",
		HasThread: true,
	}))
	for _, want := range []string{
		`rows="1"`,
		`min-h-8`,
		`data-composer-shell`,
		`data-multiline="0"`,
		`rounded-full border`,
		`data-[multiline=1]:rounded-3xl`,
		`scrollHeight`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("composer missing %q: %s", want, html)
		}
	}
	for _, notWant := range []string{
		`min-h-[2rem]`,
	} {
		if strings.Contains(html, notWant) {
			t.Fatalf("composer still has tall empty state %q: %s", notWant, html)
		}
	}
	js := composerAutosizeJS()
	if strings.Contains(js, "\n") {
		t.Fatal("autosize JS injects a newline into the HTML attribute")
	}
	if !strings.Contains(js, `indexOf('\n')`) &&
		!strings.Contains(js, `indexOf("\n")`) {
		t.Fatalf("autosize JS missing newline check: %s", js)
	}
}

func TestSharedThreadDraftHydrationIsUserAndThreadScoped(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	createDraftThread(t, queries, "thread_2")
	for _, save := range []struct{ user, thread, draft string }{{"a@example.com", "thread_1", "a-one"}, {"b@example.com", "thread_1", "b-one"}, {"a@example.com", "thread_2", "a-two"}} {
		if err := service.SaveThreadDraft(
			t.Context(),
			save.user,
			save.thread,
			save.draft,
			nextDraftOperationOrder(),
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct{ user, thread, draft string }{{"a@example.com", "thread_1", "a-one"}, {"b@example.com", "thread_1", "b-one"}, {"a@example.com", "thread_2", "a-two"}} {
		component, err := service.RenderSharedThreadChat(
			t.Context(),
			want.thread,
			want.user,
		)
		if err != nil {
			t.Fatal(err)
		}
		html := renderDraftComponent(t, component)
		if !strings.Contains(html, want.draft) ||
			strings.Count(html, "workbench_v2=1") < 2 ||
			!strings.Contains(html, "/agent-chat/thread/"+want.thread+"/draft") {
			t.Fatalf(
				"%s/%s did not render V2 draft and request markers: %s",
				want.user,
				want.thread,
				html,
			)
		}
	}
}

type draftTestTemporal struct{ err error }

func (t draftTestTemporal) StartWorkflow(
	context.Context,
	string,
	any,
	any,
) (string, error) {
	return "workflow_1", t.err
}

func TestResumeEmbeddedSharedWorkspaceThreadDoesNotRequireWorkspaceOwner(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	if _, err := queries.CreateWorkspace(t.Context(), db.CreateWorkspaceParams{
		ID:           "workspace_1",
		UserEmail:    "owner@example.com",
		Title:        "Shared workspace",
		RootDocPath:  "thoughts/plan",
		WorkflowType: "agent-chat",
		Source:       "web",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpsertThreadWorkspaceAssociation(
		t.Context(),
		db.UpsertThreadWorkspaceAssociationParams{
			ThreadID:    "thread_1",
			WorkspaceID: "workspace_1",
			IsPrimary:   1,
			Role:        "primary",
			AdoptedFrom: "test",
		},
	); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/thoughts/chat/thread/thread_1/resume?workbench_v2=1",
		http.NoBody,
	)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues("thread_1")
	ctx.Set("user_email", "reader@example.com")
	err := handler.ResumeEmbeddedThread(ctx)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest ||
		httpErr.Message != "prompt is required" {
		t.Fatalf("shared resume error = %#v", err)
	}
}

func TestResumeEmbeddedThreadAcceptedClearsDraft(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.temporal = draftTestTemporal{}
	if err := service.SaveThreadDraft(
		t.Context(),
		"owner@example.com",
		"thread_1",
		"send this",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"send this"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/thread/thread_1/resume?workbench_v2=1",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues("thread_1")
	ctx.Set("user_email", "owner@example.com")
	if err := handler.ResumeEmbeddedThread(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"chatDraft":""`) ||
		strings.Contains(rec.Body.String(), "doc-right-chat-panel") ||
		strings.Contains(rec.Body.String(), "no target") {
		t.Fatalf("v2 response did not reset only the composer: %s", rec.Body.String())
	}
	draft, err := service.GetThreadDraft(t.Context(), "owner@example.com", "thread_1")
	if err != nil || draft != "" {
		t.Fatalf("draft = %q, %v", draft, err)
	}
}

func TestResumeEmbeddedThreadRejectedRetainsDraft(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	service.temporal = draftTestTemporal{err: errors.New("start failed")}
	if err := service.SaveThreadDraft(
		t.Context(),
		"owner@example.com",
		"thread_1",
		"keep this",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"send this"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/thread/thread_1/resume",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("thread_id")
	ctx.SetParamValues("thread_1")
	ctx.Set("user_email", "owner@example.com")
	if err := handler.ResumeEmbeddedThread(ctx); err == nil {
		t.Fatal("expected rejection")
	}
	if strings.Contains(rec.Body.String(), "chatDraft") ||
		strings.Contains(rec.Body.String(), "reset") {
		t.Fatalf("rejection mutated composer: %s", rec.Body.String())
	}
	draft, err := service.GetThreadDraft(t.Context(), "owner@example.com", "thread_1")
	if err != nil || draft != "keep this" {
		t.Fatalf("draft = %q, %v", draft, err)
	}
}

func TestResetAndFocusEmbeddedComposerPatchesAcceptedDraft(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	if err := (&Handler{}).resetAndFocusEmbeddedComposer(
		datastar.NewSSE(rec, req),
	); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"chatDraft":""`) ||
		!strings.Contains(body, "agent-chat-composer-input") {
		t.Fatalf("accepted response missing draft reset/focus: %s", body)
	}
}

func TestResumeEmbeddedWorkspaceThreadRejectedRetainsDraft(t *testing.T) {
	service, queries := newThreadDraftService(t)
	createDraftThread(t, queries, "thread_1")
	if err := service.SaveThreadDraft(
		t.Context(),
		"owner@example.com",
		"thread_1",
		"keep this",
		nextDraftOperationOrder(),
	); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, nil)
	values := url.Values{"prompt": {"send this"}}
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/workspace_missing/thread/thread_1/resume",
		strings.NewReader(values.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.SetParamNames("workspace_id", "thread_id")
	ctx.SetParamValues("workspace_missing", "thread_1")
	ctx.Set("user_email", "owner@example.com")
	if err := handler.ResumeEmbeddedWorkspaceThread(ctx); err == nil {
		t.Fatal("expected workspace rejection")
	}
	if strings.Contains(rec.Body.String(), "chatDraft") ||
		strings.Contains(rec.Body.String(), "reset") {
		t.Fatalf("rejection mutated composer: %s", rec.Body.String())
	}
	draft, err := service.GetThreadDraft(t.Context(), "owner@example.com", "thread_1")
	if err != nil || draft != "keep this" {
		t.Fatalf("draft = %q, %v", draft, err)
	}
}

func TestSaveThreadDraftRuntimeRoute(t *testing.T) {
	e := echo.New()
	NewHandler(&Service{}, nil).RegisterRuntimeRoutes(e.Group("/agent-chat"))
	for _, route := range e.Routes() {
		if route.Method == http.MethodPost &&
			route.Path == "/agent-chat/thread/:thread_id/draft" {
			return
		}
	}
	t.Fatal("draft runtime route is not registered")
}
