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

	"github.com/labstack/echo/v4"
	pkgdb "github.com/CoreyCole/vamos/pkg/db"
	commentsvc "github.com/CoreyCole/vamos/server/services/comments"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func newSelectCommentV2Service(t *testing.T) (*Service, *dbsvc.Service) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "owner", "plan-a"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "owner", "plan-a", "AGENTS.md"), []byte("# Workspace\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "owner", "plan-a", "design.md"), []byte("# Design\n"), 0o644); err != nil { t.Fatal(err) }
	database, err := dbsvc.NewService(filepath.Join(t.TempDir(), "comments.db")); if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	comments := commentsvc.NewService(database.DB(), "test-commit", "https://github.com/example/repo/blob", root)
	service, err := NewService(root, comments, nil); if err != nil { t.Fatal(err) }
	return service, database
}

func TestHandleSelectCommentForWorkbenchV2FocusesMountedTarget(t *testing.T) {
	service, database := newSelectCommentV2Service(t)
	_, err := database.Queries.CreateDocumentComment(t.Context(), pkgdb.CreateDocumentCommentParams{
		ID: "comment-1", WorkspaceRoot: "owner/plan-a", DocPath: "thoughts/owner/plan-a/design.md", UserEmail: "user@example.com", CommentText: "Question", SectionHint: sql.NullString{String: "design", Valid: true},
	})
	if err != nil { t.Fatal(err) }
	form := url.Values{"comment_id":{"comment-1"}, "doc_path":{"thoughts/owner/plan-a/design.md"}, "workbench_v2":{"1"}}
	req := httptest.NewRequest(http.MethodPost, "/thoughts/actions/select-comment", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder(); c := echo.New().NewContext(req, rec); c.Set("user_email", "user@example.com")
	if err := service.HandleSelectComment(c); err != nil { t.Fatal(err) }
	body := rec.Body.String()
	for _, want := range []string{"workbenchV2Comments", "visible", `document.getElementById("comment-target-thoughts--thoughts-owner-plan-a-design-md`} { if !strings.Contains(body, want) { t.Fatalf("response missing %q: %s", want, body) } }
	for _, unwanted := range []string{"selector #workbench-root", "rightRailActiveTab", "docWorkbenchRight", "doc-right-comments-panel"} { if strings.Contains(body, unwanted) { t.Fatalf("response retained %q: %s", unwanted, body) } }
}
