package markdown

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
)

type CSVTable struct {
	Headers   []string
	Rows      [][]string
	Truncated bool
}

type DelimitedFormat struct {
	Extension string
	Delimiter rune
	Label     string
}

type CSVRenderer struct {
	MaxRows int
}

func (r CSVRenderer) Match(req DocumentRequest) bool {
	_, ok := delimitedFormatForExtension(req.Extension)
	return ok
}

func (r CSVRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
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
		return RenderedDocument{}, fmt.Errorf("unsupported delimited extension: %s", req.Extension)
	}
	table, err := parseDelimitedTable(content, format, maxRows)
	if err != nil {
		return RenderedDocument{}, err
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

func parseDelimitedTable(content []byte, format DelimitedFormat, maxRows int) (CSVTable, error) {
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
	table := CSVTable{Headers: records[0]}
	rows := records[1:]
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		table.Truncated = true
	}
	table.Rows = rows
	return table, nil
}
