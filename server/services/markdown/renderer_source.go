package markdown

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const sourceDisplayMaxBytes int64 = 256 * 1024

type SourceRenderer struct {
	MaxBytes int64
	Renderer *Renderer
}

type SourceDocument struct {
	Path      string
	Extension string
	Language  string
	Content   string
	HTML      string
	LineCount int
}

type sourceReadResult struct {
	Content []byte
	Reason  string
}

func (r SourceRenderer) Match(req DocumentRequest) bool {
	switch req.Extension {
	case ".md", ".markdown", ".html", ".htm", ".csv", ".tsv":
		return false
	default:
		return true
	}
}

func (r SourceRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
	docPath := "thoughts/" + req.CleanPath
	maxBytes := r.MaxBytes
	if maxBytes <= 0 {
		maxBytes = sourceDisplayMaxBytes
	}
	result, err := readSafeSource(req.FullPath, maxBytes)
	if err != nil {
		return unsupportedDocumentForReason(req, "Unable to read this file."), nil
	}
	if result.Reason != "" {
		return unsupportedDocumentForReason(req, result.Reason), nil
	}

	content := string(result.Content)
	language := sourceLanguageForExtension(req.Extension)
	highlightedHTML := ""
	if r.Renderer != nil {
		highlighted, err := r.Renderer.HighlightSource(content, language)
		if err == nil {
			highlightedHTML = highlighted
		}
	}
	if highlightedHTML == "" {
		highlightedHTML = escapeSourceHTML(content)
	}

	return RenderedDocument{
		Path:          docPath,
		Title:         DocumentTitle(docPath, nil),
		Kind:          DocumentKindSource,
		ClipboardText: content,
		Component: SourceDocumentView(SourceDocument{
			Path:      docPath,
			Extension: req.Extension,
			Language:  language,
			Content:   content,
			HTML:      highlightedHTML,
			LineCount: sourceLineCount(content),
		}),
		CommentMode: CommentModeDocumentOnly,
	}, nil
}

func readSafeSource(path string, maxBytes int64) (sourceReadResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return sourceReadResult{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return sourceReadResult{}, err
	}
	if int64(len(content)) > maxBytes {
		return sourceReadResult{Reason: fmt.Sprintf("File is too large for inline source display (limit %d bytes).", maxBytes)}, nil
	}
	if !isSafeUTF8Text(content) {
		return sourceReadResult{Reason: "File is binary or not valid UTF-8, so Vamos will not render it as source."}, nil
	}
	return sourceReadResult{Content: content}, nil
}

func isSafeUTF8Text(content []byte) bool {
	if !utf8.Valid(content) {
		return false
	}
	return bytes.IndexByte(content, 0) < 0
}

func escapeSourceHTML(content string) string {
	return html.EscapeString(content)
}

func sourceLineCount(content string) int {
	if content == "" {
		return 1
	}
	return strings.Count(content, "\n") + 1
}

func sourceLanguageForExtension(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".templ":
		return "go-html-template"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".sql":
		return "sql"
	case ".css":
		return "css"
	case ".sh":
		return "bash"
	case ".txt":
		return ""
	default:
		return ""
	}
}

func renderSourceFallback(ctx context.Context, req DocumentRequest, source SourceRenderer) (RenderedDocument, error) {
	return source.Render(ctx, req)
}

func unsupportedDocumentForReason(req DocumentRequest, reason string) RenderedDocument {
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:        docPath,
		Title:       DocumentTitle(docPath, nil),
		Kind:        DocumentKindUnsupported,
		Component:   UnsupportedDocumentWithReason(req.Extension, docPath, reason),
		CommentMode: CommentModeDocumentOnly,
	}
}
