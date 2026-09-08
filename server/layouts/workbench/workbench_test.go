package workbench

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
			t.Fatalf(
				"ParseViewportClass(%q) = %q/%v, want %q/%v",
				tc.input,
				got,
				ok,
				tc.want,
				tc.ok,
			)
		}
	}
}

func TestResolveViewportClassUsesHeaderThenUAFallback(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set(ViewportClassHeader, "desktop-half")
	if got := ResolveViewportClass(
		header,
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
	); got != ViewportDesktopHalf {
		t.Fatalf("header ResolveViewportClass() = %q, want desktop-half", got)
	}

	header = http.Header{
		"User-Agent": []string{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)"},
	}
	if got := ResolveViewportClass(header, ""); got != ViewportMobile {
		t.Fatalf("mobile UA ResolveViewportClass() = %q, want mobile", got)
	}

	if got := ResolveViewportClass(
		http.Header{},
		"Mozilla/5.0 (X11; Linux x86_64)",
	); got != ViewportDesktopFull {
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

	defaults := DefaultWorkbenchConfig(
		WorkbenchPageAgentChat,
		WorkbenchViewFocus,
		"",
		ViewportMobile,
	)
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
		t.Fatalf(
			"mobile active = %q, want saved navigation",
			merged.Mobile.ActiveRegionID,
		)
	}
	desktopSaved := saved
	desktopSaved.ViewportClass = ViewportDesktopHalf
	desktop := MergeWorkbenchConfig(
		DefaultWorkbenchConfig(
			WorkbenchPageAgentChat,
			WorkbenchViewFocus,
			"",
			ViewportDesktopHalf,
		),
		&desktopSaved,
		ViewportDesktopHalf,
	)
	if regionSpecByID(desktop, "agent-chat-navigation").Visible {
		t.Fatal("desktop should preserve saved visibility")
	}
	if desktop.Mobile.ActiveRegionID != defaults.Mobile.ActiveRegionID {
		t.Fatalf(
			"desktop mobile active = %q, want default",
			desktop.Mobile.ActiveRegionID,
		)
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
		t.Fatalf(
			"mobile active = %q, want saved navigation",
			stripped.Mobile.ActiveRegionID,
		)
	}
}

func TestStripDurableInteractionStateClearsMissingDefaultMobileRegion(t *testing.T) {
	t.Parallel()

	cfg := WorkbenchConfig{
		Version: 1,
		Page:    WorkbenchPageThoughts,
		View:    WorkbenchViewSplit,
		Regions: []RegionSpec{
			{
				ID:    "doc-workbench-sidebar",
				Slot:  WorkbenchSlotNavigation,
				Kind:  RegionThoughtsTree,
				Ratio: 0.22,
			},
			{
				ID:    "doc-workbench-center",
				Slot:  WorkbenchSlotPrimary,
				Kind:  RegionDocument,
				Ratio: 0.61,
			},
			{
				ID:    "doc-workbench-right",
				Slot:  WorkbenchSlotContext,
				Kind:  RegionChat,
				Ratio: 0.17,
			},
		},
	}
	defaults := DefaultWorkbenchConfig(WorkbenchPageThoughts, WorkbenchViewSplit, "")

	stripped := StripDurableInteractionState(cfg, defaults, ViewportMobile)
	if stripped.Mobile.ActiveRegionID != "" {
		t.Fatalf(
			"mobile active = %q, want empty fallback",
			stripped.Mobile.ActiveRegionID,
		)
	}
	if err := ValidateWorkbenchConfig(stripped); err != nil {
		t.Fatalf("ValidateWorkbenchConfig() error = %v", err)
	}
}

func TestDesktopFullDoesNotUseSavedMobileActiveRegion(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(
		WorkbenchPageThoughts,
		WorkbenchViewSplit,
		"chat",
		ViewportDesktopFull,
	)
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
		`class="group relative z-30 hidden w-0 shrink-0 touch-none outline-none md:!block"`,
		`absolute inset-y-0 left-1/2 z-30 w-4 -translate-x-1/2 cursor-col-resize`,
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
	for _, want := range []string{
		`/js/workbench-resize.js`,
		`/js/workbench-history.js`,
		`/js/workbench-doc-scroll.js`,
		`/js/frame-comment-bridge.js?v=4`,
		`data-commentui-mode="parent"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("Workbench html = %s, want %q", html, want)
		}
	}
}

func TestFrameCommentBridgeAssetContract(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/js/frame-comment-bridge.js")
	if err != nil {
		t.Fatalf("ReadFile(frame-comment-bridge.js) error = %v", err)
	}
	js := string(contents)
	for _, want := range []string{
		`dataset.commentuiMode`,
		`event.source !== window.parent`,
		`candidate.contentWindow === event.source`,
		`opaque-postmessage`,
		`same-origin-dom`,
		`MutationObserver`,
		`frame.addEventListener("load"`,
		`"selectionchange", "mouseup", "touchend"`,
		`maxQuoteLength`,
		`selectionClearType`,
		`clearChildSelection`,
		`validRectangle`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("frame comment bridge missing %q", want)
		}
	}
	if strings.Contains(js, "doc_path") || strings.Contains(js, "artifact_path") {
		t.Fatalf("frame comment bridge accepts artifact identity from the child")
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
		"const datastarModule = import(\"@vamos/datastar\");",
		"collapseRegion(root, navigationGroup.navigation)",
		"clampRegionWidth(",
		"regionMinWidth(before)",
		"regionMinWidth(after)",
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
		"regionHasSSRFlex",
		"workbenchPixelLock",
		"bindResizeHandles",
		"workbenchResizeBound",
		"lockPixelWidthsFromPaint",
		"syncRatiosFromPaint",
		"never auto-close from gutter drag",
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

func TestWorkbenchHistoryJSReloadsSameDocumentArtifactPopstate(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/js/workbench-history.js")
	if err != nil {
		t.Fatalf("ReadFile(workbench-history.js) error = %v", err)
	}
	js := string(contents)
	for _, want := range []string{
		`window.addEventListener("popstate"`,
		`document.getElementById("thread-artifact-pane")`,
		`window.location.reload()`,
		`history.state?.workbenchArtifactPatch`,
		`pinChatToBottom`,
		`settleChatPin`,
		`pinAfterFonts`,
		`scheduleChatPinAfterReveal`,
		`scheduleChatPinOnPageshow`,
		`chatOverflowScroller`,
		`agent-chat-messages`,
		`agent-chat-scroll-region`,
		`scrollTop = region.scrollHeight`,
		`scrollIntoView`,
		`requestAnimationFrame`,
		`fonts.ready`,
		`event?.viewTransition?.finished`,
		`window.addEventListener("pagereveal"`,
		`window.addEventListener("pageshow"`,
		`"onpagereveal" in window`,
		`queueMicrotask(pinAfterFonts)`,
		`event?.persisted`,
		`data-wb2-vt-nav`,
		`thread-switch`,
		`isThreadToThreadNavigation`,
		`setThreadSwitchChatUnname`,
		`onpageswap`,
		`pageswap`,
		`scheduleThreadSwitchChatUnnameOnReveal`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("workbench-history.js missing %q in %s", want, js)
		}
	}
	for _, unwanted := range []string{
		`navigation.type === "back_forward"`,
		`startViewTransition`,
		`function revalidateRestoredThread`,
		`scrollChatToLatest`,
		`applyChatScrollIntent`,
		`workbench-v2:chat-scroll`,
		`ResizeObserver`,
		`scheduleAgentChatScrollToLatest`,
		`seedArtifactBrowserOpenBeforePaint`,
		`restoreArtifactBrowserOpen`,
		`mergePatch({ _artifactBrowserOpen`,
		`persistArtifactBrowserOpenFromToggle`,
		`workbench-v2:doc-switch`,
		`workbench-doc-switch`,
		`data-workbench-doc-switching`,
		`rel="prefetch"`,
		`workbench-v2:composer-focused`,
		`agent-chat-composer-input`,
		`DOMParser`,
		`workbenchArtifactDoc`,
		`fetch(`,
	} {
		if strings.Contains(js, unwanted) {
			t.Fatalf("workbench-history.js should not contain %q in %s", unwanted, js)
		}
	}
}

func TestAgentChatScrollUsesSSRLatestAnchor(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/js/agent-chat-scroll.js")
	if err != nil {
		t.Fatalf("ReadFile(agent-chat-scroll.js) error = %v", err)
	}
	js := string(contents)
	for _, unwanted := range []string{
		`ResizeObserver`,
		`requestAnimationFrame`,
		`pendingLatest`,
		`scrollRestored`,
		`IntersectionObserver`,
	} {
		if strings.Contains(js, unwanted) {
			t.Fatalf(
				"agent-chat-scroll.js should not contain complex scroll machinery %q",
				unwanted,
			)
		}
	}

	transcript, err := os.ReadFile("../../../server/services/agentchat/transcript.templ")
	if err != nil {
		t.Fatalf("ReadFile(transcript.templ) error = %v", err)
	}
	transcriptSrc := string(transcript)
	if !strings.Contains(transcriptSrc, `id="chat-latest"`) {
		t.Fatalf("transcript.templ missing #chat-latest SSR sentinel")
	}
	if !strings.Contains(transcriptSrc, "autofocus") {
		t.Fatalf("transcript.templ missing autofocus on #chat-latest")
	}
	var sentinelLine string
	for _, line := range strings.Split(transcriptSrc, "\n") {
		if strings.Contains(line, `id="chat-latest"`) {
			sentinelLine = line
			break
		}
	}
	if sentinelLine == "" {
		t.Fatalf("transcript.templ missing chat-latest line")
	}
	if strings.Contains(sentinelLine, "aria-hidden") {
		t.Fatalf("#chat-latest must not have aria-hidden (browsers skip focus)")
	}
	if !strings.Contains(
		transcriptSrc,
		"\t\t<div id=\"chat-latest\" tabindex=\"-1\" autofocus",
	) {
		t.Fatalf("#chat-latest must be nested inside #agent-chat-messages")
	}

	for _, rel := range []string{
		"../../../server/services/agentchat/workbench_threads.templ",
		"../../../server/services/agentchat/shell.templ",
		"../../../server/services/agentchat/page_chat.templ",
		"../../../server/services/agentchat/embedded_chat.templ",
	} {
		body, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", rel, err)
		}
		src := string(body)
		if !strings.Contains(src, `id="agent-chat-scroll-region"`) {
			t.Fatalf("%s missing agent-chat-scroll-region", rel)
		}
		if strings.Contains(src, "flex-col-reverse") ||
			strings.Contains(src, "column-reverse") {
			t.Fatalf("%s must not use flex-col-reverse / column-reverse", rel)
		}
		if !strings.Contains(src, "flex-col") {
			t.Fatalf("%s missing normal flex-col on scroll region", rel)
		}
	}

	mobile, err := os.ReadFile("../../../server/layouts/workbench/mobile.templ")
	if err != nil {
		t.Fatalf("ReadFile(mobile.templ) error = %v", err)
	}
	if !strings.Contains(string(mobile), "mobileRegionTabClick") {
		t.Fatalf("mobile.templ missing mobileRegionTabClick for chat-latest focus")
	}
	shellBody, err := os.ReadFile("../../../server/services/agentchat/shell.templ")
	if err != nil {
		t.Fatalf("ReadFile(shell.templ) error = %v", err)
	}
	if !strings.Contains(string(shellBody), "chat-latest") ||
		!strings.Contains(string(shellBody), "queueMicrotask") {
		t.Fatalf("shell.templ Chat tab missing queueMicrotask chat-latest focus")
	}
	signalsBody, err := os.ReadFile("../../../server/layouts/workbench/signals.go")
	if err != nil {
		t.Fatalf("ReadFile(signals.go) error = %v", err)
	}
	signalsSrc := string(signalsBody)
	if !strings.Contains(signalsSrc, "chat-latest") ||
		!strings.Contains(
			signalsSrc,
			"queueMicrotask(() => document.getElementById('chat-latest')?.focus())",
		) {
		t.Fatalf(
			"signals.go mobileRegionTabClick missing queueMicrotask chat-latest focus",
		)
	}
}

func TestWorkbenchV2CSSKeepsStableRegionTransitionNames(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../../static/css/index.css")
	if err != nil {
		t.Fatalf("ReadFile(index.css) error = %v", err)
	}
	css := string(contents)
	for _, want := range CSSPresenceSnippets() {
		if !strings.Contains(css, want) {
			t.Fatalf("index.css missing name-map snippet %q", want)
		}
	}
	for _, want := range []string{
		"::view-transition-group(root),",
		"::view-transition-old(root),",
		"::view-transition-new(root) {",
		"::view-transition-old(root) {",
		"::view-transition-group(.workbench-chrome),",
		"::view-transition-old(.workbench-chrome),",
		"::view-transition-new(.workbench-chrome) {",
		"::view-transition-old(thread-artifact-document)",
		"animation: none;",
		"display: none;",
		"z-index: 20;",
		"#agent-chat-messages,",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("index.css missing %q", want)
		}
	}
	// Root must not blank BOTH old and new (Safari unmatched-chrome wipe).
	rootBlock := css[strings.Index(css, "::view-transition-group(root)"):]
	rootBlock = rootBlock[:strings.Index(rootBlock, "/* Explicit per-name")]
	if strings.Count(rootBlock, "display: none") != 0 {
		shared := rootBlock[:strings.Index(rootBlock, "::view-transition-old(root) {")]
		if strings.Contains(shared, "display: none") {
			t.Fatalf("root group/new must not display:none both snapshots: %s", shared)
		}
	}
	idx := strings.Index(css, "#doc-workbench-viewer-region")
	if idx >= 0 {
		window := css[idx : idx+180]
		if strings.Contains(window, "view-transition-name") {
			t.Fatalf("legacy viewer region has view-transition-name: %s", window)
		}
	}
	if strings.Contains(
		css,
		`html[data-workbench-doc-switching="true"] #workbench-v2-artifact`,
	) {
		t.Fatalf("doc-switching must not opacity-fade whole #workbench-v2-artifact")
	}
	if strings.Contains(
		css,
		`html[data-workbench-doc-switching="true"] #thread-artifact-document`,
	) {
		t.Fatalf("doc-switching must not opacity-fade #thread-artifact-document")
	}
	if strings.Contains(css, "view-transition-name: workbench-v2-artifact;") {
		t.Fatalf("parent #workbench-v2-artifact must stay view-transition-name: none")
	}
	if strings.Contains(css, "html:active-view-transition-type(workbench-doc-switch)") {
		t.Fatalf(
			"typed workbench-doc-switch sweep must stay removed (CSS-only chrome freeze)",
		)
	}
	if strings.Contains(css, "workbench-doc-switch-sweep") {
		t.Fatalf("workbench-doc-switch-sweep must stay removed")
	}
}

func TestBuildWorkbenchStateAppliesSavedRatiosButKeepsRouteVisibility(t *testing.T) {
	t.Parallel()

	saved := DefaultWorkbenchConfig(
		WorkbenchPageAgentChat,
		WorkbenchViewFocus,
		"",
		ViewportMobile,
	)
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

func TestMobileRegionTabsSSRSelectedForActiveRegion(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ViewportClass: ViewportMobile,
		ThreadsOpen:   true,
		ChatOpen:      true,
		ArtifactOpen:  true,
		Threads:       templ.NopComponent,
		Chat:          templ.NopComponent,
		Artifact:      templ.NopComponent,
		Comments:      templ.NopComponent,
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchV2State() error = %v", err)
	}
	var body bytes.Buffer
	if err := MobileRegionTabs(state).Render(t.Context(), &body); err != nil {
		t.Fatalf("MobileRegionTabs.Render() error = %v", err)
	}
	html := body.String()
	if !strings.Contains(html, `id="workbench-mobile-tabs"`) {
		t.Fatalf("missing workbench-mobile-tabs id: %s", html)
	}
	if !strings.Contains(html, `class="workbench-chrome flex shrink-0`) &&
		!strings.Contains(html, "workbench-chrome") {
		t.Fatalf("mobile tablist missing workbench-chrome: %s", html)
	}
	// Docs/artifact tab must paint selected in SSR HTML (before Datastar).
	if !strings.Contains(html, `aria-selected="true"`) {
		t.Fatalf("no SSR aria-selected=true: %s", html)
	}
	if !strings.Contains(html, "bg-muted text-foreground") {
		t.Fatalf("no SSR selected classes bg-muted text-foreground: %s", html)
	}
	if state.Config.Mobile.ActiveRegionID != WorkbenchV2ArtifactRegionID {
		t.Fatalf("ActiveRegionID = %q, want artifact", state.Config.Mobile.ActiveRegionID)
	}
	signals := EncodeWorkbenchSignals(state)
	if !strings.Contains(signals, `"activeRegionID":"workbenchV2Artifact"`) &&
		!strings.Contains(
			signals,
			`"activeRegionID":"`+SignalKeyForID(WorkbenchV2ArtifactRegionID)+`"`,
		) {
		t.Fatalf("signals missing artifact activeRegionID: %s", signals)
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
		`style="view-transition-name: doc-viewer;"`,
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

func TestWorkbenchV2StateHasFourIndependentMorphableBodies(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true,
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchV2State() error = %v", err)
	}
	if len(state.Regions) != 4 {
		t.Fatalf("regions = %#v, want four", state.Regions)
	}
	for index, want := range []string{
		WorkbenchV2ThreadsRegionID, WorkbenchV2ChatRegionID,
		WorkbenchV2CommentsRegionID, WorkbenchV2ArtifactRegionID,
	} {
		if state.Regions[index].ID != want ||
			state.Regions[index].BodyID == state.Regions[index].ID {
			t.Fatalf("region %d = %#v", index, state.Regions[index])
		}
	}
	if state.Regions[2].Visible {
		t.Fatal("comments should start closed")
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatalf("Workbench.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`id="workbench-v2-threads-body"`, `id="workbench-v2-chat-body"`,
		`id="workbench-v2-artifact-body"`, `id="workbench-v2-comments-body"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("Workbench html = %s, want %q", html, want)
		}
	}
	if strings.Contains(html, `data-ignore-morph`) {
		t.Fatalf("Workbench html has ignored v2 region: %s", html)
	}
}

func TestWorkbenchV2LiveBodiesAreUniqueAndMorphable(t *testing.T) {
	state, err := BuildWorkbenchV2State(
		WorkbenchV2Args{
			Threads:      templ.Raw("threads"),
			Chat:         templ.Raw("chat"),
			Artifact:     templ.Raw("artifact"),
			Comments:     templ.Raw("comments"),
			ThreadsOpen:  true,
			ChatOpen:     true,
			ArtifactOpen: true,
			CommentsOpen: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, id := range []string{"workbench-v2-threads-body", "workbench-v2-chat-body", "workbench-v2-artifact-body", "workbench-v2-comments-body"} {
		if strings.Count(html, `id="`+id+`"`) != 1 {
			t.Fatalf("%s count = %d", id, strings.Count(html, `id="`+id+`"`))
		}
	}
	if strings.Contains(html, `data-ignore-morph`) {
		t.Fatalf("v2 region body has ignored ancestor: %s", html)
	}
}

func TestWorkbenchV2CommentsSitLeftOfArtifactAndXORChat(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true, CommentsOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Regions) != 4 {
		t.Fatalf("regions = %#v", state.Regions)
	}
	if state.Regions[1].ID != WorkbenchV2ChatRegionID || state.Regions[1].Visible {
		t.Fatalf("chat should close when comments open: %#v", state.Regions[1])
	}
	if state.Regions[2].ID != WorkbenchV2CommentsRegionID || !state.Regions[2].Visible {
		t.Fatalf("comments should sit left of artifact: %#v", state.Regions[2])
	}
	if state.Regions[3].ID != WorkbenchV2ArtifactRegionID || !state.Regions[3].Visible {
		t.Fatalf("artifact = %#v", state.Regions[3])
	}
}

func TestCommentsOpenFromRequestDefaultsClosed(t *testing.T) {
	t.Parallel()
	if CommentsOpenFromRequest(nil) {
		t.Fatal("nil request should close comments")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if CommentsOpenFromRequest(req) {
		t.Fatal("missing cookie should close comments")
	}
	req.AddCookie(&http.Cookie{Name: CommentsOpenCookie, Value: "1"})
	if !CommentsOpenFromRequest(req) {
		t.Fatal("cookie 1 should open comments")
	}
}

func TestWorkbenchV2PreferencesKeepOnlyRatios(t *testing.T) {
	t.Parallel()

	defaults := DefaultWorkbenchConfig(WorkbenchPageThreads, WorkbenchViewSplit, "")
	saved := cloneWorkbenchConfig(defaults)
	saved.Regions[0].Ratio = 0.31
	saved.Regions[0].Visible = false
	saved.Mobile.ActiveRegionID = WorkbenchV2CommentsRegionID
	merged := MergeWorkbenchConfig(defaults, &saved)
	if got := regionSpecByID(merged, WorkbenchV2ThreadsRegionID).Ratio; got != 0.31 {
		t.Fatalf("ratio = %v, want 0.31", got)
	}
	if !regionSpecByID(merged, WorkbenchV2ThreadsRegionID).Visible ||
		merged.Mobile.ActiveRegionID != WorkbenchV2ArtifactRegionID {
		t.Fatalf("v2 interaction state was restored: %#v", merged)
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
		state,
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

func TestWorkbenchV2MobileDefaultsToArtifact(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ViewportClass: ViewportMobile,
		ThreadsOpen:   true,
		ChatOpen:      true,
		ArtifactOpen:  true,
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchV2State() error = %v", err)
	}
	if state.Config.Mobile.ActiveRegionID != WorkbenchV2ArtifactRegionID {
		t.Fatalf(
			"mobile active = %q, want artifact/Docs default %q",
			state.Config.Mobile.ActiveRegionID,
			WorkbenchV2ArtifactRegionID,
		)
	}
	var chat, artifact WorkbenchRegion
	for _, region := range state.Regions {
		switch region.ID {
		case WorkbenchV2ChatRegionID:
			chat = region
		case WorkbenchV2ArtifactRegionID:
			artifact = region
		}
	}
	if got := RegionInitialClass(state, artifact); !strings.Contains(got, "flex") ||
		strings.Contains(got, "hidden") {
		t.Fatalf("mobile artifact initial class = %q, want visible flex", got)
	}
	if got := RegionInitialClass(state, chat); !strings.Contains(got, "hidden") {
		t.Fatalf("mobile chat initial class = %q, want hidden until Chat tab", got)
	}
}

func TestDefaultThreadsConfigMobileActiveIsArtifact(t *testing.T) {
	t.Parallel()

	cfg := DefaultWorkbenchConfig(WorkbenchPageThreads, WorkbenchViewSplit, "")
	if cfg.Mobile.ActiveRegionID != WorkbenchV2ArtifactRegionID {
		t.Fatalf(
			"threads default mobile active = %q, want %q",
			cfg.Mobile.ActiveRegionID,
			WorkbenchV2ArtifactRegionID,
		)
	}
}

func TestWorkbenchV2MobileKeepsArtifactEvenWhenChatOpen(t *testing.T) {
	t.Parallel()

	for _, chatOpen := range []bool{false, true} {
		state, err := BuildWorkbenchV2State(WorkbenchV2Args{
			ViewportClass: ViewportMobile,
			ThreadsOpen:   true,
			ChatOpen:      chatOpen,
			ArtifactOpen:  true,
		})
		if err != nil {
			t.Fatalf("BuildWorkbenchV2State(chatOpen=%v) error = %v", chatOpen, err)
		}
		if state.Config.Mobile.ActiveRegionID != WorkbenchV2ArtifactRegionID {
			t.Fatalf(
				"chatOpen=%v mobile active = %q, want primary artifact/Docs",
				chatOpen,
				state.Config.Mobile.ActiveRegionID,
			)
		}
	}
}

func TestWorkbenchV2RegionsEnforceComposerFriendlyMinRem(t *testing.T) {
	t.Parallel()

	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  true,
		ChatOpen:     true,
		ArtifactOpen: true,
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchV2State() error = %v", err)
	}
	want := map[string]float64{
		WorkbenchV2ThreadsRegionID:  12,
		WorkbenchV2ChatRegionID:     18,
		WorkbenchV2ArtifactRegionID: 20,
		WorkbenchV2CommentsRegionID: 12,
	}
	for _, region := range state.Regions {
		got := region.MinRem
		if got != want[region.ID] {
			t.Fatalf("region %s MinRem = %v, want %v", region.ID, got, want[region.ID])
		}
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, fragment := range []string{
		`data-workbench-region="workbench-v2-chat"`,
		`data-workbench-min-rem="18"`,
		`/js/workbench-resize.js?v=10`,
		`/js/workbench-history.js?v=21`,
	} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("workbench html missing %q", fragment)
		}
	}
}

func TestWorkbenchV2RegionsHairlineGap(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(
		WorkbenchV2Args{ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	out := body.String()
	if !strings.Contains(
		out,
		`id="workbench-regions" class="flex min-h-0 w-full flex-1 gap-0 overflow-hidden"`,
	) {
		t.Fatal("workbench-regions should use gap-0 hairline columns")
	}
	if strings.Contains(
		out,
		`id="workbench-regions" class="flex min-h-0 w-full flex-1 gap-2`,
	) {
		t.Fatal("workbench-regions still has gap-2 gutter")
	}
}

func TestWorkbenchV2FlushChrome(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(
		WorkbenchV2Args{ThreadsOpen: true, ChatOpen: true, ArtifactOpen: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	out := body.String()
	if !strings.Contains(out, `class="flex h-full min-h-0 w-full overflow-hidden p-0"`) {
		t.Fatal("workbench-root should be p-0 flush")
	}
	if strings.Contains(out, "rounded-lg border border-border shadow-sm") {
		t.Fatal("regions still have rounded-lg/shadow-sm")
	}
	if !strings.Contains(
		out,
		"workbench-region overflow-hidden rounded-none border-0 shadow-none",
	) {
		t.Fatal("regions should be rounded-none border-0 shadow-none (flush shells)")
	}
	if strings.Contains(
		out,
		"workbench-region overflow-hidden rounded-none border border-border",
	) {
		t.Fatal("regions still have full card border")
	}
}

func TestWorkbenchV2FlushChromeCSS(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../../static/css/index.css")
	if err != nil {
		// path from package dir during go test
		b, err = os.ReadFile("static/css/index.css")
	}
	if err != nil {
		t.Skip("index.css not found from test cwd")
	}
	out := string(b)
	for _, want := range []string{
		"AI-470 flush chrome",
		"#workbench-root",
		"padding: 0 !important",
		"#workbench-regions > .workbench-region:not(:last-child)",
		"border-right: 1px solid",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("index.css missing flush rule %q", want)
		}
	}
}
