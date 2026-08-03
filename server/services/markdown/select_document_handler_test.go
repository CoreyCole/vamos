package markdown

import (
	"database/sql"
	stdhtml "html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"

	pkgdb "github.com/CoreyCole/vamos/pkg/db"
	commentsvc "github.com/CoreyCole/vamos/server/services/comments"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
	"github.com/labstack/echo/v4"
)

func newDocumentSelectionService(t *testing.T) (*Service, *dbsvc.Service) {
	t.Helper()
	root := t.TempDir()
	planDir := filepath.Join(root, "owner", "plan-a")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"AGENTS.md":  "# Workspace\n",
		"design.md":  "# Design\n",
		"outline.md": "# Outline\n",
	} {
		if err := os.WriteFile(
			filepath.Join(planDir, name),
			[]byte(content),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}
	database, err := dbsvc.NewService(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	commentService := commentsvc.NewService(
		database.DB(),
		"test-commit",
		"https://github.com/example/repo/blob",
		root,
	)
	service, err := NewService(root, commentService, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, database
}

func newPostFormContext(
	t *testing.T,
	path string,
	form url.Values,
) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	return c, rec
}

func TestPreserveEmbeddedChatQueryRemovesStaleWorkspaceWhenThreadExists(t *testing.T) {
	t.Parallel()

	got := PreserveEmbeddedChatQuery(
		"/thoughts/creative-mode-agent?context=chat&chat_workspace=stale_ws",
		DocumentEmbeddedChatSelection{WorkspaceID: "ws_1", ThreadID: "th_1", RunID: "run_1"},
	)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", got, err)
	}
	query := parsed.Query()
	if gotValue := query.Get("chat_workspace"); gotValue != "" {
		t.Fatalf("query[%q] = %q, want empty in %q", "chat_workspace", gotValue, got)
	}
	if gotValue := query.Get("thread"); gotValue != "th_1" {
		t.Fatalf("query[thread] = %q, want th_1 in %q", gotValue, got)
	}
}

func TestThoughtsDocURLWithChatStatePreservesChatQuery(t *testing.T) {
	t.Parallel()

	got := ThoughtsDocURLWithChatState(
		"creative-mode-agent/plans/a/next.md",
		"section-1",
		DocumentEmbeddedChatSelection{
			WorkspaceID: "ws_1",
			ThreadID:    "th_1",
			RunID:       "run_1",
		},
	)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", got, err)
	}
	if parsed.Path != "/thoughts/creative-mode-agent/plans/a/next.md" ||
		parsed.Fragment != "section-1" {
		t.Fatalf("url = %q, path=%q fragment=%q", got, parsed.Path, parsed.Fragment)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"context": "chat",
		"thread":  "th_1",
		"run":     "run_1",
	} {
		if gotValue := query.Get(key); gotValue != want {
			t.Fatalf("query[%q] = %q, want %q in %q", key, gotValue, want, got)
		}
	}
	if gotValue := query.Get("chat_workspace"); gotValue != "" {
		t.Fatalf("query[%q] = %q, want empty in %q", "chat_workspace", gotValue, got)
	}
}

func createDocumentSelectionComment(t *testing.T, database *dbsvc.Service, id string) {
	t.Helper()
	_, err := database.Queries.CreateDocumentComment(t.Context(), pkgdb.CreateDocumentCommentParams{
		ID:            id,
		WorkspaceRoot: "owner/plan-a",
		DocPath:       "thoughts/owner/plan-a/design.md",
		UserEmail:     "user@example.com",
		CommentText:   "Needs review",
		SelectedText:  "Design",
		SectionHint:   sql.NullString{String: "design", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateDocumentComment() error = %v", err)
	}
}

func selectCommentFormValues(t *testing.T, rendered string) url.Values {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(rendered))
	if err != nil {
		t.Fatal(err)
	}
	var selected *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if selected != nil {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "form" && formHasOpenCommentButton(node) {
			selected = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if selected == nil {
		t.Fatalf("select-comment form not found in %s", rendered)
	}
	values := url.Values{}
	var inputs func(*xhtml.Node)
	inputs = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "input" {
			name, value := "", ""
			for _, attr := range node.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				}
			}
			if name != "" {
				values.Set(name, value)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inputs(child)
		}
	}
	inputs(selected)
	return values
}

func formHasOpenCommentButton(form *xhtml.Node) bool {
	var found bool
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "button" {
			for _, attr := range node.Attr {
				if attr.Key == "aria-label" && attr.Val == "Open comment in document" {
					found = true
					return
				}
			}
		}
		for child := node.FirstChild; child != nil && !found; child = child.NextSibling {
			walk(child)
		}
	}
	walk(form)
	return found
}

func assertThreadsSelectionURLPatch(t *testing.T, body string) {
	t.Helper()
	marker := `data-replace-url="`
	start := strings.LastIndex(body, marker)
	if start < 0 {
		t.Fatalf("URL patch missing from %s", body)
	}
	value := body[start+len(marker):]
	end := strings.Index(value, `"`)
	if end < 0 {
		t.Fatalf("URL patch value unterminated in %s", body)
	}
	quoted := stdhtml.UnescapeString(value[:end])
	patched, err := strconv.Unquote(quoted)
	if err != nil {
		t.Fatalf("unquote URL patch %q: %v", quoted, err)
	}
	parsed, err := url.Parse(patched)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"context": "threads", "chat_workspace": "ws_1", "thread": "chat_1",
		"run": "run_1", "hermes_thread": "hermes_1",
	} {
		if parsed.Query().Get(key) != want {
			t.Fatalf("patched URL %q query[%s] = %q, want %q", patched, key, parsed.Query().Get(key), want)
		}
	}
}

func TestRenderedThoughtsCommentCardSubmitsCombinedWorkbenchState(t *testing.T) {
	service, database := newDocumentSelectionService(t)
	createDocumentSelectionComment(t, database, "comment-card")
	comments, err := service.commentService.GetCommentsForScopeInternal(t.Context(), "thoughts/owner/plan-a/design.md")
	if err != nil {
		t.Fatal(err)
	}
	page := &PageArgs{
		FilePath: "owner/plan-a/design.md",
		WorkbenchLinkState: ThoughtsWorkbenchLinkState{
			Context: "threads", ChatWorkspaceID: "ws_1", ChatThreadID: "chat_1", ChatRunID: "run_1",
			HermesThreadID: "hermes_1", SelectedPlanPath: "owner/plan-a",
		},
	}
	component := CommentsRightRailPanel(service.buildCommentUI(page, "user@example.com", thoughtsCommentThreads(comments.Comments)))
	var rendered strings.Builder
	if err := component.Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	form := selectCommentFormValues(t, rendered.String())
	for key, want := range map[string]string{"context": "threads", "chat_workspace": "ws_1", "thread": "chat_1", "run": "run_1", "hermes_thread": "hermes_1"} {
		if form.Get(key) != want {
			t.Fatalf("rendered form[%s] = %q, want %q", key, form.Get(key), want)
		}
	}
	context, recorder := newPostFormContext(t, "/thoughts/actions/select-comment", form)
	if err := service.HandleSelectComment(context); err != nil {
		t.Fatal(err)
	}
	assertThreadsSelectionURLPatch(t, recorder.Body.String())
}

func TestInPlaceCommentsPanelSubmitsCombinedWorkbenchState(t *testing.T) {
	service, database := newDocumentSelectionService(t)
	createDocumentSelectionComment(t, database, "comment-panel")
	openContext, openRecorder := newPostFormContext(t, "/thoughts/actions/open-comments", url.Values{
		"doc_path": {"thoughts/owner/plan-a/design.md"}, "context": {"threads"},
		"chat_workspace": {"ws_1"}, "thread": {"chat_1"}, "run": {"run_1"}, "hermes_thread": {"hermes_1"},
	})
	if err := service.HandleOpenCommentsInPlace(openContext); err != nil {
		t.Fatal(err)
	}
	form := selectCommentFormValues(t, openRecorder.Body.String())
	for key, want := range map[string]string{"context": "threads", "chat_workspace": "ws_1", "thread": "chat_1", "run": "run_1", "hermes_thread": "hermes_1"} {
		if form.Get(key) != want {
			t.Fatalf("in-place form[%s] = %q, want %q", key, form.Get(key), want)
		}
	}
	selectContext, selectRecorder := newPostFormContext(t, "/thoughts/actions/select-comment", form)
	if err := service.HandleSelectComment(selectContext); err != nil {
		t.Fatal(err)
	}
	assertThreadsSelectionURLPatch(t, selectRecorder.Body.String())
}

func TestHandleSelectCommentPatchesTargetDocumentAndPreservesChatState(t *testing.T) {
	service, database := newDocumentSelectionService(t)
	createDocumentSelectionComment(t, database, "comment-1")
	c, rec := newPostFormContext(t, "/thoughts/actions/select-comment", url.Values{
		"comment_id":     {"comment-1"},
		"chat_workspace": {"ws_1"},
		"thread_id":      {"th_1"},
		"run_id":         {"run_1"},
	})

	if err := service.HandleSelectComment(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #workbench-root",
		"thoughts-document-panel",
		"thoughts-shared-sidebar",
		"thoughts-url-sync",
		`data-replace-url="&#34;/thoughts/owner/plan-a/design.md?chat_workspace=ws_1&amp;context=chat&amp;run=run_1&amp;thread=th_1#design&#34;"`,
		"workbench-section-nav",
		`detail: { hash: 'design', updateURL: false }`,
		"comment-thread-comment-1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
}
