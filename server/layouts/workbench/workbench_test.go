package workbench

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestParseViewportClass(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		want  ViewportClass
		ok    bool
	}{
		{input: "mobile", want: ViewportMobile, ok: true},
		{input: "desktop-half", want: ViewportDesktopHalf, ok: true},
		{input: "desktop-full", want: ViewportDesktopFull, ok: true},
		{input: " desktop-full ", want: ViewportDesktopFull, ok: true},
		{input: "tablet", ok: false},
	} {
		got, ok := ParseViewportClass(tc.input)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ParseViewportClass(%q) = %q/%v, want %q/%v", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolveViewportClassUsesHeaderThenUAFallback(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set(ViewportClassHeader, "desktop-half")
	if got := ResolveViewportClass(header, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)"); got != ViewportDesktopHalf {
		t.Fatalf("header ResolveViewportClass() = %q, want desktop-half", got)
	}

	header = http.Header{"User-Agent": []string{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)"}}
	if got := ResolveViewportClass(header, ""); got != ViewportMobile {
		t.Fatalf("mobile UA ResolveViewportClass() = %q, want mobile", got)
	}

	if got := ResolveViewportClass(http.Header{}, "Mozilla/5.0 (X11; Linux x86_64)"); got != ViewportDesktopFull {
		t.Fatalf("desktop fallback ResolveViewportClass() = %q, want desktop-full", got)
	}
}

func TestDefaultWorkbenchLayoutUsesWorkspaceDocChat(t *testing.T) {
	t.Parallel()

	for _, page := range []WorkbenchPage{WorkbenchPageAgentChat, WorkbenchPageThoughts} {
		if got := DefaultWorkbenchLayout(page); got != WorkbenchWorkspaceDocChat {
			t.Fatalf(
				"DefaultWorkbenchLayout(%q) = %q, want %q",
				page,
				got,
				WorkbenchWorkspaceDocChat,
			)
		}
	}
}

func TestDefaultWorkbenchConfigAgentChatWorkspaceDocChat(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	if err := ValidateWorkbenchConfig(cfg); err != nil {
		t.Fatalf("ValidateWorkbenchConfig() error = %v", err)
	}
	if cfg.Mobile.ActiveRegionID != "agent-chat-primary" {
		t.Fatalf("mobile active region = %q", cfg.Mobile.ActiveRegionID)
	}
	byID := map[string]RegionSpec{}
	for _, region := range cfg.Regions {
		byID[region.ID] = region
	}
	for id, kind := range map[string]RegionKind{
		"agent-chat-navigation": RegionWorkspaceTopology,
		"agent-chat-primary":    RegionDoc,
		"agent-chat-context":    RegionChat,
	} {
		region := byID[id]
		if region.Kind != kind || !region.Visible {
			t.Fatalf("region %s = %#v, want visible %s", id, region, kind)
		}
	}
}

func TestDefaultWorkbenchConfigThoughts(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewFocus, "")
	if err := ValidateWorkbenchConfig(cfg); err != nil {
		t.Fatalf("ValidateWorkbenchConfig() error = %v", err)
	}
	visible := map[string]bool{}
	for _, region := range cfg.Regions {
		visible[region.ID] = region.Visible
	}
	if !visible["thoughts-sections"] || !visible["thoughts-document"] {
		t.Fatalf("thoughts document and sections should be visible: %#v", visible)
	}
	if visible["thoughts-context"] {
		t.Fatal("thoughts context should default hidden in focus view")
	}

	split := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewSplit, "comments")
	for _, region := range split.Regions {
		if region.ID == "thoughts-context" && !region.Visible {
			t.Fatal("thoughts context should be visible in split/comments view")
		}
	}
}

func TestDefaultAgentChatSplitKeepsChatAndContextBalanced(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewSplit, "artifacts")
	ratios := map[string]float64{}
	for _, region := range cfg.Regions {
		ratios[region.ID] = region.Ratio
	}
	if ratios["agent-chat-primary"] != ratios["agent-chat-context"] {
		t.Fatalf(
			"primary/context ratios = %v/%v, want balanced",
			ratios["agent-chat-primary"],
			ratios["agent-chat-context"],
		)
	}
}

func TestMergeWorkbenchConfigMigratesLegacyAgentChatSplitRatios(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(
		WorkbenchPageAgentChat,
		WorkbenchViewSplit,
		"artifacts",
	)
	saved := defaults
	for i := range saved.Regions {
		switch saved.Regions[i].ID {
		case "agent-chat-primary":
			saved.Regions[i].Ratio = 0.56
		case "agent-chat-context":
			saved.Regions[i].Ratio = 0.22
		}
	}
	merged := MergeWorkbenchConfig(defaults, &saved)
	ratios := map[string]float64{}
	for _, region := range merged.Regions {
		ratios[region.ID] = region.Ratio
	}
	if ratios["agent-chat-primary"] != ratios["agent-chat-context"] {
		t.Fatalf(
			"primary/context ratios = %v/%v, want migrated balanced defaults",
			ratios["agent-chat-primary"],
			ratios["agent-chat-context"],
		)
	}
}

func TestMergeWorkbenchConfigPartitionsDurableFieldsByViewportClass(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "", ViewportMobile)
	saved := cloneWorkbenchConfig(defaults)
	saved.Regions[0].Visible = false
	saved.Regions[0].Ratio = 0.31
	saved.Mobile.ActiveRegionID = "agent-chat-navigation"

	merged := MergeWorkbenchConfig(defaults, &saved, ViewportMobile)
	if got := regionSpecByID(merged, "agent-chat-navigation").Ratio; got != 0.31 {
		t.Fatalf("navigation ratio = %v, want 0.31", got)
	}
	if !regionSpecByID(merged, "agent-chat-navigation").Visible {
		t.Fatal("mobile should keep default visibility")
	}
	if merged.Mobile.ActiveRegionID != "agent-chat-navigation" {
		t.Fatalf("mobile active = %q, want saved navigation", merged.Mobile.ActiveRegionID)
	}
	desktopSaved := saved
	desktopSaved.ViewportClass = ViewportDesktopHalf
	desktop := MergeWorkbenchConfig(
		DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "", ViewportDesktopHalf),
		&desktopSaved,
		ViewportDesktopHalf,
	)
	if regionSpecByID(desktop, "agent-chat-navigation").Visible {
		t.Fatal("desktop should preserve saved visibility")
	}
	if desktop.Mobile.ActiveRegionID != defaults.Mobile.ActiveRegionID {
		t.Fatalf("desktop mobile active = %q, want default", desktop.Mobile.ActiveRegionID)
	}
}

func TestStripDurableInteractionState(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	cfg := cloneWorkbenchConfig(defaults)
	cfg.Regions[0].Visible = true
	cfg.Regions[0].Ratio = 0.34
	cfg.Mobile.ActiveRegionID = "agent-chat-navigation"

	stripped := StripDurableInteractionState(cfg, defaults, ViewportMobile)
	if got := regionSpecByID(stripped, "agent-chat-navigation").Ratio; got != 0.34 {
		t.Fatalf("ratio = %v, want preserved 0.34", got)
	}
	if !regionSpecByID(stripped, "agent-chat-navigation").Visible {
		t.Fatal("mobile visibility should be reset to default")
	}
	if stripped.Mobile.ActiveRegionID != "agent-chat-navigation" {
		t.Fatalf("mobile active = %q, want saved navigation", stripped.Mobile.ActiveRegionID)
	}
}

func TestStripDurableInteractionStateClearsMissingDefaultMobileRegion(t *testing.T) {
	t.Parallel()

	cfg := WorkbenchConfig{
		Version: 1,
		Page:    WorkbenchPageThoughts,
		View:    WorkbenchViewSplit,
		Regions: []RegionSpec{
			{ID: "doc-workbench-sidebar", Slot: WorkbenchSlotNavigation, Kind: RegionThoughtsTree, Ratio: 0.22},
			{ID: "doc-workbench-center", Slot: WorkbenchSlotPrimary, Kind: RegionDocument, Ratio: 0.61},
			{ID: "doc-workbench-right", Slot: WorkbenchSlotContext, Kind: RegionChat, Ratio: 0.17},
		},
	}
	defaults := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewSplit, "")

	stripped := StripDurableInteractionState(cfg, defaults, ViewportMobile)
	if stripped.Mobile.ActiveRegionID != "" {
		t.Fatalf("mobile active = %q, want empty fallback", stripped.Mobile.ActiveRegionID)
	}
	if err := ValidateWorkbenchConfig(stripped); err != nil {
		t.Fatalf("ValidateWorkbenchConfig() error = %v", err)
	}
}

func TestDesktopFullDoesNotUseSavedMobileActiveRegion(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewSplit, "chat", ViewportDesktopFull)
	mobileSaved := cloneWorkbenchConfig(defaults)
	mobileSaved.ViewportClass = ViewportMobile
	mobileSaved.Mobile.ActiveRegionID = "thoughts-context"

	merged := MergeWorkbenchConfig(defaults, &mobileSaved, ViewportDesktopFull)
	if merged.Mobile.ActiveRegionID == "thoughts-context" {
		t.Fatal("desktop-full used saved mobile active region")
	}
}

func TestMergeWorkbenchConfigIgnoresWrongPageOrView(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	saved := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewFocus, "")
	merged := MergeWorkbenchConfig(defaults, &saved)
	if merged.Page != WorkbenchPageAgentChat {
		t.Fatalf("merged page = %q", merged.Page)
	}
	for _, region := range merged.Regions {
		if region.ID == "agent-chat-navigation" && !region.Visible {
			t.Fatal(
				"wrong-page saved config should keep default agent-chat navigation visibility",
			)
		}
	}
}

func TestWorkbenchValidatesWorkspaceTopologyRegion(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	cfg.Regions[0].Kind = RegionWorkspaceTopology
	if err := ValidateWorkbenchConfig(cfg); err != nil {
		t.Fatalf("ValidateWorkbenchConfig() error = %v", err)
	}
}

func TestValidateWorkbenchConfigRejectsDuplicateIDsAndInvalidRatios(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	cfg.Regions = append(cfg.Regions, cfg.Regions[0])
	if err := ValidateWorkbenchConfig(
		cfg,
	); err == nil ||
		!strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate ids error = %v, want duplicate", err)
	}

	cfg = DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "")
	cfg.Regions[0].Ratio = 2
	if err := ValidateWorkbenchConfig(
		cfg,
	); err == nil ||
		!strings.Contains(err.Error(), "invalid ratio") {
		t.Fatalf("invalid ratio error = %v, want invalid ratio", err)
	}
}

func TestResizeHandleHelpersFindNextRegion(t *testing.T) {
	t.Parallel()

	state := WorkbenchState{Regions: []WorkbenchRegion{
		{ID: "agent-chat-navigation", Visible: false},
		{ID: "agent-chat-primary", Visible: true},
		{ID: "agent-chat-context", Visible: false},
	}}
	if !CanResizeAfter(state, 0) || !CanResizeAfter(state, 1) {
		t.Fatal("CanResizeAfter() should render handles between adjacent regions")
	}
	if got := NextVisibleSignalKey(state, 0); got != "agentChatPrimary" {
		t.Fatalf("NextVisibleSignalKey() = %q, want agentChatPrimary", got)
	}
	if CanResizeAfter(state, 2) {
		t.Fatal("CanResizeAfter() = true for final region")
	}
}

func TestResizeHandleRendersInvisibleGutterTarget(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := ResizeHandle(
		WorkbenchRegion{ID: "agent-chat-primary"},
		"agentChatContext",
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("ResizeHandle.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`data-workbench-resize-handle`,
		`class="group relative hidden w-0 shrink-0 outline-none md:!block"`,
		`absolute inset-y-2 -left-2 z-20 w-4 cursor-col-resize`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("resize handle html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{
		`data-show="!$workbench.focused"`,
		`w-2 shrink-0`,
		`w-px rounded-full bg-border`,
		`border-border`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"resize handle html = %s, should not contain visible separator %q",
				html,
				unwanted,
			)
		}
	}
}

func TestRegionDataClassTracksMobileActiveRegion(t *testing.T) {
	t.Parallel()

	region := WorkbenchRegion{ID: "thoughts-sections", Visible: true}
	dataClass := RegionDataClass(region)
	if !strings.Contains(dataClass, "$workbench.activeRegionID === 'thoughtsSections'") ||
		!strings.Contains(dataClass, "max-md:!hidden") ||
		!strings.Contains(dataClass, "max-md:!flex") {
		t.Fatalf("RegionDataClass() = %q, want mobile active-region classes", dataClass)
	}
}

func TestRegionRenderForcesActiveMobileRegionFullWidth(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := Region(
		WorkbenchState{},
		WorkbenchRegion{ID: "doc-workbench-sidebar"},
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("Region.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{"max-md:!w-full", "max-md:!flex-1"} {
		if !strings.Contains(html, want) {
			t.Fatalf("Region html = %s, want %q", html, want)
		}
	}
	if strings.Contains(html, "max-w-full") {
		t.Fatalf("Region html = %s, should not contain max width classes", html)
	}
}

func TestWorkbenchRootExposesFocusedAttributeForResizeJS(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		Page:         WorkbenchPageThoughts,
		View:         WorkbenchViewFocus,
		FocusDefault: true,
		Regions: []WorkbenchRegion{{
			ID:       "thoughts-document",
			Slot:     WorkbenchSlotPrimary,
			Kind:     RegionDocument,
			Visible:  true,
			TargetID: "thoughts-document-region",
		}},
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatalf("Workbench.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`id="workbench-root"`,
		`data-attr:data-workbench-focused="$workbench.focused ? 'true' : 'false'"`,
		`data-workbench-page="thoughts"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("Workbench html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{
		`>Focus</button>`,
		`>Exit focus</button>`,
		`>Reset layout</button>`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"Workbench html = %s, should not render global control %q",
				html,
				unwanted,
			)
		}
	}
}

func TestWorkbenchLoadsDocScrollScript(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := Workbench(WorkbenchState{}).Render(t.Context(), &body); err != nil {
		t.Fatalf("Workbench.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{`/js/workbench-resize.js`, `/js/workbench-doc-scroll.js`} {
		if !strings.Contains(html, want) {
			t.Fatalf("Workbench html = %s, want %q", html, want)
		}
	}
}

func TestWorkbenchDocScrollAssetContract(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/js/workbench-doc-scroll.js")
	if err != nil {
		t.Fatalf("ReadFile(workbench-doc-scroll.js) error = %v", err)
	}
	js := string(contents)
	for _, want := range []string{
		"scheduleCurrentDocRevealScroll",
		"scrollCurrentDocInContainer",
		"data-doc-scroll-on-reveal",
		"data-doc-scroll-container",
		"data-current-doc-tree-item",
		"doc-scroll-reveal",
		"requestAnimationFrame",
		"navigateWorkbenchSection",
		"navigateCurrentWorkbenchHash",
		"location.hash",
		"hashchange",
		"data-workbench-section-link",
		"workbench-section-nav",
		"thoughts-markdown-scroll-region",
		"docWorkbenchCenter",
		"scrollTargetInsideContainer",
		"data-workbench-signal",
		"handleWorkbenchSectionClick, true",
		"sectionTargetFromEventDetail",
		"Object.hasOwn(detail, 'hash')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("workbench-doc-scroll.js missing %q in %s", want, js)
		}
	}
	for _, unwanted := range []string{
		"setTimeout(",
		"window.workbenchScrollDocumentSection",
		"data-doc-section-target",
		"docSectionCloseSignal",
		"style.display",
		"style.setProperty('display'",
		"data-workbench-region-key",
		"workbench-layout-save",
		"root.dataset.workbenchMobileActive = key",
	} {
		if strings.Contains(js, unwanted) {
			t.Fatalf("workbench-doc-scroll.js should not contain %q in %s", unwanted, js)
		}
	}
}

func TestWorkbenchResizeJSShowsHandlesForVisibleAdjacentRegions(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/js/workbench-resize.js")
	if err != nil {
		t.Fatalf("ReadFile(workbench-resize.js) error = %v", err)
	}
	js := string(contents)
	for _, want := range []string{
		"const show = Boolean(",
		"before && after && isVisible(before) && isVisible(after)",
		"const content = visible.filter((region) => region !== navigation)",
		"if (!navigation || content.length === 0) return null",
		"const datastarModule =",
		"https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.1/bundles/datastar.js",
		"collapseRegion(root, navigationGroup.navigation)",
		"regionSlot(region) !== \"primary\"",
		"attributeFilter: [\"class\", \"style\", \"data-workbench-focused\"]",
		"function currentViewportClass(root)",
		"function activeRegionID(root)",
		"function visibleRegionSpecs(root)",
		"viewportClass: currentViewportClass(root)",
		"mobile: { activeRegionID: activeRegionID(root) }",
		"body: JSON.stringify({",
		"viewportClass: currentViewportClass(root)",
		"workbench-layout-save",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("workbench-resize.js missing %q in %s", want, js)
		}
	}
	for _, unwanted := range []string{
		"!focused && before",
		"workbenchFocused(root) ||",
		"regionMaxWidth",
		"workbenchMaxRem",
		"localStorage",
		"document.cookie",
		"mobile: { activeRegionID: \"\" }",
	} {
		if strings.Contains(js, unwanted) {
			t.Fatalf("workbench-resize.js should not contain %q in %s", unwanted, js)
		}
	}
}

func TestBuildWorkbenchStateAppliesSavedRatiosButKeepsRouteVisibility(t *testing.T) {
	t.Parallel()

	saved := DefaultWorkbenchConfig(WorkbenchPageAgentChat, WorkbenchViewFocus, "", ViewportMobile)
	saved.Regions[0].Visible = true
	saved.Regions[0].Ratio = 0.33
	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		Page:          WorkbenchPageAgentChat,
		View:          WorkbenchViewFocus,
		ViewportClass: ViewportMobile,
		SavedConfig:   &saved,
		Regions: []WorkbenchRegion{
			{
				ID:      "agent-chat-navigation",
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionPlanSidebar,
				Ratio:   0.22,
				Visible: false,
			},
			{
				ID:      "agent-chat-primary",
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionChat,
				Ratio:   0.39,
				Visible: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}
	if state.Regions[0].Ratio != 0.33 {
		t.Fatalf("navigation ratio = %v, want saved 0.33", state.Regions[0].Ratio)
	}
	if state.Regions[0].Visible {
		t.Fatal("route-hidden navigation should stay hidden despite saved visibility")
	}
}

func TestBuildWorkbenchStateFocusDefaultInitializesPrimaryOnlyLiveVisibility(
	t *testing.T,
) {
	t.Parallel()

	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		Page:         WorkbenchPageThoughts,
		View:         WorkbenchViewFocus,
		FocusDefault: true,
		Regions: []WorkbenchRegion{
			{
				ID:      "thoughts-sections",
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionSections,
				Ratio:   0.22,
				Visible: true,
			},
			{
				ID:      "thoughts-document",
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionDocument,
				Ratio:   0.39,
				Visible: true,
			},
			{
				ID:      "thoughts-context",
				Slot:    WorkbenchSlotContext,
				Kind:    RegionComments,
				Ratio:   0.22,
				Visible: false,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}
	if state.Regions[0].Visible || !state.Regions[1].Visible || state.Regions[2].Visible {
		t.Fatalf("focused regions = %#v, want only primary visible", state.Regions)
	}
	if !routeNormalVisible(state, state.Regions[0]) {
		t.Fatal("normal navigation visibility should remain available for focus exit")
	}
}

func TestFocusExitActionRestoresNormalVisibility(t *testing.T) {
	t.Parallel()

	state := WorkbenchState{Regions: []WorkbenchRegion{
		{ID: "thoughts-sections", Slot: WorkbenchSlotNavigation},
		{ID: "thoughts-document", Slot: WorkbenchSlotPrimary},
		{ID: "thoughts-context", Slot: WorkbenchSlotContext},
	}}
	action := FocusExitAction(state)
	for _, want := range []string{
		"$workbench.focused = false",
		"$workbench.regions.thoughtsSections.visible = Boolean($workbench.normalRegions.thoughtsSections?.visible)",
		"$workbench.activeRegionID = 'thoughtsDocument'",
	} {
		if !strings.Contains(action, want) {
			t.Fatalf("FocusExitAction() = %q, want %q", action, want)
		}
	}
}

func TestMobileRegionTabsRenderFromWorkbench(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		UserEmail: "user@example.com",
		Page:      WorkbenchPageThoughts,
		View:      WorkbenchViewSplit,
		Regions: []WorkbenchRegion{
			{
				ID:        "doc-workbench-sidebar",
				Slot:      WorkbenchSlotNavigation,
				Kind:      RegionThoughtsTree,
				TargetID:  "doc-workbench-sidebar-region",
				Visible:   true,
				Component: templ.NopComponent,
			},
			{
				ID:        "doc-workbench-center",
				Slot:      WorkbenchSlotPrimary,
				Kind:      RegionDocument,
				TargetID:  "doc-workbench-center-region",
				Visible:   true,
				Component: templ.NopComponent,
			},
			{
				ID:        "doc-workbench-right",
				Slot:      WorkbenchSlotContext,
				Kind:      RegionChat,
				TargetID:  "doc-workbench-right-region",
				Visible:   true,
				Component: templ.NopComponent,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatalf("Workbench.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`aria-label="Workbench regions"`,
		`aria-controls="doc-workbench-sidebar-region"`,
		`aria-controls="doc-workbench-center-region"`,
		`aria-controls="doc-workbench-right-region"`,
		`data-attr:aria-selected`,
		`id="workbench-regions"`,
		`data-workbench-mobile-active="docWorkbenchCenter"`,
		`data-workbench-viewport-class="desktop-full"`,
		`workbench-layout-save`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("Workbench missing %s: %s", want, html)
		}
	}
}

func TestMobileRegionTabsRenderUnavailableHiddenNotRemoved(t *testing.T) {
	t.Parallel()

	state := WorkbenchState{Regions: []WorkbenchRegion{
		{
			ID:       "agent-chat-navigation",
			TargetID: "agent-chat-navigation",
			Slot:     WorkbenchSlotNavigation,
			Visible:  false,
		},
		{
			ID:       "agent-chat-primary",
			TargetID: "agent-chat-primary",
			Slot:     WorkbenchSlotPrimary,
			Visible:  true,
		},
	}}
	var body bytes.Buffer
	if err := MobileRegionTabs(state).Render(t.Context(), &body); err != nil {
		t.Fatalf("MobileRegionTabs.Render() error = %v", err)
	}
	html := body.String()
	if strings.Count(html, "role=\"tab\"") != 2 {
		t.Fatalf("mobile tabs html = %s, want hidden nav tab still rendered", html)
	}
	if !strings.Contains(html, "$workbench.normalRegions.agentChatNavigation.available") {
		t.Fatalf("mobile tabs html = %s, want normal availability data-show", html)
	}
}

func TestSharedSidebarRendersRouteNeutralTabs(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := SharedSidebar(WorkbenchSidebarArgs{
		ID:         "test-sidebar",
		DefaultTab: SidebarTabFiles,
		Tabs:       DefaultSidebarTabs(),
		Workspaces: WorkspacesPanelModel{
			Roots: []WorkspaceRootItem{
				{Label: "Workspace", Href: "/agent-chat/ws", Active: true},
			},
		},
		Files: FilesPanelModel{
			Document: DocumentPanelModel{Sections: []DocumentSectionItem{{
				ID: "summary", Title: "Summary", Level: 1,
			}}},
			Nodes: []FileTreeItem{
				{
					Name:     "design.md",
					Path:     "design.md",
					Href:     "/thoughts/design.md",
					IsActive: true,
				},
			},
		},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("SharedSidebar.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`id="test-sidebar"`,
		`sidebarActiveTab: &#39;files&#39;`,
		`grid min-w-0 flex-1 grid-cols-2`,
		`Workspaces`,
		`Files`,
		`Summary`,
		`href="#summary"`,
		`data-workbench-section-link`,
		`data-workbench-section-target="summary"`,
		`data-workbench-section-region="docWorkbenchCenter"`,
		`data-workbench-section-container="thoughts-markdown-scroll-region"`,
		`href="/thoughts/design.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("SharedSidebar html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{
		`grid-cols-3`,
		`onclick=`,
		`workbenchScrollDocumentSection`,
		`data-doc-section-target`,
		`setTimeout`,
		`scrollIntoView`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("SharedSidebar html = %s, should not contain %q", html, unwanted)
		}
	}
}

func TestSharedSidebarRendersTOCSectionAttrs(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := SharedSidebar(WorkbenchSidebarArgs{
		DefaultTab: SidebarTabFiles,
		Tabs:       DefaultSidebarTabs(),
		Files: FilesPanelModel{
			Document: DocumentPanelModel{TOC: []DocumentTOCItem{{
				ID: "intro", Text: "Intro", Level: 2,
			}}},
		},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("SharedSidebar.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`href="#intro"`,
		`data-workbench-section-link`,
		`data-workbench-section-target="intro"`,
		`data-workbench-section-region="docWorkbenchCenter"`,
		`data-workbench-section-container="thoughts-markdown-scroll-region"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("SharedSidebar html = %s, want %q", html, want)
		}
	}
}

func TestSharedSidebarDefaultTabDiffersOnlyByPage(t *testing.T) {
	t.Parallel()

	base := WorkbenchSidebarArgs{
		Tabs: DefaultSidebarTabs(),
		Workspaces: WorkspacesPanelModel{Roots: []WorkspaceRootItem{{
			ID: "ws-1", Label: "Workspace", Href: "/agent-chat/ws-1", Active: true,
		}}},
		Files: FilesPanelModel{Nodes: []FileTreeItem{{
			Name: "plan.md", Path: "plan.md", Href: "/thoughts/plan.md", IsActive: true,
		}}},
	}
	thoughts := base
	thoughts.ID = "thoughts-shared-sidebar"
	thoughts.DefaultTab = SidebarTabFiles
	chat := base
	chat.ID = "agent-chat-shared-sidebar"
	chat.DefaultTab = SidebarTabWorkspaces

	thoughtsHTML := renderSharedSidebarForTest(t, thoughts)
	chatHTML := renderSharedSidebarForTest(t, chat)
	for _, tc := range []struct {
		name string
		html string
		want string
	}{
		{name: "thoughts", html: thoughtsHTML, want: `sidebarActiveTab: &#39;files&#39;`},
		{name: "chat", html: chatHTML, want: `sidebarActiveTab: &#39;workspaces&#39;`},
	} {
		if !strings.Contains(tc.html, tc.want) {
			t.Fatalf("%s sidebar html = %s, want %q", tc.name, tc.html, tc.want)
		}
	}
	for _, want := range []string{`Workspaces`, `Files`, `data-doc-scroll-container="global-files"`} {
		if !strings.Contains(thoughtsHTML, want) || !strings.Contains(chatHTML, want) {
			t.Fatalf(
				"shared sidebar structure missing %q\nthoughts=%s\nchat=%s",
				want,
				thoughtsHTML,
				chatHTML,
			)
		}
	}
}

func TestSharedSidebarRendersWorkspaceMetadataAndNestedRoots(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := SharedSidebar(WorkbenchSidebarArgs{
		DefaultTab: SidebarTabWorkspaces,
		Tabs:       DefaultSidebarTabs(),
		Workspaces: WorkspacesPanelModel{
			Roots: []WorkspaceRootItem{{
				ID:            "root-1",
				Label:         "Current plan",
				Href:          "/agent-chat/root-1",
				Active:        true,
				KindLabel:     "qrspi",
				Timestamp:     "May 17",
				CountLabel:    "2 sources",
				InitiallyOpen: true,
				Metadata: []WorkspaceMetadataItem{{
					Label: "Root",
					Value: "creative-mode-agent/plans/demo",
				}},
				Children: []WorkspaceRootItem{{
					ID:    "child-1",
					Label: "design.md",
					Href:  "/agent-chat/root-1?doc=design.md",
				}},
			}},
		},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("SharedSidebar.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`Toggle workspace tree`,
		`sidebarWorkspaceOpen_`,
		`Current plan`,
		`qrspi`,
		`May 17`,
		`2 sources`,
		`Root`,
		`creative-mode-agent/plans/demo`,
		`design.md`,
		`border-primary/30 bg-background shadow-sm`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("SharedSidebar html = %s, want %q", html, want)
		}
	}
	if strings.Contains(html, "freeform · Freeform · active") {
		t.Fatalf(
			"SharedSidebar html = %s, should not render workflow-status banner",
			html,
		)
	}
}

func TestSidebarRevealScrollAttrsOnlyRenderForFilesTab(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := SharedSidebar(WorkbenchSidebarArgs{
		DefaultTab: SidebarTabWorkspaces,
		Tabs:       DefaultSidebarTabs(),
		Files: FilesPanelModel{Nodes: []FileTreeItem{
			{
				Name:     "deep.md",
				Path:     "deep.md",
				Href:     "/thoughts/deep.md",
				IsActive: true,
			},
		}},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("SharedSidebar.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`data-doc-scroll-on-reveal="global-files"`,
		`data-doc-scroll-container="global-files"`,
		`data-current-doc-tree-item="true"`,
		`data-scroll-into-view__smooth__vnearest`,
		`doc-scroll-reveal`,
		`requestAnimationFrame`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("SharedSidebar html = %s, want %q", html, want)
		}
	}
	if strings.Count(html, `data-doc-scroll-on-reveal="global-files"`) != 1 {
		t.Fatalf("SharedSidebar html = %s, want exactly one Files reveal trigger", html)
	}
	for _, unwanted := range []string{`setTimeout`, `data-current-file-tree-item`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("SharedSidebar html = %s, should not contain %q", html, unwanted)
		}
	}
}

func TestCurrentDocScrollAttrsOnlyRenderForActiveRows(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := FilesPanel(FilesPanelModel{Nodes: []FileTreeItem{
		{Name: "inactive.md", Path: "inactive.md", Href: "/thoughts/inactive.md"},
		{
			Name:     "active.md",
			Path:     "active.md",
			Href:     "/thoughts/active.md",
			IsActive: true,
		},
	}}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("FilesPanel.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`data-doc-scroll-container="global-files"`,
		`href="/thoughts/active.md"`,
		`data-current-doc-tree-item="true"`,
		`data-scroll-into-view__smooth__vnearest`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("FilesPanel html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{
		`data-current-file-tree-item`,
		`setTimeout`,
		`scrollIntoView`,
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("FilesPanel html = %s, should not contain %q", html, unwanted)
		}
	}
}

func TestViewTransitionNameSanitizesInputs(t *testing.T) {
	t.Parallel()

	if got := ViewTransitionName(
		"doc viewer",
		"plans/demo.md",
	); got != "doc-viewer-plans-demo-md" {
		t.Fatalf("ViewTransitionName() = %q", got)
	}
	if got := ViewTransitionName("", ""); got != "doc-item" {
		t.Fatalf("ViewTransitionName(empty) = %q", got)
	}
}

func TestSharedDocWorkbenchViewerHasStableViewTransitionName(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := CenterDocPane(CenterDocPaneArgs{}).Render(t.Context(), &body); err != nil {
		t.Fatalf("CenterDocPane.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`id="doc-workbench-viewer-region"`,
		`data-view-transition-name="doc-viewer"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("CenterDocPane html = %s, want %q", html, want)
		}
	}
}

func TestFilesPanelMinimizesDocumentMapByDefault(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := FilesPanel(FilesPanelModel{
		Document: DocumentPanelModel{
			TOC:      []DocumentTOCItem{{ID: "intro", Text: "Intro", Level: 1}},
			Sections: []DocumentSectionItem{{ID: "details", Title: "Details", Level: 2}},
		},
		Nodes: []FileTreeItem{{
			Name:     "plan.md",
			Path:     "plan.md",
			Href:     "/thoughts/plan.md",
			IsActive: true,
		}},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("FilesPanel.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`Document`,
		`data-signals="{sidebarDocumentMapOpen: false}"`,
		`data-attr:aria-expanded="$sidebarDocumentMapOpen ? &#39;true&#39; : &#39;false&#39;"`,
		`data-show="$sidebarDocumentMapOpen"`,
		`Files`,
		`data-doc-scroll-container="global-files"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("FilesPanel html = %s, want %q", html, want)
		}
	}
}

func TestSharedSidebarFilesPanelUsesRouteOwnedFormActions(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	err := FilesPanel(FilesPanelModel{
		Document: DocumentPanelModel{TOC: []DocumentTOCItem{{
			ID: "intro", Text: "Intro", Level: 1,
		}}},
		Nodes: []FileTreeItem{{
			Name:       "artifact.md",
			Path:       "artifact.md",
			FormAction: "@post('/agent-chat/ws/artifacts/select', {contentType: 'form'})",
			HiddenFields: map[string]string{
				"artifact_rel_path": "artifact.md",
			},
		}},
	}).Render(t.Context(), &body)
	if err != nil {
		t.Fatalf("FilesPanel.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`href="#intro"`,
		`data-on:submit="@post(&#39;/agent-chat/ws/artifacts/select&#39;, {contentType: &#39;form&#39;})"`,
		`name="artifact_rel_path" value="artifact.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("FilesPanel html = %s, want %q", html, want)
		}
	}
}

func TestSignalsReflectInitialVisibility(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchState(BuildWorkbenchStateInput{
		Page: WorkbenchPageAgentChat,
		View: WorkbenchViewFocus,
		Regions: []WorkbenchRegion{
			{
				ID:      "agent-chat-navigation",
				Slot:    WorkbenchSlotNavigation,
				Kind:    RegionPlanSidebar,
				Visible: false,
			},
			{
				ID:      "agent-chat-primary",
				Slot:    WorkbenchSlotPrimary,
				Kind:    RegionChat,
				Visible: true,
			},
			{
				ID:      "agent-chat-context",
				Slot:    WorkbenchSlotContext,
				Kind:    RegionArtifact,
				Visible: false,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchState() error = %v", err)
	}
	signals := EncodeWorkbenchSignals(state)
	if !strings.Contains(signals, `"agentChatPrimary":{"ratio":0.39,"visible":true}`) {
		t.Fatalf("signals = %s, want visible agentChatPrimary", signals)
	}
	if got := RegionInitialClass(
		state.Regions[1],
	); !strings.Contains(got, "flex") ||
		strings.Contains(got, "hidden") {
		t.Fatalf("primary initial class = %q", got)
	}
}

func renderSharedSidebarForTest(t *testing.T, args WorkbenchSidebarArgs) string {
	t.Helper()
	var body bytes.Buffer
	if err := SharedSidebar(args).Render(t.Context(), &body); err != nil {
		t.Fatalf("SharedSidebar.Render() error = %v", err)
	}
	return body.String()
}

func regionSpecByID(config WorkbenchConfig, id string) RegionSpec {
	for _, region := range config.Regions {
		if region.ID == id {
			return region
		}
	}
	return RegionSpec{}
}
