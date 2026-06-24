//go:build !integration || unit
// +build !integration unit

package comments

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/commentui"
	dbsvc "github.com/CoreyCole/vamos/server/services/db"
)

func newTestCommentsService(t *testing.T) *Service {
	t.Helper()
	return newTestCommentsServiceWithBase(t, filepath.Join(t.TempDir(), "thoughts"))
}

func newTestCommentsServiceWithBase(t *testing.T, markdownBasePath string) *Service {
	t.Helper()
	database, err := dbsvc.NewService(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewService(
		database.DB(),
		"test-commit",
		"https://github.com/example/repo/blob",
		markdownBasePath,
	)
}

func mkdirAllWithAgents(t *testing.T, dir string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# workspace\n"), 0o644)
}

func newCommentFormRequest(
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

func TestGetCommentsForScopeInternalReturnsQRSPIRootCommentsCurrentFirst(t *testing.T) {
	t.Parallel()
	markdownBase := filepath.Join(t.TempDir(), "thoughts")
	if err := mkdirAllWithAgents(
		t,
		filepath.Join(markdownBase, "creative-mode-agent", "plans", "plan-a"),
	); err != nil {
		t.Fatalf("create workspace marker: %v", err)
	}
	svc := newTestCommentsServiceWithBase(t, markdownBase)
	currentPath := "thoughts/creative-mode-agent/plans/plan-a/design.md"
	siblingPath := "thoughts/creative-mode-agent/plans/plan-a/outline.md"
	outsidePath := "thoughts/creative-mode-agent/plans/plan-b/design.md"

	for _, input := range []struct {
		path string
		text string
	}{
		{path: siblingPath, text: "sibling"},
		{path: currentPath, text: "current"},
		{path: outsidePath, text: "outside"},
	} {
		if _, err := svc.createCommentInternal(
			t.Context(),
			"user@example.com",
			CreateCommentRequest{
				FilePath:    input.path,
				CommentText: input.text,
				SectionID:   "document",
			},
		); err != nil {
			t.Fatalf("createCommentInternal(%q) error = %v", input.path, err)
		}
	}

	response, err := svc.GetCommentsForScopeInternal(t.Context(), currentPath)
	if err != nil {
		t.Fatalf("GetCommentsForScopeInternal() error = %v", err)
	}
	if len(response.Comments) != 2 {
		t.Fatalf("len(response.Comments) = %d, want 2", len(response.Comments))
	}
	if got := response.Comments[0].Comment.DocPath; got != currentPath {
		t.Fatalf("first comment path = %q, want current path %q", got, currentPath)
	}
	if got := response.Comments[1].Comment.DocPath; got != siblingPath {
		t.Fatalf("second comment path = %q, want sibling path %q", got, siblingPath)
	}
}

func TestThoughtsCommentCreateWithoutAgentChatWorkspaceUsesDocumentScope(
	t *testing.T,
) {
	t.Parallel()
	svc := newTestCommentsService(t)
	path := "thoughts/creative-mode-agent/plans/no-owner/doc.md"

	comment, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:     path,
			CommentText:  "works without workspace",
			SectionID:    "document",
			SelectedText: "selected quote",
		},
	)
	if err != nil {
		t.Fatalf("createCommentInternal() error = %v", err)
	}
	if got := comment.DocPath; got != path {
		t.Fatalf("DocPath = %q, want %q", got, path)
	}
	if comment.WorkspaceRoot != "" {
		t.Fatalf(
			"WorkspaceRoot = %q, want empty root for non-workspace doc",
			comment.WorkspaceRoot,
		)
	}

	response, err := svc.GetCommentsForFileInternal(t.Context(), path)
	if err != nil {
		t.Fatalf("GetCommentsForFileInternal() error = %v", err)
	}
	if len(response.Comments) != 1 {
		t.Fatalf("len(response.Comments) = %d, want 1", len(response.Comments))
	}
	got := response.Comments[0].Comment
	if got.ID != comment.ID || got.DocPath != path ||
		got.CommentText != "works without workspace" {
		t.Fatalf("listed comment = %#v, want created comment for %q", got, path)
	}
}

func TestThoughtsCommentShowFormPatchesMountedTargets(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	form := url.Values{}
	form.Set("file_path", "thoughts/plan.md")
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	form.Set("selected_text", "Selected paragraph")
	c, rec := newCommentFormRequest(t, "/forms/comments/show", form)

	if err := svc.HandleShowCommentForm(c); err != nil {
		t.Fatalf("HandleShowCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Selected paragraph`,
		commentui.CommentsContextPanelID,
		`rightRailActiveTab`,
		`section_section-1_form_open`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, commentui.MobileSectionCommentContentID) {
		t.Fatalf("canonical doc comment response patched legacy mobile target: %s", body)
	}
	if strings.Contains(body, "PatchElementsNoTargetsFound") {
		t.Fatalf("response contains Datastar target error: %s", body)
	}
}

func TestThoughtsCommentShowFormPatchOnlyTargetOmitsVisibleSectionChrome(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	form := url.Values{}
	form.Set("file_path", "thoughts/source.go")
	form.Set("section_hint", "document")
	form.Set("heading_hint", "Document")
	form.Set("selected_text", "Selected paragraph")
	form.Set("comment_target_chrome", string(commentui.CommentTargetChromePatchOnly))
	c, rec := newCommentFormRequest(t, "/forms/comments/show", form)

	if err := svc.HandleShowCommentForm(c); err != nil {
		t.Fatalf("HandleShowCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="selected_text" value="Selected paragraph"`,
		`commentui-popover-target`,
		commentui.TargetID(commentui.SafeCommentTargetSlug("thoughts", "thoughts/source.go"), "document"),
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("patch-only show response missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{`aria-label="Section actions"`, `Add comment`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("patch-only show response included visible section chrome %q: %s", unwanted, body)
		}
	}
}

func TestThoughtsCommentCreatePatchOnlyTargetDoesNotReintroduceSectionChrome(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	form := url.Values{}
	form.Set("file_path", "thoughts/source.go")
	form.Set("section_hint", "document")
	form.Set("heading_hint", "Document")
	form.Set("selected_text", "Selected paragraph")
	form.Set("comment_text", "Please clarify")
	form.Set("comment_target_chrome", string(commentui.CommentTargetChromePatchOnly))
	c, rec := newCommentFormRequest(t, "/forms/comments", form)

	if err := svc.HandleCommentForm(c); err != nil {
		t.Fatalf("HandleCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Please clarify`,
		commentui.TargetID(commentui.SafeCommentTargetSlug("thoughts", "thoughts/source.go"), "document"),
		`data-comment-target="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("patch-only create response missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{`aria-label="Section actions"`, `Add comment`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("patch-only create response included visible section chrome %q: %s", unwanted, body)
		}
	}
}

func TestThoughtsCommentCreatePatchesInlineTarget(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	form := url.Values{}
	form.Set("file_path", "thoughts/plan.md")
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Plan")
	form.Set("comment_text", "Please clarify")
	c, rec := newCommentFormRequest(t, "/forms/comments", form)

	if err := svc.HandleCommentForm(c); err != nil {
		t.Fatalf("HandleCommentForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Please clarify`,
		commentui.CommentsContextPanelID,
		`rightRailActiveTab`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, commentui.MobileSectionCommentContentID) {
		t.Fatalf("canonical doc comment response patched legacy mobile target: %s", body)
	}
	if strings.Contains(body, "PatchElementsNoTargetsFound") {
		t.Fatalf("response contains Datastar target error: %s", body)
	}
}

func TestThoughtsCommentExpandOpensRightRail(t *testing.T) {
	t.Parallel()
	markdownBase := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(markdownBase, "plan.md"),
		[]byte("# Overview\n\nIntro text.\n\n## Implementation Notes\n\nBody.\n\n## Risks\n\nRisk body."),
		0o644,
	); err != nil {
		t.Fatalf("write markdown fixture: %v", err)
	}
	svc := newTestCommentsServiceWithBase(t, markdownBase)
	if _, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:    "thoughts/plan.md",
			CommentText: "Question",
			SectionID:   "section-1",
		},
	); err != nil {
		t.Fatalf("createCommentInternal() error = %v", err)
	}
	if _, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:    "thoughts/plan.md",
			CommentText: "Question in another section",
			SectionID:   "section-2",
		},
	); err != nil {
		t.Fatalf("createCommentInternal() second comment error = %v", err)
	}
	form := url.Values{}
	form.Set("doc_path", "thoughts/plan.md")
	form.Set("section_hint", "section-1")
	form.Set("heading_hint", "Implementation Notes")
	c, rec := newCommentFormRequest(t, "/forms/comments/expand", form)

	if err := svc.HandleExpandSectionComments(c); err != nil {
		t.Fatalf("HandleExpandSectionComments() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Question`,
		`Question in another section`,
		commentui.CommentsContextPanelID,
		`rightRailActiveTab`,
		`docWorkbenchRight`,
		`visible`,
		`Implementation Notes`,
		`Risks`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "PatchElementsNoTargetsFound") {
		t.Fatalf("response contains Datastar target error: %s", body)
	}
}

func TestThoughtsCommentReplyPatchesInlineTarget(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	comment, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:    "thoughts/plan.md",
			CommentText: "Question",
			SectionID:   "section-1",
		},
	)
	if err != nil {
		t.Fatalf("createCommentInternal() error = %v", err)
	}
	form := url.Values{}
	form.Set("doc_path", "thoughts/plan.md")
	form.Set("comment_id", comment.ID)
	form.Set("reply_text", "Reply from sidebar test")
	c, rec := newCommentFormRequest(t, "/forms/replies", form)

	if err := svc.HandleReplyForm(c); err != nil {
		t.Fatalf("HandleReplyForm() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Reply from sidebar test`,
		commentui.CommentsContextPanelID,
		`rightRailActiveTab`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}

func TestThoughtsCommentResolvePatchesInlineTarget(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	comment, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:    "thoughts/plan.md",
			CommentText: "Question",
			SectionID:   "section-1",
		},
	)
	if err != nil {
		t.Fatalf("createCommentInternal() error = %v", err)
	}
	form := url.Values{}
	form.Set("doc_path", "thoughts/plan.md")
	form.Set("comment_id", comment.ID)
	c, rec := newCommentFormRequest(t, "/forms/resolve", form)

	if err := svc.HandleResolveComment(c); err != nil {
		t.Fatalf("HandleResolveComment() error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`comment-target-`,
		commentui.CommentsContextPanelID,
		`rightRailActiveTab`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}
