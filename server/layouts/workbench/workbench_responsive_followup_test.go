package workbench

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestWorkbenchResizeReflowsVisibleColumnsOnThreadsToggle(t *testing.T) {
	contents, err := os.ReadFile("../../../static/js/workbench-resize.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(contents)
	for _, want := range []string{
		"function reflowVisibleRegionFlex(root)",
		`document.addEventListener("workbench-layout-reflow", reflowWorkbenchFromEvent)`,
		`grow.toFixed(4) + " 1 0%"`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("threads toggle reflow missing %q", want)
		}
	}
}

func TestWorkbenchResizeReflowsEphemeralPixelWidthsOnWindowResize(t *testing.T) {
	contents, err := os.ReadFile("../../../static/js/workbench-resize.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(contents)
	for _, want := range []string{
		`window.addEventListener("resize", reflowWorkbenchesAfterWindowResize`,
		"requestAnimationFrame(() => {",
		"delete region.dataset.workbenchWidthPx",
		"applyRegionRatios(root)",
		"passive: true",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("workbench resize listener missing %q", want)
		}
	}
	callback := js[strings.Index(js, "function reflowWorkbenchesAfterWindowResize"):strings.Index(js, `window.addEventListener("resize"`)]
	for _, forbidden := range []string{"saveConfig(", "mergeWorkbenchPaths(", "collapseRegion("} {
		if strings.Contains(callback, forbidden) {
			t.Fatalf("window resize callback contains %q", forbidden)
		}
	}
}

func TestWorkbenchV2ThreadsHasIndependentHideAndReopenControls(t *testing.T) {
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{ThreadsOpen: true})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`data-workbench-threads-reopen`,
		`aria-label="Show threads sidebar"`,
		`$workbench.regions.workbenchV2Threads.visible = true`,
		`wb2_threads_open=1`,
		`/js/workbench-resize.js?v=10`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("workbench threads reopen control missing %q", want)
		}
	}
	if strings.Contains(
		html,
		`data-workbench-threads-reopen data-on:click="workbench-layout-save`,
	) {
		t.Fatal("threads reopen control persists temporary visibility")
	}
}
