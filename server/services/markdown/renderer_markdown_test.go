package markdown

import (
	"path/filepath"
	"testing"
)

func TestRenderThoughtsDocumentMarkdownParity(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner"))
	mustWriteFile(t, filepath.Join(root, "owner", "doc.md"), []byte("---\ntopic: Demo\n---\n# Heading\n\nBody"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RenderThoughtsDocument(t.Context(), "owner/doc")
	if err != nil {
		t.Fatal(err)
	}
	if page.FilePath != "thoughts/owner/doc.md" {
		t.Fatalf("FilePath=%q", page.FilePath)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindMarkdown {
		t.Fatalf("kind=%q", page.ViewerArgs.DocumentKind)
	}
	if page.ViewerArgs.Frontmatter == nil || page.ViewerArgs.Frontmatter.Topic != "Demo" {
		t.Fatalf("frontmatter=%+v", page.ViewerArgs.Frontmatter)
	}
	if len(page.TableOfContents) != 1 || len(page.ViewerArgs.Sections) == 0 {
		t.Fatalf("missing markdown metadata: %+v", page)
	}
	if page.ViewerArgs.RawMarkdown != "# Heading\n\nBody" {
		t.Fatalf("RawMarkdown=%q", page.ViewerArgs.RawMarkdown)
	}
	if page.ViewerArgs.HTMLContent == "" {
		t.Fatal("HTMLContent empty")
	}
}
