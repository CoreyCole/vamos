package workbench

import (
	"strings"
	"testing"
)

func TestRegionSSRFlexStyle_MatchesVisibleRatios(t *testing.T) {
	t.Parallel()
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		ThreadsOpen:  true,
		ChatOpen:     true,
		ArtifactOpen: true,
		CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	threads := state.Regions[0]
	got := RegionSSRFlexStyle(state, threads)
	if !strings.HasPrefix(got, "flex: ") || !strings.HasSuffix(got, " 1 0%") {
		t.Fatalf("threads SSR flex = %q", got)
	}
	closed := threads
	closed.Visible = false
	if RegionSSRFlexStyle(state, closed) != "" {
		t.Fatal("hidden region should omit SSR flex")
	}
}

func TestRegionSSRFlexStyle_UsesSavedRatios(t *testing.T) {
	t.Parallel()
	saved := &WorkbenchConfig{
		Version: 1,
		Page:    WorkbenchPageThreads,
		View:    WorkbenchViewSplit,
		Regions: []RegionSpec{
			{ID: WorkbenchV2ThreadsRegionID, Slot: WorkbenchSlotNavigation, Kind: RegionPlanSidebar, Ratio: 0.3, Visible: true},
			{ID: WorkbenchV2ChatRegionID, Slot: WorkbenchSlotContext, Kind: RegionChat, Ratio: 0.35, Visible: true},
			{ID: WorkbenchV2ArtifactRegionID, Slot: WorkbenchSlotPrimary, Kind: RegionArtifact, Ratio: 0.35, Visible: true},
			{ID: WorkbenchV2CommentsRegionID, Slot: WorkbenchSlotContext, Kind: RegionComments, Ratio: 0.22, Visible: false},
		},
	}
	state, err := BuildWorkbenchV2State(WorkbenchV2Args{
		SavedConfig:  saved,
		ThreadsOpen:  true,
		ChatOpen:     true,
		ArtifactOpen: true,
		CommentsOpen: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// threads ratio 0.3 / (0.3+0.35+0.35) = 0.3
	got := RegionSSRFlexStyle(state, state.Regions[0])
	if !strings.Contains(got, "0.3000") {
		t.Fatalf("expected saved threads share in %q", got)
	}
}
