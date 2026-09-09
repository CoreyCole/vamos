package markdown

import (
	"strings"
	"testing"
)

func TestNormalizeMarkdownTablesPadsAndTrimsColumns(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"# Title",
		"",
		"| Age | Owner | Follow-up | Source review |",
		"| --- | --- | --- | --- | --- |",
		"| 57 days | Nick | fix | AI-113 |",
		"",
		"done",
	}, "\n")
	got := string(normalizeMarkdownTables([]byte(input)))
	wantHeader := "| Age | Owner | Follow-up | Source review |"
	wantDelim := "| --- | --- | --- | --- |"
	if !strings.Contains(got, wantHeader) {
		t.Fatalf("normalized header missing: %s", got)
	}
	if !strings.Contains(got, wantDelim) {
		t.Fatalf("normalized delimiter missing: %s", got)
	}
	if strings.Contains(got, "| --- | --- | --- | --- | --- |") {
		t.Fatalf("extra delimiter column was not trimmed: %s", got)
	}
}

func TestNormalizeMarkdownTablesPadsShortDelimiter(t *testing.T) {
	t.Parallel()

	input := "| A | B | C |\n| --- | --- |\n| 1 | 2 | 3 |\n"
	got := string(normalizeMarkdownTables([]byte(input)))
	if !strings.Contains(got, "| --- | --- | --- |") {
		t.Fatalf("short delimiter was not padded: %s", got)
	}
}

func TestNormalizeMarkdownTablesLeavesFencedTablesAlone(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"```md",
		"| Age | Owner |",
		"| --- | --- | --- |",
		"| 1 | 2 |",
		"```",
	}, "\n")
	got := string(normalizeMarkdownTables([]byte(input)))
	if got != input {
		t.Fatalf("fenced table rewritten: got %q want %q", got, input)
	}
}

func TestNormalizeMarkdownTablesPreservesPipesInCodeSpans(t *testing.T) {
	t.Parallel()

	input := "| Name | Pattern |\n| --- | --- |\n| pipe | `|` |\n"
	got := string(normalizeMarkdownTables([]byte(input)))
	if !strings.Contains(got, "| pipe | `|` |") {
		t.Fatalf("code-span pipe was split: %s", got)
	}
}

func TestSplitPipeRowIgnoresBorderPipes(t *testing.T) {
	t.Parallel()

	cells, ok := splitPipeRow("| Age | Owner | Follow-up | Source review |")
	if !ok {
		t.Fatal("splitPipeRow ok=false")
	}
	if len(cells) != 4 || cells[0] != "Age" || cells[3] != "Source review" {
		t.Fatalf("cells=%q", cells)
	}
}
