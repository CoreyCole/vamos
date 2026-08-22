package markdown

import (
	"bytes"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	"github.com/CoreyCole/vamos/server"
)

const (
	DefaultCodeStyle = "github-dark"
)

type Renderer struct {
	highlightStyle      *chroma.Style
	htmlFormatter       *html.Formatter
	sourceHTMLFormatter *html.Formatter
	projects            server.ProjectsConfig
}

func NewRenderer(highlightStyleString string) (*Renderer, error) {
	return NewRendererWithProjects(highlightStyleString, server.ProjectsConfig{})
}

func NewRendererWithProjects(
	highlightStyleString string,
	projects server.ProjectsConfig,
) (*Renderer, error) {
	var styleName string
	if highlightStyleString == "" {
		styleName = DefaultCodeStyle
	} else {
		styleName = highlightStyleString
	}
	highlightStyle := styles.Get(DefaultCodeStyle)
	if style, ok := styles.Registry[styleName]; ok {
		highlightStyle = style
	} else {
		fmt.Printf("invalid style: %s\n", styleName)
	}
	// Use WithClasses to avoid injecting Monokai theme CSS (which sets body background to
	// #272822)
	htmlFormatter := html.New(html.WithClasses(true), html.TabWidth(2))
	if htmlFormatter == nil {
		return nil, errors.New("couldn't create html formatter")
	}
	sourceHTMLFormatter := html.New(
		html.WithClasses(true),
		html.TabWidth(2),
		html.WithLineNumbers(true),
		html.WithLinkableLineNumbers(true, "L"),
	)
	if sourceHTMLFormatter == nil {
		return nil, errors.New("couldn't create source html formatter")
	}
	return &Renderer{
		highlightStyle:      highlightStyle,
		htmlFormatter:       htmlFormatter,
		sourceHTMLFormatter: sourceHTMLFormatter,
		projects:            projects,
	}, nil
}

func (m Renderer) MarkdownBytesToHTML(md []byte) (string, error) {
	md = renderableMarkdown(md)

	// Use parser with NoEmptyLineBeforeBlock so lists work without a preceding blank line
	p := parser.NewWithExtensions(
		parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock,
	)
	state := &renderState{}
	htmlBytes := markdown.ToHTML(
		md,
		p,
		mdhtmlRenderer(m.highlightStyle, m.htmlFormatter, state),
	)
	if err := state.Err(); err != nil {
		return "", err
	}
	return string(htmlBytes), nil
}

func (m Renderer) markdownBytesToInlineHTML(md []byte) (string, error) {
	p := parser.NewWithExtensions(parser.CommonExtensions)
	paragraph := &ast.Paragraph{}
	p.Inline(paragraph, md)

	state := &renderState{}
	renderer := mdhtmlRendererWithOptions(
		m.highlightStyle,
		m.htmlFormatter,
		state,
		markdownHTMLOptions{
			Inline:                   true,
			EscapeHTML:               true,
			SafeLinks:                true,
			DisableThoughtsAutoLinks: true,
		},
	)
	var rendered bytes.Buffer
	ast.WalkFunc(paragraph, func(node ast.Node, entering bool) ast.WalkStatus {
		return renderer.RenderNode(&rendered, node, entering)
	})
	if err := state.Err(); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (m Renderer) HighlightSource(source, lang string) (string, error) {
	var buf bytes.Buffer
	formatter := m.sourceHTMLFormatter
	if formatter == nil {
		formatter = m.htmlFormatter
	}
	if err := htmlHighlight(
		&buf,
		source,
		lang,
		"",
		m.highlightStyle,
		formatter,
	); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderableMarkdown(md []byte) []byte {
	frontmatter, body, ok := splitYAMLFrontmatter(md)
	if !ok {
		return md
	}

	frontmatter = bytes.Trim(frontmatter, "\r\n")
	body = bytes.TrimLeft(body, "\r\n")

	var rendered bytes.Buffer
	rendered.WriteString("```yaml\n")
	rendered.Write(frontmatter)
	if len(frontmatter) == 0 || frontmatter[len(frontmatter)-1] != '\n' {
		rendered.WriteByte('\n')
	}
	rendered.WriteString("```")
	if len(body) > 0 {
		rendered.WriteString("\n\n")
		rendered.Write(body)
	} else {
		rendered.WriteByte('\n')
	}

	return rendered.Bytes()
}

func splitYAMLFrontmatter(md []byte) ([]byte, []byte, bool) {
	switch {
	case bytes.HasPrefix(md, []byte("---\r\n")):
		const prefix = "---\r\n"
		const marker = "\r\n---\r\n"
		endIdx := bytes.Index(md[len(prefix):], []byte(marker))
		if endIdx < 0 {
			return nil, nil, false
		}
		start := len(prefix)
		end := start + endIdx
		return md[start:end], md[end+len(marker):], true
	case bytes.HasPrefix(md, []byte("---\n")):
		const prefix = "---\n"
		const marker = "\n---\n"
		endIdx := bytes.Index(md[len(prefix):], []byte(marker))
		if endIdx < 0 {
			return nil, nil, false
		}
		start := len(prefix)
		end := start + endIdx
		return md[start:end], md[end+len(marker):], true
	default:
		return nil, nil, false
	}
}

// based on https://github.com/alecthomas/chroma/blob/master/quick/quick.go
//
//nolint:misspell // Chroma API uses British spelling: lexers.Analyse.
func htmlHighlight(
	w io.Writer,
	source, lang,
	defaultLang string,
	highlightStyle *chroma.Style,
	htmlFormatter *html.Formatter,
) error {
	if lang == "" {
		lang = defaultLang
	}
	l := lexers.Get(lang)
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)

	it, err := l.Tokenise(nil, source)
	if err != nil {
		return err
	}
	return htmlFormatter.Format(w, highlightStyle, it)
}

func renderCode(
	w io.Writer,
	codeBlock *ast.CodeBlock,
	highlightStyle *chroma.Style,
	htmlFormatter *html.Formatter,
) error {
	defaultLang := ""
	lang := string(codeBlock.Info)
	return htmlHighlight(
		w,
		string(codeBlock.Literal),
		lang,
		defaultLang,
		highlightStyle,
		htmlFormatter,
	)
}

type renderState struct {
	err error
}

func (s *renderState) Err() error { return s.err }

func (s *renderState) record(err error) ast.WalkStatus {
	if s.err != nil {
		return ast.Terminate
	}
	if err != nil {
		s.err = err
		return ast.Terminate
	}
	return ast.GoToNext
}

func (s *renderState) recordWrite(n, want int, err error) ast.WalkStatus {
	if err != nil {
		return s.record(err)
	}
	if n != want {
		return s.record(io.ErrShortWrite)
	}
	return s.record(nil)
}

func (s *renderState) write(w io.Writer, p []byte) ast.WalkStatus {
	n, err := w.Write(p)
	return s.recordWrite(n, len(p), err)
}

func (s *renderState) writeString(w io.Writer, value string) ast.WalkStatus {
	return s.write(w, []byte(value))
}

func (s *renderState) printf(w io.Writer, format string, args ...any) ast.WalkStatus {
	return s.writeString(w, fmt.Sprintf(format, args...))
}

type stateWriter struct {
	state  *renderState
	writer io.Writer
}

func (w stateWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.state.recordWrite(n, len(p), err)
	if err != nil {
		return n, err
	}
	if n != len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

type markdownHTMLOptions struct {
	Inline                   bool
	EscapeHTML               bool
	SafeLinks                bool
	DisableThoughtsAutoLinks bool
}

func mdhtmlRenderer(
	highlightStyle *chroma.Style,
	htmlFormatter *html.Formatter,
	state *renderState,
) *mdhtml.Renderer {
	return mdhtmlRendererWithOptions(
		highlightStyle,
		htmlFormatter,
		state,
		markdownHTMLOptions{},
	)
}

func mdhtmlRendererWithOptions(
	highlightStyle *chroma.Style,
	htmlFormatter *html.Formatter,
	state *renderState,
	options markdownHTMLOptions,
) *mdhtml.Renderer {
	write := func(w io.Writer, value string) ast.WalkStatus { return state.writeString(w, value) }
	printf := func(w io.Writer, format string, args ...any) ast.WalkStatus {
		return state.printf(w, format, args...)
	}
	return mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags:           mdhtml.CommonFlags,
		HeadingIDPrefix: "",
		HeadingIDSuffix: "",
		RenderNodeHook: func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
			if state.Err() != nil {
				return ast.Terminate, true
			}
			if options.Inline {
				if _, ok := node.(*ast.Paragraph); ok {
					return ast.GoToNext, true
				}
			}
			if options.EscapeHTML && entering {
				switch htmlNode := node.(type) {
				case *ast.HTMLSpan:
					return write(w, stdhtml.EscapeString(string(htmlNode.Literal))), true
				case *ast.HTMLBlock:
					return write(w, stdhtml.EscapeString(string(htmlNode.Literal))), true
				}
			}
			if code, ok := node.(*ast.CodeBlock); ok {
				if write(w, `<div class="markdown-code-block">`) == ast.Terminate {
					return ast.Terminate, true
				}
				if err := renderCode(
					stateWriter{state: state, writer: w},
					code,
					highlightStyle,
					htmlFormatter,
				); err != nil {
					return state.record(err), true
				}
				return write(w, "</div>"), true
			}
			if code, ok := node.(*ast.Code); ok {
				if path, ok := normalizeThoughtsPath(string(code.Literal)); ok {
					if printf(
						w,
						`<a class="%s" href="%s"><code>`,
						thoughtsLinkClass,
						path,
					) == ast.Terminate {
						return ast.Terminate, true
					}
					if write(
						w,
						stdhtml.EscapeString(string(code.Literal)),
					) == ast.Terminate {
						return ast.Terminate, true
					}
					return write(w, "</code></a>"), true
				}
				return ast.GoToNext, false
			}
			if link, ok := node.(*ast.Link); ok {
				dest := string(link.Destination)
				if options.SafeLinks && !isSafeMarkdownLinkDestination(dest) {
					return ast.GoToNext, true
				}
				if entering {
					isInternal := strings.HasPrefix(dest, "/thoughts") ||
						strings.HasPrefix(dest, "thoughts/")
					if write(
						w,
						`<a class="font-medium text-primary hover:text-primary/80 transition-colors underline decoration-primary/30 hover:decoration-primary/80"`,
					) == ast.Terminate {
						return ast.Terminate, true
					}
					if isInternal {
						if !strings.HasPrefix(dest, "/") {
							dest = "/" + dest
						}
						if printf(
							w,
							` href="%s"`,
							stdhtml.EscapeString(dest),
						) == ast.Terminate {
							return ast.Terminate, true
						}
					} else if len(dest) > 0 && printf(w, ` href="%s" target="_blank" rel="noopener noreferrer"`, stdhtml.EscapeString(dest)) == ast.Terminate {
						return ast.Terminate, true
					}
					if len(link.Title) > 0 &&
						printf(
							w,
							` title="%s"`,
							stdhtml.EscapeString(string(link.Title)),
						) == ast.Terminate {
						return ast.Terminate, true
					}
					return write(w, ">"), true
				}
				return write(w, "</a>"), true
			}
			if heading, ok := node.(*ast.Heading); ok && entering {
				attr := heading.Attribute
				if attr == nil {
					attr = &ast.Attribute{}
				}
				attr.Classes = append(
					attr.Classes,
					[]byte(fmt.Sprintf("myh myh-%d", min(heading.Level, 6))),
				)
				heading.Attribute = attr
			}
			if list, ok := node.(*ast.List); ok {
				tag, class := "ul", "list-disc"
				if list.ListFlags&ast.ListTypeOrdered != 0 {
					tag, class = "ol", "list-decimal"
				}
				if entering {
					return printf(
						w,
						`<%s class="%s pl-6 my-4 space-y-2">`,
						tag,
						class,
					), true
				}
				return printf(w, "</%s>", tag), true
			}
			if item, ok := node.(*ast.ListItem); ok {
				if entering {
					checked, checkbox := detectCheckbox(item)
					if !checkbox {
						return write(w, `<li class="leading-relaxed">`), true
					}
					if write(
						w,
						`<li class="leading-relaxed flex items-start gap-2 list-none -ml-6">`,
					) == ast.Terminate {
						return ast.Terminate, true
					}
					if checked {
						if write(
							w,
							`<input type="checkbox" checked disabled class="mt-1.5 h-4 w-4 shrink-0 rounded border border-primary bg-primary text-primary-foreground accent-primary" />`,
						) == ast.Terminate {
							return ast.Terminate, true
						}
					} else if write(w, `<input type="checkbox" disabled class="mt-1.5 h-4 w-4 shrink-0 rounded border border-input bg-background accent-primary" />`) == ast.Terminate {
						return ast.Terminate, true
					}
					return write(w, `<span>`), true
				}
				if _, checkbox := detectCheckbox(
					item,
				); checkbox &&
					write(w, "</span>") == ast.Terminate {
					return ast.Terminate, true
				}
				return write(w, "</li>"), true
			}
			if text, ok := node.(*ast.Text); ok && entering {
				content := string(text.Literal)
				if isInsideCheckboxListItem(text) {
					content = strings.TrimPrefix(
						strings.TrimPrefix(strings.TrimPrefix(content, "[ ] "), "[x] "),
						"[X] ",
					)
					return write(w, content), true
				}
				if !options.DisableThoughtsAutoLinks &&
					strings.Contains(content, "thoughts/") && !isInsideCode(text) {
					return write(w, autoLinkThoughtsPaths(content)), true
				}
			}
			if paragraph, ok := node.(*ast.Paragraph); ok {
				attr := paragraph.Attribute
				if attr == nil {
					attr = &ast.Attribute{}
				}
				attr.Classes = append(attr.Classes, []byte("text-lg"))
				paragraph.Attribute = attr
			}
			if _, ok := node.(*ast.Table); ok {
				if entering {
					return write(w, `<div class="table-wrapper"><table>`), true
				}
				return write(w, "</table></div>"), true
			}
			if _, ok := node.(*ast.TableHeader); ok {
				if entering {
					return write(w, `<thead>`), true
				}
				return write(w, "</thead>"), true
			}
			if _, ok := node.(*ast.TableBody); ok {
				if entering {
					return write(w, "<tbody>"), true
				}
				return write(w, "</tbody>"), true
			}
			if _, ok := node.(*ast.TableRow); ok {
				if entering {
					return write(w, `<tr>`), true
				}
				return write(w, "</tr>"), true
			}
			if cell, ok := node.(*ast.TableCell); ok {
				tag, align := "td", ""
				if cell.IsHeader {
					tag = "th"
				}
				switch cell.Align {
				case ast.TableAlignmentLeft:
					align = " text-left"
				case ast.TableAlignmentRight:
					align = " text-right"
				case ast.TableAlignmentCenter:
					align = " text-center"
				}
				if cell.IsHeader && align == "" {
					align = " text-left"
				}
				if entering {
					if cell.IsHeader {
						return printf(
							w,
							`<%s class="px-4 py-3 text-sm font-semibold text-foreground%s">`,
							tag,
							align,
						), true
					}
					return printf(
						w,
						`<%s class="px-4 py-3 text-sm text-foreground%s">`,
						tag,
						align,
					), true
				}
				return printf(w, "</%s>", tag), true
			}
			return ast.GoToNext, false
		},
	})
}

func isSafeMarkdownLinkDestination(destination string) bool {
	if destination == "" || strings.ContainsAny(destination, "\x00\r\n") {
		return false
	}
	parsed, err := url.Parse(destination)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "", "http", "https", "mailto", "ftp":
		return true
	default:
		return false
	}
}

// detectCheckbox checks if a list item contains a task list checkbox pattern
// Returns (isChecked, hasCheckbox)
func detectCheckbox(listItem *ast.ListItem) (bool, bool) {
	// Walk through children to find the first text content
	for _, child := range listItem.GetChildren() {
		// List items typically contain a Paragraph which contains Text
		if para, ok := child.(*ast.Paragraph); ok {
			for _, paraChild := range para.GetChildren() {
				if text, ok := paraChild.(*ast.Text); ok {
					content := string(text.Literal)
					// Check for checkbox patterns at the start
					if strings.HasPrefix(content, "[ ] ") {
						return false, true
					}
					if strings.HasPrefix(content, "[x] ") ||
						strings.HasPrefix(content, "[X] ") {
						return true, true
					}
					// No checkbox pattern found
					return false, false
				}
			}
		}
		// Direct text child (less common)
		if text, ok := child.(*ast.Text); ok {
			content := string(text.Literal)
			if strings.HasPrefix(content, "[ ] ") {
				return false, true
			}
			if strings.HasPrefix(content, "[x] ") || strings.HasPrefix(content, "[X] ") {
				return true, true
			}
			return false, false
		}
	}
	return false, false
}

// isInsideCheckboxListItem checks if a text node is the first text inside a checkbox list
// item
func isInsideCheckboxListItem(text *ast.Text) bool {
	content := string(text.Literal)
	// Only process text that starts with a checkbox pattern
	if !strings.HasPrefix(content, "[ ] ") &&
		!strings.HasPrefix(content, "[x] ") &&
		!strings.HasPrefix(content, "[X] ") {
		return false
	}

	// Walk up to find if we're inside a ListItem
	parent := text.GetParent()
	for parent != nil {
		if listItem, ok := parent.(*ast.ListItem); ok {
			// Verify this list item has the checkbox pattern
			_, hasCheckbox := detectCheckbox(listItem)
			return hasCheckbox
		}
		parent = parent.GetParent()
	}
	return false
}

// isInsideCode checks if a text node is inside an inline code span
func isInsideCode(text *ast.Text) bool {
	parent := text.GetParent()
	for parent != nil {
		if _, ok := parent.(*ast.Code); ok {
			return true
		}
		parent = parent.GetParent()
	}
	return false
}

// thoughtsPathRe matches thoughts/ paths embedded in normal text. Starts with optional /
// then thoughts/, continues with path chars, and ends at .md or a word/slash boundary.
var thoughtsPathRe = regexp.MustCompile(
	`(?:^|([^/\w]))(/?\bthoughts/[\w./-]+\.md|/?\bthoughts/[\w./-]+(?:/|[\w-]))`,
)

// thoughtsPathOnlyRe matches an entire inline-code literal that is just a thoughts path.
var thoughtsPathOnlyRe = regexp.MustCompile(`^/?thoughts/[\w./-]+(?:\.md|/|[\w-])$`)

const thoughtsLinkClass = `font-medium text-primary hover:text-primary/80 transition-colors underline decoration-primary/30 hover:decoration-primary/80`

func normalizeThoughtsPath(content string) (string, bool) {
	path := strings.TrimSpace(content)
	if !strings.Contains(path, "thoughts/") || !thoughtsPathOnlyRe.MatchString(path) {
		return "", false
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, true
}

func autoLinkThoughtsPaths(content string) string {
	return thoughtsPathRe.ReplaceAllStringFunc(content, func(match string) string {
		// Find where the actual path starts
		path := match
		prefix := ""
		for i, r := range match {
			if r == 't' || r == '/' {
				prefix = match[:i]
				path = match[i:]
				break
			}
		}
		// Ensure leading slash
		href := path
		if !strings.HasPrefix(href, "/") {
			href = "/" + href
		}
		return fmt.Sprintf(
			`%s<a class="%s" href="%s">%s</a>`,
			prefix, thoughtsLinkClass, href, path,
		)
	})
}

// RenderToSections parses markdown and returns sections with metadata
func (m Renderer) RenderToSections(md []byte) ([]Section, error) {
	md = renderableMarkdown(md)

	// Parse markdown to AST - NoEmptyLineBeforeBlock allows lists without preceding blank
	// line
	p := parser.NewWithExtensions(
		parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock,
	)
	doc := p.Parse(md)

	state := &renderState{}
	renderer := mdhtmlRenderer(m.highlightStyle, m.htmlFormatter, state)
	sections := []Section{}
	sectionID := 0
	currentSectionNodes := []ast.Node{}
	currentSectionTitle := ""
	lineStart := 1

	// Walk through top-level nodes
	for _, node := range doc.GetChildren() {
		// Check if this is a heading (starts a new section)
		if heading, ok := node.(*ast.Heading); ok {
			// Save previous section if it has content
			if len(currentSectionNodes) > 0 {
				section, err := m.renderSection(
					renderer,
					state,
					sectionID,
					currentSectionNodes,
					lineStart,
					currentSectionTitle,
				)
				if err != nil {
					return nil, err
				}
				sections = append(sections, section)
				sectionID++
			}

			// Start new section with this heading
			currentSectionNodes = []ast.Node{node}
			currentSectionTitle = extractHeadingText(heading)
			lineStart = getNodeLine(node)
		} else {
			// Add node to current section
			currentSectionNodes = append(currentSectionNodes, node)
		}
	}

	// Add final section
	if len(currentSectionNodes) > 0 {
		section, err := m.renderSection(
			renderer,
			state,
			sectionID,
			currentSectionNodes,
			lineStart,
			currentSectionTitle,
		)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	return sections, nil
}

// renderSection renders a group of AST nodes into a section
func (m Renderer) renderSection(
	renderer *mdhtml.Renderer,
	state *renderState,
	id int,
	nodes []ast.Node,
	lineStart int,
	title string,
) (Section, error) {
	sectionID := fmt.Sprintf("section-%d", id)

	// Render heading and body separately
	var headingBuf bytes.Buffer
	var bodyBuf bytes.Buffer
	var fullBuf bytes.Buffer

	for i, node := range nodes {
		// First node is the heading (if section has a title)
		if i == 0 && title != "" {
			ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
				return renderer.RenderNode(&headingBuf, n, entering)
			})
			ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
				return renderer.RenderNode(&fullBuf, n, entering)
			})
		} else {
			ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
				return renderer.RenderNode(&bodyBuf, n, entering)
			})
			ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
				return renderer.RenderNode(&fullBuf, n, entering)
			})
		}
		if err := state.Err(); err != nil {
			return Section{}, err
		}
	}

	// Calculate line end (approximate based on nodes)
	lineEnd := lineStart + 10 // Default span
	if len(nodes) > 0 {
		lastNode := nodes[len(nodes)-1]
		lineEnd = getNodeLine(lastNode) + 5 // Approximate end
	}

	// Generate title if not provided
	if title == "" {
		title = fmt.Sprintf("Section %d", id+1)
	}

	return Section{
		ID:          sectionID,
		HeadingHTML: headingBuf.String(),
		BodyHTML:    bodyBuf.String(),
		HTMLContent: fullBuf.String(),
		LineStart:   lineStart,
		LineEnd:     lineEnd,
		Title:       title,
	}, nil
}

// extractHeadingText extracts plain text from a heading node
func extractHeadingText(heading *ast.Heading) string {
	var text strings.Builder
	ast.WalkFunc(heading, func(node ast.Node, entering bool) ast.WalkStatus {
		if entering {
			if textNode, ok := node.(*ast.Text); ok {
				text.Write(textNode.Literal)
			}
		}
		return ast.GoToNext
	})
	return text.String()
}

// getNodeLine attempts to get the line number for a node
// Note: gomarkdown doesn't provide line numbers directly, so we estimate
func getNodeLine(node ast.Node) int {
	// This is a simplified version - in reality, tracking line numbers
	// requires parsing with line tracking enabled
	return 1 // TODO: Implement proper line tracking
}

// GitHubRepo holds resolved GitHub repository URL and branch
type GitHubRepo struct {
	URL    string
	Branch string
}

// ResolveGitHubRepo maps a frontmatter repository name to configured GitHub URL and
// branch.
// Returns nil if the repo is unknown or empty.
func (m Renderer) ResolveGitHubRepo(repo string) *GitHubRepo {
	return ResolveGitHubRepoFromProjects(m.projects, repo)
}

func ResolveGitHubRepoFromProjectID(
	projects server.ProjectsConfig,
	projectID string,
) *GitHubRepo {
	return ResolveGitHubRepoFromProjects(projects, projectID)
}

func ResolveGitHubRepoFromProjects(
	projects server.ProjectsConfig,
	repo string,
) *GitHubRepo {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil
	}
	configured, ok := projects.Repos[repo]
	if !ok {
		lowerRepo := strings.ToLower(repo)
		for name, candidate := range projects.Repos {
			if strings.ToLower(name) == lowerRepo {
				configured = candidate
				ok = true
				break
			}
		}
	}
	if !ok || strings.TrimSpace(configured.GitHubURL) == "" {
		return nil
	}
	return &GitHubRepo{
		URL:    strings.TrimRight(configured.GitHubURL, "/"),
		Branch: server.BaselineBranch(configured, server.CheckoutConfig{}),
	}
}

func GitHubURLForPath(projects server.ProjectsConfig, docPath string) (string, bool) {
	clean := filepath.ToSlash(strings.Trim(strings.TrimSpace(docPath), "/"))
	if clean == "" {
		return "", false
	}
	for repoName, repo := range projects.Repos {
		if strings.TrimSpace(repo.GitHubURL) == "" {
			continue
		}
		branch := server.BaselineBranch(repo, server.CheckoutConfig{})
		for _, checkout := range repo.Checkouts {
			root := filepath.ToSlash(
				strings.Trim(strings.TrimSpace(checkout.RootPath), "/"),
			)
			if root != "" && (clean == root || strings.HasPrefix(clean, root+"/")) {
				rel := strings.TrimPrefix(strings.TrimPrefix(clean, root), "/")
				return githubBlobURL(repo.GitHubURL, branch, rel), true
			}
		}
		if clean == repoName || strings.HasPrefix(clean, repoName+"/") {
			rel := strings.TrimPrefix(strings.TrimPrefix(clean, repoName), "/")
			return githubBlobURL(repo.GitHubURL, branch, rel), true
		}
	}
	return "", false
}

func githubBlobURL(baseURL, branch, relPath string) string {
	url := strings.TrimRight(baseURL, "/") + "/blob/" + branch
	if strings.TrimSpace(relPath) == "" {
		return url
	}
	return url + "/" + strings.TrimLeft(filepath.ToSlash(relPath), "/")
}

// codeFilePathRe matches file paths: must contain /, end with .ext, optional :line or
// :line-line
var codeFilePathRe = regexp.MustCompile(
	`^[\w][\w./@-]*?/[\w./@-]*\.\w{1,10}(?::\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*)?$`,
)

// githubCodeRe matches <code>...</code> optionally wrapped in <a>...</a>
var githubCodeRe = regexp.MustCompile(`(?:<a[^>]*>)?<code>([^<]+)</code>(?:</a>)?`)

// LinkCodePathsToGitHub post-processes rendered HTML to wrap file paths in <code> tags
// with GitHub links. Skips <code> already inside <a> tags (e.g., thoughts links).
func LinkCodePathsToGitHub(html string, gh *GitHubRepo) string {
	if gh == nil {
		return html
	}
	return githubCodeRe.ReplaceAllStringFunc(html, func(match string) string {
		// Already wrapped in <a> tag — skip
		if strings.HasPrefix(match, "<a") {
			return match
		}

		// Extract content between <code> and </code>
		content := match[6 : len(match)-7]

		// Skip if not a file path
		if !codeFilePathRe.MatchString(content) {
			return match
		}

		// Strip line numbers if present — just link to the file
		filePath := content
		if idx := strings.Index(content, ":"); idx > 0 {
			filePath = content[:idx]
		}

		href := fmt.Sprintf("%s/blob/%s/%s", gh.URL, gh.Branch, filePath)

		return fmt.Sprintf(
			`<a class="%s" href="%s" target="_blank" rel="noopener noreferrer"><code>%s</code></a>`,
			thoughtsLinkClass,
			href,
			content,
		)
	})
}
