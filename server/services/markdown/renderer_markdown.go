package markdown

import (
	"context"
	"fmt"
	"os"

	"github.com/gomarkdown/markdown/parser"

	"github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

type MarkdownDocumentRenderer struct {
	service  *Service
	renderer *Renderer
	projects server.ProjectsConfig
}

func NewMarkdownDocumentRenderer(
	service *Service,
	renderer *Renderer,
	projects server.ProjectsConfig,
) MarkdownDocumentRenderer {
	return MarkdownDocumentRenderer{service: service, renderer: renderer, projects: projects}
}

func (r MarkdownDocumentRenderer) Match(req DocumentRequest) bool {
	return req.Extension == ".md" || req.Extension == ".markdown"
}

func (r MarkdownDocumentRenderer) Render(_ context.Context, req DocumentRequest) (RenderedDocument, error) {
	content, err := os.ReadFile(req.FullPath)
	if err != nil {
		return RenderedDocument{}, fmt.Errorf("error reading file: %w", err)
	}
	frontmatter, markdownContent, err := parseFrontmatter(content)
	if err != nil {
		return RenderedDocument{}, fmt.Errorf("error parsing frontmatter: %w", err)
	}

	parser := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	parsed := parser.Parse(markdownContent)
	toc := r.service.extractTableOfContents(parsed)
	sections := r.renderer.RenderToSections(markdownContent)
	htmlContent := r.renderer.MarkdownBytesToHTML(markdownContent)
	if frontmatter != nil {
		gh := r.renderer.ResolveGitHubRepo(githubRepoKeyForFrontmatter(frontmatter))
		if gh != nil {
			htmlContent = LinkCodePathsToGitHub(htmlContent, gh)
			for i := range sections {
				sections[i].HeadingHTML = LinkCodePathsToGitHub(sections[i].HeadingHTML, gh)
				sections[i].BodyHTML = LinkCodePathsToGitHub(sections[i].BodyHTML, gh)
				sections[i].HTMLContent = LinkCodePathsToGitHub(sections[i].HTMLContent, gh)
			}
		}
	}

	docPath := "thoughts/" + req.CleanPath
	commentArgs := commentui.CommentableMarkdownArgs{DocPath: docPath, HTML: htmlContent}
	return RenderedDocument{
		Path:          docPath,
		Title:         DocumentTitle(docPath, frontmatter),
		Kind:          DocumentKindMarkdown,
		Frontmatter:   frontmatter,
		TOC:           toc,
		Sections:      sections,
		HTMLContent:   htmlContent,
		ClipboardText: string(markdownContent),
		Component:     commentui.CommentableMarkdown(commentArgs),
		CommentMode:   CommentModeSections,
	}, nil
}
