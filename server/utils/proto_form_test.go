package utils

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	commentsv1 "github.com/CoreyCole/vamos/pkg/proto/comments/v1"
)

func TestBindFormToProto_URLEncodedCamelCase(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Form data with camelCase field names (matching proto json mapping)
	form := url.Values{}
	form.Set("filePath", "thoughts/test.md")
	form.Set("commentText", "This is a test comment")
	form.Set("selectedText", "selected text here")
	form.Set("startLine", "10")
	form.Set("startColumn", "5")
	form.Set("endLine", "15")
	form.Set("endColumn", "20")
	form.Set("sectionId", "section-123")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	// Verify all fields were parsed correctly
	if msg.FilePath != "thoughts/test.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/test.md")
	}
	if msg.CommentText != "This is a test comment" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "This is a test comment")
	}
	if msg.SelectedText != "selected text here" {
		t.Errorf("SelectedText = %q, want %q", msg.SelectedText, "selected text here")
	}
	if msg.StartLine != 10 {
		t.Errorf("StartLine = %d, want %d", msg.StartLine, 10)
	}
	if msg.StartColumn != 5 {
		t.Errorf("StartColumn = %d, want %d", msg.StartColumn, 5)
	}
	if msg.EndLine != 15 {
		t.Errorf("EndLine = %d, want %d", msg.EndLine, 15)
	}
	if msg.EndColumn != 20 {
		t.Errorf("EndColumn = %d, want %d", msg.EndColumn, 20)
	}
	if msg.SectionId != "section-123" {
		t.Errorf("SectionId = %q, want %q", msg.SectionId, "section-123")
	}
}

func TestBindFormToProto_URLEncodedSnakeCase(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Form data with snake_case field names (also accepted by protojson)
	form := url.Values{}
	form.Set("file_path", "thoughts/snake.md")
	form.Set("comment_text", "Snake case comment")
	form.Set("selected_text", "snake selected")
	form.Set("start_line", "1")
	form.Set("start_column", "2")
	form.Set("end_line", "3")
	form.Set("end_column", "4")
	form.Set("section_id", "snake-section")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	if msg.FilePath != "thoughts/snake.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/snake.md")
	}
	if msg.CommentText != "Snake case comment" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "Snake case comment")
	}
	if msg.StartLine != 1 {
		t.Errorf("StartLine = %d, want %d", msg.StartLine, 1)
	}
	if msg.SectionId != "snake-section" {
		t.Errorf("SectionId = %q, want %q", msg.SectionId, "snake-section")
	}
}

func TestBindFormToProto_JSONFlat(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Flat JSON payload
	jsonBody := `{
		"filePath": "thoughts/json.md",
		"commentText": "JSON comment",
		"selectedText": "json selected",
		"startLine": 100,
		"endLine": 200
	}`

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	if msg.FilePath != "thoughts/json.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/json.md")
	}
	if msg.CommentText != "JSON comment" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "JSON comment")
	}
	if msg.StartLine != 100 {
		t.Errorf("StartLine = %d, want %d", msg.StartLine, 100)
	}
	if msg.EndLine != 200 {
		t.Errorf("EndLine = %d, want %d", msg.EndLine, 200)
	}
}

func TestBindFormToProto_JSONNestedWithFormID(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Nested JSON payload with explicit formID
	jsonBody := `{
		"commentForm": {
			"filePath": "thoughts/nested.md",
			"commentText": "Nested comment",
			"startLine": 42
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "commentForm")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	if msg.FilePath != "thoughts/nested.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/nested.md")
	}
	if msg.CommentText != "Nested comment" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "Nested comment")
	}
	if msg.StartLine != 42 {
		t.Errorf("StartLine = %d, want %d", msg.StartLine, 42)
	}
}

func TestBindFormToProto_JSONNestedWrongFormID(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Nested JSON payload with wrong formID - fields should not be found
	jsonBody := `{
		"commentForm": {
			"filePath": "thoughts/nested.md",
			"commentText": "Nested comment"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "wrongFormID")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	// Fields should be empty since formID didn't match
	if msg.FilePath != "" {
		t.Errorf("FilePath = %q, want empty (wrong formID)", msg.FilePath)
	}
	if msg.CommentText != "" {
		t.Errorf("CommentText = %q, want empty (wrong formID)", msg.CommentText)
	}
}

func TestBindFormToProto_IgnoresUnknownFields(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Form with extra fields (like CSRF token) that should be ignored
	form := url.Values{}
	form.Set("filePath", "thoughts/extra.md")
	form.Set("commentText", "With extras")
	form.Set("_csrf", "some-token")
	form.Set("unknownField", "should be ignored")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto should ignore unknown fields, got error: %v", err)
	}

	if msg.FilePath != "thoughts/extra.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/extra.md")
	}
	if msg.CommentText != "With extras" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "With extras")
	}
}

func TestBindFormToProto_CreateReplyRequest(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Test with CreateReplyRequest proto
	form := url.Values{}
	form.Set("commentId", "comment-uuid-123")
	form.Set("replyText", "This is a reply")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateReplyRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	if msg.CommentId != "comment-uuid-123" {
		t.Errorf("CommentId = %q, want %q", msg.CommentId, "comment-uuid-123")
	}
	if msg.ReplyText != "This is a reply" {
		t.Errorf("ReplyText = %q, want %q", msg.ReplyText, "This is a reply")
	}
}

func TestBindFormToProto_EmptyOptionalFields(t *testing.T) {
	t.Parallel()
	e := echo.New()

	// Only required fields, optional fields should default to zero values
	form := url.Values{}
	form.Set("filePath", "thoughts/minimal.md")
	form.Set("commentText", "Minimal comment")

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var msg commentsv1.CreateCommentRequest
	err := BindFormToProto(c, &msg, "")
	if err != nil {
		t.Fatalf("BindFormToProto failed: %v", err)
	}

	if msg.FilePath != "thoughts/minimal.md" {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, "thoughts/minimal.md")
	}
	if msg.CommentText != "Minimal comment" {
		t.Errorf("CommentText = %q, want %q", msg.CommentText, "Minimal comment")
	}
	// Optional fields should be zero values
	if msg.SelectedText != "" {
		t.Errorf("SelectedText = %q, want empty string", msg.SelectedText)
	}
	if msg.StartLine != 0 {
		t.Errorf("StartLine = %d, want 0", msg.StartLine)
	}
	if msg.SectionId != "" {
		t.Errorf("SectionId = %q, want empty string", msg.SectionId)
	}
}
