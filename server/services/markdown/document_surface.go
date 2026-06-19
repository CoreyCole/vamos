package markdown

import (
	"path/filepath"
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

// WorkbenchDocument is the route-neutral document model shared by Thoughts and
// Agent Chat artifact viewers. Route packages own how they populate actions and
// comment routes; the surface owns only document chrome and markdown rendering.
type WorkbenchDocument struct {
	Path          string
	Title         string
	Kind          DocumentKind
	RawMarkdown   string
	Frontmatter   *Frontmatter
	Sections      []Section
	TOC           []TocItem
	CurrentPath   string
	PageSessionID string
	Component     templ.Component
	CommentMode   CommentMode
	CommentUI     commentui.CommentableMarkdownArgs
	Actions       []DocumentAction
	QRSPIMetadata QRSPIMetadata
}

func BuildThoughtsDocument(pageArgs *PageArgs) WorkbenchDocument {
	if pageArgs == nil {
		return WorkbenchDocument{}
	}
	kind := pageArgs.ViewerArgs.DocumentKind
	if kind == "" {
		kind = DocumentKindMarkdown
	}
	component := pageArgs.ViewerArgs.BodyComponent
	if component == nil {
		component = commentui.CommentableMarkdown(pageArgs.CommentUI)
	}
	return WorkbenchDocument{
		Path:          pageArgs.FilePath,
		Title:         DocumentTitle(pageArgs.FilePath, pageArgs.ViewerArgs.Frontmatter),
		Kind:          kind,
		RawMarkdown:   pageArgs.ViewerArgs.RawMarkdown,
		Frontmatter:   pageArgs.ViewerArgs.Frontmatter,
		Sections:      pageArgs.ViewerArgs.Sections,
		TOC:           pageArgs.TableOfContents,
		CurrentPath:   pageArgs.FilePath,
		PageSessionID: pageArgs.PageSessionID,
		Component:     component,
		CommentMode:   pageArgs.ViewerArgs.CommentMode,
		CommentUI:     pageArgs.CommentUI,
		Actions:       nil,
		QRSPIMetadata: pageArgs.QRSPIMetadata,
	}
}

func DocumentTitle(path string, frontmatter *Frontmatter) string {
	if frontmatter != nil && strings.TrimSpace(frontmatter.Topic) != "" {
		return strings.TrimSpace(frontmatter.Topic)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return "Document"
	}
	return strings.Title(base)
}

func commentsContextHref(documentPath string) string {
	return "/" + strings.TrimPrefix(documentPath, "/") + "?context=comments"
}
