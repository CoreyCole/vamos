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
	View               WorkbenchView
	ViewportClass      ViewportClass
	SavedConfig        *WorkbenchConfig
	Sidebar            WorkbenchSidebarArgs
	Center             CenterDocPaneArgs
	RightRail          RightRailArgs
	InitialSidebarOpen bool
	InitialRailOpen    bool
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
	sidebar := input.Sidebar
	sidebar.DefaultTab = defaults.SidebarTab

	rightRail := input.RightRail
	if rightRail.ActiveTab == "" {
		rightRail.ActiveTab = defaults.RightTab
	}

	page := WorkbenchPageThoughts
	if input.EntryMode == DocEntryModeAgentChat {
		page = WorkbenchPageAgentChat
	}
	view := input.View
	if view == "" {
		view = WorkbenchViewSplit
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

	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		UserEmail:     input.UserEmail,
		Page:          page,
		View:          view,
		ViewportClass: input.ViewportClass,
		ActivePath:    input.SelectedPath,
		ContextMode:   string(rightRail.ActiveTab),
		RouteHref:     routeHref,
		SavedConfig:   input.SavedConfig,
		Regions:       regions,
		FocusDefault:  !input.InitialSidebarOpen && !input.InitialRailOpen,
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
	if err != nil {
		return WorkbenchState{}, err
	}
	if input.InitialSidebarOpen {
		forceDocWorkbenchRegionVisible(&state, "doc-workbench-sidebar")
	}
	if input.InitialRailOpen {
		forceDocWorkbenchRegionVisible(&state, "doc-workbench-right")
	}
	return state, ValidateWorkbenchConfig(state.Config)
}

func forceDocWorkbenchRegionVisible(state *WorkbenchState, id string) {
	for i := range state.Regions {
		if state.Regions[i].ID == id {
			state.Regions[i].Visible = true
		}
	}
	for i := range state.Config.Regions {
		if state.Config.Regions[i].ID == id {
			state.Config.Regions[i].Visible = true
		}
	}
}

func rightRailKind(tab RightRailTab) RegionKind {
	if tab == RightRailTabChat {
		return RegionChat
	}
	return RegionComments
}
