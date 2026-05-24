package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/layoutprefs"
)

type ThemeProvider interface {
	GetCurrentTheme(c echo.Context) string     // Returns syntax theme
	GetCurrentThemeMode(c echo.Context) string // Returns "dark" or "light"
}

type DocumentWorkspaceResolver interface {
	ResolveWorkspaceForDocument(
		ctx context.Context,
		userEmail, documentPath string,
	) (DocumentWorkspaceContext, error)
}

type WorkspaceListResolver interface {
	ListWorkspaces(ctx context.Context, limit int64) ([]db.Workspace, error)
}

type WorkspaceDocTreeResolver interface {
	ListWorkspaceDocs(ctx context.Context, workspaceID string) ([]db.WorkspaceDoc, error)
}

type ChatWorkspaceCandidateResolver interface {
	ResolveChatWorkspaceCandidates(
		ctx context.Context,
		userEmail, documentPath string,
	) ([]ChatWorkspaceCandidate, error)
	OpenChatWorkspace(
		ctx context.Context,
		userEmail, rootPath string,
	) (OpenChatWorkspaceResult, error)
}

type ServiceOptions struct {
	Projects server.ProjectsConfig
}

type Service struct {
	renderer              *Renderer
	basePath              string
	commentService        *comments.Service
	themeService          ThemeProvider
	workspaceResolver     DocumentWorkspaceResolver
	chatWorkspaceResolver ChatWorkspaceCandidateResolver
	layoutPrefs           *layoutprefs.Service
	embeddedChatRenderer  EmbeddedChatRenderer
}

func NewService(
	basePath string,
	commentService *comments.Service,
	themeService ThemeProvider,
) (*Service, error) {
	return NewServiceWithOptions(basePath, commentService, themeService, ServiceOptions{})
}

func NewServiceWithOptions(
	basePath string,
	commentService *comments.Service,
	themeService ThemeProvider,
	opts ServiceOptions,
) (*Service, error) {
	renderer, err := NewRendererWithProjects("github-dark", opts.Projects)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown renderer: %w", err)
	}

	return &Service{
		renderer:       renderer,
		basePath:       basePath,
		commentService: commentService,
		themeService:   themeService,
	}, nil
}

func (s *Service) WithWorkspaceResolver(resolver DocumentWorkspaceResolver) *Service {
	s.workspaceResolver = resolver
	return s
}

func (s *Service) WithChatWorkspaceResolver(
	resolver ChatWorkspaceCandidateResolver,
) *Service {
	s.chatWorkspaceResolver = resolver
	return s
}

func (s *Service) WithLayoutPreferenceService(service *layoutprefs.Service) *Service {
	s.layoutPrefs = service
	return s
}

func (s *Service) WithEmbeddedChatRenderer(renderer EmbeddedChatRenderer) *Service {
	s.embeddedChatRenderer = renderer
	return s
}

// parseFrontmatter extracts YAML frontmatter from markdown content
// Returns the frontmatter struct, the remaining content, and any error
func parseFrontmatter(content []byte) (*Frontmatter, []byte, error) {
	// Check if content starts with frontmatter delimiter
	if !bytes.HasPrefix(content, []byte("---\n")) &&
		!bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, content, nil // No frontmatter
	}

	// Find the end delimiter
	startIdx := bytes.Index(content, []byte("---"))
	if startIdx == -1 {
		return nil, content, nil
	}

	// Find the second occurrence of ---
	remaining := content[startIdx+3:]
	endIdx := bytes.Index(remaining, []byte("\n---\n"))
	if endIdx == -1 {
		endIdx = bytes.Index(remaining, []byte("\r\n---\r\n"))
		if endIdx == -1 {
			return nil, content, nil // No closing delimiter
		}
	}

	// Extract frontmatter YAML (skip the first --- and get content before second ---)
	yamlContent := remaining[:endIdx]

	// Parse YAML into temporary struct with string dates
	var rawFm struct {
		Date           string   `yaml:"date"`
		Researcher     string   `yaml:"researcher"`
		GitCommit      string   `yaml:"git_commit"`
		Branch         string   `yaml:"branch"`
		Repository     string   `yaml:"repository"`
		Topic          string   `yaml:"topic"`
		Tags           []string `yaml:"tags"`
		Status         string   `yaml:"status"`
		LastUpdated    string   `yaml:"last_updated"`
		LastUpdatedBy  string   `yaml:"last_updated_by"`
		Stage          string   `yaml:"stage"`
		Ticket         string   `yaml:"ticket"`
		PlanDir        string   `yaml:"plan_dir"`
		Verdict        string   `yaml:"verdict"`
		RelatedADRs    []string `yaml:"related_adrs"`
		BrainstormDocs []string `yaml:"brainstorm_docs"`
	}

	if err := yaml.Unmarshal(yamlContent, &rawFm); err != nil {
		return nil, content, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Convert to final Frontmatter with parsed dates
	fm := &Frontmatter{
		Researcher:     rawFm.Researcher,
		GitCommit:      rawFm.GitCommit,
		Branch:         rawFm.Branch,
		Repository:     rawFm.Repository,
		Topic:          rawFm.Topic,
		Tags:           rawFm.Tags,
		Status:         rawFm.Status,
		LastUpdatedBy:  rawFm.LastUpdatedBy,
		Stage:          rawFm.Stage,
		Ticket:         rawFm.Ticket,
		PlanDir:        rawFm.PlanDir,
		Verdict:        rawFm.Verdict,
		RelatedADRs:    rawFm.RelatedADRs,
		BrainstormDocs: rawFm.BrainstormDocs,
		Date:           parseDate(rawFm.Date),
		LastUpdated:    parseDate(rawFm.LastUpdated),
	}

	// Return frontmatter and the content after the closing ---
	contentStart := startIdx + 3 + endIdx + 5 // skip "---\n" + yaml + "\n---\n"
	return fm, content[contentStart:], nil
}

// parseDate tries to parse a date string in various formats
func parseDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}

	// Try common date formats
	formats := []string{
		time.RFC3339,                // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05-07:00", // ISO 8601 with timezone
		"2006-01-02T15:04:05",       // ISO 8601 without timezone
		"2006-01-02",                // Simple date
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	// If parsing fails, return zero time
	return time.Time{}
}

// ProcessMarkdownFile handles all markdown processing logic
func (s *Service) ProcessMarkdownFile(requestPath string) (*PageArgs, error) {
	// Clean and validate the path
	cleanPath, err := CanonicalThoughtsDocPath(requestPath)
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(s.basePath, filepath.FromSlash(cleanPath))

	// Security check: ensure the path doesn't escape the base directory
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(s.basePath)) {
		return nil, errors.New("access denied: path escapes base directory")
	}

	// Check if file exists
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		// Try adding .md extension
		mdPath := fullPath + ".md"
		if mdInfo, err := os.Stat(mdPath); err == nil && !mdInfo.IsDir() {
			fullPath = mdPath
			fileInfo = mdInfo
		} else {
			return nil, fmt.Errorf("file not found: %s", requestPath)
		}
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", requestPath)
	}

	// Read the markdown file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Parse frontmatter if present
	frontmatter, markdownContent, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("error parsing frontmatter: %w", err)
	}

	// Parse markdown to extract table of contents
	parser := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	doc := parser.Parse(markdownContent)
	toc := s.extractTableOfContents(doc)

	// Render markdown to sections
	sections := s.renderer.RenderToSections(markdownContent)

	// Also render full HTML for fallback
	htmlContent := s.renderer.MarkdownBytesToHTML(markdownContent)

	// Post-process: link code file paths to GitHub if repository is in frontmatter
	if frontmatter != nil {
		gh := s.renderer.ResolveGitHubRepo(frontmatter.Repository)
		if gh != nil {
			htmlContent = LinkCodePathsToGitHub(htmlContent, gh)
			for i := range sections {
				sections[i].HeadingHTML = LinkCodePathsToGitHub(
					sections[i].HeadingHTML,
					gh,
				)
				sections[i].BodyHTML = LinkCodePathsToGitHub(sections[i].BodyHTML, gh)
				sections[i].HTMLContent = LinkCodePathsToGitHub(
					sections[i].HTMLContent,
					gh,
				)
			}
		}
	}

	// Build page args
	// Prepend "thoughts/" to match the full URL path structure
	fullFilePath := "thoughts/" + cleanPath
	return &PageArgs{
		ViewerArgs: ViewerArgs{
			HTMLContent: htmlContent,
			Frontmatter: frontmatter,
			Sections:    sections,
			RawMarkdown: string(markdownContent),
		},
		TableOfContents: toc,
		FilePath:        fullFilePath,
	}, nil
}

// extractDateFromFilename attempts to extract a date from the beginning of a filename
// Expected format: YYYY-MM-DD-rest-of-filename.md
func extractDateFromFilename(filename string) time.Time {
	// Remove .md extension if present
	name := strings.TrimSuffix(filename, ".md")

	// Check if filename starts with a date pattern YYYY-MM-DD
	if len(name) >= 10 {
		datePart := name[:10]
		if t, err := time.Parse("2006-01-02", datePart); err == nil {
			return t
		}
	}

	// Return zero time if no date found
	return time.Time{}
}

// GetDirectoryListing returns entries for a directory sorted by date in filename (newest
// first)
func (s *Service) GetDirectoryListing(dirPath string) (*DirectoryArgs, error) {
	fullDirPath := filepath.Join(s.basePath, dirPath)

	// Security check: ensure the path doesn't escape the base directory
	if !strings.HasPrefix(filepath.Clean(fullDirPath), filepath.Clean(s.basePath)) {
		return nil, errors.New("access denied: path escapes base directory")
	}

	entries, err := os.ReadDir(fullDirPath)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}

	// Create slice to hold items with their dates for sorting
	type itemWithDate struct {
		item DirectoryItem
		date time.Time
	}
	var itemsWithDate []itemWithDate

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			itemsWithDate = append(itemsWithDate, itemWithDate{
				item: DirectoryItem{
					Name:  name,
					Path:  filepath.Join(dirPath, name),
					IsDir: true,
				},
				date: extractDateFromFilename(name),
			})
		} else if strings.HasSuffix(name, ".md") {
			displayName := strings.TrimSuffix(name, ".md")
			itemsWithDate = append(itemsWithDate, itemWithDate{
				item: DirectoryItem{
					Name:  displayName,
					Path:  filepath.Join(dirPath, name),
					IsDir: false,
				},
				date: extractDateFromFilename(name),
			})
		}
	}

	// Sort by date in filename, most recent first
	// Items without dates will have zero time and appear at the end
	sort.Slice(itemsWithDate, func(i, j int) bool {
		// If both have dates, sort by date (most recent first)
		if !itemsWithDate[i].date.IsZero() && !itemsWithDate[j].date.IsZero() {
			return itemsWithDate[i].date.After(itemsWithDate[j].date)
		}
		// If only i has a date, i comes first
		if !itemsWithDate[i].date.IsZero() {
			return true
		}
		// If only j has a date, j comes first
		if !itemsWithDate[j].date.IsZero() {
			return false
		}
		// If neither has a date, sort alphabetically
		return itemsWithDate[i].item.Name < itemsWithDate[j].item.Name
	})

	// Extract sorted items
	items := make([]DirectoryItem, len(itemsWithDate))
	for i, item := range itemsWithDate {
		items[i] = item.item
	}

	return &DirectoryArgs{
		Path:  dirPath,
		Items: items,
	}, nil
}

// GetFileTree builds a recursive file tree rooted at basePath.
// Folders along activePath are expanded; the node matching activePath is marked active.
func (s *Service) GetFileTree(activePath string) []FileTreeNode {
	return s.buildTree("", activePath)
}

func InferWorkspaceRoot(markdownBasePath, docPath string) (string, bool) {
	cleanDoc := NormalizeWorkspaceDocPath(docPath)
	if cleanDoc == "" {
		return "", false
	}
	base, err := filepath.Abs(markdownBasePath)
	if err != nil {
		return "", false
	}
	abs := filepath.Join(base, filepath.FromSlash(cleanDoc))
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			rel, relErr := filepath.Rel(base, dir)
			if relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel), true
			}
		}
		if dir == base || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return "", false
}

func NormalizeWorkspaceDocPath(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	raw = strings.TrimPrefix(raw, "thoughts/")
	clean := path.Clean("/" + raw)
	return strings.Trim(clean, "/")
}

func (s *Service) BuildWorkspaceDocTreeFromRoot(
	rootDocPath, currentDocPath string,
) ([]workbench.WorkspaceDocNode, error) {
	rootRel, rootAbs, err := s.resolveThoughtsRelAndAbs(rootDocPath)
	if err != nil {
		return nil, err
	}
	currentRel := normalizeThoughtsRelativePath(currentDocPath)

	var build func(string) ([]workbench.WorkspaceDocNode, error)
	build = func(relDir string) ([]workbench.WorkspaceDocNode, error) {
		entries, err := os.ReadDir(filepath.Join(s.basePath, filepath.FromSlash(relDir)))
		if err != nil {
			return nil, err
		}

		var dirs, files []os.DirEntry
		for _, entry := range entries {
			name := entry.Name()
			if skipWorkspaceDocTreeEntry(name, entry.IsDir()) {
				continue
			}
			if entry.IsDir() {
				dirs = append(dirs, entry)
			} else if isWorkspaceDocTreeFile(name) {
				files = append(files, entry)
			}
		}
		sort.Slice(dirs, func(i, j int) bool {
			return strings.ToLower(dirs[i].Name()) < strings.ToLower(dirs[j].Name())
		})
		sort.Slice(files, func(i, j int) bool {
			return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
		})

		nodes := make([]workbench.WorkspaceDocNode, 0, len(dirs)+len(files))
		for _, dir := range dirs {
			rel := pathJoinSlash(relDir, dir.Name())
			children, err := build(rel)
			if err != nil {
				return nil, err
			}
			active := currentRel == rel || strings.HasPrefix(currentRel, rel+"/")
			nodes = append(nodes, workbench.WorkspaceDocNode{
				Path:       rel,
				RelPath:    strings.TrimPrefix(strings.TrimPrefix(rel, rootRel), "/"),
				Label:      dir.Name(),
				Kind:       workbench.WorkspaceDocKindDir,
				IsActive:   currentRel == rel,
				IsExpanded: active,
				Children:   children,
			})
		}
		for _, file := range files {
			rel := pathJoinSlash(relDir, file.Name())
			nodes = append(nodes, workbench.WorkspaceDocNode{
				Path:     rel,
				RelPath:  strings.TrimPrefix(strings.TrimPrefix(rel, rootRel), "/"),
				Label:    file.Name(),
				Kind:     workbench.WorkspaceDocKindFile,
				Href:     workbench.WorkspaceDocNodeHref(workbench.DocEntryModeThoughts, rel),
				IsActive: currentRel == rel,
			})
		}
		return nodes, nil
	}

	if stat, err := os.Stat(rootAbs); err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("workspace doc root is not a directory: %s", rootDocPath)
	}
	return build(rootRel)
}

func (s *Service) resolveThoughtsRelAndAbs(path string) (string, string, error) {
	rel := normalizeThoughtsRelativePath(path)
	if filepath.IsAbs(strings.TrimSpace(path)) {
		abs := filepath.Clean(strings.TrimSpace(path))
		base := filepath.Clean(s.basePath)
		if !pathWithinRoot(abs, base) && abs != base {
			return "", "", fmt.Errorf("path escapes thoughts root: %s", path)
		}
		computedRel, err := filepath.Rel(base, abs)
		if err != nil {
			return "", "", err
		}
		rel = filepath.ToSlash(computedRel)
	}
	if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", "", fmt.Errorf("path escapes thoughts root: %s", path)
	}
	return rel, filepath.Join(s.basePath, filepath.FromSlash(rel)), nil
}

func normalizeThoughtsRelativePath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "thoughts/")
	return strings.Trim(clean, "/")
}

func pathJoinSlash(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

func skipWorkspaceDocTreeEntry(name string, isDir bool) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if !isDir {
		return false
	}
	switch strings.ToLower(name) {
	case ".git", "node_modules", "dist", "build", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func isWorkspaceDocTreeFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md",
		".mdx",
		".txt",
		".go",
		".templ",
		".ts",
		".tsx",
		".js",
		".jsx",
		".json",
		".yaml",
		".yml",
		".toml",
		".sql",
		".css",
		".html",
		".sh":
		return true
	default:
		return false
	}
}

func (s *Service) buildTree(relDir, activePath string) []FileTreeNode {
	fullDir := filepath.Join(s.basePath, relDir)
	entries, err := os.ReadDir(fullDir)
	if err != nil {
		return nil
	}

	// Separate dirs and files, then sort each group alphabetically
	var dirs, files []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else if strings.HasSuffix(name, ".md") {
			files = append(files, e)
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name()) < strings.ToLower(dirs[j].Name())
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
	})

	// Normalize activePath for comparison (strip "thoughts/" prefix)
	cleanActive := strings.TrimPrefix(activePath, "thoughts/")
	cleanActive = strings.TrimSuffix(cleanActive, "/")

	var nodes []FileTreeNode

	for _, d := range dirs {
		dirPath := filepath.Join(relDir, d.Name())
		// Expand if activePath starts with this directory
		isOnActivePath := cleanActive == dirPath ||
			strings.HasPrefix(cleanActive, dirPath+"/")
		node := FileTreeNode{
			Name:       d.Name(),
			Path:       dirPath,
			IsDir:      true,
			IsExpanded: isOnActivePath,
			IsActive:   cleanActive == dirPath,
		}
		if isOnActivePath {
			node.Children = s.buildTree(dirPath, activePath)
		}
		nodes = append(nodes, node)
	}

	for _, f := range files {
		filePath := filepath.Join(relDir, f.Name())
		displayName := strings.TrimSuffix(f.Name(), ".md")
		nodes = append(nodes, FileTreeNode{
			Name:  displayName,
			Path:  filePath,
			IsDir: false,
			IsActive: cleanActive == filePath ||
				cleanActive == strings.TrimSuffix(filePath, ".md"),
		})
	}

	return nodes
}

// extractTableOfContents extracts headings from the markdown AST
func (s *Service) extractTableOfContents(doc ast.Node) []TocItem {
	var toc []TocItem

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if heading, ok := node.(*ast.Heading); ok && entering {
			if heading.Level <= 3 { // Only include h1, h2, h3 in TOC
				text := extractText(heading)
				id := generateID(text)

				// Set the heading ID for anchoring
				if heading.Attribute == nil {
					heading.Attribute = &ast.Attribute{}
				}
				heading.ID = []byte(id)

				toc = append(toc, TocItem{
					ID:    id,
					Text:  text,
					Level: heading.Level,
				})
			}
		}
		return ast.GoToNext
	})

	return toc
}

// extractText extracts text content from an AST node
func extractText(node ast.Node) string {
	var text strings.Builder

	ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
		if t, ok := n.(*ast.Text); ok && entering {
			text.Write(t.Literal)
		}
		return ast.GoToNext
	})

	return text.String()
}

// generateID creates a URL-safe ID from text
func generateID(text string) string {
	// Convert to lowercase
	id := strings.ToLower(text)
	// Replace spaces with hyphens
	id = strings.ReplaceAll(id, " ", "-")
	// Remove special characters
	id = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, id)
	// Remove multiple consecutive hyphens
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	// Trim hyphens from start and end
	id = strings.Trim(id, "-")

	return id
}
