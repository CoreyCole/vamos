package markdown

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestHandleSelectCommentPatchesTargetDocumentAndPreservesChatState(t *testing.T) {
	service, database := newDocumentSelectionService(t)
	_, err := database.Queries.CreateDocumentComment(t.Context(), pkgdb.CreateDocumentCommentParams{
		ID:           "comment-1",
		DocPath:      "thoughts/owner/plan-a/design.md",
		UserEmail:    "user@example.com",
		CommentText:  "Needs review",
		SelectedText: "Design",
		SectionHint: sql.NullString{
			String: "design",
			Valid:  true,
		},
	})
	if err != nil {
		t.Fatalf("CreateDocumentComment() error = %v", err)
	}
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
		`data-replace-url="&#34;/thoughts/owner/plan-a/design.md?context=chat&amp;run=run_1&amp;thread=th_1#design&#34;"`,
		"workbench-section-nav",
		`detail: { hash: 'design', updateURL: false }`,
		"comment-thread-comment-1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q:\n%s", want, body)
		}
	}
}
