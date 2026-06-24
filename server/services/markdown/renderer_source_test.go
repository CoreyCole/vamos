package markdown

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceRendererRendersJSONAsSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := []byte("{\"ok\": true}\n")
	mustWriteFile(t, filepath.Join(root, "data.json"), content)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.json")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindSource {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindSource)
	}
	if page.ViewerArgs.CommentMode != CommentModeSelectionOnly {
		t.Fatalf("CommentMode=%q, want %q", page.ViewerArgs.CommentMode, CommentModeSelectionOnly)
	}
	if page.ViewerArgs.RawMarkdown != string(content) {
		t.Fatalf("RawMarkdown=%q", page.ViewerArgs.RawMarkdown)
	}

	html := page.ViewerArgs.HTMLContent
	for _, want := range []string{"source-document-content", "source-code-block", `id="L1"`, `class="ln"`, "ok"} {
		if !strings.Contains(html, want) {
			t.Fatalf("source component missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "thoughts/data.json") {
		t.Fatalf("source component should not repeat document path already present in URL: %s", html)
	}
}

func TestSourceRendererEscapesSourceHTML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "script.txt"), []byte("<script>alert(1)</script>"))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "script.txt")
	if err != nil {
		t.Fatal(err)
	}
	html := page.ViewerArgs.HTMLContent
	if strings.Contains(html, "<script>") {
		t.Fatalf("source HTML was not escaped: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("escaped source missing: %s", html)
	}
}

func TestSourceRendererRejectsNullByteBinary(t *testing.T) {
	t.Parallel()

	assertUnsafeSourceUnsupported(t, []byte{'V', 'A', 0, 'M', 'O'}, "File is binary")
}

func TestSourceRendererRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	assertUnsafeSourceUnsupported(t, []byte{0xff, 0xfe, 0xfd}, "not valid UTF-8")
}

func TestSourceRendererRejectsOversize(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "large.txt"), bytes.Repeat([]byte("a"), int(sourceDisplayMaxBytes)+1))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindUnsupported {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindUnsupported)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "too large") {
		t.Fatalf("oversize reason missing: %s", html)
	}
	if strings.Contains(html, root) {
		t.Fatalf("unsupported output exposed temp root: %s", html)
	}
}

func TestSourceLanguageForExtension(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		".go":    "go",
		".templ": "go-html-template",
		".json":  "json",
		".yaml":  "yaml",
		".ts":    "typescript",
		".js":    "javascript",
		".sql":   "sql",
		".css":   "css",
		".sh":    "bash",
		".txt":   "",
	}
	for ext, want := range cases {
		if got := sourceLanguageForExtension(ext); got != want {
			t.Fatalf("sourceLanguageForExtension(%q)=%q, want %q", ext, got, want)
		}
	}
}

func assertUnsafeSourceUnsupported(t *testing.T, content []byte, reasonSubstring string) {
	t.Helper()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.txt"), content)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.txt")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindUnsupported {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindUnsupported)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, reasonSubstring) {
		t.Fatalf("unsupported reason missing %q: %s", reasonSubstring, html)
	}
	if strings.Contains(html, root) {
		t.Fatalf("unsupported output exposed temp root: %s", html)
	}
}
