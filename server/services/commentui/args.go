package commentui

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

type CommentSurface string

const (
	CommentSurfaceThoughts CommentSurface = "thoughts"
	CommentSurfaceArtifact CommentSurface = "artifact"
	CommentSurfaceDoc      CommentSurface = CommentSurfaceArtifact
)

type CommentableMarkdownArgs struct {
	Surface          CommentSurface
	IDPrefix         string
	WorkspaceID      string
	DocPath          string
	HTML             string
	Sections         []CommentSectionView
	Frontmatter      *CommentFrontmatterView
	Comments         []CommentThreadView
	Routes           CommentRoutes
	HiddenFields     map[string]string
	SelectionSignals SelectionSignalArgs
	UserEmail        string
}

type CommentSectionView struct {
	ID          string
	HeadingHTML string
	BodyHTML    string
	HTMLContent string
	LineStart   int
	LineEnd     int
	Title       string
}

type CommentFrontmatterView struct {
	Date          time.Time
	Researcher    string
	GitCommit     string
	Branch        string
	Repository    string
	Topic         string
	Tags          []string
	Status        string
	LastUpdated   time.Time
	LastUpdatedBy string
}

type CommentRoutes struct {
	Show          string
	Create        string
	Cancel        string
	Expand        string
	SelectComment string
	Reply         func(commentID string) string
	Resolve       func(commentID string) string
	Reopen        func(commentID string) string
}

type CommentThreadView struct {
	ID           string
	AuthorEmail  string
	ActorLabel   string
	CreatedAt    time.Time
	Body         string
	SelectedText string
	SectionID    string
	HeadingHint  string
	Resolved     bool
	Replies      []CommentReplyView
	HiddenFields map[string]string
}

type CommentReplyView struct {
	AuthorEmail string
	ActorLabel  string
	CreatedAt   time.Time
	Body        string
}

type CommentTargetChrome string

const (
	CommentTargetChromeVisible   CommentTargetChrome = "visible"
	CommentTargetChromePatchOnly CommentTargetChrome = "patch-only"
)

type CommentTargetView struct {
	ID           string
	SignalKey    string
	Surface      CommentSurface
	DocPath      string
	SectionID    string
	HeadingHint  string
	UserEmail    string
	Threads      []CommentThreadView
	Routes       CommentRoutes
	HiddenFields map[string]string
	Chrome       CommentTargetChrome
}

const (
	CommentsContextPanelID        = "comments-context-panel"
	CommentsContextThreadListID   = "comments-context-thread-list"
	MobileSectionCommentContentID = "mobile-section-comment-content"
	CommentSidebarSignal          = "commentSidebarExpanded"
)

type CommentSidebarView struct {
	ID                string
	SignalName        string
	InitiallyExpanded bool
	Threads           []CommentThreadView
	Target            CommentTargetView
}

type CommentAuthorsView struct {
	Authors        []string
	DisplayAuthors []string
	RemainingCount int
	TotalCount     int
	FirstAuthor    string
	OthersCount    int
}

type CommentFormView struct {
	ID           string
	Target       CommentTargetView
	SelectedText string
	Error        string
}

type CommentsPanelArgs struct {
	Surface            CommentSurface
	TargetPrefix       string
	DocPath            string
	Threads            []CommentThreadView
	ActiveSectionID    string
	ActiveSectionLabel string
	Routes             CommentRoutes
	HiddenFields       map[string]string
	UserEmail          string
}

type CommentThreadOptions struct {
	ShowSection      bool
	ShowSelectedText bool
	Compact          bool
}

func BuildCommentsPanelArgs(
	args CommentableMarkdownArgs,
	activeSectionID string,
) CommentsPanelArgs {
	_ = activeSectionID
	return CommentsPanelArgs{
		Surface:      args.Surface,
		TargetPrefix: args.IDPrefix,
		DocPath:      args.DocPath,
		Threads:      args.Comments,
		Routes:       args.Routes,
		HiddenFields: args.HiddenFields,
		UserEmail:    args.UserEmail,
	}
}

func CommentSectionLabel(thread CommentThreadView) string {
	if label := strings.TrimSpace(thread.HeadingHint); label != "" {
		return label
	}
	sectionID := strings.TrimSpace(sectionOrDocument(thread.SectionID))
	if sectionID == "document" {
		return "Document"
	}
	return sectionID
}

type CommentPopoverPlacement string

const (
	CommentPopoverPlacementTarget    CommentPopoverPlacement = "target"
	CommentPopoverPlacementSelection CommentPopoverPlacement = "selection"
)

type SelectionSignalArgs struct {
	Prefix          string
	ExcludeSelector string
	ShowRoute       string
	HiddenFields    map[string]string
	ContainerID     string
}

func CommentAnchorClass(signalKey string) string {
	return "commentui-anchor relative"
}

func CommentPopoverClass(placement CommentPopoverPlacement) string {
	base := "commentui-popover space-y-3 rounded-xl border border-border/70 bg-popover/95 p-2 shadow-xl backdrop-blur"
	switch placement {
	case CommentPopoverPlacementSelection:
		return base + " commentui-popover-selection"
	default:
		return base + " commentui-popover-target"
	}
}

func SelectionTriggerClass() string {
	return "commentui-selection-trigger"
}

func TargetChromeOrVisible(chrome CommentTargetChrome) CommentTargetChrome {
	if chrome == "" {
		return CommentTargetChromeVisible
	}
	return chrome
}

var slugUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func SafeCommentTargetSlug(parts ...string) string {
	joined := strings.Join(parts, "--")
	cleaned := strings.Trim(slugUnsafe.ReplaceAllString(joined, "-"), "-")
	if cleaned == "" {
		cleaned = "target"
	}
	h := sha1.Sum([]byte(joined))
	suffix := hex.EncodeToString(h[:])[:10]
	if len(cleaned) > 48 {
		cleaned = strings.Trim(cleaned[:48], "-")
		if cleaned == "" {
			cleaned = "target"
		}
	}
	return strings.ToLower(cleaned + "-" + suffix)
}

func SafeSelectionSignalPrefix(parts ...string) string {
	return "comment_selection_" + safeSignalIdentifier(SafeCommentTargetSlug(parts...))
}

func TargetID(prefix, sectionID string) string {
	return "comment-target-" + SafeCommentTargetSlug(prefix, sectionOrDocument(sectionID))
}

func SignalKey(prefix, sectionID string) string {
	return safeSignalIdentifier(
		SafeCommentTargetSlug(prefix, sectionOrDocument(sectionID)),
	)
}

func CommentsForSection(
	threads []CommentThreadView,
	sectionID, headingHint string,
) []CommentThreadView {
	sectionID = sectionOrDocument(sectionID)
	out := make([]CommentThreadView, 0)
	for _, thread := range threads {
		threadSection := sectionOrDocument(thread.SectionID)
		if threadSection == sectionID ||
			(headingHint != "" && thread.HeadingHint == headingHint) {
			out = append(out, thread)
		}
	}
	return out
}

func DocumentComments(threads []CommentThreadView) []CommentThreadView {
	return CommentsForSection(threads, "document", "")
}

func BuildCommentSidebarView(args CommentableMarkdownArgs) CommentSidebarView {
	target := CommentTargetView{
		ID:        TargetID(args.IDPrefix, "document"),
		SignalKey: SignalKey(args.IDPrefix, "document"),
		Surface:   args.Surface,
		DocPath:   args.DocPath,
		SectionID: "document",
		UserEmail: args.UserEmail,
		Threads:   args.Comments,
		Routes:    args.Routes,
		HiddenFields: MergeHidden(
			args.HiddenFields,
			map[string]string{"section_hint": "document", "heading_hint": ""},
		),
	}
	return CommentSidebarView{
		ID:                "comment-sidebar",
		SignalName:        CommentSidebarSignal,
		InitiallyExpanded: hasUnresolvedThreads(args.Comments),
		Threads:           args.Comments,
		Target:            target,
	}
}

func hasUnresolvedThreads(threads []CommentThreadView) bool {
	for _, thread := range threads {
		if !thread.Resolved {
			return true
		}
	}
	return false
}

func commentSidebarSignals(initiallyExpanded bool) string {
	if initiallyExpanded {
		return "{" + CommentSidebarSignal + ": true}"
	}
	return "{" + CommentSidebarSignal + ": false}"
}

func commentSidebarClass(initiallyExpanded bool) string {
	base := "shrink-0 min-h-screen transition-[width] duration-150"
	if initiallyExpanded {
		return base + " w-80"
	}
	return base + " w-10"
}

func unresolvedThreads(threads []CommentThreadView) []CommentThreadView {
	out := make([]CommentThreadView, 0, len(threads))
	for _, thread := range threads {
		if !thread.Resolved {
			out = append(out, thread)
		}
	}
	return out
}

func resolvedThreads(threads []CommentThreadView) []CommentThreadView {
	out := make([]CommentThreadView, 0, len(threads))
	for _, thread := range threads {
		if thread.Resolved {
			out = append(out, thread)
		}
	}
	return out
}

func sidebarThreadTarget(
	base CommentTargetView,
	thread CommentThreadView,
) CommentTargetView {
	out := base
	out.SectionID = sectionOrDocument(thread.SectionID)
	out.HeadingHint = thread.HeadingHint
	out.Threads = []CommentThreadView{thread}
	out.HiddenFields = MergeHidden(base.HiddenFields, map[string]string{
		"section_hint": out.SectionID,
		"heading_hint": out.HeadingHint,
	})
	out.HiddenFields = MergeHidden(out.HiddenFields, thread.HiddenFields)
	return out
}

func commentAuthorsView(threads []CommentThreadView, maxAvatars int) CommentAuthorsView {
	authors := uniqueAuthors(threads)
	if maxAvatars <= 0 {
		maxAvatars = 1
	}
	display := authors
	if len(display) > maxAvatars {
		display = display[:maxAvatars]
	}
	view := CommentAuthorsView{
		Authors:        authors,
		DisplayAuthors: display,
		RemainingCount: len(authors) - len(display),
		TotalCount:     totalThreadCount(threads),
	}
	if len(authors) > 0 {
		view.FirstAuthor = authors[0]
		view.OthersCount = len(authors) - 1
	}
	return view
}

func commentActorName(email string) string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "Unknown"
	}
	if at := strings.Index(trimmed, "@"); at > 0 {
		return trimmed[:at]
	}
	return trimmed
}

func commentInitial(name string) string {
	actor := commentActorName(name)
	runes := []rune(actor)
	if len(runes) == 0 {
		return "?"
	}
	return strings.ToUpper(string(runes[0]))
}

func MergeHidden(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

func sectionOrDocument(sectionID string) string {
	section := strings.TrimSpace(sectionID)
	if section == "" {
		return "document"
	}
	return section
}

func safeSignalIdentifier(value string) string {
	identifier := strings.ReplaceAll(value, "-", "_")
	if identifier == "" {
		return "signal"
	}
	return identifier
}
