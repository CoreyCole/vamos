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

type CSVRenderer struct {
	MaxRows int
}

func (r CSVRenderer) Match(req DocumentRequest) bool {
	return req.Extension == ".csv"
}

func (r CSVRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
	content, err := os.ReadFile(req.FullPath)
	if err != nil {
		return RenderedDocument{}, fmt.Errorf("read CSV: %w", err)
	}
	maxRows := r.MaxRows
	if maxRows <= 0 {
		maxRows = 500
	}
	table, err := parseCSVTable(content, maxRows)
	if err != nil {
		return RenderedDocument{}, err
	}
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:          docPath,
		Title:         DocumentTitle(docPath, nil),
		Kind:          DocumentKindCSVTable,
		ClipboardText: string(content),
		Component:     CSVTableDocument(docPath, table),
		CommentMode:   CommentModeDocumentOnly,
	}, nil
}

func parseCSVTable(content []byte, maxRows int) (CSVTable, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return CSVTable{}, fmt.Errorf("parse CSV: %w", err)
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
