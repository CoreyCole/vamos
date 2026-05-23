//go:build !integration || unit
// +build !integration unit

package comments

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestCommentForm_FieldsParseToProto(t *testing.T) {
	t.Parallel()

	// 1. Create component args with test values
	args := CommentFormArgs{
		ID:           "test-form",
		FilePath:     "thoughts/test.md",
		SelectedText: "selected text here",
		StartLine:    10,
		StartColumn:  5,
		EndLine:      15,
		EndColumn:    20,
		SectionID:    "section-123",
	}

	// 2. Render component to goquery document
	helper := testhelpers.RenderToDocument(t, CommentForm(args))

	// 3. Extract form values - form element has id="test-form"
	formValues := helper.GetFormValues("#test-form")

	// 4. Add user input for textarea (comment_text field requires user input)
	formValues.Set("comment_text", "Test comment text")

	// 5. Verify expected fields are present in form
	expectedFields := []string{
		"doc_path",
		"selected_text",
		"start_line",
		"start_column",
		"end_line",
		"end_column",
		"section_hint",
	}
	for _, field := range expectedFields {
		if !formValues.Has(field) {
			t.Errorf("Missing expected form field: %s", field)
		}
	}

	// 6. Create echo context with form data
	req := httptest.NewRequest(
		http.MethodPost,
		"/",
		bytes.NewBufferString(formValues.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	e := echo.New()
	ctx := e.NewContext(req, httptest.NewRecorder())

	// 7. Parse the canonical shared comment form names used by handlers.
	parsed := parseCommentForm(ctx)

	// 8. Verify parsed fields match original args
	if parsed.FilePath != args.FilePath {
		t.Errorf("FilePath = %q, want %q", parsed.FilePath, args.FilePath)
	}
	if parsed.SelectedText != args.SelectedText {
		t.Errorf("SelectedText = %q, want %q", parsed.SelectedText, args.SelectedText)
	}
	if parsed.StartLine != args.StartLine {
		t.Errorf("StartLine = %d, want %d", parsed.StartLine, args.StartLine)
	}
	if parsed.StartColumn != args.StartColumn {
		t.Errorf("StartColumn = %d, want %d", parsed.StartColumn, args.StartColumn)
	}
	if parsed.EndLine != args.EndLine {
		t.Errorf("EndLine = %d, want %d", parsed.EndLine, args.EndLine)
	}
	if parsed.EndColumn != args.EndColumn {
		t.Errorf("EndColumn = %d, want %d", parsed.EndColumn, args.EndColumn)
	}
	if parsed.SectionID != args.SectionID {
		t.Errorf("SectionID = %q, want %q", parsed.SectionID, args.SectionID)
	}
	if parsed.CommentText != "Test comment text" {
		t.Errorf("CommentText = %q, want %q", parsed.CommentText, "Test comment text")
	}
}

func TestCommentForm_SelectedTextIsHiddenAndPreviewOnly(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentForm(
		CommentFormArgs{
			ID:           "test-form",
			FilePath:     "thoughts/test.md",
			SelectedText: "selected text",
		},
	).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `name="selected_text" value="selected text"`) {
		t.Fatalf("selected text hidden input missing: %s", html)
	}
	if !strings.Contains(html, "selected text") {
		t.Fatalf("selected text preview missing: %s", html)
	}
	if strings.Contains(html, `textarea name="selected_text"`) {
		t.Fatalf("selected text rendered as editable textarea: %s", html)
	}
}

func TestDiscussionsMenuEntryRendersWithZeroComments(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := DiscussionsMenuEntry(
		&GetCommentsResponse{},
		"user@example.com",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"Discussions", "$comments_sheet.open = true", "$user_profile.open = false"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
}

func TestReplyInput_FieldsParseToProto(t *testing.T) {
	t.Parallel()

	// Test data
	commentID := "test-comment-123"
	filePath := "thoughts/test/document.md"
	userEmail := "test@example.com"

	// Render the ReplyInput component
	helper := testhelpers.RenderToDocument(t, ReplyInput(commentID, filePath, userEmail))

	// The form ID is "reply_{commentID}"
	formSelector := "#reply_" + commentID
	formValues := helper.GetFormValues(formSelector)

	// Verify hidden fields are present with correct names
	expectedFields := []string{"comment_id", "doc_path", "reply_text"}
	for _, field := range expectedFields {
		if !formValues.Has(field) {
			t.Errorf("Missing expected form field: %s", field)
		}
	}

	// Verify field values
	if formValues.Get("comment_id") != commentID {
		t.Errorf("comment_id = %q, want %q", formValues.Get("comment_id"), commentID)
	}
	if formValues.Get("doc_path") != filePath {
		t.Errorf("doc_path = %q, want %q", formValues.Get("doc_path"), filePath)
	}

	// Add user input for textarea (reply_text field requires user input)
	formValues.Set("reply_text", "Test reply text")

	if formValues.Get("reply_text") != "Test reply text" {
		t.Errorf(
			"reply_text = %q, want %q",
			formValues.Get("reply_text"),
			"Test reply text",
		)
	}
}
