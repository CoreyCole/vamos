package markdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-h/templ"
)

type DocumentKind string

const (
	DocumentKindMarkdown    DocumentKind = "markdown"
	DocumentKindHTMLApplet  DocumentKind = "html-applet"
	DocumentKindCSVTable    DocumentKind = "csv-table"
	DocumentKindSource      DocumentKind = "source"
	DocumentKindUnsupported DocumentKind = "unsupported"
)

type CommentMode string

const (
	CommentModeSections      CommentMode = "sections"
	CommentModeDocumentOnly  CommentMode = "document-only"
	CommentModeSelectionOnly CommentMode = "selection-only"
	CommentModeNone          CommentMode = "none"
)

type DocumentRenderOptions struct {
	CurrentTheme string
}

type DocumentRequest struct {
	RequestPath  string
	CleanPath    string
	FullPath     string
	Extension    string
	CurrentTheme string
}

type RenderedDocument struct {
	Path          string
	Title         string
	Kind          DocumentKind
	Frontmatter   *Frontmatter
	TOC           []TocItem
	Sections      []Section
	HTMLContent   string
	ClipboardText string
	Component     templ.Component
	CommentMode   CommentMode
}

type DocumentRenderer interface {
	Match(DocumentRequest) bool
	Render(context.Context, DocumentRequest) (RenderedDocument, error)
}

type DocumentRendererRegistry struct {
	renderers []DocumentRenderer
	fallback  DocumentRenderer
}

func NewDocumentRendererRegistry(fallback DocumentRenderer, renderers ...DocumentRenderer) *DocumentRendererRegistry {
	return &DocumentRendererRegistry{renderers: renderers, fallback: fallback}
}

func (r *DocumentRendererRegistry) Select(req DocumentRequest) DocumentRenderer {
	if r != nil {
		for _, renderer := range r.renderers {
			if renderer.Match(req) {
				return renderer
			}
		}
		if r.fallback != nil {
			return r.fallback
		}
	}
	return UnsupportedRenderer{}
}

type UnsupportedRenderer struct{}

func (r UnsupportedRenderer) Match(DocumentRequest) bool { return false }

func (r UnsupportedRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
	docPath := "thoughts/" + req.CleanPath
	return RenderedDocument{
		Path:        docPath,
		Title:       DocumentTitle(docPath, nil),
		Kind:        DocumentKindUnsupported,
		Component:   UnsupportedDocument(req.Extension, docPath),
		CommentMode: CommentModeDocumentOnly,
	}, nil
}

func (s *Service) RenderThoughtsDocument(ctx context.Context, requestPath string) (*PageArgs, error) {
	return s.RenderThoughtsDocumentWithOptions(ctx, requestPath, DocumentRenderOptions{})
}

func (s *Service) RenderThoughtsDocumentWithOptions(ctx context.Context, requestPath string, opts DocumentRenderOptions) (*PageArgs, error) {
	req, err := s.resolveThoughtsDocumentRequest(requestPath)
	if err != nil {
		return nil, err
	}
	req.CurrentTheme = normalizeHTMLAppletTheme(opts.CurrentTheme)
	doc, err := s.documentRenderers.Select(req).Render(ctx, req)
	if err != nil {
		return nil, err
	}
	return pageArgsFromRenderedDocument(doc), nil
}

func (s *Service) resolveThoughtsDocumentRequest(requestPath string) (DocumentRequest, error) {
	cleanPath, err := CanonicalThoughtsDocPath(requestPath)
	if err != nil {
		return DocumentRequest{}, err
	}
	fullPath := filepath.Join(s.basePath, filepath.FromSlash(cleanPath))
	if !pathWithinRoot(filepath.Clean(fullPath), filepath.Clean(s.basePath)) {
		return DocumentRequest{}, errors.New("access denied: path escapes base directory")
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		mdPath := fullPath + ".md"
		if mdInfo, mdErr := os.Stat(mdPath); mdErr == nil && !mdInfo.IsDir() {
			fullPath = mdPath
			cleanPath += ".md"
			info = mdInfo
		} else {
			return DocumentRequest{}, fmt.Errorf("file not found: %s", requestPath)
		}
	}
	if info.IsDir() {
		return DocumentRequest{}, fmt.Errorf("path is a directory: %s", requestPath)
	}
	return DocumentRequest{
		RequestPath: requestPath,
		CleanPath:   cleanPath,
		FullPath:    fullPath,
		Extension:   strings.ToLower(filepath.Ext(fullPath)),
	}, nil
}

func pageArgsFromRenderedDocument(doc RenderedDocument) *PageArgs {
	return &PageArgs{
		ViewerArgs: ViewerArgs{
			HTMLContent:   doc.HTMLContent,
			Frontmatter:   doc.Frontmatter,
			Sections:      doc.Sections,
			RawMarkdown:   doc.ClipboardText,
			DocumentKind:  doc.Kind,
			CommentMode:   doc.CommentMode,
			BodyComponent: doc.Component,
		},
		TableOfContents: doc.TOC,
		FilePath:        doc.Path,
	}
}
