package markdown

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
)

type CSVCell struct {
	Text string
	html string
}

type CSVTable struct {
	Headers   []CSVCell
	Rows      [][]CSVCell
	Truncated bool
}

type DelimitedFormat struct {
	Extension string
	Delimiter rune
	Label     string
}

type CSVRenderer struct {
	MaxRows        int
	Renderer       *Renderer
	SourceFallback *SourceRenderer
}

func (r CSVRenderer) Match(req DocumentRequest) bool {
	_, ok := delimitedFormatForExtension(req.Extension)
	return ok
}

func (r CSVRenderer) Render(
	ctx context.Context,
	req DocumentRequest,
) (RenderedDocument, error) {
	content, err := os.ReadFile(req.FullPath)
	if err != nil {
		return RenderedDocument{}, fmt.Errorf("read delimited table: %w", err)
	}
	maxRows := r.MaxRows
	if maxRows <= 0 {
		maxRows = 500
	}
	format, ok := delimitedFormatForExtension(req.Extension)
	if !ok {
		return RenderedDocument{}, fmt.Errorf(
			"unsupported delimited extension: %s",
			req.Extension,
		)
	}
	table, err := parseDelimitedTable(content, format, maxRows)
	if err != nil {
		if r.SourceFallback != nil {
			return renderSourceFallback(ctx, req, *r.SourceFallback)
		}
		return RenderedDocument{}, err
	}
	if err := renderCSVTableMarkdown(&table, r.Renderer); err != nil {
		return RenderedDocument{}, fmt.Errorf("render %s cells: %w", format.Label, err)
	}
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:          docPath,
		Title:         DocumentTitle(docPath, nil),
		Kind:          DocumentKindCSVTable,
		ClipboardText: string(content),
		Component:     CSVTableDocument(docPath, table, format.Label),
		CommentMode:   CommentModeDocumentOnly,
	}, nil
}

func delimitedFormatForExtension(ext string) (DelimitedFormat, bool) {
	switch ext {
	case ".csv":
		return DelimitedFormat{Extension: ".csv", Delimiter: ',', Label: "CSV"}, true
	case ".tsv":
		return DelimitedFormat{Extension: ".tsv", Delimiter: '\t', Label: "TSV"}, true
	default:
		return DelimitedFormat{}, false
	}
}

func parseCSVTable(content []byte, maxRows int) (CSVTable, error) {
	format, _ := delimitedFormatForExtension(".csv")
	return parseDelimitedTable(content, format, maxRows)
}

func parseDelimitedTable(
	content []byte,
	format DelimitedFormat,
	maxRows int,
) (CSVTable, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = format.Delimiter
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return CSVTable{}, fmt.Errorf("parse %s: %w", format.Label, err)
	}
	if len(records) == 0 {
		return CSVTable{}, nil
	}
	table := CSVTable{Headers: csvCells(records[0])}
	rows := records[1:]
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		table.Truncated = true
	}
	table.Rows = make([][]CSVCell, len(rows))
	for i, row := range rows {
		table.Rows[i] = csvCells(row)
	}
	return table, nil
}

func csvCells(values []string) []CSVCell {
	cells := make([]CSVCell, len(values))
	for i, value := range values {
		cells[i].Text = value
	}
	return cells
}

func renderCSVTableMarkdown(table *CSVTable, renderer *Renderer) error {
	if renderer == nil {
		return nil
	}
	for i := range table.Headers {
		html, err := renderer.markdownBytesToInlineHTML([]byte(table.Headers[i].Text))
		if err != nil {
			return err
		}
		table.Headers[i].html = html
	}
	for rowIndex := range table.Rows {
		for cellIndex := range table.Rows[rowIndex] {
			html, err := renderer.markdownBytesToInlineHTML(
				[]byte(table.Rows[rowIndex][cellIndex].Text),
			)
			if err != nil {
				return err
			}
			table.Rows[rowIndex][cellIndex].html = html
		}
	}
	return nil
}
