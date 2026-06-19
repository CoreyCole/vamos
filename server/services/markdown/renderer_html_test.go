package markdown

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLAppletRendererReturnsSandboxedFrame(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.html"), []byte("<h1>Demo</h1>"))

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.RenderThoughtsDocument(t.Context(), "demo.html")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindHTMLApplet {
		t.Fatalf("kind=%q", page.ViewerArgs.DocumentKind)
	}
	if page.ViewerArgs.CommentMode != CommentModeDocumentOnly {
		t.Fatalf("comment mode=%q", page.ViewerArgs.CommentMode)
	}

	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/thoughts/_render/html/demo.html"`) {
		t.Fatalf("missing iframe src: %s", html)
	}
	if !strings.Contains(html, `sandbox="allow-scripts allow-forms allow-downloads"`) {
		t.Fatalf("missing sandbox: %s", html)
	}
	if strings.Contains(html, "allow-same-origin") {
		t.Fatalf("sandbox permits same-origin: %s", html)
	}
}
