package markdown

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedRendererHandlesUnsafeExactFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.bin"), []byte{'V', 'A', 0, 'M', 'O'})
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.bin")
	if err != nil {
		t.Fatal(err)
	}
	if page.FilePath != "thoughts/data.bin" {
		t.Fatalf("FilePath=%q", page.FilePath)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindUnsupported {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindUnsupported)
	}
	if page.ViewerArgs.CommentMode != CommentModeDocumentOnly {
		t.Fatalf("CommentMode=%q, want %q", page.ViewerArgs.CommentMode, CommentModeDocumentOnly)
	}

	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{"Unsupported document type", "thoughts/data.bin", ".bin", "File is binary"} {
		if !strings.Contains(html, want) {
			t.Fatalf("unsupported component missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, root) {
		t.Fatalf("unsupported component exposed temp root: %s", html)
	}
}

func TestBuildCommentUINonSectionDocumentsDisableSelectionSignals(t *testing.T) {
	t.Parallel()

	service := &Service{}
	page := &PageArgs{
		FilePath: "thoughts/app.html",
		ViewerArgs: ViewerArgs{
			DocumentKind: DocumentKindHTMLApplet,
			CommentMode:  CommentModeDocumentOnly,
		},
	}
	commentUI := service.buildCommentUI(page, "playwright@localhost", nil)
	if commentUI.DocPath != "thoughts/app.html" {
		t.Fatalf("DocPath=%q", commentUI.DocPath)
	}
	if commentUI.SelectionSignals.ShowRoute != "" || commentUI.SelectionSignals.ContainerID != "" {
		t.Fatalf("selection signals enabled for document-only comments: %#v", commentUI.SelectionSignals)
	}
}
