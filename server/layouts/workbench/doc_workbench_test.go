package workbench

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestDocWorkbenchDefaultsForEntryMode(t *testing.T) {
	thoughts := DocWorkbenchDefaultsFor(DocEntryModeThoughts)
	if thoughts.SidebarTab != SidebarTabFiles ||
		thoughts.RightTab != RightRailTabChat {
		t.Fatalf("thoughts defaults = %#v, want files/chat", thoughts)
	}

	agentChat := DocWorkbenchDefaultsFor(DocEntryModeAgentChat)
	if agentChat.SidebarTab != SidebarTabWorkspaces ||
		agentChat.RightTab != RightRailTabChat {
		t.Fatalf("agent chat defaults = %#v, want workspaces/chat", agentChat)
	}
}

func TestBuildDocWorkbenchStateUsesSharedRegionIDs(t *testing.T) {
	base := WorkbenchDocContext{
		UserEmail:    "dev@example.com",
		SelectedPath: "plans/demo/design.md",
		Sidebar: WorkbenchSidebarArgs{
			ID:   "doc-workbench-shared-sidebar",
			Tabs: DefaultSidebarTabs(),
		},
		Center: CenterDocPaneArgs{Document: templ.Raw("<p>doc</p>")},
		RightRail: RightRailArgs{
			Chat:     templ.Raw("<p>chat</p>"),
			Comments: templ.Raw("<p>comments</p>"),
		},
	}

	thoughts, err := BuildDocWorkbenchState(base)
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState(thoughts) error = %v", err)
	}
	base.EntryMode = DocEntryModeAgentChat
	agentChat, err := BuildDocWorkbenchState(base)
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState(agent chat) error = %v", err)
	}

	wantIDs := []string{
		"doc-workbench-sidebar",
		"doc-workbench-center",
		"doc-workbench-right",
	}
	for i, want := range wantIDs {
		if thoughts.Regions[i].ID != want || agentChat.Regions[i].ID != want {
			t.Fatalf(
				"region %d ids = %q/%q, want %q",
				i,
				thoughts.Regions[i].ID,
				agentChat.Regions[i].ID,
				want,
			)
		}
	}
	if thoughts.ContextMode != "chat" || agentChat.ContextMode != "chat" {
		t.Fatalf(
			"context modes = %q/%q, want chat/chat",
			thoughts.ContextMode,
			agentChat.ContextMode,
		)
	}
	assertRegionVisible(t, thoughts, "doc-workbench-sidebar", false)
	assertRegionVisible(t, thoughts, "doc-workbench-center", true)
	assertRegionVisible(t, thoughts, "doc-workbench-right", false)
	if !thoughts.FocusDefault {
		t.Fatalf("thoughts FocusDefault = false, want true")
	}
}

func TestBuildDocWorkbenchStateKeepsDocumentActiveWhenRailInitiallyOpen(t *testing.T) {
	t.Parallel()

	state, err := BuildDocWorkbenchState(WorkbenchDocContext{
		EntryMode:       DocEntryModeThoughts,
		SelectedPath:    "plans/demo/design.md",
		InitialRailOpen: true,
		Center:          CenterDocPaneArgs{Document: templ.Raw("<p>doc</p>")},
		RightRail: RightRailArgs{
			ActiveTab: RightRailTabChat,
			Chat:      templ.Raw("<p>chat</p>"),
		},
	})
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState error = %v", err)
	}
	assertRegionVisible(t, state, "doc-workbench-center", true)
	assertRegionVisible(t, state, "doc-workbench-right", true)
	if state.FocusDefault {
		t.Fatalf("FocusDefault = true, want false when rail opens from context query")
	}
	if state.Config.Mobile.ActiveRegionID != "doc-workbench-center" {
		t.Fatalf(
			"mobile active region = %q, want document center",
			state.Config.Mobile.ActiveRegionID,
		)
	}
}

func TestBuildDocWorkbenchStateAppliesSavedConfig(t *testing.T) {
	t.Parallel()

	saved := WorkbenchConfig{
		Version:       1,
		Page:          WorkbenchPageThoughts,
		View:          WorkbenchViewSplit,
		ViewportClass: ViewportDesktopFull,
		Regions: []RegionSpec{
			{ID: "doc-workbench-sidebar", Slot: WorkbenchSlotNavigation, Kind: RegionThoughtsTree, Ratio: 0.18, Visible: false},
			{ID: "doc-workbench-center", Slot: WorkbenchSlotPrimary, Kind: RegionDocument, Ratio: 0.49, Visible: true},
			{ID: "doc-workbench-right", Slot: WorkbenchSlotContext, Kind: RegionChat, Ratio: 0.33, Visible: true},
		},
		Mobile: MobileSpec{ActiveRegionID: "doc-workbench-center"},
	}

	state, err := BuildDocWorkbenchState(WorkbenchDocContext{
		EntryMode:     DocEntryModeThoughts,
		SelectedPath:  "plans/demo/design.md",
		ViewportClass: ViewportDesktopFull,
		SavedConfig:   &saved,
		Center:        CenterDocPaneArgs{Document: templ.Raw("<p>doc</p>")},
		RightRail: RightRailArgs{
			ActiveTab: RightRailTabChat,
			Chat:      templ.Raw("<p>chat</p>"),
		},
	})
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState error = %v", err)
	}

	assertRegionRatio(t, state, "doc-workbench-right", 0.33)
	assertRegionVisible(t, state, "doc-workbench-sidebar", false)
	if state.ViewportClass != ViewportDesktopFull || state.Config.ViewportClass != ViewportDesktopFull {
		t.Fatalf("viewport class = %q/%q, want desktop-full", state.ViewportClass, state.Config.ViewportClass)
	}
}

func TestBuildDocWorkbenchStateUsesRequestedViewForSavedConfig(t *testing.T) {
	t.Parallel()

	saved := WorkbenchConfig{
		Version:       1,
		Page:          WorkbenchPageThoughts,
		View:          WorkbenchViewFocus,
		ViewportClass: ViewportDesktopFull,
		Regions: []RegionSpec{
			{ID: "doc-workbench-sidebar", Slot: WorkbenchSlotNavigation, Kind: RegionThoughtsTree, Ratio: 0.18, Visible: false},
			{ID: "doc-workbench-center", Slot: WorkbenchSlotPrimary, Kind: RegionDocument, Ratio: 0.49, Visible: true},
			{ID: "doc-workbench-right", Slot: WorkbenchSlotContext, Kind: RegionChat, Ratio: 0.33, Visible: true},
		},
		Mobile: MobileSpec{ActiveRegionID: "doc-workbench-center"},
	}

	state, err := BuildDocWorkbenchState(WorkbenchDocContext{
		EntryMode:     DocEntryModeThoughts,
		SelectedPath:  "plans/demo/design.md",
		View:          WorkbenchViewFocus,
		ViewportClass: ViewportDesktopFull,
		SavedConfig:   &saved,
		Center:        CenterDocPaneArgs{Document: templ.Raw("<p>doc</p>")},
		RightRail: RightRailArgs{
			ActiveTab: RightRailTabChat,
			Chat:      templ.Raw("<p>chat</p>"),
		},
	})
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState error = %v", err)
	}

	if state.View != WorkbenchViewFocus || state.Config.View != WorkbenchViewFocus {
		t.Fatalf("view = %q/%q, want focus", state.View, state.Config.View)
	}
	assertRegionRatio(t, state, "doc-workbench-right", 0.33)
}

func TestBuildDocWorkbenchStateIgnoresWrongViewportSavedConfig(t *testing.T) {
	t.Parallel()

	saved := WorkbenchConfig{
		Version:       1,
		Page:          WorkbenchPageThoughts,
		View:          WorkbenchViewSplit,
		ViewportClass: ViewportMobile,
		Regions: []RegionSpec{
			{ID: "doc-workbench-sidebar", Slot: WorkbenchSlotNavigation, Kind: RegionThoughtsTree, Ratio: 0.12, Visible: false},
			{ID: "doc-workbench-center", Slot: WorkbenchSlotPrimary, Kind: RegionDocument, Ratio: 0.12, Visible: true},
			{ID: "doc-workbench-right", Slot: WorkbenchSlotContext, Kind: RegionChat, Ratio: 0.76, Visible: true},
		},
		Mobile: MobileSpec{ActiveRegionID: "doc-workbench-right"},
	}

	state, err := BuildDocWorkbenchState(WorkbenchDocContext{
		EntryMode:     DocEntryModeThoughts,
		SelectedPath:  "plans/demo/design.md",
		ViewportClass: ViewportDesktopFull,
		SavedConfig:   &saved,
		Center:        CenterDocPaneArgs{Document: templ.Raw("<p>doc</p>")},
		RightRail: RightRailArgs{
			ActiveTab: RightRailTabChat,
			Chat:      templ.Raw("<p>chat</p>"),
		},
	})
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState error = %v", err)
	}

	assertRegionRatio(t, state, "doc-workbench-right", 0.26)
	if state.Config.Mobile.ActiveRegionID == "doc-workbench-right" {
		t.Fatalf("desktop state used mobile active region")
	}
}

func TestDocWorkbenchRenderContainsSharedChrome(t *testing.T) {
	state, err := BuildDocWorkbenchState(WorkbenchDocContext{
		EntryMode:    DocEntryModeThoughts,
		SelectedPath: "plans/demo/design.md",
		Sidebar: WorkbenchSidebarArgs{
			ID:   "doc-workbench-shared-sidebar",
			Tabs: DefaultSidebarTabs(),
		},
		Center: CenterDocPaneArgs{
			Title:    "design.md",
			Document: templ.Raw("<p>doc</p>"),
		},
		RightRail: RightRailArgs{
			Chat:     templ.Raw("<p>chat</p>"),
			Comments: templ.Raw("<p>comments</p>"),
		},
	})
	if err != nil {
		t.Fatalf("BuildDocWorkbenchState error = %v", err)
	}

	var buf bytes.Buffer
	if err := Workbench(state).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`id="doc-workbench-sidebar-region"`,
		`id="doc-workbench-center-region"`,
		`id="doc-workbench-right-region"`,
		`id="doc-workbench-center-pane"`,
		`id="doc-right-rail"`,
		`Open Workspaces and Files`,
		`Open Chat`,
		`Back to document`,
		`design.md`,
		`$workbench.activeRegionID = &#39;docWorkbenchRight&#39;`,
		`$workbench.activeRegionID = 'docWorkbenchCenter'`,
		`data-workbench-mobile-active="docWorkbenchCenter"`,
		`workbench-layout-save`,
		`$rightRailActiveTab = &#39;chat&#39;`,
		"Workspaces",
		"Files",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered workbench missing %q:\n%s", want, html)
		}
	}
	for _, unwanted := range []string{
		`id="workspace-doc-tree-header"`,
		`demo workspace`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"rendered workbench should not put related docs in topbar %q:\n%s",
				unwanted,
				html,
			)
		}
	}
}

func assertRegionVisible(t *testing.T, state WorkbenchState, regionID string, want bool) {
	t.Helper()
	for _, region := range state.Regions {
		if region.ID == regionID {
			if region.Visible != want {
				t.Fatalf(
					"region %s visible = %v, want %v",
					regionID,
					region.Visible,
					want,
				)
			}
			return
		}
	}
	t.Fatalf("region %s not found", regionID)
}

func assertRegionRatio(t *testing.T, state WorkbenchState, regionID string, want float64) {
	t.Helper()
	for _, region := range state.Regions {
		if region.ID == regionID {
			if region.Ratio != want {
				t.Fatalf("region %s ratio = %v, want %v", regionID, region.Ratio, want)
			}
			return
		}
	}
	t.Fatalf("region %s not found", regionID)
}
