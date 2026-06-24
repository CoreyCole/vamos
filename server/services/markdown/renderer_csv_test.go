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
	if err := CSVTableDocument("thoughts/data.csv", table, "CSV").Render(t.Context(), &buf); err != nil {
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

func TestDelimitedRendererParsesTSV(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.tsv"), []byte("name\tvalue\nAda\t1\nGrace\t2"))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindCSVTable {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindCSVTable)
	}
	if page.ViewerArgs.RawMarkdown != "name\tvalue\nAda\t1\nGrace\t2" {
		t.Fatalf("RawMarkdown=%q", page.ViewerArgs.RawMarkdown)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{"Ada", "Grace", "document-table-content", `class="table-wrapper"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
}

func TestParseDelimitedTableUsesTabDelimiter(t *testing.T) {
	t.Parallel()

	format, _ := delimitedFormatForExtension(".tsv")
	table, err := parseDelimitedTable([]byte("a\tb\n1\t2"), format, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Headers) != 2 || table.Headers[0] != "a" || table.Rows[0][1] != "2" {
		t.Fatalf("table=%#v", table)
	}
}

func TestDelimitedRendererFallsBackToSourceOnMalformedSafeText(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "bad.tsv"), []byte("name\tvalue\n\"unterminated"))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "bad.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindSource {
		t.Fatalf("DocumentKind=%q, want %q", page.ViewerArgs.DocumentKind, DocumentKindSource)
	}
	if !strings.Contains(page.ViewerArgs.RawMarkdown, "unterminated") {
		t.Fatalf("RawMarkdown=%q", page.ViewerArgs.RawMarkdown)
	}
}

func TestDelimitedRendererParseFallbackKeepsUnsafeContentUnsupported(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "bad.csv"), []byte{'n', 'a', 'm', 'e', ',', 'v', 'a', 'l', 'u', 'e', '\n', '"', 'u', 'n', 't', 'e', 'r', 'm', 'i', 'n', 'a', 't', 'e', 'd', 0})
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "bad.csv")
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
	if !strings.Contains(html, "File is binary") {
		t.Fatalf("unsafe reason missing: %s", html)
	}
	if strings.Contains(html, root) {
		t.Fatalf("unsupported output exposed temp root: %s", html)
	}
}
