package markdown

import (
	"context"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/layouts"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/comments"
	"github.com/CoreyCole/vamos/server/services/commentui"
)

// Frontmatter represents the YAML frontmatter in markdown files
type Frontmatter struct {
	Date           time.Time `yaml:"date"`
	Researcher     string    `yaml:"researcher"`
	GitCommit      string    `yaml:"git_commit"`
	Branch         string    `yaml:"branch"`
	Repository     string    `yaml:"repository"`
	Topic          string    `yaml:"topic"`
	Tags           []string  `yaml:"tags"`
	Status         string    `yaml:"status"`
	LastUpdated    time.Time `yaml:"last_updated"`
	LastUpdatedBy  string    `yaml:"last_updated_by"`
	Stage          string    `yaml:"stage"`
	Ticket         string    `yaml:"ticket"`
	PlanDir        string    `yaml:"plan_dir"`
	Verdict        string    `yaml:"verdict"`
	RelatedADRs    []string  `yaml:"related_adrs"`
	BrainstormDocs []string  `yaml:"brainstorm_docs"`
}

// ViewerArgs for the markdown content renderer
type ViewerArgs struct {
	HTMLContent string
	Frontmatter *Frontmatter
	Sections    []Section // Parsed sections with metadata for layout
	RawMarkdown string    // Original markdown content for clipboard
}

// PageArgs for the full markdown page
type DocumentWorkspaceContext struct {
	WorkspaceID  string
	RootDocPath  string
	RelativePath string
	Attached     bool
	Ambiguous    bool
}

type EmbeddedChatLinkState struct {
	Active      bool
	WorkspaceID string
	ThreadID    string
	RunID       string
}

type PageArgs struct {
	ViewerArgs           ViewerArgs
	TableOfContents      []TocItem
	FilePath             string
	UserEmail            string                        // User email from auth context
	Comments             *comments.GetCommentsResponse // Comments for this file
	SectionsWithComments []SectionWithComments         // Sections paired with their comments
	CurrentTheme         string                        // "dark" or "light" - from user preferences
	CurrentSyntaxTheme   string                        // Current syntax theme name
	PageSessionID        string                        // Unique page session ID for this tab
	FileTree             []FileTreeNode                // Sidebar file tree
	ChatLinkState        EmbeddedChatLinkState
	CommentUI            commentui.CommentableMarkdownArgs
	WorkspaceContext     DocumentWorkspaceContext
	QRSPIMetadata        QRSPIMetadata
}

type DocumentPanelArgs struct {
	Document      WorkbenchDocument
	WorkspaceTree *workbench.WorkspaceDocTreeHeaderModel
}

type DocumentAction struct {
	Label        string
	FormAction   string
	Href         string
	Icon         string
	AriaLabel    string
	HiddenFields map[string]string
}

type SectionMapArgs struct {
	FilePath string
	TOC      []TocItem
	Sections []Section
}

type MarkdownWorkbenchArgs struct {
	PageArgs  *PageArgs
	Workbench workbench.WorkbenchState
}

type DirectoryWorkbenchArgs struct {
	Directory *DirectoryArgs
	Workbench workbench.WorkbenchState
}

type ThoughtsContextArgs struct {
	Mode      string
	PageArgs  *PageArgs
	CommentUI commentui.CommentableMarkdownArgs
	Component templ.Component
}

type EmbeddedChatRenderRequest struct {
	UserEmail        string
	DocPath          string
	Context          string
	WorkspaceID      string
	ThreadID         string
	RunID            string
	AttachDoc        bool
	WorkspaceContext DocumentWorkspaceContext
}

type EmbeddedChatURLReplacement struct {
	URL string
}

type EmbeddedChatRenderer interface {
	RenderEmbeddedChatPanel(
		ctx context.Context,
		request EmbeddedChatRenderRequest,
	) (templ.Component, EmbeddedChatURLReplacement, error)
}

// TocItem for table of contents navigation
type TocItem struct {
	ID    string
	Text  string
	Level int
}

// Section represents a rendered markdown section with metadata
type Section struct {
	ID          string // Unique section identifier (e.g., "section-1")
	HeadingHTML string // Rendered HTML for the heading only (h1, h2, etc.)
	BodyHTML    string // Rendered HTML for the section body (everything after heading)
	HTMLContent string // Full rendered HTML (heading + body) - for backwards compatibility
	LineStart   int    // Starting line number in source markdown
	LineEnd     int    // Ending line number in source markdown
	Title       string // Section title (from heading or generated)
}

// SectionWithComments pairs a section with its associated comments
type SectionWithComments struct {
	Section  Section
	Comments []comments.CommentWithReplies
}

// DirectoryArgs for directory listing
type DirectoryArgs struct {
	Path               string
	Items              []DirectoryItem
	UserEmail          string         // User email from auth context
	CurrentTheme       string         // "dark" or "light" - from user preferences
	CurrentSyntaxTheme string         // Current syntax theme name
	FileTree           []FileTreeNode // Sidebar file tree
	ChatLinkState      EmbeddedChatLinkState
}

// DirectoryItem represents a file or directory
type DirectoryItem struct {
	Name  string
	Path  string
	IsDir bool
}

// FileTreeNode represents a node in the file/folder tree sidebar
type FileTreeNode struct {
	Name       string
	Path       string
	IsDir      bool
	IsExpanded bool // folder is open (along the active path)
	IsActive   bool // this node is the currently viewed file/directory
	Children   []FileTreeNode
}

// BuildRootArgs creates RootArgs for the layout
func (args *PageArgs) BuildRootArgs() layouts.RootArgs {
	// Determine page type from file path
	pageType := determinePageType(args.FilePath)

	return layouts.RootArgs{
		Title:              getTitle(args.FilePath),
		CurrentPath:        args.FilePath,
		PageType:           pageType,
		ShowHeader:         true,
		UserEmail:          args.UserEmail,
		CurrentTheme:       args.CurrentTheme,
		CurrentSyntaxTheme: args.CurrentSyntaxTheme,
		ClipboardContent:   args.ViewerArgs.RawMarkdown,
		BreadcrumbChatLinkState: layouts.BreadcrumbLinkState{
			Active:      args.ChatLinkState.Active,
			WorkspaceID: args.ChatLinkState.WorkspaceID,
			ThreadID:    args.ChatLinkState.ThreadID,
			RunID:       args.ChatLinkState.RunID,
		},
	}
}

// BuildRootArgs creates RootArgs for directory listing
func (args *DirectoryArgs) BuildRootArgs() layouts.RootArgs {
	title := "Documentation"
	if args.Path != "" {
		title = args.Path + " - Documentation"
	}

	return layouts.RootArgs{
		Title:              title,
		CurrentPath:        args.Path,
		PageType:           layouts.PageTypeDirectory,
		ShowHeader:         true,
		UserEmail:          args.UserEmail,
		CurrentTheme:       args.CurrentTheme,
		CurrentSyntaxTheme: args.CurrentSyntaxTheme,
		BreadcrumbChatLinkState: layouts.BreadcrumbLinkState{
			Active:      args.ChatLinkState.Active,
			WorkspaceID: args.ChatLinkState.WorkspaceID,
			ThreadID:    args.ChatLinkState.ThreadID,
			RunID:       args.ChatLinkState.RunID,
		},
	}
}

// DirectoryTitle is the icon-free title for directory center headers.
func DirectoryTitle(path string) string {
	if path == "" {
		return "Documentation Root"
	}
	return path
}

// GetDisplayPath is a helper function to get display path for directory listings.
func GetDisplayPath(path string) string {
	return DirectoryTitle(path)
}

// getTitle creates a display title from a file path
func getTitle(filePath string) string {
	parts := GetPathParts(filePath)
	if len(parts) > 0 {
		title := parts[len(parts)-1]
		// Convert snake_case or kebab-case to Title Case
		title = strings.ReplaceAll(title, "-", " ")
		title = strings.ReplaceAll(title, "_", " ")
		return strings.Title(title) + " - Documentation"
	}
	return "Documentation"
}

// GetPathParts splits a file path into parts for breadcrumb navigation
func GetPathParts(filePath string) []string {
	// Remove leading slash and split
	cleaned := strings.TrimPrefix(filePath, "/")
	cleaned = strings.TrimPrefix(cleaned, "thoughts/")
	cleaned = strings.TrimSuffix(cleaned, ".md")
	if cleaned == "" {
		return []string{"home"}
	}
	return strings.Split(cleaned, "/")
}

// determinePageType determines the page type from the file path
func determinePageType(filePath string) layouts.PageType {
	if strings.Contains(filePath, "/plans/") {
		return layouts.PageTypePlans
	}
	if strings.Contains(filePath, "/research/") {
		return layouts.PageTypeResearch
	}
	if strings.Contains(filePath, "/spec/") {
		return layouts.PageTypeSpec
	}
	return layouts.PageTypeMarkdown
}
