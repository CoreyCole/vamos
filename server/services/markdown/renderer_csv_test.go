package markdown

import (
	"bytes"
	"fmt"
	"html"
	"os"
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
		t.Fatalf(
			"DocumentKind=%q, want %q",
			page.ViewerArgs.DocumentKind,
			DocumentKindCSVTable,
		)
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

func TestCSVRendererRendersMarkdownLinksInCells(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "links.csv"),
		[]byte(
			"name,resource\nPlan,[Open plan](thoughts/example/plan.md)\nDocs,[Read docs](https://example.com/docs)",
		),
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "links.csv")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`href="/thoughts/example/plan.md"`,
		`>Open plan</a>`,
		`href="https://example.com/docs" target="_blank" rel="noopener noreferrer"`,
		`>Read docs</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "[Open plan]") || strings.Contains(html, "[Read docs]") {
		t.Fatalf("CSV renderer left markdown link syntax visible: %s", html)
	}
}

func TestCSVRendererDoesNotActivateUnsafeMarkdownLinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "links.csv"),
		[]byte("name,resource\nBad,[click](javascript:alert(1))"),
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "links.csv")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `href="javascript:`) || strings.Contains(html, `<a `) {
		t.Fatalf("CSV renderer activated unsafe markdown link: %s", html)
	}
	if !strings.Contains(html, "click") {
		t.Fatalf("CSV renderer dropped unsafe link label: %s", html)
	}
}

func TestParseCSVTableUnlimitedRendersAllRows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		rows    int
		maxRows int
	}{
		{name: "501 rows maxRows 0", rows: 501, maxRows: 0},
		{name: "501 rows maxRows negative", rows: 501, maxRows: -3},
		{name: "5000 rows maxRows 0", rows: 5000, maxRows: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			table, err := parseCSVTable(delimitedRows(",", tc.rows), tc.maxRows)
			if err != nil {
				t.Fatal(err)
			}
			if table.TooLarge {
				t.Fatal("TooLarge=true")
			}
			if len(table.Rows) != tc.rows {
				t.Fatalf("len(rows)=%d, want %d", len(table.Rows), tc.rows)
			}
			got := table.Rows[len(table.Rows)-1][1].Text
			want := fmt.Sprintf("row-%d", tc.rows)
			if got != want {
				t.Fatalf("last row name=%q, want %q", got, want)
			}
		})
	}
}

func TestParseCSVTableCapsRowsWhenMaxRowsSet(t *testing.T) {
	t.Parallel()

	table, err := parseCSVTable([]byte("a,b\n1,2\n3,4\n5,6"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if table.TooLarge {
		t.Fatal("TooLarge=true")
	}
	if len(table.Rows) != 2 || table.Rows[0][0].Text != "1" ||
		table.Rows[1][0].Text != "3" {
		t.Fatalf("rows=%#v", table.Rows)
	}
}

func TestCSVRendererRendersMoreThan500RowsWithoutTruncationNotice(t *testing.T) {
	t.Parallel()

	const rows = 501
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.csv"), delimitedRows(",", rows))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.csv")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	last := fmt.Sprintf("row-%d", rows)
	if !strings.Contains(html, last) {
		t.Fatalf("missing last row %q: %s", last, html)
	}
	assertNoTruncationCopy(t, html)
}

func TestTSVRendererRendersMoreThan500RowsWithoutTruncationNotice(t *testing.T) {
	t.Parallel()

	const rows = 501
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "data.tsv"), delimitedRows("\t", rows))
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	last := fmt.Sprintf("row-%d", rows)
	if !strings.Contains(html, last) {
		t.Fatalf("missing last row %q: %s", last, html)
	}
	assertNoTruncationCopy(t, html)
}

func TestCSVRendererTooLargeOmitsTableAndChrome(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ext     string
		label   string
		content []byte
	}{
		{
			name:    "csv",
			ext:     ".csv",
			label:   "CSV",
			content: []byte("name,value\nAda,1\nGrace,2\n"),
		},
		{
			name:    "tsv",
			ext:     ".tsv",
			label:   "TSV",
			content: []byte("name\tvalue\nAda\t1\nGrace\t2\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			name := "data" + tc.ext
			fullPath := filepath.Join(root, name)
			mustWriteFile(t, fullPath, tc.content)
			info, err := os.Stat(fullPath)
			if err != nil {
				t.Fatal(err)
			}
			maxBytes := info.Size() - 1
			if maxBytes <= 0 {
				t.Fatal("fixture too small to exceed MaxBytes")
			}
			doc, err := (CSVRenderer{MaxBytes: maxBytes}).Render(
				t.Context(),
				DocumentRequest{
					CleanPath: name,
					FullPath:  fullPath,
					Extension: tc.ext,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if doc.ClipboardText != "" {
				t.Fatalf("ClipboardText=%q, want empty", doc.ClipboardText)
			}
			var buf bytes.Buffer
			if err := doc.Component.Render(t.Context(), &buf); err != nil {
				t.Fatal(err)
			}
			body := buf.String()
			wantNotice := html.EscapeString(
				csvTooLargeNotice(tc.label, info.Size(), maxBytes),
			)
			if !strings.Contains(body, wantNotice) {
				t.Fatalf("missing notice %q: %s", wantNotice, body)
			}
			if strings.Contains(body, "<table") || strings.Contains(body, "<tbody") {
				t.Fatalf("too-large notice rendered a table: %s", body)
			}
			if strings.Contains(body, "Ada") || strings.Contains(body, "Grace") {
				t.Fatalf("too-large notice included table rows: %s", body)
			}
			if strings.Contains(body, "thoughts/"+name) ||
				strings.Contains(body, tc.label+" table:") {
				t.Fatalf("too-large notice reintroduced document chrome: %s", body)
			}
			if strings.Contains(body, "Download") || strings.Contains(body, "raw file") {
				t.Fatalf("too-large notice promised download/raw: %s", body)
			}
			assertNoTruncationCopy(t, body)
		})
	}
}

func TestCSVTooLargeNoticeWording(t *testing.T) {
	t.Parallel()

	got := csvTooLargeNotice("CSV", csvRenderMaxBytes+1, csvRenderMaxBytes)
	want := "This CSV is too large to display (16.0 MB > 16 MB)."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = csvTooLargeNotice("TSV", 32<<20, csvRenderMaxBytes)
	want = "This TSV is too large to display (32 MB > 16 MB)."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCSVRendererMaxBytesDefaultsTo16MiB(t *testing.T) {
	t.Parallel()

	if got := (CSVRenderer{}).renderMaxBytes(); got != csvRenderMaxBytes {
		t.Fatalf("default MaxBytes=%d, want %d", got, csvRenderMaxBytes)
	}
	if got := (CSVRenderer{MaxBytes: 12}).renderMaxBytes(); got != 12 {
		t.Fatalf("override MaxBytes=%d, want 12", got)
	}
}

func delimitedRows(sep string, rows int) []byte {
	var b strings.Builder
	b.WriteString("id" + sep + "name\n")
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&b, "%d%srow-%d\n", i, sep, i)
	}
	return []byte(b.String())
}

func assertNoTruncationCopy(t *testing.T, html string) {
	t.Helper()
	for _, banned := range []string{
		"truncated",
		"Showing first rows",
	} {
		if strings.Contains(html, banned) {
			t.Fatalf("found %q: %s", banned, html)
		}
	}
}

func TestDelimitedRendererParsesTSV(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "data.tsv"),
		[]byte("name\tvalue\nAda\t1\nGrace\t2"),
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindCSVTable {
		t.Fatalf(
			"DocumentKind=%q, want %q",
			page.ViewerArgs.DocumentKind,
			DocumentKindCSVTable,
		)
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

func TestParseDelimitedTableWidensToLongestRow(t *testing.T) {
	t.Parallel()

	table, err := parseCSVTable([]byte("a,b\n1,2,3\n4"), 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Headers) != 3 {
		t.Fatalf("headers=%#v", table.Headers)
	}
	if table.Headers[0].Text != "a" || table.Headers[1].Text != "b" ||
		table.Headers[2].Text != "" {
		t.Fatalf("headers=%#v", table.Headers)
	}
	if len(table.Rows) != 2 || len(table.Rows[0]) != 3 || table.Rows[0][2].Text != "3" {
		t.Fatalf("rows=%#v", table.Rows)
	}
	if len(table.Rows[1]) != 3 || table.Rows[1][0].Text != "4" ||
		table.Rows[1][1].Text != "" {
		t.Fatalf("short row=%#v", table.Rows[1])
	}
}

func TestDelimitedRendererKeepsExtraTSVColumns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "data.tsv"),
		[]byte("name\tvalue\nAda\t1\textra\nGrace\t2"),
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "data.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := page.ViewerArgs.BodyComponent.Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "extra") {
		t.Fatalf("extra TSV column dropped: %s", html)
	}
	if strings.Count(html, "</th>") != 3 {
		t.Fatalf("expected 3 header cells, html=%s", html)
	}
}

func TestParseDelimitedTableUsesTabDelimiter(t *testing.T) {
	t.Parallel()

	format, _ := delimitedFormatForExtension(".tsv")
	table, err := parseDelimitedTable([]byte("a\tb\n1\t2"), format, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Headers) != 2 || table.Headers[0].Text != "a" ||
		table.Rows[0][1].Text != "2" {
		t.Fatalf("table=%#v", table)
	}
}

func TestDelimitedRendererFallsBackToSourceOnMalformedSafeText(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "bad.tsv"),
		[]byte("name\tvalue\n\"unterminated"),
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "bad.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindSource {
		t.Fatalf(
			"DocumentKind=%q, want %q",
			page.ViewerArgs.DocumentKind,
			DocumentKindSource,
		)
	}
	if !strings.Contains(page.ViewerArgs.RawMarkdown, "unterminated") {
		t.Fatalf("RawMarkdown=%q", page.ViewerArgs.RawMarkdown)
	}
}

func TestDelimitedRendererParseFallbackKeepsUnsafeContentUnsupported(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(
		t,
		filepath.Join(root, "bad.csv"),
		[]byte{
			'n',
			'a',
			'm',
			'e',
			',',
			'v',
			'a',
			'l',
			'u',
			'e',
			'\n',
			'"',
			'u',
			'n',
			't',
			'e',
			'r',
			'm',
			'i',
			'n',
			'a',
			't',
			'e',
			'd',
			0,
		},
	)
	service, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.RenderThoughtsDocument(t.Context(), "bad.csv")
	if err != nil {
		t.Fatal(err)
	}
	if page.ViewerArgs.DocumentKind != DocumentKindUnsupported {
		t.Fatalf(
			"DocumentKind=%q, want %q",
			page.ViewerArgs.DocumentKind,
			DocumentKindUnsupported,
		)
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
