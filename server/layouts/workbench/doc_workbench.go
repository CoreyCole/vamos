package workbench

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
)

type DocEntryMode string

const (
	DocEntryModeThoughts  DocEntryMode = "thoughts"
	DocEntryModeAgentChat DocEntryMode = "agent-chat"
)

type RightRailTab string

const (
	RightRailTabChat     RightRailTab = "chat"
	RightRailTabComments RightRailTab = "comments"
)

type DocWorkbenchDefaults struct {
	SidebarTab SidebarTabKind
	RightTab   RightRailTab
}

func DocWorkbenchDefaultsFor(mode DocEntryMode) DocWorkbenchDefaults {
	switch mode {
	case DocEntryModeAgentChat:
		return DocWorkbenchDefaults{
			SidebarTab: SidebarTabWorkspaces,
			RightTab:   RightRailTabChat,
		}
	default:
		return DocWorkbenchDefaults{
			SidebarTab: SidebarTabFiles,
			RightTab:   RightRailTabChat,
		}
	}
}

type WorkbenchDocContext struct {
	EntryMode          DocEntryMode
	UserEmail          string
	SelectedPath       string
	RouteHref          string
	Sidebar            WorkbenchSidebarArgs
	Center             CenterDocPaneArgs
	RightRail          RightRailArgs
	InitialSidebarOpen bool
	InitialRailOpen    bool
	SavedConfig        *WorkbenchConfig
}

type CenterDocPaneArgs struct {
	Title               string
	Document            templ.Component
	WorkspaceDocTree    *WorkspaceDocTreeArgs
	HeaderWorkspaceTree *WorkspaceDocTreeHeaderModel
	Actions             templ.Component
}

type RightRailArgs struct {
	ActiveTab RightRailTab
	Chat      templ.Component
	Comments  templ.Component
}

func BuildDocWorkbenchState(input WorkbenchDocContext) (WorkbenchState, error) {
	if strings.TrimSpace(input.SelectedPath) == "" {
		return WorkbenchState{}, fmt.Errorf("selected doc path is required")
	}
	defaults := DocWorkbenchDefaultsFor(input.EntryMode)
	sidebarTab := defaults.SidebarTab
	rightTab := defaults.RightTab
	if input.SavedConfig != nil {
		if input.SavedConfig.Tabs.SidebarTab != "" {
			sidebarTab = input.SavedConfig.Tabs.SidebarTab
		}
		if input.SavedConfig.Tabs.RightRailTab != "" {
			rightTab = input.SavedConfig.Tabs.RightRailTab
		}
	}
	sidebar := input.Sidebar
	sidebar.DefaultTab = sidebarTab

	rightRail := input.RightRail
	if rightRail.ActiveTab == "" || input.SavedConfig != nil && input.SavedConfig.Tabs.RightRailTab != "" {
		rightRail.ActiveTab = rightTab
	}

	page := WorkbenchPageThoughts
	if input.EntryMode == DocEntryModeAgentChat {
		page = WorkbenchPageAgentChat
	}
	routeHref := strings.TrimSpace(input.RouteHref)
	if routeHref == "" {
		routeHref = input.SelectedPath
	}

	regions := []WorkbenchRegion{
		{
			ID:        "doc-workbench-sidebar",
			Slot:      WorkbenchSlotNavigation,
			Kind:      RegionThoughtsTree,
			Ratio:     0.22,
			MinRem:    14,
			Visible:   true,
			TargetID:  "doc-workbench-sidebar-region",
			Title:     "Navigation",
			Component: SharedSidebar(sidebar),
		},
		{
			ID:        "doc-workbench-center",
			Slot:      WorkbenchSlotPrimary,
			Kind:      RegionDocument,
			Ratio:     0.52,
			MinRem:    28,
			Visible:   true,
			TargetID:  "doc-workbench-center-region",
			Title:     "Document",
			Component: CenterDocPane(input.Center),
		},
		{
			ID:        "doc-workbench-right",
			Slot:      WorkbenchSlotContext,
			Kind:      rightRailKind(rightRail.ActiveTab),
			Ratio:     0.26,
			MinRem:    18,
			Visible:   true,
			TargetID:  "doc-workbench-right-region",
			Title:     "Chat / Comments",
			Component: RightRail(rightRail),
		},
	}

	return BuildWorkbenchState(BuildWorkbenchStateInput{
		UserEmail:    input.UserEmail,
		Page:         page,
		View:         WorkbenchViewSplit,
		ActivePath:   input.SelectedPath,
		ContextMode:  string(rightRail.ActiveTab),
		RouteHref:    routeHref,
		SavedConfig:  input.SavedConfig,
		Regions:      regions,
		FocusDefault: input.SavedConfig == nil && !input.InitialSidebarOpen && !input.InitialRailOpen,
		NormalRegions: []RegionNormalState{
			{
				SignalKey: "doc-workbench-sidebar",
				Available: true,
				Visible:   input.InitialSidebarOpen,
			},
			{SignalKey: "doc-workbench-center", Available: true, Visible: true},
			{
				SignalKey: "doc-workbench-right",
				Available: true,
				Visible:   input.InitialRailOpen,
			},
		},
	})
}

func RightRailPanelInitialClass(active RightRailTab, panel RightRailTab) string {
	if active == panel {
		return "h-full min-h-0"
	}
	return "hidden h-full min-h-0"
}

func rightRailKind(tab RightRailTab) RegionKind {
	if tab == RightRailTabChat {
		return RegionChat
	}
	return RegionComments
}
