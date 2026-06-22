package appfiles

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a-h/templ"
)

type BrowserService struct{}

func NewService() *BrowserService { return &BrowserService{} }

func (s *BrowserService) SafeOpenPath(root, relPath string) (string, error) {
	return SafeOpenPath(root, relPath)
}

func (s *BrowserService) List(_ context.Context, cfg BrowserConfig, relPath string) (FilesViewModel, error) {
	root := filepath.Clean(cfg.Root)
	current := CleanRelPath(relPath)
	fullPath, err := SafeOpenPath(root, current)
	if err != nil {
		return FilesViewModel{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return FilesViewModel{}, fmt.Errorf("open files: %w", err)
	}
	if !info.IsDir() {
		current = path.Dir(current)
		if current == "." {
			current = ""
		}
		fullPath, err = SafeOpenPath(root, current)
		if err != nil {
			return FilesViewModel{}, err
		}
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return FilesViewModel{}, fmt.Errorf("list files: %w", err)
	}
	nodes := make([]FileNode, 0, len(entries))
	for _, entry := range entries {
		rel := path.Join(current, entry.Name())
		if IsHidden(rel, cfg.HiddenPaths) {
			continue
		}
		node := FileNode{
			Path:       rel,
			Name:       entry.Name(),
			IsDir:      entry.IsDir(),
			Renderable: !entry.IsDir() && IsRenderable(entry.Name()),
		}
		if !entry.IsDir() {
			node.DownloadURL = joinURLPath(cfg.RoutePrefix, rel)
		}
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	return FilesViewModel{
		Title:       "Files",
		CurrentPath: current,
		Nodes:       nodes,
		Preview:     filesEmptyPreview("Choose a file to preview."),
	}, nil
}

func (s *BrowserService) Render(_ context.Context, req RenderRequest) (templ.Component, error) {
	fullPath, err := SafeOpenPath(req.Root, req.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	if info.IsDir() {
		return filesEmptyPreview("Choose a file to preview."), nil
	}
	name := path.Base(CleanRelPath(req.Path))
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html", ".htm":
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read HTML: %w", err)
		}
		return htmlPreview(name, string(content)), nil
	case ".csv":
		table, err := readCSVTable(fullPath, 500)
		if err != nil {
			return nil, err
		}
		return csvPreview(name, table), nil
	case ".md", ".txt":
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		return textPreview(name, string(content)), nil
	default:
		return unsupportedPreview(name), nil
	}
}

func IsRenderable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".csv", ".md", ".txt":
		return true
	default:
		return false
	}
}

type CSVTable struct {
	Headers   []string
	Rows      [][]string
	Truncated bool
}

func readCSVTable(fullPath string, maxRows int) (CSVTable, error) {
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return CSVTable{}, fmt.Errorf("read CSV: %w", err)
	}
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

func joinURLPath(prefix, rel string) string {
	cleanPrefix := strings.TrimRight(strings.TrimSpace(prefix), "/")
	if cleanPrefix == "" {
		return ""
	}
	return cleanPrefix + "/" + path.Clean("/" + CleanRelPath(rel))[1:]
}

func escapeHTMLText(text string) string { return html.EscapeString(text) }
