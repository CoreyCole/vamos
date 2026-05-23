package markdown

import (
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

// WorkbenchDocument is the route-neutral document model shared by Thoughts and
// Agent Chat artifact viewers. Route packages own how they populate actions and
// comment routes; the surface owns only document chrome and markdown rendering.
type WorkbenchDocument struct {
	Path          string
	Title         string
	RawMarkdown   string
	Frontmatter   *Frontmatter
	Sections      []Section
	TOC           []TocItem
	CurrentPath   string
	PageSessionID string
	CommentUI     commentui.CommentableMarkdownArgs
	Actions       []DocumentAction
	QRSPIMetadata QRSPIMetadata
}

func BuildThoughtsDocument(pageArgs *PageArgs) WorkbenchDocument {
	if pageArgs == nil {
		return WorkbenchDocument{}
	}
	return WorkbenchDocument{
		Path:          pageArgs.FilePath,
		Title:         DocumentTitle(pageArgs.FilePath, pageArgs.ViewerArgs.Frontmatter),
		RawMarkdown:   pageArgs.ViewerArgs.RawMarkdown,
		Frontmatter:   pageArgs.ViewerArgs.Frontmatter,
		Sections:      pageArgs.ViewerArgs.Sections,
		TOC:           pageArgs.TableOfContents,
		CurrentPath:   pageArgs.FilePath,
		PageSessionID: pageArgs.PageSessionID,
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
