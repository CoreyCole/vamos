package markdown

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVRendererEscapesCells(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.csv"), []byte("name,value\n<script>,1"))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.csv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindCSVTable {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindCSVTable)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "<script>") {
		t.Fatalf("CSV cell unescaped: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("escaped cell missing: %s", html)
	}
}

func TestParseCSVTableTruncatesRows(t *testing.T) {
	t.Parallel()

	table, err := parseCSVTable([]byte("a,b\n1,2\n3,4"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !table.Truncated {
		t.Fatal("table.Truncated=false")
	}
	if len(table.Rows) != 1 || table.Rows[0][0] != "1" {
		t.Fatalf("rows=%#v", table.Rows)
	}
}
