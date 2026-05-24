package workbench

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

type SidebarTabKind string

const (
	SidebarTabWorkspaces SidebarTabKind = "workspaces"
	SidebarTabFiles      SidebarTabKind = "files"

	GlobalFilesScrollContainer = "global-files"
	CurrentDocTreeItemAttr     = "data-current-doc-tree-item"
	RevealScrollTriggerAttr    = "data-doc-scroll-on-reveal"

	WorkbenchSectionLinkAttr      = "data-workbench-section-link"
	WorkbenchSectionTargetAttr    = "data-workbench-section-target"
	WorkbenchSectionRegionAttr    = "data-workbench-section-region"
	WorkbenchSectionContainerAttr = "data-workbench-section-container"

	DefaultSectionRegionKey   = "docWorkbenchCenter"
	DefaultSectionContainerID = "thoughts-markdown-scroll-region"
)

type WorkbenchSidebarArgs struct {
	ID         string
	DefaultTab SidebarTabKind
	Tabs       []SidebarTab
	Workspaces WorkspacesPanelModel
	Files      FilesPanelModel
}

type SidebarTab struct {
	Kind      SidebarTabKind
	Label     string
	Available bool
}

type WorkspacesPanelModel struct {
	Roots         []WorkspaceRootItem
	CurrentRootID string
	CurrentThread string
	ThreadGroups  []WorkspaceThreadGroup
	EmptyLabel    string
}

type WorkspaceRootItem struct {
	ID            string
	Label         string
	Href          string
	Active        bool
	KindLabel     string
	Timestamp     string
	CountLabel    string
	Metadata      []WorkspaceMetadataItem
	Children      []WorkspaceRootItem
	InitiallyOpen bool
}

type WorkspaceMetadataItem struct {
	Label string
	Value string
}

type WorkspaceThreadGroup struct {
	ID          string
	Label       string
	KindLabel   string
	Timestamp   string
	ThreadCount int
	Active      bool
	Threads     []WorkspaceThreadItem
}

type WorkspaceThreadItem struct {
	ID          string
	Label       string
	Href        string
	Active      bool
	SourceLabel string
	CwdLabel    string
}

type DocumentPanelModel struct {
	CurrentPath        string
	TOC                []DocumentTOCItem
	Sections           []DocumentSectionItem
	CollapsedByDefault bool
	SignalName         string
}

type DocumentTOCItem struct {
	ID    string
	Text  string
	Level int
}

type DocumentSectionItem struct {
	ID    string
	Title string
	Level int
}

type FilesPanelModel struct {
	CurrentPath string
	Document    DocumentPanelModel
	Nodes       []FileTreeItem
}

type FileTreeItem struct {
	Name, Path   string
	Href         string
	FormAction   string
	HiddenFields map[string]string
	IsDir        bool
	IsExpanded   bool
	IsActive     bool
	Children     []FileTreeItem
}

func DefaultSidebarTabs() []SidebarTab {
	return []SidebarTab{
		{Kind: SidebarTabWorkspaces, Label: "Workspaces", Available: true},
		{Kind: SidebarTabFiles, Label: "Files", Available: true},
	}
}

func NewFilesPanelModel(
	currentPath string,
	document DocumentPanelModel,
	nodes []FileTreeItem,
) FilesPanelModel {
	return FilesPanelModel{
		CurrentPath: currentPath,
		Document:    document,
		Nodes:       nodes,
	}
}

func NewDocumentPanelModel(
	currentPath string,
	toc []DocumentTOCItem,
	sections []DocumentSectionItem,
) DocumentPanelModel {
	return DocumentPanelModel{
		CurrentPath: currentPath,
		TOC:         toc,
		Sections:    sections,
	}
}

func NormalizeSidebarDefault(args WorkbenchSidebarArgs) SidebarTabKind {
	for _, tab := range args.Tabs {
		if tab.Kind == args.DefaultTab && tab.Available {
			return tab.Kind
		}
	}
	for _, tab := range args.Tabs {
		if tab.Available {
			return tab.Kind
		}
	}
	return SidebarTabWorkspaces
}

func SidebarSignals(args WorkbenchSidebarArgs) string {
	return "{sidebarActiveTab: '" + string(NormalizeSidebarDefault(args)) + "'}"
}

func SidebarPanelInitialClass(active SidebarTabKind, panel SidebarTabKind) string {
	if active == panel {
		return ""
	}
	return "hidden"
}

func sidebarID(args WorkbenchSidebarArgs) string {
	if strings.TrimSpace(args.ID) != "" {
		return args.ID
	}
	return "workbench-shared-sidebar"
}

func sidebarTabActiveClass(kind SidebarTabKind) string {
	return "{'bg-background text-foreground shadow-sm': $sidebarActiveTab === '" + string(
		kind,
	) + "'}"
}

func sidebarTabClick(kind SidebarTabKind) string {
	expr := "$sidebarActiveTab = '" + string(kind) + "'; el.closest('#workbench-root')?.dispatchEvent(new CustomEvent('workbench-state-changed', { bubbles: true }))"
	if kind != SidebarTabFiles {
		return expr
	}
	return expr + "; const tab = el; requestAnimationFrame(() => tab.dispatchEvent(new CustomEvent('doc-scroll-reveal', { bubbles: true })))"
}

func sidebarRevealScrollAttrs(kind SidebarTabKind) templ.Attributes {
	if kind != SidebarTabFiles {
		return templ.Attributes{}
	}
	return templ.Attributes{RevealScrollTriggerAttr: GlobalFilesScrollContainer}
}

func sidebarTabShow(kind SidebarTabKind) string {
	return "$sidebarActiveTab === '" + string(kind) + "'"
}

func sidebarSectionHref(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "#"
	}
	return "#" + id
}

func sidebarSectionAttrs(id string) templ.Attributes {
	id = strings.TrimSpace(id)
	if id == "" {
		return templ.Attributes{}
	}
	return templ.Attributes{
		WorkbenchSectionLinkAttr:      true,
		WorkbenchSectionTargetAttr:    id,
		WorkbenchSectionRegionAttr:    DefaultSectionRegionKey,
		WorkbenchSectionContainerAttr: DefaultSectionContainerID,
	}
}

func sidebarDepthClass(level int) string {
	switch {
	case level <= 1:
		return "pl-2"
	case level == 2:
		return "pl-4"
	case level == 3:
		return "pl-6"
	default:
		return "pl-8"
	}
}

var sidebarSignalUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

func fileTreeOpenSignal(node FileTreeItem) string {
	key := strings.TrimSpace(node.Path)
	if key == "" {
		key = node.Name
	}
	return "sidebarFilesOpen_" + safeSidebarSignalKey(key)
}

func documentMapSignal(model DocumentPanelModel) string {
	if strings.TrimSpace(model.SignalName) != "" {
		return safeSidebarSignalKey(model.SignalName)
	}
	return "sidebarDocumentMapOpen"
}

func workspaceRootOpenSignal(root WorkspaceRootItem) string {
	key := strings.TrimSpace(root.ID)
	if key == "" {
		key = root.Label
	}
	return "sidebarWorkspaceOpen_" + safeSidebarSignalKey(key)
}

func safeSidebarSignalKey(key string) string {
	key = sidebarSignalUnsafe.ReplaceAllString(key, "_")
	return SignalKeyForID(key)
}

func sidebarBool(value bool) string {
	return strconv.FormatBool(value)
}

func CurrentDocScrollAttrs(isActive bool) templ.Attributes {
	if !isActive {
		return templ.Attributes{}
	}
	return templ.Attributes{
		CurrentDocTreeItemAttr:                    "true",
		"data-scroll-into-view__smooth__vnearest": true,
	}
}

func ViewTransitionName(scope, id string) string {
	scope = sidebarSignalUnsafe.ReplaceAllString(strings.TrimSpace(scope), "-")
	id = sidebarSignalUnsafe.ReplaceAllString(strings.TrimSpace(id), "-")
	if scope == "" {
		scope = "doc"
	}
	if id == "" {
		id = "item"
	}
	return scope + "-" + id
}
