package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/services/commentui"
)

func TestBuildThoughtsDocumentCarriesDocumentModel(t *testing.T) {
	args := &PageArgs{
		FilePath:        "thoughts/example/design.md",
		PageSessionID:   "page-1",
		TableOfContents: []TocItem{{ID: "overview", Text: "Overview", Level: 1}},
		ViewerArgs: ViewerArgs{
			Frontmatter: &Frontmatter{Topic: "Shared Workbench"},
			RawMarkdown: "# Overview",
			Sections:    []Section{{ID: "section-1", Title: "Overview"}},
		},
		CommentUI: commentui.CommentableMarkdownArgs{
			Surface:  commentui.CommentSurfaceThoughts,
			IDPrefix: "doc",
			DocPath:  "thoughts/example/design.md",
			Sections: []commentui.CommentSectionView{
				{ID: "section-1", Title: "Overview"},
			},
		},
	}

	doc := BuildThoughtsDocument(args)
	if doc.Path != args.FilePath || doc.CurrentPath != args.FilePath ||
		doc.Title != "Shared Workbench" {
		t.Fatalf("unexpected document identity: %+v", doc)
	}
	if len(doc.Sections) != 1 || len(doc.TOC) != 1 {
		t.Fatalf("document did not carry sections/toc: %+v", doc)
	}
	if doc.Kind != DocumentKindMarkdown {
		t.Fatalf("doc.Kind=%q", doc.Kind)
	}
	if doc.Component == nil {
		t.Fatal("doc.Component nil")
	}
	if len(doc.Actions) != 0 {
		t.Fatalf("Thoughts document actions = %#v, want none", doc.Actions)
	}
	if doc.CommentUI.DocPath != args.FilePath || doc.PageSessionID != "page-1" {
		t.Fatalf("document did not carry comment UI/session: %+v", doc)
	}
}

func TestDocumentSurfaceRendersRendererComponent(t *testing.T) {
	doc := WorkbenchDocument{
		Path:          "thoughts/example.csv",
		PageSessionID: "page-1",
		Component:     templ.Raw(`<div id="csv-doc">csv</div>`),
	}

	var buf bytes.Buffer
	if err := DocumentSurface(doc, nil).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `id="csv-doc"`) {
		t.Fatalf("missing renderer component: %s", buf.String())
	}
}

func TestDocumentSurfaceRendersHTMLAppletEdgeToEdge(t *testing.T) {
	doc := WorkbenchDocument{
		Path:          "thoughts/example.html",
		Kind:          DocumentKindHTMLApplet,
		PageSessionID: "page-1",
		Component:     templ.Raw(`<iframe data-vamos-html-applet></iframe>`),
	}

	var buf bytes.Buffer
	if err := DocumentSurface(doc, nil).Render(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `id="thoughts-markdown-scroll-region" class="min-h-0 flex-1 overflow-hidden"`) {
		t.Fatalf("HTML document surface is not edge-to-edge: %s", html)
	}
	if strings.Contains(html, `p-4 md:p-10`) || strings.Contains(html, `overflow-y-auto`) {
		t.Fatalf("HTML document surface kept padded scroll wrapper: %s", html)
	}
}

func TestDocumentSurfaceRendersSharedChromeWithoutDuplicateActions(t *testing.T) {
	doc := WorkbenchDocument{
		Path:          "thoughts/example/design.md",
		Title:         "Design",
		PageSessionID: "page-1",
		CommentUI: commentui.CommentableMarkdownArgs{
			Surface:  commentui.CommentSurfaceThoughts,
			IDPrefix: "doc",
			DocPath:  "thoughts/example/design.md",
			Sections: []commentui.CommentSectionView{{
				ID:          "section-1",
				Title:       "Overview",
				HeadingHTML: `<h1 id="overview">Overview</h1>`,
				BodyHTML:    `<p>Body</p>`,
			}},
		},
	}

	var buf bytes.Buffer
	if err := DocumentSurface(doc, nil).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`id="document-surface"`,
		`id="thoughts-markdown-scroll-region"`,
		`data-comment-target="true"`,
		`id="thoughts-open-chat-result"`,
		`class="hidden"`,
		`page-1`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("DocumentSurface render missing %q in %s", want, html)
		}
	}
	for _, unwanted := range []string{
		`id="document-header-actions"`,
		`action="/thoughts/actions/open-chat"`,
		`data-on:submit="@post(&#39;/thoughts/actions/open-chat&#39;, {contentType: &#39;form&#39;})"`,
		`aria-label="Open Chat"`,
		`title="Open Chat"`,
		`action="/thoughts/actions/open-comments"`,
		`data-on:submit="@post(&#39;/thoughts/actions/open-comments&#39;, {contentType: &#39;form&#39;})"`,
		`aria-label="Open comments"`,
		`title="Open comments"`,
		`name="attach"`,
		`context=comments`,
		`href="/agent-chat`,
		`action="/agent-chat`,
		`data-on:submit="@post(&#39;/agent-chat`,
		`Open in Agent Chat`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"DocumentSurface render kept duplicate action %q in %s",
				unwanted,
				html,
			)
		}
	}
}
