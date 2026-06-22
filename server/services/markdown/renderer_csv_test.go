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
	if strings.Contains(html, "CSV table:") {
		t.Fatalf("CSV renderer includes duplicate chrome: %s", html)
	}
	if strings.Contains(html, "max-w-6xl") || strings.Contains(html, "mx-auto") {
		t.Fatalf("CSV renderer keeps capped wrapper classes: %s", html)
	}
	if !strings.Contains(html, "document-table-content") {
		t.Fatalf("CSV renderer missing document table content wrapper: %s", html)
	}
	if !strings.Contains(html, `class="table-wrapper"`) {
		t.Fatalf("CSV renderer missing table wrapper: %s", html)
	}
}

func TestCSVTableDocumentShowsTruncationWithoutDocumentChrome(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	table := CSVTable{
		Headers:   []string{"name"},
		Rows:      [][]string{{"a"}},
		Truncated: true,
	}
	if err := CSVTableDocument("thoughts/data.csv", table).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "truncated") {
		t.Fatalf("missing truncation status: %s", html)
	}
	if strings.Contains(html, "thoughts/data.csv") || strings.Contains(html, "CSV table:") {
		t.Fatalf("truncation status reintroduced document chrome: %s", html)
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
